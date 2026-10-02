// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestShareDAV(t *testing.T) {
	cases := []struct {
		in, dav, title string
		ok             bool
	}{
		{"https://cloud.x.com/index.php/s/pgKcDcbSDTCzXCs?dir=/ENGLISH/Long%20man%206th%20ed",
			"https://cloud.x.com/public.php/dav/files/pgKcDcbSDTCzXCs/ENGLISH/Long%20man%206th%20ed/", "Long man 6th ed", true},
		{"https://cloud.x.com/s/abc", "https://cloud.x.com/public.php/dav/files/abc/", "", true},
		{"https://x.com/nextcloud/index.php/s/abc/?path=/a", "https://x.com/nextcloud/public.php/dav/files/abc/a/", "a", true},
		{"https://x.com/s/abc?dir=/../../etc", "https://x.com/public.php/dav/files/abc/etc/", "etc", true},
		{"https://x.com/dict/folder/", "", "", false},
		{"https://x.com/s/abc/more", "", "", false},
		{"https://x.com/s/", "", "", false},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.in)
		dav, title, ok := shareDAV(u)
		if ok != c.ok {
			t.Errorf("%s: ok = %v", c.in, ok)
			continue
		}
		if !ok {
			continue
		}
		if dav.String() != c.dav || title != c.title {
			t.Errorf("%s:\n got %s %q\nwant %s %q", c.in, dav, title, c.dav, c.title)
		}
	}
}

// A Nextcloud folder share: the share page names no file, the WebDAV listing
// does, and the files it names are downloaded from their WebDAV addresses.
func TestCollectionFromANextcloudShare(t *testing.T) {
	const tok = "pgKcDcbSDTCzXCs"
	davDir := "/public.php/dav/files/" + tok + "/EN/Long man/"
	files := map[string]string{
		"Long man.mdx": "english english",
		"Long man.mdd": "media media",
		"Readme.md":    "# not a dictionary",
	}
	date := time.Date(2026, 1, 25, 4, 1, 42, 0, time.UTC)
	var propfinds int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/index.php/s/"+tok:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><a href="/core/css/server.css">x</a><noscript>enable js</noscript></body></html>`))
			return
		case r.Method == "PROPFIND" && r.URL.Path == davDir:
			propfinds++
			if r.Header.Get("Depth") != "1" {
				t.Errorf("Depth = %q", r.Header.Get("Depth"))
			}
			esc := (&url.URL{Path: davDir}).EscapedPath()
			var b strings.Builder
			b.WriteString(`<?xml version="1.0"?><d:multistatus xmlns:d="DAV:">`)
			b.WriteString(`<d:response><d:href>` + esc + `</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`)
			b.WriteString(`<d:response><d:href>` + esc + `sub/</d:href><d:propstat><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response>`)
			for name := range files {
				b.WriteString(`<d:response><d:href>` + esc + url.PathEscape(name) + `</d:href><d:propstat><d:prop><d:resourcetype/></d:prop></d:propstat></d:response>`)
			}
			b.WriteString(`</d:multistatus>`)
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = w.Write([]byte(b.String()))
			return
		case strings.HasPrefix(r.URL.Path, davDir) && (r.Method == http.MethodGet || r.Method == http.MethodHead):
			body, ok := files[strings.TrimPrefix(r.URL.Path, davDir)]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, "", date, strings.NewReader(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	dest := t.TempDir()
	m := &Manager{}
	link := srv.URL + "/index.php/s/" + tok + "?dir=/EN/Long%20man"
	if _, err := m.BeginURL(dest, link, loopback()); err != nil {
		t.Fatal(err)
	}
	st := colReady(t, m)
	if propfinds != 1 {
		t.Errorf("PROPFIND requests = %d, want 1", propfinds)
	}
	if got := candNames(st.Candidates); !slices.Equal(got, []string{"Long man"}) {
		t.Fatalf("candidates = %v", got)
	}
	if st.Source != "Long man" {
		t.Errorf("title = %q, want the shared folder's name", st.Source)
	}
	if st.Candidates[0].Date != "2026-01-25" {
		t.Errorf("date = %q", st.Candidates[0].Date)
	}
	if len(st.Extras) != 1 || st.Extras[0].Name != "Long man.mdd" {
		t.Fatalf("extras = %+v", st.Extras)
	}
	if _, err := m.Confirm(dest, []int{0}, Options{Keep: true, Extras: []int{0}}); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.State != StateDone || st.Error != "" {
		t.Fatalf("state = %q (%s)", st.State, st.Error)
	}
	for _, f := range []string{"Long man/Long man.mdx", "Long man/Long man.mdd"} {
		if _, err := os.Stat(filepath.Join(dest, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}

// A site with "/s/<word>" in a path that is not Nextcloud: the PROPFIND
// fails, and the page is read as the folder page it is.
func TestShareShapedPathThatIsNotNextcloud(t *testing.T) {
	s := &colSite{files: map[string]string{"x.slob": "slob"}, date: time.Now()}
	inner := s.serve(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/s/page" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<a href="` + inner.URL + `/dict/x.slob">x</a>`))
			return
		}
		http.Error(w, "no", http.StatusMethodNotAllowed)
	}))
	t.Cleanup(srv.Close)
	m := &Manager{}
	if _, err := m.BeginURL(t.TempDir(), srv.URL+"/s/page", loopback()); err != nil {
		t.Fatal(err)
	}
	st := colReady(t, m)
	if got := candNames(st.Candidates); !slices.Equal(got, []string{"x"}) {
		t.Fatalf("candidates = %v", got)
	}
}
