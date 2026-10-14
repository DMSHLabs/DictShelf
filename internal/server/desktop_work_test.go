// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"encoding/json"
	"testing"
)

func TestDesktopIndexWorkStartAndStop(t *testing.T) {
	for _, method := range []string{"startIndexWork", "startSingleIndexWork"} {
		t.Run(method, func(t *testing.T) {
			s, _ := newPrefsServer(t)
			e := s.reg.all()[0]
			e.ingestMu.Lock()
			defer func() { e.ingestMu.Unlock(); s.jobs.wg.Wait() }()
			body, _ := json.Marshal([]indexOperation{{e.ID, "contains", "create"}})
			response, err := s.DesktopJSON("http://127.0.0.1:6888", method, string(body))
			if err != nil {
				t.Fatal(err)
			}
			key := bulkIndexKey
			if method == "startSingleIndexWork" {
				key = "single-index:" + e.ID
			}
			data, _ := json.Marshal(response)
			var started indexWorkStatus
			if err := json.Unmarshal(data, &started); err != nil || started.ID != key || !started.Running {
				t.Fatalf("wrong start response: %s error=%v", data, err)
			}
			if _, err := s.DesktopJSON("http://127.0.0.1:6888", "stopIndexWork", ""); err != nil {
				t.Fatal(err)
			}
			st, _ := s.jobs.status(key)
			if !st.StopRequested {
				t.Fatal("native Stop did not request the existing job's safe stop")
			}
		})
	}
}

func TestDesktopReindexWithNoOutdatedDictionaries(t *testing.T) {
	s := newTestServer(t)
	for _, method := range []string{"startReindex", "stopReindex"} {
		result, err := s.DesktopJSON("http://127.0.0.1:6888", method, "")
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		data, _ := json.Marshal(result)
		var status ReindexStatus
		if err := json.Unmarshal(data, &status); err != nil || status.Running {
			t.Fatalf("%s: unexpected response %s error=%v", method, data, err)
		}
	}
}
