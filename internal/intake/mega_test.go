// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package intake

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/limits"
)

// TestMegaFetch serves an encrypted file the way MEGA does and checks that
// Fetch resolves the link, names the file from its attributes and decrypts it.
func TestMegaFetch(t *testing.T) {
	linkKey := make([]byte, 32)
	for i := range linkKey {
		linkKey[i] = byte(i*7 + 3)
	}
	key := make([]byte, 16)
	for i := range key {
		key[i] = linkKey[i] ^ linkKey[i+16]
	}
	block, _ := aes.NewCipher(key)

	plain := bytes.Repeat([]byte("0123456789abcdefXYZ"), 5000) // not a block multiple
	enc := make([]byte, len(plain))
	iv := append(append([]byte{}, linkKey[16:24]...), make([]byte, 8)...)
	cipher.NewCTR(block, iv).XORKeyStream(enc, plain)

	makeAttrs := func(name string) string {
		attr := []byte(`MEGA{"n":"` + name + `"}`)
		attr = append(attr, make([]byte, (16-len(attr)%16)%16)...)
		cipher.NewCBCEncrypter(block, make([]byte, 16)).CryptBlocks(attr, attr)
		return base64.RawURLEncoding.EncodeToString(attr)
	}
	// These are the main dictionary names accepted by intake: every linked
	// reader format, its supported DSL compression variants, and the two
	// content-claimed Markdown forms. MEGA must resolve them through the same
	// Installable gate before downloading any bytes. Archive containers are
	// accepted by that gate too and can carry one or more of these formats.
	names := []string{
		"bundle.zip", "bundle.7z",
		"oxford.mdx", "star.ifo", "words.dsl", "words.dsl.dz", "words.dsl.dz2",
		"words.lsd", "book.slob", "old.bgl", "wiki.zim", "wiki.zimaa",
		"guide.md", "guide.wudict.md", "guide.wudict.md.gz", "guide.wudict.md.dz",
	}
	nameByID := make(map[string]string, len(names))
	attrsByID := make(map[string]string, len(names))
	for i, name := range names {
		id := fmt.Sprintf("Format%02d", i)
		nameByID[id], attrsByID[id] = name, makeAttrs(name)
	}

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cs":
			var req []map[string]any
			if json.NewDecoder(r.Body).Decode(&req) != nil {
				_, _ = io.WriteString(w, "[-9]")
				return
			}
			id, _ := req[0]["p"].(string)
			_, ok := nameByID[id]
			if !ok {
				_, _ = io.WriteString(w, "[-9]")
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"s": len(enc), "g": srv.URL + "/dl", "at": attrsByID[id],
			}})
		case "/dl":
			http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(enc))
		}
	}))
	defer srv.Close()
	old := megaAPI
	megaAPI = srv.URL + "/cs"
	defer func() { megaAPI = old }()

	f := Fetcher{Insecure: true}
	k := base64.RawURLEncoding.EncodeToString(linkKey)
	// The same encrypted Fetch path works for every dictionary format this
	// intake build recognizes, not only MDX.
	for i, want := range names {
		id := fmt.Sprintf("Format%02d", i)
		link := "https://mega.nz/file/" + id + "#" + k
		src, err := f.Fetch(context.Background(), t.TempDir(), Unwrap(link), nil)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		got, _ := os.ReadFile(src.Path)
		if src.Name != want || !bytes.Equal(got, plain) {
			t.Fatalf("%s: name %q, want %q; %d bytes, equal %v", link, src.Name, want, len(got), bytes.Equal(got, plain))
		}
	}
	// Preserve the other link spellings as compatibility coverage.
	for _, link := range []string{"https://mega.nz/#!Format00!" + k,
		"https://legbehindneck.com/wudict#https://mega.nz/file/Format00%23" + k} {
		src, err := f.Fetch(context.Background(), t.TempDir(), Unwrap(link), nil)
		if err != nil {
			t.Fatalf("%s: %v", link, err)
		}
		got, _ := os.ReadFile(src.Path)
		if src.Name != names[0] || !bytes.Equal(got, plain) {
			t.Fatalf("%s: name %q, bytes equal %v", link, src.Name, bytes.Equal(got, plain))
		}
	}
	// The resolved payload still goes through the ordinary MAX_IMPORT_SIZE
	// check, even though its bytes are decrypted while they are copied.
	oldLimits := limits.Get()
	t.Cleanup(func() { limits.Set(oldLimits) })
	limits.Set(limits.Limits{ImportSize: 1024})
	_, err := f.Fetch(context.Background(), t.TempDir(), "https://mega.nz/file/Format00#"+k, nil)
	if err == nil || !strings.Contains(err.Error(), "MAX_IMPORT_SIZE") {
		t.Fatalf("oversized MEGA file error = %v, want MAX_IMPORT_SIZE refusal", err)
	}

	// A resume mid-block continues the keystream where it stopped.
	u, _ := url.Parse("https://mega.nz/file/Format00#" + k)
	m, err := f.megaResolve(context.Background(), "Format00", linkKey)
	if err != nil {
		t.Fatal(err)
	}
	const off = 1000 + 5
	tail, _ := io.ReadAll(m.reader(bytes.NewReader(enc[off:]), off))
	if !bytes.Equal(tail, plain[off:]) || partBase(u) != "mega-Format00" {
		t.Fatalf("resume matches=%v length=%d want=%d partBase=%q", bytes.Equal(tail, plain[off:]), len(tail), len(plain[off:]), partBase(u))
	}

	if _, err := f.megaResolve(context.Background(), "Missing1", linkKey); err != ErrMegaRefused {
		t.Fatalf("refusal = %v", err)
	}
	if _, _, ok := megaLink(&url.URL{Host: "mega.nz", Path: "/folder/AbCd_123", Fragment: k}); ok {
		t.Fatal("a folder link is not a file link")
	}
}

