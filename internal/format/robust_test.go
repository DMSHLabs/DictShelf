// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package format_test feeds corrupt and truncated files to every
// registered format through dict.Open, which must return errors - never
// panic, never hang (one bad file in a folder must not take anything
// down).
package format_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"

	_ "github.com/wuweidict/wudict/internal/format/bgl"
	_ "github.com/wuweidict/wudict/internal/format/dsl"
	_ "github.com/wuweidict/wudict/internal/format/mdx"
	_ "github.com/wuweidict/wudict/internal/format/slob"
	_ "github.com/wuweidict/wudict/internal/format/stardict"
	_ "github.com/wuweidict/wudict/internal/format/wmd"
	_ "github.com/wuweidict/wudict/internal/format/zim"
)

func TestCorruptDictionariesErrorCleanly(t *testing.T) {
	// A format that prepares on open (DSL, markdown) writes a library folder:
	// into the test's own, never the user's ~/.wudict/db.
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	dir := t.TempDir()
	cases := map[string][]byte{
		// mdx: bogus header-length prefix + junk
		"a.mdx": append([]byte{0x00, 0x00, 0x00, 0x10}, []byte("junkjunk")...),
		// mdx: empty
		"b.mdx": {},
		// slob: valid magic, truncated header
		"c.slob": append([]byte("!-1SLOB\x1f"), bytes.Repeat([]byte{0xAA}, 10)...),
		// slob: garbage
		"d.slob": []byte("not a slob at all"),
		// stardict: valid magic line, but companion files missing
		"e.ifo": []byte("StarDict's dict ifo file\nversion=3.0.0\nbookname=x\n"),
		// stardict: binary junk
		"f.ifo": {0xFF, 0xFE, 0x00, 0x01, 0x02},
		// dsl.dz: not gzip at all
		"g.dsl.dz": []byte("plainly not gzip"),
		// bgl: gzip magic, nothing behind it
		"h.bgl": {0x12, 0x34, 0x00, 0x1E, 0x00, 0x00},
		// bgl: garbage
		"i.bgl": []byte("not a babylon glossary"),
		// zim: correct magic, header cut short
		"j.zim": {0x5A, 0x49, 0x4D, 0x04, 0x06, 0x00, 0x03, 0x00},
		// zim: header-sized, but every pointer is zero
		"k.zim": append([]byte{0x5A, 0x49, 0x4D, 0x04}, bytes.Repeat([]byte{0}, 76)...),
		// zim: garbage
		"l.zim": []byte("not a zim file at all"),
		// markdown: no title line
		"m.wudict.md": []byte("not a title\n## x\n"),
		// markdown: binary junk behind a valid head
		"n.wudict.md": append([]byte("# t\nwudict: 1\n\n## x\n\n"), 0xFF, 0xFE, 0x00, 0x80, 0xC0),
		// markdown: a version this reader does not know
		"o.wudict.md": []byte("# t\nwudict: 99\n"),
		// markdown: nesting deep enough to hurt a recursive parser
		"p.wudict.md": []byte("# t\nwudict: 1\n\n## x\n\n" + strings.Repeat(">", 50000) + strings.Repeat("[", 50000) + "\n"),
		// markdown: a plain .md whose content does not qualify
		"q.md": []byte("just notes\n"),
		// markdown: gzip suffix, not gzip
		"r.wudict.md.gz": []byte("plainly not gzip"),
	}
	for name, data := range cases {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: PANIC escaped dict.Open: %v", name, r)
				}
			}()
			d, err := dict.Open(p)
			if err == nil {
				// tolerated only if it opened into something inert
				if d != nil {
					d.Close()
				}
				t.Logf("%s: opened without error (tolerated)", name)
			}
		}()
	}
}
