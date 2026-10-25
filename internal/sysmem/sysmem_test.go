// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package sysmem

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	mib = int64(1) << 20
	gib = int64(1) << 30
)

func TestMachineBudget(t *testing.T) {
	for _, tt := range []struct {
		total, avail int64
		ok           bool
		want         int64
	}{
		{0, 0, false, 256 * mib},              // nothing known
		{32 * gib, 16 * gib, true, 4 * gib},   // capped at an eighth
		{32 * gib, 2 * gib, true, 1 * gib},    // half of what is free
		{24 * gib, 3 * gib, true, 1536 * mib}, // half of what is free
		{8 * gib, 3 * gib, true, 1 * gib},     // an eighth
		{32 * gib, 100 * mib, true, 256 * mib},
		{32 * gib, 0, false, 256 * mib},      // free unknown: the floor
		{1 * gib, 1 * gib, true, 128 * mib},  // the floor never above an eighth
		{1 * gib, 10 * mib, true, 128 * mib}, // ... and is still the floor
	} {
		if got := machineBudget(tt.total, tt.avail, tt.ok); got != tt.want {
			t.Errorf("machineBudget(%d, %d, %v) = %d, want %d", tt.total, tt.avail, tt.ok, got, tt.want)
		}
	}
}

func TestLimitBudget(t *testing.T) {
	const l = 384 * mib
	for _, tt := range []struct {
		live, avail int64
		ok          bool
		want        int64
	}{
		{0, 0, false, l / 2},                 // before the first collection
		{100 * mib, 0, false, l/2 - 100*mib}, // half of what the limit leaves
		{300 * mib, 0, false, l / 8},         // never below an eighth of the limit ...
		{0, 100 * mib, true, 50 * mib},       // ... except where the system has less
		{0, 4 * gib, true, l / 2},
	} {
		if got := limitBudget(l, tt.live, tt.avail, tt.ok); got != tt.want {
			t.Errorf("limitBudget(%d, %d, %v) = %d, want %d", tt.live, tt.avail, tt.ok, got, tt.want)
		}
	}
}

func TestAccount(t *testing.T) {
	pool.Lock()
	if pool.used != 0 {
		pool.Unlock()
		t.Skip("pool in use by another test")
	}
	pool.Unlock()
	// More than any budget: never admitted, whatever Budget reads.
	const huge = int64(1) << 50
	var a, b Account
	if a.TryAcquire(huge) {
		t.Fatal("past the budget was admitted")
	}
	if !a.TryAcquire(0) || !a.TryAcquire(-1) || a.Held() != 0 {
		t.Error("nothing must always be admitted, and hold nothing")
	}
	if !a.TryAcquire(1) || !b.TryAcquire(2) || a.Held() != 1 || b.Held() != 2 {
		t.Fatalf("small draws: held %d, %d", a.Held(), b.Held())
	}
	b.Release(5) // more than it holds: only what it holds
	if b.Held() != 0 {
		t.Errorf("held %d after releasing more than held", b.Held())
	}
	a.Close()
	a.Close() // idempotent
	b.Close()
	pool.Lock()
	used := pool.used
	pool.Unlock()
	if used != 0 {
		t.Errorf("pool used %d after every account closed", used)
	}
}

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMeminfo(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"proc/meminfo": "MemTotal:       16384000 kB\nMemFree:  1 kB\nMemAvailable:    4096000 kB\n"})
	total, avail, ok := meminfo(root)
	if total != 16384000<<10 || avail != 4096000<<10 || !ok {
		t.Errorf("meminfo = %d %d %v", total, avail, ok)
	}
	if _, _, ok := meminfo(t.TempDir()); ok {
		t.Error("no meminfo read as available")
	}
}

func TestCgroup(t *testing.T) {
	const total = 16 * gib
	for _, tt := range []struct {
		name  string
		files map[string]string
		want  int64
		ok    bool
	}{
		{"v2 own limit", map[string]string{
			"proc/self/cgroup":                 "0::/app\n",
			"sys/fs/cgroup/app/memory.max":     "2147483648\n",
			"sys/fs/cgroup/app/memory.current": "1073741824\n",
			"sys/fs/cgroup/app/memory.stat":    "anon 1\ninactive_file 536870912\n",
			"sys/fs/cgroup/memory.stat":        "inactive_file 0\n", // the root has no memory.max
		}, 2*gib - (1*gib - 512*mib), true},
		{"v2 ancestor limit is tighter", map[string]string{
			"proc/self/cgroup":                   "0::/slice/app\n",
			"sys/fs/cgroup/slice/app/memory.max": "max\n",
			"sys/fs/cgroup/slice/memory.max":     "1073741824\n",
			"sys/fs/cgroup/slice/memory.current": "805306368\n",
		}, 256 * mib, true},
		{"v2 path missing under the mount", map[string]string{
			"proc/self/cgroup":             "0::/elsewhere\n",
			"sys/fs/cgroup/memory.max":     "1073741824\n",
			"sys/fs/cgroup/memory.current": "0\n",
		}, 1 * gib, true},
		{"v2 no limit anywhere", map[string]string{
			"proc/self/cgroup":             "0::/app\n",
			"sys/fs/cgroup/app/memory.max": "max\n",
		}, 0, false},
		{"v1 limit", map[string]string{
			"proc/self/cgroup": "4:cpu,cpuacct:/x\n9:memory:/app\n",
			"sys/fs/cgroup/memory/app/memory.limit_in_bytes": "1073741824\n",
			"sys/fs/cgroup/memory/app/memory.usage_in_bytes": "805306368\n",
			"sys/fs/cgroup/memory/app/memory.stat":           "cache 1\ntotal_inactive_file 268435456\n",
		}, 512 * mib, true},
		{"v1 unlimited sentinel", map[string]string{
			"proc/self/cgroup": "9:memory:/app\n",
			"sys/fs/cgroup/memory/app/memory.limit_in_bytes": "9223372036854771712\n",
		}, 0, false},
		{"no cgroup file", map[string]string{}, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, tt.files)
			got, ok := cgroupAvailable(root, total)
			if got != tt.want || ok != tt.ok {
				t.Errorf("cgroupAvailable = %d %v, want %d %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDecodeSysctl(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want uint64
		ok   bool
	}{
		{"\x00\x40\x00", 16384, true},                       // 4 bytes, top NUL dropped
		{"\x8b\x0e\x00\x00", 3723, true},                    // 4 bytes
		{"\x11", 17, true},                                  // 4 bytes, three NULs: one dropped, two kept? either way
		{"\x00\x00\x00\x00\x06\x00\x00", 25769803776, true}, // 8 bytes, top NUL dropped (hw.memsize 24 GiB)
		{"\x01\x02\x03\x04\x05\x06\x07\x08", 0x0807060504030201, true},
		{"123456789", 0, false},
	} {
		if got, ok := decodeSysctl(tt.in); got != tt.want || ok != tt.ok {
			t.Errorf("decodeSysctl(%q) = %d %v, want %d %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// TestThisMachine reads the real sources: what is known is plausible.
func TestThisMachine(t *testing.T) {
	total := Total()
	avail, ok := Available()
	t.Logf("total %d MiB, available %d MiB (%v), budget %d MiB", total>>20, avail>>20, ok, Budget()>>20)
	if ok && total > 0 && (avail < 0 || avail > total) {
		t.Errorf("available %d outside [0, total %d]", avail, total)
	}
	if b := Budget(); b <= 0 {
		t.Errorf("budget %d", b)
	}
}
