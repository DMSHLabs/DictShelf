// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDesktopRescanStreams(t *testing.T) {
	s := newTestServer(t)
	preparedDSL(t, s)
	for _, tc := range []struct {
		method, argument, terminal string
	}{
		{"rescan", "", "end"},
		{"rescanIndexes", `{"new":{"index":true},"existing":{"index":"keep"}}`, "done"},
	} {
		t.Run(tc.method, func(t *testing.T) {
			terminal := false
			err := s.DesktopStream(context.Background(), "http://127.0.0.1:6888", tc.method, tc.argument, func(raw []byte) {
				var message struct {
					Type   string   `json:"t"`
					Failed []string `json:"failed"`
				}
				if err := json.Unmarshal(raw, &message); err != nil {
					t.Fatal(err)
				}
				if terminal {
					t.Fatal("record after terminal response")
				}
				if len(message.Failed) != 0 {
					t.Fatalf("rescan failed: %v", message.Failed)
				}
				terminal = message.Type == tc.terminal
			})
			if err != nil || !terminal {
				t.Fatalf("stream did not complete: terminal=%v error=%v", terminal, err)
			}
		})
	}
}
