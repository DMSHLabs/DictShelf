// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package sysmem

import "syscall"

// stdlib syscall, not golang.org/x/sys (see internal/fsx/free_windows.go).

func sysctl(name string) (uint64, bool) {
	s, err := syscall.Sysctl(name)
	if err != nil {
		return 0, false
	}
	return decodeSysctl(s)
}

func total() int64 {
	n, _ := sysctl("hw.memsize")
	return int64(n)
}

// available is the memory macOS can hand out without compressing or paging:
// free, speculative and purgeable pages, and the file-backed pages it drops on
// demand. kern.memorystatus_level is not it: it counts anonymous active and
// inactive pages, and rarely falls below a quarter. vm.page_purgeable_count
// is 8 bytes, the others 4; one that cannot be read counts as none, except
// the free count and the page size, without which nothing is known.
func available() (int64, bool) {
	page, ok := sysctl("vm.pagesize")
	free, fok := sysctl("vm.page_free_count")
	if !ok || !fok || page == 0 {
		return 0, false
	}
	spec, _ := sysctl("vm.page_speculative_count")
	purge, _ := sysctl("vm.page_purgeable_count")
	ext, _ := sysctl("vm.page_pageable_external_count")
	return int64((free + spec + purge + ext) * page), true
}
