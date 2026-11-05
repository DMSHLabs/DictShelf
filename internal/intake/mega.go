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
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// MEGA file links: mega.nz/file/<id>#<key>, and the older mega.nz/#!<id>!<key>.
//
// The file is encrypted and the key travels in the fragment, which no server
// sees. One API call ("g") turns the id into a temporary download address,
// the size and the encrypted attributes holding the file name; the bytes are
// then an ordinary download decrypted as they land - AES-128-CTR, so a resume
// continues the keystream at offset/16 and the size on the wire is the file's.
// Folder links are not handled. The MAC is not checked: TLS guards the
// transfer and every reader here rejects a corrupt file.

// ErrMegaRefused is MEGA declining the link: removed, wrong, or over the
// anonymous download quota.
var ErrMegaRefused = errors.New("MEGA did not hand over the file: check the link, " +
	"or try again later if MEGA's download quota is used up")

var errMegaFolderTooLarge = errors.New("MEGA folder contains too many files")

// megaAPI is MEGA's API endpoint; a variable only so a test can stand a local
// server in its place.
var megaAPI = "https://g.api.mega.co.nz/cs"

var megaIDRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8}$`)

// megaCheck extends an already accepted MEGA link's host policy to MEGA's API
// and its file storage subdomains, while retaining scheme and address checks.
func (f Fetcher) megaCheck(raw string) (*url.URL, error) {
	return f.megaFetcher().Check(raw)
}

func (f Fetcher) megaFetcher() Fetcher {
	if len(f.Hosts) > 0 {
		f.Hosts = append(append([]string(nil), f.Hosts...), "mega.nz", "mega.co.nz")
	}
	return f
}

type megaFile struct {
	url  *url.URL
	name string
	key  []byte // AES-128 key
	iv   []byte // 8-byte nonce; the low 8 bytes of the counter block count blocks
}

// megaLink returns the id and the 32-byte link key of a MEGA file link.
func megaLink(u *url.URL) (id string, key []byte, ok bool) {
	switch strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") {
	case "mega.nz", "mega.co.nz":
	default:
		return "", nil, false
	}
	var k string
	if rest, found := strings.CutPrefix(u.Path, "/file/"); found {
		// "/file/<id>%23<key>": a share link whose inner link had only its
		// "#" escaped. A MEGA id never holds "#", so the cut is unambiguous.
		k = u.Fragment
		if i, j, cut := strings.Cut(rest, "#"); cut && k == "" {
			rest, k = i, j
		}
		id = strings.TrimSuffix(rest, "/")
	} else if rest, found := strings.CutPrefix(u.Fragment, "!"); found {
		id, k, _ = strings.Cut(rest, "!")
	}
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k, "="))
	if err != nil || len(key) != 32 || !megaIDRE.MatchString(id) {
		return "", nil, false
	}
	return id, key, true
}

// megaResolve asks MEGA for the download address and name of a file link.
func (f Fetcher) megaResolve(ctx context.Context, id string, linkKey []byte) (*megaFile, error) {
	return f.megaResolveContext(ctx, id, linkKey, "")
}

func (f Fetcher) megaResolveContext(ctx context.Context, id string, linkKey []byte, folder string) (*megaFile, error) {
	api, err := f.megaCheck(megaAPI + "?id=" + strconv.FormatUint(uint64(rand.Uint32()), 10))
	if err != nil {
		return nil, err
	}
	cmd := map[string]any{"a": "g", "g": 1, "ssl": 2}
	if folder != "" {
		cmd["n"] = id
	} else {
		cmd["p"] = id
	}
	body, _ := json.Marshal([]map[string]any{cmd})
	if folder != "" {
		q := api.Query()
		q.Set("n", folder)
		api.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.megaFetcher().Client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	// An error is a bare number ([-9], or -3 unwrapped), which fails to
	// decode into the struct: every refusal reads the same to the user.
	var out []struct {
		G  string `json:"g"`
		At string `json:"at"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(b, &out) != nil || len(out) != 1 || out[0].G == "" {
		return nil, ErrMegaRefused
	}
	dl, err := url.Parse(out[0].G)
	if err != nil {
		return nil, ErrMegaRefused
	}
	if dl.Scheme == "http" && !f.Insecure {
		dl.Scheme = "https" // MEGA's storage hosts serve both
	}
	if dl, err = f.megaCheck(dl.String()); err != nil {
		return nil, err
	}
	m := &megaFile{url: dl, key: make([]byte, 16), iv: linkKey[16:24]}
	for i := range m.key {
		m.key[i] = linkKey[i] ^ linkKey[i+16]
	}
	m.name = safeDirName(filepath.Base(megaName(out[0].At, m.key)))
	if !Installable(m.name) {
		return nil, ErrNotArchive
	}
	return m, nil
}

