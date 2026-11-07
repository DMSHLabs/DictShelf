// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package mdictwrite_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	_ "github.com/DMSHLabs/DictShelf/internal/format/mdx"
	"github.com/DMSHLabs/DictShelf/internal/mdictwrite"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	for _, media := range []bool{false, true} {
		ext, key, body := ".mdx", "Яблоко", strings.Repeat("<b>Яблоко</b>", 100000)
		if media {
			ext, key, body = ".mdd", `\images\яблоко.png`, strings.Repeat("image-data", 200000)
		}
		path := filepath.Join(dir, "Example"+ext)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		open := func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }
		err = mdictwrite.Write(f, "Example", "About", media, []mdictwrite.Entry{{Key: key, Size: int64(len(body)), Open: open}})
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		d, err := dict.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		if media {
			r, _, err := d.Resource("images/яблоко.png")
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			r.Close()
			if err != nil || string(got) != body {
				t.Fatalf("media mismatch: %v, got %d bytes, want %d", err, len(got), len(body))
			}
		} else {
			if got := d.Keywords(0, 5); len(got) != 1 || got[0] != key {
				t.Fatalf("keys: %v", got)
			}
			hits, err := d.Exact(key, 5)
			if err != nil || len(hits) != 1 {
				t.Fatalf("exact: %v %v", hits, err)
			}
		}
	}
}
