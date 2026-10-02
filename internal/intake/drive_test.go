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

func TestDriveDirect(t *testing.T) {
	const id = "1IZGqgDMeaa4cA28Rt5D7LI89xNBf9pey"
	want := "https://drive.usercontent.google.com/download?confirm=t&export=download&id=" + id
	cases := []struct {
		in, want string
	}{
		{"https://drive.google.com/file/d/" + id + "/view?usp=drive_link", want},
		{"https://drive.google.com/file/d/" + id, want},
		{"https://drive.google.com/file/u/1/d/" + id + "/view", want},
		{"https://drive.google.com/open?id=" + id, want},
		{"https://drive.google.com/uc?id=" + id + "&export=download", want},
		{"https://docs.google.com/uc?id=" + id, want},
		{"https://drive.usercontent.google.com/download?id=" + id, want},
		{"https://drive.google.com/file/d/" + id + "/view?resourcekey=0-abc", want + "&resourcekey=0-abc"},
		{"https://drive.google.com/drive/folders/" + id, ""},
		{"https://docs.google.com/document/d/" + id + "/edit", ""},
		{"https://drive.google.com/file/d/short/view", ""},
		{"https://example.org/file/d/" + id + "/view", ""},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.in)
		d, ok := driveDirect(u)
		got := ""
		if ok {
			got = d.String()
		}
		if got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	// Check is where the rewrite happens, so every path that fetches gets it.
	u, err := Fetcher{}.Check(cases[0].in)
	if err != nil || u.String() != want {
		t.Fatalf("Check = %v, %v", u, err)
	}
	if partBase(u) != "gdrive-"+id {
		t.Errorf("partBase = %q", partBase(u))
	}
}

func TestUnwrapAll(t *testing.T) {
	const share = "https://legbehindneck.com/wudict#"
	a, b := "https://drive.google.com/file/d/AAAAAAAAAAAA/view", "https://drive.google.com/file/d/BBBBBBBBBBBB/view"
	cases := []struct {
		in   string
		want []string
	}{
		{share + a + "," + b, []string{a, b}},
		{share + a + ";" + b, []string{a, b}},
		{share + a + "|" + b, []string{a, b}},
		{share + a + "%20" + b, []string{a, b}},
		{share + a + "%2C" + b, []string{a, b}},
		{share + a + ",%20" + b + ",", []string{a, b}},
		{"https://legbehindneck.com/wudict/#" + a + "," + b, []string{a, b}},
		// One link that carries another, or a comma that starts no link.
		{share + "https://x.org/get?u=https://y.org/a.mdx", []string{"https://x.org/get?u=https://y.org/a.mdx"}},
		{share + "https://x.org/a,b.mdx", []string{"https://x.org/a,b.mdx"}},
		{share + "/dict/euskera/", []string{"https://legbehindneck.com/dict/euskera/"}},
		// Not a share link: untouched, commas and all.
		{a + "," + b, []string{a + "," + b}},
	}
	for _, c := range cases {
		if got := UnwrapAll(c.in); !slices.Equal(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	// And through parseText, which is what BeginURL reads a paste with.
	if l := parseText(share+a+","+b, nil); len(l.links) != 2 {
		t.Errorf("parseText found %d links", len(l.links))
	}
}

// driveSite stands in for drive.usercontent.google.com: files by id, named
// only by Content-Disposition; an id it does not share answers with a page.
func driveSite(t *testing.T, files map[string][2]string) {
	t.Helper()
	date := time.Date(2026, 9, 19, 10, 18, 10, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/download" || q.Get("confirm") != "t" {
			http.NotFound(w, r)
			return
		}
		f, ok := files[q.Get("id")]
		if !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><a href="https://support.google.com/x">help</a></html>`))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+f[0]+`"`)
		http.ServeContent(w, r, "", date, strings.NewReader(f[1]))
	}))
	t.Cleanup(srv.Close)
	su, _ := url.Parse(srv.URL)
	prev := driveDownload
	driveDownload = url.URL{Scheme: su.Scheme, Host: su.Host, Path: "/download"}
	t.Cleanup(func() { driveDownload = prev })
}

