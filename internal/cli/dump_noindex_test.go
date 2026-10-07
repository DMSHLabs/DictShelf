// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuweidict/wudict/internal/format/wmd"
	"github.com/wuweidict/wudict/internal/store"
)

// TestDumpPreparesNothing: a dictionary that indexes itself when opened as a
// Dictionary (DSL, BGL, wudict markdown) dumps with its resources, in every
// output, and the library is left as the dump found it.
func TestDumpPreparesNothing(t *testing.T) {
	for _, tc := range []struct {
		format string
		write  func(t *testing.T, dir string) string // the source file
		res    string                                // a resource it ships
		want   string                                // that resource's bytes
	}{
		{"dsl", writeDSLWithFiles, "bark.wav", "WAV"},
		{"wmd", writeWMDWithFiles, "style.css", "b{color:red}"},
		{"bgl", writeBGLWithResource, "pic.png", "PNGDATA"},
	} {
		for _, args := range [][]string{
			{"-format", "csv"},
			{"-format", "md"},
			{"-format", "md", "-mode", "clean"}, // reads the stylesheets too
			{"-format", "md", "-mode", "clean", "-resources", "none"},
		} {
			name := tc.format + " " + strings.Join(args, " ")
			t.Run(name, func(t *testing.T) {
				// The dump loads the config, and a DB_DIR there or in the
				// environment outranks WUDICT_DB_DIR: without these the check
				// below would look at an empty folder while the dump indexed
				// into the developer's own library.
				t.Setenv("HOME", t.TempDir())
				t.Setenv("DB_DIR", "")
				lib := t.TempDir()
				t.Setenv("WUDICT_DB_DIR", lib)
				src := tc.write(t, t.TempDir())
				out := t.TempDir()
				if err := silence(t, func() error { return cmdDump(append(args, "-o", out, src)) }); err != nil {
					t.Fatal(err)
				}
				resDir := filepath.Join(out, dumpBase(src)+".csv_res")
				if args[1] == "md" {
					resDir = filepath.Join(out, dumpBase(src)+".wudict.files")
				}
				if args[len(args)-1] == "none" {
					if _, err := os.Stat(resDir); !os.IsNotExist(err) {
						t.Errorf("-resources none wrote %s", resDir)
					}
				} else if b, err := os.ReadFile(filepath.Join(resDir, tc.res)); err != nil || string(b) != tc.want {
					t.Errorf("resource %s: %q, %v; want %q", tc.res, b, err, tc.want)
				}
				if left, _ := os.ReadDir(lib); len(left) > 0 {
					t.Errorf("the dump prepared the dictionary: the library holds %v", left[0].Name())
				}
			})
		}
	}
}

// TestDumpPrepared: a prepared dictionary dumps by either of its names, the
// library folder a listing shows and the text.db inside it, with its entries
// and the media packed beside it. The text.db still says the format it was
// built from ("dsl"), and that must not send the dump looking for a DSL's
// containers, where there are none: a text.db is a dictionary of its own.
func TestDumpPrepared(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DB_DIR", "")
	lib := t.TempDir()
	t.Setenv("WUDICT_DB_DIR", lib)
	store.SetDBDir(lib)
	t.Cleanup(func() { store.SetDBDir("") })
	src := writeDSLWithFiles(t, t.TempDir())
	prep, err := store.Reconcile(src, store.Target{Media: store.MediaOn}, store.Hooks{})
	// The fork's DSL pipeline packs the fixture's files twice over (the GD
	// variant's own media beside the original's), so the count is only
	// asserted non-zero: the point is that media.db holds them at all.
	if err != nil || prep.Packed < 1 {
		t.Fatalf("preparing the fixture: packed %d, %v", prep.Packed, err)
	}
	// The media now lives only in media.db.
	if err := os.RemoveAll(filepath.Join(filepath.Dir(src), "Dogs.dsl.files")); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Dir(prep.TextDB)
	base := filepath.Base(folder)
	for _, arg := range []string{folder, prep.TextDB} {
		for _, args := range [][]string{
			{"-format", "csv"},
			{"-format", "md"},
			{"-format", "md", "-mode", "clean"},
		} {
			t.Run(filepath.Base(arg)+" "+strings.Join(args, " "), func(t *testing.T) {
				out := t.TempDir()
				if err := silence(t, func() error { return cmdDump(append(args, "-o", out, arg)) }); err != nil {
					t.Fatal(err)
				}
				doc, resDir := filepath.Join(out, base+".csv"), filepath.Join(out, base+".csv_res")
				if args[1] == "md" {
					doc, resDir = filepath.Join(out, base+wmd.Ext), filepath.Join(out, base+".wudict.files")
				}
				if b, err := os.ReadFile(doc); err != nil || !strings.Contains(string(b), "canine") {
					t.Errorf("entries in %s: %v; want the article", filepath.Base(doc), err)
				}
				if b, err := os.ReadFile(filepath.Join(resDir, "bark.wav")); err != nil || string(b) != "WAV" {
					t.Errorf("packed media: %q, %v; want %q", b, err, "WAV")
				}
			})
		}
	}
	if left, _ := os.ReadDir(lib); len(left) != 1 {
		t.Errorf("the library holds %d folders after the dumps; want the 1 prepared above", len(left))
	}
}

func writeDSLWithFiles(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "Dogs.dsl")
	writeFile(t, src, "#NAME \"Dogs\"\n\ndog\n\tcanine [s]bark.wav[/s]\n")
	writeFile(t, filepath.Join(dir, "Dogs.dsl.files", "bark.wav"), "WAV")
	return src
}

func writeWMDWithFiles(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "Dogs"+wmd.Ext)
	writeFile(t, src, "# Dogs\nwudict: 1\n\n## dog\n\n<link rel=\"stylesheet\" href=\"style.css\"><b>canine</b>\n")
	writeFile(t, filepath.Join(dir, "Dogs.wudict.files", "style.css"), "b{color:red}")
	return src
}

// writeBGLWithResource assembles a minimal .bgl: a title, one entry and one
// embedded resource (block layout as in internal/format/bgl's tests).
func writeBGLWithResource(t *testing.T, dir string) string {
	t.Helper()
	block := func(typ byte, data []byte) []byte {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(data)))
		return append(append([]byte{0x30 | typ}, l[:]...), data...)
	}
	var stream []byte
	stream = append(stream, block(3, append([]byte{0, 0x01}, "Dogs"...))...) // title
	stream = append(stream, block(0, []byte{8, 0x42})...)                    // cp1252
	entry := append([]byte{3}, "dog"...)
	entry = append(entry, 0, 6)
	stream = append(stream, block(1, append(entry, "canine"...))...)
	stream = append(stream, block(2, append([]byte{7}, "pic.pngPNGDATA"...))...)

	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write(stream)
	zw.Close()
	src := filepath.Join(dir, "Dogs.bgl")
	writeFile(t, src, string(append([]byte{0x12, 0x34, 0x00, 0x02, 0x00, 0x06}, gz.Bytes()...)))
	return src
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