// megaFolderListing resolves just the immediate files in a public folder.
// It requests a non-recursive listing and checks parent and type again before
// exposing a node, so descendants and child folders cannot enter the import.
func (f Fetcher) megaFolderListing(ctx context.Context, u *url.URL) (listing, bool, error) {
	if strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != "mega.nz" && strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.") != "mega.co.nz" {
		return listing{}, false, nil
	}
	rest, ok := strings.CutPrefix(u.Path, "/folder/")
	if !ok {
		return listing{}, false, nil
	}
	id := strings.TrimSuffix(rest, "/")
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(u.Fragment, "="))
	if err != nil || !megaIDRE.MatchString(id) || len(key) != 16 {
		return listing{}, false, ErrMegaRefused
	}
	api, err := f.megaCheck(megaAPI + "?id=" + strconv.FormatUint(uint64(rand.Uint32()), 10))
	if err != nil {
		return listing{}, true, err
	}
	api.RawQuery += "&n=" + url.QueryEscape(id)
	body, _ := json.Marshal([]map[string]any{{"a": "f", "c": 1, "r": 0, "ca": 1}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.String(), bytes.NewReader(body))
	if err != nil {
		return listing{}, true, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.megaFetcher().Client().Do(req)
	if err != nil {
		return listing{}, true, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return listing{}, true, err
	}
	var response []struct {
		F []struct {
			H string `json:"h"`
			P string `json:"p"`
			T int    `json:"t"`
			S int64  `json:"s"`
			K string `json:"k"`
			A string `json:"a"`
		} `json:"f"`
	}
	if resp.StatusCode != 200 || json.Unmarshal(b, &response) != nil {
		return listing{}, true, ErrMegaRefused
	}
	if len(response) != 1 {
		return listing{}, true, ErrMegaRefused
	}
	nodes := response[0].F
	root := ""
	for _, n := range nodes {
		if n.T == 1 && strings.Contains(n.K, n.H+":") {
			root = n.H
			break
		}
	}
	if root == "" {
		return listing{}, true, ErrMegaRefused
	}
	l := listing{title: "MEGA"}
	count := 0
	for _, n := range nodes {
		if n.T != 0 || n.P != root || n.S < 0 {
			continue
		}
		count++
		if count > maxLinks {
			return listing{}, true, errMegaFolderTooLarge
		}
		var enc []byte
		for _, part := range strings.Split(n.K, "/") {
			owner, val, has := strings.Cut(part, ":")
			if owner == root && has {
				enc, _ = base64.RawURLEncoding.DecodeString(strings.TrimRight(val, "="))
				break
			}
		}
		if len(enc) != 32 {
			continue
		}
		block, _ := aes.NewCipher(key)
		block.Decrypt(enc[:16], enc[:16])
		block.Decrypt(enc[16:], enc[16:])
		words := make([]uint32, 8)
		for i := range words {
			words[i] = binary.BigEndian.Uint32(enc[i*4:])
		}
		lk := append([]byte(nil), enc...)
		contentKey := make([]byte, 16)
		for i := 0; i < 4; i++ {
			binary.BigEndian.PutUint32(contentKey[i*4:], words[i]^words[i+4])
		}
		plainName := megaName(n.A, contentKey)
		if plainName == "" {
			continue
		}
		name := safeDirName(plainName)
		fileURL := "https://mega.nz/file/" + n.H + "?folder=" + url.QueryEscape(id) + "#" + base64.RawURLEncoding.EncodeToString(lk)
		l.files = append(l.files, &remoteFile{name: name, url: fileURL, size: n.S})
	}
	if len(l.files) == 0 {
		return listing{}, true, ErrNoLinks
	}
	return l, true, nil
}

func isMegaFolderURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return (host == "mega.nz" || host == "mega.co.nz") && strings.HasPrefix(u.Path, "/folder/")
}

// megaName decrypts the attributes - AES-CBC, zero IV, "MEGA{json}" padded
// with NULs - and returns the file name they carry, or "".
func megaName(at string, key []byte) string {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(at, "="))
	if err != nil || len(b) == 0 || len(b)%aes.BlockSize != 0 {
		return ""
	}
	block, _ := aes.NewCipher(key) // 16 bytes by construction
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(b, b)
	js, ok := bytes.CutPrefix(bytes.TrimRight(b, "\x00"), []byte("MEGA"))
	var a struct {
		N string `json:"n"`
	}
	if !ok || json.Unmarshal(js, &a) != nil {
		return ""
	}
	return a.N
}

// reader decrypts r, the file's bytes from offset on.
func (m *megaFile) reader(r io.Reader, offset int64) io.Reader {
	block, _ := aes.NewCipher(m.key)
	ctr := make([]byte, aes.BlockSize)
	copy(ctr, m.iv)
	binary.BigEndian.PutUint64(ctr[8:], uint64(offset/aes.BlockSize))
	s := cipher.NewCTR(block, ctr)
	if skip := offset % aes.BlockSize; skip > 0 {
		var d [aes.BlockSize]byte
		s.XORKeyStream(d[:skip], d[:skip])
	}
	return cipher.StreamReader{S: s, R: r}
}