// The case asked for: an .mdx and its .mdd as two Drive links in one share
// link. Neither URL names its file; the server's answers do, the two group
// into one dictionary, and both land in one folder under their real names.
func TestDriveCollectionFromAShareLink(t *testing.T) {
	const mdx, mdd = "MDXMDXMDXMDXmdx", "MDDMDDMDDMDDmdd"
	driveSite(t, map[string][2]string{
		mdx: {"eu-es-Elhuyar.mdx", "basque spanish"},
		mdd: {"eu-es-Elhuyar.mdd", "media"},
	})
	link := "https://legbehindneck.com/wudict#https://drive.google.com/file/d/" + mdx +
		"/view?usp=drive_link,https://drive.google.com/file/d/" + mdd + "/view?usp=drive_link"
	dest := t.TempDir()
	m := &Manager{}
	if _, err := m.BeginURL(dest, link, loopback()); err != nil {
		t.Fatal(err)
	}
	st := colReady(t, m)
	if got := candNames(st.Candidates); !slices.Equal(got, []string{"eu-es-Elhuyar"}) {
		t.Fatalf("candidates = %v", got)
	}
	if st.Source != "Google Drive" {
		t.Errorf("title = %q", st.Source)
	}
	if len(st.Extras) != 1 || st.Extras[0].Name != "eu-es-Elhuyar.mdd" {
		t.Fatalf("extras = %+v", st.Extras)
	}
	if _, err := m.Confirm(dest, []int{0}, Options{Keep: true, Extras: []int{0}}); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.State != StateDone || st.Error != "" {
		t.Fatalf("state = %q (%s)", st.State, st.Error)
	}
	for _, f := range []string{"eu-es-Elhuyar/eu-es-Elhuyar.mdx", "eu-es-Elhuyar/eu-es-Elhuyar.mdd"} {
		b, err := os.ReadFile(filepath.Join(dest, f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := map[bool]string{true: "basque spanish", false: "media"}[strings.HasSuffix(f, ".mdx")]; string(b) != want {
			t.Errorf("%s = %q: the two downloads were mixed up", f, b)
		}
	}
}

// One Drive link alone, and one that Drive will not hand over.
func TestDriveSingleLink(t *testing.T) {
	const id = "SLOBSLOBSLOBslob"
	driveSite(t, map[string][2]string{id: {"wiki.slob", "slob bytes"}})
	m := &Manager{}
	dest := t.TempDir()
	if _, err := m.BeginURL(dest, "https://drive.google.com/file/d/"+id+"/view", loopback()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.State != StateReady || st.Source != "wiki.slob" {
		t.Fatalf("state = %q source = %q (%s)", st.State, st.Source, st.Error)
	}

	if _, err := m.BeginURL(dest, "https://drive.google.com/file/d/NOTSHAREDNOTSHARED/view", loopback()); err != nil {
		t.Fatal(err)
	}
	m.Wait()
	if st := m.Status(); st.State != StateError || !strings.Contains(st.Error, "Google Drive") {
		t.Fatalf("state = %q error = %q, want Drive's refusal named", st.State, st.Error)
	}
}

func TestSharePageIsNotALink(t *testing.T) {
	for _, s := range []string{
		"https://legbehindneck.com/wudict", "https://legbehindneck.com/wudict/",
		"https://www.legbehindneck.com/wudict#", "https://legbehindneck.com/wudict/# ",
		"  https://legbehindneck.com/wudict  ",
	} {
		if !IsSharePage(s) {
			t.Errorf("IsSharePage(%q) = false", s)
		}
		m := &Manager{}
		if _, err := m.BeginURL(t.TempDir(), s, loopback()); err != ErrSharePage {
			t.Errorf("BeginURL(%q) = %v, want ErrSharePage", s, err)
		}
	}
	for _, s := range []string{
		"https://legbehindneck.com/wudict#https://x.org/a.mdx", "https://legbehindneck.com/wudict#/dict/eu/",
		"https://legbehindneck.com/dict/euskera/", "https://example.org/wudict",
	} {
		if IsSharePage(s) {
			t.Errorf("IsSharePage(%q) = true", s)
		}
	}
}
