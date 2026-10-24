// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clusterFirstZIM changes the builder's physical order from dirents, clusters
// to clusters, dirents. Pointer lists remain in place and point at the moved
// bytes. This exercises the ZIM layout where dirents follow the last cluster.
func clusterFirstZIM(t *testing.T, original []byte) []byte {
	t.Helper()
	b := bytes.Clone(original)
	pathPtr := int(binary.LittleEndian.Uint64(b[32:]))
	clusterPtr := int(binary.LittleEndian.Uint64(b[48:]))
	entries := int(binary.LittleEndian.Uint32(b[24:]))
	clusters := int(binary.LittleEndian.Uint32(b[28:]))
	if entries == 0 || clusters == 0 {
		t.Fatal("fixture needs entries and clusters")
	}
	firstDirent := pathPtr
	for i := 0; i < entries; i++ {
		firstDirent = min(firstDirent, int(binary.LittleEndian.Uint64(b[pathPtr+i*8:])))
	}
	firstCluster := pathPtr
	for i := 0; i < clusters; i++ {
		firstCluster = min(firstCluster, int(binary.LittleEndian.Uint64(b[clusterPtr+i*8:])))
	}
	if firstDirent >= firstCluster || firstCluster >= pathPtr {
		t.Fatalf("unexpected builder layout: %d, %d, %d", firstDirent, firstCluster, pathPtr)
	}
	dirents := bytes.Clone(b[firstDirent:firstCluster])
	clusterData := bytes.Clone(b[firstCluster:pathPtr])
	copy(b[firstDirent:], clusterData)
	copy(b[firstDirent+len(clusterData):pathPtr], dirents)
	for i := 0; i < entries; i++ {
		at := pathPtr + i*8
		p := binary.LittleEndian.Uint64(b[at:])
		binary.LittleEndian.PutUint64(b[at:], p+uint64(len(clusterData)))
	}
	for i := 0; i < clusters; i++ {
		at := clusterPtr + i*8
		p := binary.LittleEndian.Uint64(b[at:])
		binary.LittleEndian.PutUint64(b[at:], p-uint64(len(dirents)))
	}
	return b
}

func openZIMBytes(t *testing.T, b []byte) *Dict {
	t.Helper()
	path := filepath.Join(t.TempDir(), "layout.zim")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestClusterBoundariesAndDirectResources(t *testing.T) {
	for _, codec := range []byte{1, 2, 4, 5} {
		for _, clusterFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("codec=%d clusterFirst=%v", codec, clusterFirst), func(t *testing.T) {
				f := sample()
				for i := range f.clusters {
					f.clusters[i].codec = codec
				}
				f.gap = []byte(strings.Repeat("not a cluster\x00", 9))
				b := buildZIM(t, f)
				if clusterFirst {
					b = clusterFirstZIM(t, b)
				}
				d := openZIMBytes(t, b)
				_, end := d.c.extent(d.c.clusterCount - 1)
				wantEnd := uint64(d.c.pathPtrPos)
				if clusterFirst {
					wantEnd = d.c.firstDirent
				}
				if end != wantEnd {
					t.Errorf("last cluster bound = %d, want %d", end, wantEnd)
				}
				res, err := d.Exact("cat", 1)
				if err != nil || len(res) != 1 {
					t.Fatalf("Exact(cat) = %v, %v", res, err)
				}
				rc, mime, err := d.Resource("f.png")
				if err != nil {
					t.Fatal(err)
				}
				defer rc.Close()
				body, err := io.ReadAll(rc)
				if err != nil || mime != "image/png" || string(body) != "\x89PNG\r\n\x1a\n" {
					t.Fatalf("resource = %q, %q, %v", body, mime, err)
				}
				if codec == 1 {
					seeker, ok := rc.(io.Seeker)
					if !ok {
						t.Fatal("stored resource lost seekability")
					}
					if _, err := seeker.Seek(1, io.SeekStart); err != nil {
						t.Fatal(err)
					}
					one := make([]byte, 1)
					if _, err := io.ReadFull(rc, one); err != nil || one[0] != 'P' {
						t.Fatalf("seeked read = %q, %v", one, err)
					}
				}
			})
		}
	}
}

func TestClusterPointersOutOfFileOrder(t *testing.T) {
	for _, codec := range []byte{1, 5} {
		t.Run(fmt.Sprint(codec), func(t *testing.T) {
			f := sample()
			f.reverseClusters = true
			for i := range f.clusters {
				f.clusters[i].codec = codec
			}
			d := openZIMBytes(t, buildZIM(t, f))
			if d.c.nextStart == nil {
				t.Fatal("unordered cluster list treated as file order")
			}
			res, err := d.Exact("cat", 1)
			if err != nil || len(res) != 1 {
				t.Fatalf("Exact(cat) = %v, %v", res, err)
			}
			rc, _, err := d.Resource("f.png")
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(rc)
			closeErr := rc.Close()
			if readErr != nil || closeErr != nil || string(body) != "\x89PNG\r\n\x1a\n" {
				t.Fatalf("Resource(f.png) = %q, read %v, close %v", body, readErr, closeErr)
			}
		})
	}
}

func TestRedirectResolvesReferencesAtStoredArticlePath(t *testing.T) {
	f := bFile{
		major: 6, minor: 3,
		clusters: []bCluster{{codec: 1}},
		entries: []bEntry{
			{ns: 'C', path: "alias", target: "C/docs/page"},
			{ns: 'C', path: "assets/pic.png", mime: "image/png", body: []byte("image"), cluster: 0},
			{ns: 'C', path: "docs/page", mime: "text/html", body: []byte(`<body><img src="../assets/pic.png"><a href="../other">other</a></body>`), cluster: 0},
			{ns: 'C', path: "other", mime: "text/html", body: []byte("other"), cluster: 0},
		},
	}
	d := openZIMBytes(t, buildZIM(t, f))
	res, err := d.Exact("alias", 1)
	if err != nil || len(res) != 1 {
		t.Fatalf("Exact(alias) = %v, %v", res, err)
	}
	if !strings.Contains(res[0].Body, `src="assets/pic.png"`) || !strings.Contains(res[0].Body, `href="entry://other"`) {
		t.Fatalf("redirect body references = %q", res[0].Body)
	}
	rc, _, err := d.Resource("assets/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(rc)
	closeErr := rc.Close()
	if readErr != nil || closeErr != nil || string(got) != "image" {
		t.Fatalf("resolved resource = %q, read %v, close %v", got, readErr, closeErr)
	}
}