func TestMegaFolderListsOnlyDirectFiles(t *testing.T) {
	folderKey := bytes.Repeat([]byte{0x39}, 16)
	contentKey := bytes.Repeat([]byte{0x71}, 16)
	fileKey := make([]byte, 32)
	for i := range fileKey {
		fileKey[i] = byte(i*11 + 5)
	}
	// The first four words XOR the last four to the content key.
	for i := 0; i < 16; i++ {
		fileKey[i] = contentKey[i] ^ fileKey[i+16]
	}
	seal := func(b []byte, key []byte) []byte {
		block, _ := aes.NewCipher(key)
		out := append([]byte(nil), b...)
		for i := 0; i < len(out); i += aes.BlockSize {
			block.Encrypt(out[i:i+aes.BlockSize], out[i:i+aes.BlockSize])
		}
		return out
	}
	attrs := func(name string) string {
		b := []byte(`MEGA{"n":"` + name + `"}`)
		b = append(b, make([]byte, (16-len(b)%16)%16)...)
		block, _ := aes.NewCipher(contentKey)
		cipher.NewCBCEncrypter(block, make([]byte, 16)).CryptBlocks(b, b)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	root := "Ldp3iDhI"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("n") != "PVQmBb7Q" {
			t.Errorf("folder context = %q", r.URL.Query().Get("n"))
		}
		var cmd []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil || len(cmd) != 1 || cmd[0]["r"] != float64(0) {
			t.Errorf("folder listing was not non-recursive: %v %v", cmd, err)
		}
		nodes := []map[string]any{
			{"h": root, "p": "TA5X2bgC", "t": 1, "k": root + ":" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))},
			{"h": "FileNode", "p": root, "t": 0, "s": 314, "k": root + ":" + base64.RawURLEncoding.EncodeToString(seal(fileKey, folderKey)), "a": attrs("sample.mdx")},
			{"h": "Image001", "p": root, "t": 0, "s": 27, "k": root + ":" + base64.RawURLEncoding.EncodeToString(seal(fileKey, folderKey)), "a": attrs("sample.bmp")},
			{"h": "Nested01", "p": root, "t": 1, "k": root + ":" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))},
			{"h": "Child001", "p": "Nested01", "t": 0, "s": 99, "k": root + ":" + base64.RawURLEncoding.EncodeToString(seal(fileKey, folderKey)), "a": attrs("sample.mdd")},
		}
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"f": nodes}})
	}))
	defer api.Close()
	old := megaAPI
	megaAPI = api.URL
	defer func() { megaAPI = old }()
	u, _ := url.Parse("https://mega.nz/folder/PVQmBb7Q#" + base64.RawURLEncoding.EncodeToString(folderKey))
	l, ok, err := (Fetcher{Insecure: true}).megaFolderListing(context.Background(), u)
	if err != nil || !ok {
		t.Fatalf("listing: ok=%v err=%v", ok, err)
	}
	if len(l.files) != 2 || l.files[0].name != "sample.mdx" || l.files[0].size != 314 || l.files[1].name != "sample.bmp" || l.files[1].size != 27 {
		t.Fatalf("direct files = %#v", l.files)
	}
}
