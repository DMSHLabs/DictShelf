// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package server

import (
	"syscall"
	"testing"
	"time"
)

// A groups.ini that is not a regular file - here a pipe nothing writes to -
// is refused at once: reading it would never end, under the cache lock that
// every list load waits on.
func TestGroupsPipeDoesNotHang(t *testing.T) {
	s := newGroupsServer(t)
	if err := syscall.Mkfifo(s.User.Groups(), 0o644); err != nil {
		t.Skip("mkfifo:", err)
	}
	done := make(chan groupsResp, 1)
	go func() { r, _ := callGroups(t, s, "GET", ""); done <- r }()
	select {
	case r := <-done:
		if !r.Unusable {
			t.Errorf("a pipe is not reported unusable: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading groups.ini hung on a pipe")
	}
}
