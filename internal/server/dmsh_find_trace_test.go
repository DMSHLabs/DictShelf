// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"strings"
	"testing"
)

// The "Trace all" controls are a developer switch (config TEST_FIND_TRACE): the
// walk they start writes every step of both find layers into the system log, and
// a reader has no use for it. A server that was not asked must serve a page whose
// flag is false - the strip keeps both controls hidden on that alone - and a
// server that was asked must serve true. The markup starts them hidden as well,
// so a slow script never shows them for a moment first.
func TestThePageCarriesTheFindTraceSwitch(t *testing.T) {
	plain := newTestServer(t)
	page, _ := plain.pageFor("", "", "")
	if got := string(page); !strings.Contains(got, "window.wudictFindTrace=false") {
		t.Error("a server without TEST_FIND_TRACE does not serve the flag as false")
	}
	for _, id := range []string{`id="articleFindTrace"`, `id="articleFindTraceRun"`} {
		at := strings.Index(string(page), id)
		if at < 0 {
			t.Fatalf("the strip lost its %s control", id)
		}
		line := string(page)[at:]
		if end := strings.Index(line, ">"); end < 0 || !strings.Contains(line[:end], "hidden") {
			t.Errorf("%s does not start hidden: it would show for a moment in a release", id)
		}
	}

	asked := newTestServer(t)
	asked.FindTrace = true
	marked, _ := asked.pageFor("", "", "")
	if got := string(marked); !strings.Contains(got, "window.wudictFindTrace=true") {
		t.Error("a server with TEST_FIND_TRACE does not serve the flag as true")
	}
}
