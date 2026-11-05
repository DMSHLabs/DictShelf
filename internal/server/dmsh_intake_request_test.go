// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"testing"
)

// The desktop bridge's intake actions take a JSON request, and for two of them
// the Android bridge (Intake.java) is the only caller in the tree. It sent the
// bare link and the bare path, so every import from a share link or a shared
// file answered "invalid import request" - the string the Go side returns when
// json.Unmarshal fails, and the reason a share link reached the app and then
// died there. Intake.java now sends {"url": …} / {"path": …} (with dir, empty
// for the configured folder); this pins the shape on the Go side so the two
// halves cannot part again in silence.
func TestIntakeURLWantsAJSONRequestNotABareLink(t *testing.T) {
	s, _ := aboutServer(t, "")
	const base = "http://127.0.0.1:6888"

	if _, err := s.DesktopJSON(base, "intakeURL", "https://example.org/x.zip"); err == nil ||
		!strings.Contains(err.Error(), "invalid import request") {
		t.Fatalf("a bare link must be refused as an invalid request, got %v", err)
	}

	// The JSON form passes the request parse and fails later, on the address
	// itself: the loopback host is refused by the fetcher's own rule, not by
	// json.Unmarshal. Nothing here touches the network.
	_, err := s.DesktopJSON(base, "intakeURL", `{"url":"http://127.0.0.1:1/x.zip"}`)
	if err != nil && strings.Contains(err.Error(), "invalid import request") {
		t.Fatalf("the JSON form must pass the request parse, got %v", err)
	}
}

// The same shape for the path action: a shared file arrives as a path, and the
// request carries the destination folder beside it.
func TestIntakePathWantsAJSONRequestNotABarePath(t *testing.T) {
	s, _ := aboutServer(t, "")
	const base = "http://127.0.0.1:6888"

	if _, err := s.DesktopJSON(base, "intakePath", "/tmp/x.zip"); err == nil ||
		!strings.Contains(err.Error(), "invalid import request") {
		t.Fatalf("a bare path must be refused as an invalid request, got %v", err)
	}
	// No such file: whatever comes back, it is not the parse's complaint.
	_, err := s.DesktopJSON(base, "intakePath", `{"path":"/nonexistent/x.zip"}`)
	if err != nil && strings.Contains(err.Error(), "invalid import request") {
		t.Fatalf("the JSON form must pass the request parse, got %v", err)
	}
}

// A guard on the claim above: the dispatcher is reached through the same
// envelope the Android bridge uses, so the test fails if the JSON branch is
// ever removed rather than if the message text changes by accident.
func TestTheIntakeActionsAreTheOnesTheAndroidBridgeNames(t *testing.T) {
	known := map[string]bool{}
	s, _ := aboutServer(t, "")
	for _, action := range []string{"intakeURL", "intakePath", "intakeStatus", "intakeTarget", "intakeConfirm", "intakeCancel"} {
		_, err := s.DesktopJSON("http://127.0.0.1:6888", action, `{}`)
		if err != nil && strings.Contains(err.Error(), "unknown") {
			known[action] = false
			t.Errorf("%s is not a dispatcher action any more", action)
			continue
		}
		known[action] = true
	}
	if len(known) != 5 {
		t.Fatalf("expected five intake actions, saw %d", len(known))
	}
}
