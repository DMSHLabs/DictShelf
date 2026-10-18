// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

// stubFree makes diskFree report free bytes (ok as given) for the test.
func stubFree(t *testing.T, free int64, ok bool) {
	t.Helper()
	old := diskFree
	diskFree = func(string) (int64, bool) { return free, ok }
	t.Cleanup(func() { diskFree = old })
}

func TestRoomFor(t *testing.T) {
	for _, tc := range []struct {
		name       string
		free, need int64
		ok, refuse bool
	}{
		{"fits", 100, 100, true, false},
		{"one byte over", 100, 101, true, true},
		{"nothing needed", 0, 0, true, false},
		{"unknown need", 0, -1, true, false},
		{"free space unreadable", 0, 1 << 40, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stubFree(t, tc.free, tc.ok)
			err := roomFor("/lib", "to install it", tc.need)
			var re *dict.NoRoomError
			if got := errors.As(err, &re); got != tc.refuse {
				t.Fatalf("roomFor(need %d, free %d) = %v, refuse %v", tc.need, tc.free, err, tc.refuse)
			}
			if tc.refuse && (re.Need != tc.need || re.Free != tc.free || re.Dir != "/lib") {
				t.Errorf("error carries %+v", re)
			}
		})
	}
}

// A download whose length is known and does not fit is refused before a byte
// is written: no .part, no download folder.
func TestFetchRefusesWithoutRoom(t *testing.T) {
	body := strings.Repeat("wudict", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		http.ServeContent(w, r, "bundle.zip", zeroTime, strings.NewReader(body))
	}))
	defer srv.Close()
	stubFree(t, int64(len(body))-1, true)

	dest := t.TempDir()
	_, err := loopback().Fetch(context.Background(), dest, srv.URL+"/bundle.zip", nil)
	var re *dict.NoRoomError
	if !errors.As(err, &re) || re.Need != int64(len(body)) {
		t.Fatalf("Fetch = %v, want a NoRoomError for %d bytes", err, len(body))
	}
	if ents, _ := os.ReadDir(DownloadDir(dest)); len(ents) != 0 {
		t.Errorf("a refused download left %d entries in the download folder", len(ents))
	}
}

// An install that does not fit fails at once: nothing reaches the library,
// the stage is gone, and the archive the user pointed at is still there.
func TestInstallRefusesWithoutRoom(t *testing.T) {
	dest := t.TempDir()
	src := buildZip(t, member{"Oxford.mdx", "main"}, member{"Oxford.mdd", "media"})
	m := &Manager{}
	mustBegin(t, m, dest, src, false)
	stubFree(t, 1, true) // after Begin: only the install asks
	if _, err := m.Confirm(dest, []int{0}, Options{Keep: true}); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	m.Wait()

	st := m.Status()
	if st.State != StateError || !strings.Contains(st.Error, "not enough free disk space") || len(st.Installed) != 0 {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(StageRoot(dest)); !os.IsNotExist(err) {
		t.Errorf("staging root left behind: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("a refused install removed the source: %v", err)
	}
	if ents, _ := os.ReadDir(dest); len(ents) != 0 {
		t.Errorf("a refused install left %d entries in the library", len(ents))
	}
}
