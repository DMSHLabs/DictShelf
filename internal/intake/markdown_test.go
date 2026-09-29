// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/wuweidict/wudict/internal/format/wmd"
)

// TestMarkdownGate: WuWeiDict markdown is a dictionary by its content - the
// `wudict` field on line 2 - whatever it is called, in an archive and as a
// loose file alike (spec R2.2). A README beside it is carried by nobody.
func TestMarkdownGate(t *testing.T) {
	const dictMD = "# Dict\nwudict: 1\n\n## word\n\ndef\n"
	const notes = "# Notes\n\n## Install\n"
	bodies := map[string]string{
		"a/Plain.md":               dictMD,
		"a/Named.wudict.md":        dictMD,
		"a/README.md":              notes,
		"a/Bare.wudict.md":         notes, // the name alone does not qualify
		"a/Named.wudict.files.zip": "PK",
	}
	var entries []Entry
	for name, b := range bodies {
		entries = append(entries, Entry{Name: name, Dir: "a", Base: filepath.Base(name), Size: int64(len(b)), Compressed: int64(len(b))})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	cands, err := Sniff(&fakeArchive{entries: entries, bodies: bodies})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cands {
		got = append(got, c.Main+"="+strings.Join(c.Files, "+"))
	}
	sort.Strings(got)
	want := "a/Named.wudict.md=a/Named.wudict.md+a/Named.wudict.files.zip,a/Plain.md=a/Plain.md"
	if strings.Join(got, ",") != want {
		t.Errorf("candidates = %v, want %s", got, want)
	}

	// Loose files: an import may begin with any of these names, and the
	// content settles it.
	dir := t.TempDir()
	for name, b := range map[string]string{"g.md": dictMD, "readme.md": notes} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"g.md", "g.wudict.md", "g.wudict.md.gz", "readme.md"} {
		if !Installable(name) {
			t.Errorf("Installable(%q) = false", name)
		}
	}
	if _, err := OpenPlain(filepath.Join(dir, "g.md")); err != nil {
		t.Errorf("OpenPlain(dictionary) = %v", err)
	}
	if _, err := OpenPlain(filepath.Join(dir, "readme.md")); err == nil {
		t.Error("OpenPlain(readme) accepted")
	}
	if !holdsDictionary(dir) {
		t.Error("holdsDictionary missed the markdown dictionary")
	}
	os.Remove(filepath.Join(dir, "g.md"))
	if holdsDictionary(dir) {
		t.Error("holdsDictionary took a README for a dictionary")
	}
}
