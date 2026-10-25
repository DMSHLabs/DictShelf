// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package sysmem

import (
	"bufio"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Linux and Android read memory from files, so the readers take the root they
// are under and the tests give them fixtures. Built on every platform.

// meminfo reads MemTotal and MemAvailable from root/proc/meminfo, in bytes.
func meminfo(root string) (total, avail int64, availOK bool) {
	f, err := os.Open(filepath.Join(root, "proc/meminfo"))
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		// "MemAvailable:    3908352 kB"
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fs := strings.Fields(v)
		if len(fs) == 0 {
			continue
		}
		n, err := strconv.ParseInt(fs[0], 10, 64)
		if err != nil || n < 0 {
			continue
		}
		switch k {
		case "MemTotal":
			total = n << 10
		case "MemAvailable":
			avail, availOK = n<<10, true
		}
	}
	return total, avail, availOK
}

// cgroupAvailable is what the memory cgroups of this process leave free: the
// least, over the process's cgroup and its ancestors, of the limit less the
// memory charged that cannot be reclaimed (page cache that is inactive can).
// cgroup v2 first, then v1. ok is false when no limit applies.
func cgroupAvailable(root string, total int64) (int64, bool) {
	b, err := os.ReadFile(filepath.Join(root, "proc/self/cgroup"))
	if err != nil {
		return 0, false
	}
	var v2, v1 string
	hasV2, hasV1 := false, false
	for _, line := range strings.Split(string(b), "\n") {
		// "hierarchy-ID:controller-list:cgroup-path"
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		switch {
		case parts[0] == "0" && parts[1] == "":
			v2, hasV2 = parts[2], true
		case hasController(parts[1], "memory"):
			v1, hasV1 = parts[2], true
		}
	}
	if hasV2 {
		if n, ok := walkCgroup(filepath.Join(root, "sys/fs/cgroup"), v2, total, cgroupV2); ok {
			return n, true
		}
	}
	if hasV1 {
		return walkCgroup(filepath.Join(root, "sys/fs/cgroup/memory"), v1, total, cgroupV1)
	}
	return 0, false
}

func hasController(list, name string) bool {
	for _, c := range strings.Split(list, ",") {
		if c == name {
			return true
		}
	}
	return false
}

// walkCgroup applies level to the process's cgroup directory under mount and
// to each ancestor up to the mount, returning the least it reports. A cgroup
// path missing under the mount (a container without its own cgroup
// namespace) is read as the mount itself.
func walkCgroup(mount, path string, total int64, level func(dir string, total int64) (int64, bool)) (int64, bool) {
	dir := filepath.Join(mount, filepath.FromSlash(path))
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		dir = mount
	}
	best, found := int64(0), false
	for {
		if n, ok := level(dir, total); ok && (!found || n < best) {
			best, found = n, true
		}
		if dir == mount || len(dir) <= len(mount) {
			break
		}
		dir = filepath.Dir(dir)
	}
	return max(best, 0), found
}

// cgroupV2 is memory.max less (memory.current less inactive_file); the root
// cgroup has no memory.max, and "max" is no limit.
func cgroupV2(dir string, _ int64) (int64, bool) {
	limit, ok := readInt(filepath.Join(dir, "memory.max"))
	if !ok {
		return 0, false
	}
	cur, _ := readInt(filepath.Join(dir, "memory.current"))
	inactive := statValue(filepath.Join(dir, "memory.stat"), "inactive_file")
	return limit - max(cur-inactive, 0), true
}

// cgroupV1 is memory.limit_in_bytes less usage plus total_inactive_file; a
// limit at or above the machine's memory (9223372036854771712 when unset)
// is no limit.
func cgroupV1(dir string, total int64) (int64, bool) {
	limit, ok := readInt(filepath.Join(dir, "memory.limit_in_bytes"))
	if !ok || total > 0 && limit >= total || limit >= 1<<62 {
		return 0, false
	}
	usage, _ := readInt(filepath.Join(dir, "memory.usage_in_bytes"))
	inactive := statValue(filepath.Join(dir, "memory.stat"), "total_inactive_file")
	return limit - max(usage-inactive, 0), true
}

// readInt reads a file holding one non-negative integer; "max" and anything
// else unreadable are not one.
func readInt(path string) (int64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// statValue is the value of key in a memory.stat file, 0 when absent.
func statValue(path, key string) int64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(line, " "); ok && k == key {
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// decodeSysctl decodes a sysctl integer as syscall.Sysctl returns it: little
// endian, with one trailing NUL byte - for an integer, its top byte when that
// is zero - dropped. Up to 4 bytes is a 32-bit value, up to 8 a 64-bit one.
func decodeSysctl(s string) (uint64, bool) {
	if len(s) > 8 {
		return 0, false
	}
	var b [8]byte
	copy(b[:], s)
	if len(s) <= 4 {
		return uint64(binary.LittleEndian.Uint32(b[:4])), true
	}
	return binary.LittleEndian.Uint64(b[:]), true
}
