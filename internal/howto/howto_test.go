// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package howto

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/format/wmd"
)

type article struct {
	names []string
	body  string
}

// read installs the guide into a temporary folder and reads it back through
// the wudict markdown reader, as the server will.
func read(t *testing.T) (dict.Meta, []article, string) {
	t.Helper()
	dir := t.TempDir()
	p, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := wmd.NewReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []article
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, article{e.Headwords, e.Body})
	}
	if w, dropped := r.Warnings(); len(w)+dropped > 0 {
		t.Errorf("the guide reads with warnings: %v", w)
	}
	return r.Meta(), out, dir
}

// appPages are the app's own pages the guide may link to.
var appPages = map[string]bool{"/": true, "/browse": true, "/setup": true, "/lemmas": true, "/groups": true}

// TestGuide: the guide is a dictionary like any other - it passes the
// `wudict: 1` gate and reads without a warning - and it keeps its promises:
// every headword is under "wudict ", every link leads somewhere, and every
// image it shows is shipped with it.
func TestGuide(t *testing.T) {
	if err := wmd.Claim(FileName, bytes.NewReader(Source())); err != nil {
		t.Fatalf("the guide fails the gate: %v", err)
	}
	meta, arts, dir := read(t)
	if meta.Name != "wudict howto" {
		t.Errorf("title = %q", meta.Name)
	}
	if len(arts) < 10 {
		t.Fatalf("only %d articles", len(arts))
	}
	if arts[0].names[0] != "wudict welcome" {
		t.Errorf("the first article is %q, want wudict welcome", arts[0].names[0])
	}
	names := map[string]bool{}
	for _, a := range arts {
		for _, n := range a.names {
			if !strings.HasPrefix(n, "wudict ") {
				t.Errorf("headword %q is not under %q", n, "wudict ")
			}
			if names[n] {
				t.Errorf("headword %q twice", n)
			}
			names[n] = true
		}
	}
	used := map[string]bool{}
	check := func(where, tag, attr, v string) {
		switch {
		case strings.HasPrefix(v, "entry://"):
			target, _, _ := strings.Cut(strings.TrimPrefix(v, "entry://"), "#")
			if w, err := url.PathUnescape(target); err != nil || !names[w] {
				t.Errorf("%s: link to %q, which is no headword of the guide", where, target)
			}
		case strings.HasPrefix(v, "/"):
			u, err := url.Parse(v)
			if err != nil || !appPages[u.Path] {
				t.Errorf("%s: link to %q, which is no page of the app", where, v)
			}
			if d := u.Query().Get("dict"); d != "" && d != ID {
				t.Errorf("%s: link names dictionary %q, want %q", where, d, ID)
			}
		case strings.HasPrefix(v, "https://") && tag == "a":
			// the project's own pages - its source, its site - open in a new tab
		case tag == "img":
			if _, err := os.Stat(filepath.Join(dir, FilesDir, v)); err != nil {
				t.Errorf("%s: image %q is not shipped", where, v)
			}
			used[v] = true
		default:
			t.Errorf("%s: %s %s=%q leads outside the app", where, tag, attr, v)
		}
	}
	for _, a := range append(arts, article{[]string{"(description)"}, meta.Description}) {
		z := html.NewTokenizer(strings.NewReader(a.body))
		for tt := z.Next(); tt != html.ErrorToken; tt = z.Next() {
			if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
				continue
			}
			tok := z.Token()
			for _, at := range tok.Attr {
				if at.Key == "href" || at.Key == "src" {
					check(a.names[0], tok.Data, at.Key, at.Val)
				}
			}
		}
	}
	shipped, _ := fs.ReadDir(files, FilesDir)
	for _, f := range shipped {
		if !used[f.Name()] {
			t.Errorf("image %s is shipped but never shown", f.Name())
		}
	}
}

// TestGuideIsCleanMarkdown: the guide is written in exactly the form `wudict
// dump -mode clean` writes - dumping it gives it back byte for byte - which is
// what makes it the proof of the format rather than a sample of it.
func TestGuideIsCleanMarkdown(t *testing.T) {
	meta, arts, _ := read(t)
	w, err := wmd.NewWriter(wmd.Head{
		Name: meta.Name, From: meta.IndexLang, To: meta.ContentsLang,
		Fields: meta.Header, Description: meta.Description,
	}, wmd.ModeClean, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, a := range arts {
		if err := w.Article(a.names, a.body); err != nil {
			t.Fatalf("%s: %v", a.names[0], err)
		}
	}
	var b bytes.Buffer
	if _, err := w.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	if got, want := b.String(), string(Source()); got != want {
		gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
		for i := 0; i < max(len(gl), len(wl)); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Fatalf("a clean dump differs from the guide at line %d:\n dump:  %q\n guide: %q", i+1, g, w)
			}
		}
	}
}

// TestInstall: an unchanged guide is never rewritten; a changed one is, and
// an image it no longer has goes.
func TestInstall(t *testing.T) {
	dir := t.TempDir()
	p, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	stale := filepath.Join(dir, FilesDir, "old.svg")
	os.WriteFile(stale, []byte("<svg/>"), 0o644)
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if fi2, _ := os.Stat(p); !fi2.ModTime().Equal(fi.ModTime()) {
		t.Error("an unchanged guide was rewritten")
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("an image the guide no longer has was kept")
	}
	os.WriteFile(p, []byte("edited"), 0o644)
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, Source()) {
		t.Error("a changed guide was not restored")
	}
}

// TestCopyTo: a copy for the user to edit, never over one already there.
func TestCopyTo(t *testing.T) {
	dir := t.TempDir()
	p, err := CopyTo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, Source()) {
		t.Error("the copy differs")
	}
	if _, err := os.Stat(filepath.Join(dir, FilesDir, "cog.svg")); err != nil {
		t.Errorf("images not copied: %v", err)
	}
	os.WriteFile(p, []byte("mine"), 0o644)
	if _, err := CopyTo(dir); !errors.Is(err, fs.ErrExist) {
		t.Errorf("second copy: %v, want ErrExist", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "mine" {
		t.Error("the user's copy was overwritten")
	}
}

// TestRemoveRestore: a removed guide stays removed until Restore brings it
// back whole.
func TestRemoveRestore(t *testing.T) {
	dir := t.TempDir()
	p, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	if IsRemoved(dir) {
		t.Fatal("removed before anything was removed")
	}
	if err := MarkRemoved(dir); err != nil {
		t.Fatal(err)
	}
	if !IsRemoved(dir) {
		t.Error("not marked removed")
	}
	for _, f := range []string{p, filepath.Join(dir, FilesDir)} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s is still there", f)
		}
	}
	if _, err := Restore(dir); err != nil {
		t.Fatal(err)
	}
	if IsRemoved(dir) {
		t.Error("still marked removed after Restore")
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, Source()) {
		t.Error("Restore did not bring the guide back")
	}
}
