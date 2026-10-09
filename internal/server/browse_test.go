// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net/url"
	"testing"
)

func TestBrowseFindUsesPreparedHeadwordIndex(t *testing.T) {
	s := newTestServer(t)
	id := getDicts(t, s, "/api/dicts")[0].ID
	path := "/api/browse/find?dict=" + url.QueryEscape(id) + "&q=cor&p=1"
	e, err := s.reg.get(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.setFeatures(features{}, nil); err != nil {
		t.Fatal(err)
	}
	var result browseFindResp
	if rec := getJSON(t, s, path, &result); rec.Code != 200 {
		t.Fatalf("indexed dictionary: status %d: %s", rec.Code, rec.Body.String())
	}
	if result.Total != 1 || result.Page != 1 || result.Pages != 1 || len(result.Words) != 1 || result.Words[0] != "corazón" {
		t.Fatalf("search result = %+v", result)
	}
}
