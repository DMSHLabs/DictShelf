// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"bytes"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/wuweidict/wudict/internal/dict"
)

// writeSplit cuts b into parts of size n (the last one shorter), named as
// `split` names them, and returns the first part's path.
func writeSplit(t *testing.T, b []byte, n int, name string) string {
	t.Helper()
	dir := t.TempDir()
	suffixes := append([]string{".zimaa"}, dict.ZIMPartSuffixes()...)
	for k := 0; len(b) > 0; k++ {
		m := min(n, len(b))
		if err := os.WriteFile(filepath.Join(dir, name+suffixes[k]), b[:m], 0o644); err != nil {
			t.Fatal(err)
		}
		b = b[m:]
	}
	return filepath.Join(dir, name+".zimaa")
}

// Every range of the joined parts reads as the same range of the whole: across
// boundaries, at the end, past it.
func TestJoinedReadAt(t *testing.T) {
	whole := make([]byte, 10_000)
	rng := rand.New(rand.NewPCG(3, 4))
	for i := range whole {
		whole[i] = byte(rng.Uint32())
	}
	for _, part := range []int{16, 97, 4096, 9_999, 10_000, 20_000} { // 16: 625 parts of the 676 names
		first := writeSplit(t, whole, part, "w")
		src, size, err := openJoined(dict.ZIMParts(first))
		if err != nil {
			t.Fatal(err)
		}
		if size != int64(len(whole)) {
			t.Fatalf("part %d: size %d, want %d", part, size, len(whole))
		}
		ref := bytes.NewReader(whole)
		for range 300 {
			off := rng.Int64N(int64(len(whole)) + 50)
			n := rng.IntN(300)
			got, want := make([]byte, n), make([]byte, n)
			gn, gerr := src.ReadAt(got, off)
			wn, werr := ref.ReadAt(want, off)
			if gn != wn || !bytes.Equal(got[:gn], want[:wn]) || (gerr == nil) != (werr == nil) {
				t.Fatalf("part %d: ReadAt(%d bytes at %d) = %d, %v; want %d, %v", part, n, off, gn, gerr, wn, werr)
			}
		}
		src.Close()
	}
}

// A split archive reads as the archive it was cut from, whatever the cut: a
// part size of 97 bytes puts a boundary inside nearly every dirent and blob.
func TestSplitArchive(t *testing.T) {
	whole := buildZIM(t, sample())
	ref, err := Open(writeTruncated(t, whole))
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Close()
	refBlobs, _ := readAll(t, ref)

	for _, part := range []int{97, 512, len(whole) / 2} {
		d, err := Open(writeSplit(t, whole, part, "sample"))
		if err != nil {
			t.Fatalf("part %d: %v", part, err)
		}
		if d.Meta().Name != ref.Meta().Name || d.Meta().EntryCount != ref.Meta().EntryCount {
			t.Errorf("part %d: meta %+v, want %+v", part, d.Meta(), ref.Meta())
		}
		blobs, errs := readAll(t, d)
		if len(errs) > 0 || len(blobs) != len(refBlobs) {
			t.Fatalf("part %d: %d blobs, errors %v", part, len(blobs), errs)
		}
		for k, b := range blobs {
			if !bytes.Equal(b, refBlobs[k]) {
				t.Errorf("part %d: %s differs", part, k)
			}
		}
		// A resource is still a seekable window, now across parts.
		for _, name := range d.Resources() {
			rc, _, err := d.Resource(name)
			if err != nil {
				t.Fatal(err)
			}
			if s, ok := rc.(io.Seeker); ok {
				if _, err := s.Seek(1, io.SeekStart); err != nil {
					t.Errorf("%s: seek: %v", name, err)
				}
			}
			rc.Close()
		}
		if res, err := d.Exact("cat", 1); err != nil || len(res) != 1 {
			t.Errorf("part %d: Exact(cat) = %v, %v", part, res, err)
		}
		d.Close()
		readThrough(t, writeSplit(t, whole, part, "again"), true)
	}
}

// A missing middle part leaves a run that ends early: the archive is refused
// at open or fails where it is read, and never panics.
func TestSplitArchiveMissingPart(t *testing.T) {
	first := writeSplit(t, buildZIM(t, sample()), 97, "gap")
	if err := os.Remove(filepath.Join(filepath.Dir(first), "gap.zimac")); err != nil {
		t.Fatal(err)
	}
	if got := len(dict.ZIMParts(first)); got != 2 {
		t.Fatalf("parts before the gap = %d, want 2", got)
	}
	d, err := Open(first)
	if err == nil {
		defer d.Close()
		if _, errs := readAll(t, d); len(errs) == 0 {
			t.Error("an archive missing a part read without an error")
		}
	}
}
