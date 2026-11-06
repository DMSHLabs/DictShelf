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
	"time"

	"github.com/DMSHLabs/DictShelf/internal/logx"
)

// MEGA file links: mega.nz/file/<id>#<key>, and the older mega.nz/#!<id>!<key>.
//
// The file is encrypted and the key travels in the fragment, which no server
// sees. One API call ("g") turns the id into a temporary download address,
// the size and the encrypted attributes holding the file name; the bytes are
// then an ordinary download decrypted as they land - AES-128-CTR, so a resume
// continues the keystream at offset/16 and the size on the wire is the file's.
// Folder links use MEGA's public folder listing API below. The MAC is not
// checked: TLS guards the transfer and every reader here rejects a corrupt file.

// ErrMegaRefused is MEGA declining the link: removed, wrong, or over the
// anonymous download quota.
var ErrMegaRefused = errors.New("MEGA did not hand over the file: check the link, " +
	"or try again later if MEGA's download quota is used up")

var errMegaFolderTooLarge = errors.New("MEGA folder contains too many files")

// megaAPI is MEGA's API endpoint; a variable only so a test can stand a local
// server in its place.
var megaAPI = "https://g.api.mega.co.nz/cs"

const megaRequestTimeout = 60 * time.Second

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

func (f Fetcher) megaResolveContext(ctx context.Context, id string, linkKey []byte, folder string) (file *megaFile, resultErr error) {
	return f.megaResolveContextAllowIcon(ctx, id, linkKey, folder, false)
}

func (f Fetcher) megaResolveContextAllowIcon(ctx context.Context, id string, linkKey []byte, folder string, allowIcon bool) (file *megaFile, resultErr error) {
	started := time.Now()
	stage := "policy"
	logx.Download("mega file resolve start folder=%t key_bytes=%d", folder != "", len(linkKey))
	defer func() {
		logx.Download("mega file resolve finished folder=%t stage=%s elapsed=%s error=%v", folder != "", stage, time.Since(started).Round(time.Millisecond), resultErr)
	}()
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
	stage = "api-request"
	logx.Download("mega file resolve request action=g host=%s", api.Hostname())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.megaFetcher().Client().Do(req)
	if err != nil {
		stage = "api-do"
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		stage = "api-body"
		return nil, err
	}
	logx.Download("mega file resolve response status=%d bytes=%d content_length=%d", resp.StatusCode, len(b), resp.ContentLength)
	// An error is a bare number ([-9], or -3 unwrapped), which fails to
	// decode into the struct: every refusal reads the same to the user.
	var out []struct {
		G  string `json:"g"`
		At string `json:"at"`
	}
	decodeErr := json.Unmarshal(b, &out)
	if resp.StatusCode != http.StatusOK || decodeErr != nil || len(out) != 1 || out[0].G == "" {
		stage = "api-rejected"
		logx.Download("mega file resolve response rejected status=%d bytes=%d decode_error=%v result_count=%d download_address_present=%t", resp.StatusCode, len(b), decodeErr, len(out), len(out) == 1 && out[0].G != "")
		return nil, ErrMegaRefused
	}
	stage = "decrypt-name"
	dl, err := url.Parse(out[0].G)
	if err != nil {
		logx.Download("mega file resolve download address rejected parse_error=%v", err)
		return nil, ErrMegaRefused
	}
	if dl.Scheme == "http" && !f.Insecure {
		dl.Scheme = "https" // MEGA's storage hosts serve both
	}
	if dl, err = f.megaCheck(dl.String()); err != nil {
		logx.Download("mega file resolve storage host rejected error=%v", err)
		return nil, err
	}
	m := &megaFile{url: dl, key: make([]byte, 16), iv: linkKey[16:24]}
	for i := range m.key {
		m.key[i] = linkKey[i] ^ linkKey[i+16]
	}
	m.name = safeDirName(filepath.Base(megaName(out[0].At, m.key)))
	if !Installable(m.name) && !(allowIcon && dictionaryIconName(m.name)) {
		stage = "name-not-installable"
		logx.Download("mega file resolved name=%q installable=false", m.name)
		return nil, ErrNotArchive
	}
	stage = "complete"
	logx.Download("mega file resolved name=%q storage_host=%s", m.name, dl.Hostname())
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
		logx.Download("mega folder link rejected host=%s id_valid=%t key_bytes=%d key_decode_error=%v", u.Hostname(), megaIDRE.MatchString(id), len(key), err)
		return listing{}, true, ErrMegaRefused
	}
	logx.Download("mega folder link accepted host=%s key_bytes=%d recursive=false", u.Hostname(), len(key))
	ctx, cancel := context.WithTimeout(ctx, megaRequestTimeout)
	defer cancel()
	api, err := f.megaCheck(megaAPI + "?id=" + strconv.FormatUint(uint64(rand.Uint32()), 10))
	if err != nil {
		logx.Download("mega folder API policy failed error=%v", err)
		return listing{}, true, err
	}
	api.RawQuery += "&n=" + url.QueryEscape(id)
	body, _ := json.Marshal([]map[string]any{{"a": "f", "c": 1, "r": 0, "ca": 1}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.String(), bytes.NewReader(body))
	if err != nil {
		logx.Download("mega folder API request build failed error=%v", err)
		return listing{}, true, err
	}
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	logx.Download("mega api folder request start host=%s", api.Hostname())
	resp, err := f.megaFetcher().Client().Do(req)
	if err != nil {
		logx.Download("mega api folder request failed host=%s elapsed=%s error=%v", api.Hostname(), time.Since(started).Round(time.Millisecond), err)
		return listing{}, true, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		logx.Download("mega api folder body failed status=%d bytes=%d elapsed=%s error=%v", resp.StatusCode, len(b), time.Since(started).Round(time.Millisecond), err)
		return listing{}, true, err
	}
	logx.Download("mega api folder response status=%d bytes=%d content_length=%d elapsed=%s", resp.StatusCode, len(b), resp.ContentLength, time.Since(started).Round(time.Millisecond))
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
	decodeErr := json.Unmarshal(b, &response)
	if resp.StatusCode != 200 || decodeErr != nil {
		logx.Download("mega api folder response rejected status=%d bytes=%d decode_error=%v elapsed=%s", resp.StatusCode, len(b), decodeErr, time.Since(started).Round(time.Millisecond))
		return listing{}, true, ErrMegaRefused
	}
	if len(response) != 1 {
		logx.Download("mega api folder response rejected result_count=%d", len(response))
		return listing{}, true, ErrMegaRefused
	}
	nodes := response[0].F
	logx.Download("mega api folder nodes received=%d", len(nodes))
	root := ""
	for _, n := range nodes {
		if n.T == 1 && strings.Contains(n.K, n.H+":") {
			root = n.H
			break
		}
	}
	if root == "" {
		logx.Download("mega api folder root node missing nodes=%d", len(nodes))
		return listing{}, true, ErrMegaRefused
	}
	logx.Download("mega api folder root resolved")
	l := listing{title: "MEGA"}
	count, skippedKey, skippedName, skippedType, skippedParent := 0, 0, 0, 0, 0
	for _, n := range nodes {
		if n.T != 0 {
			skippedType++
			continue
		}
		if n.P != root {
			skippedParent++
			continue
		}
		if n.S < 0 {
			continue
		}
		count++
		if count > maxLinks {
			logx.Download("mega folder direct-file limit exceeded limit=%d", maxLinks)
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
			skippedKey++
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
			skippedName++
			continue
		}
		name := safeDirName(plainName)
		fileURL := "https://mega.nz/file/" + n.H + "?folder=" + url.QueryEscape(id) + "#" + base64.RawURLEncoding.EncodeToString(lk)
		l.files = append(l.files, &remoteFile{name: name, url: fileURL, size: n.S})
		logx.Download("mega folder file decrypted name=%q size=%d extension=%q", name, n.S, strings.ToLower(filepath.Ext(name)))
	}
	logx.Download("mega folder node processing finished direct_nodes=%d accepted=%d skipped_type=%d skipped_parent=%d skipped_key=%d skipped_name=%d", count, len(l.files), skippedType, skippedParent, skippedKey, skippedName)
	if len(l.files) == 0 {
		logx.Download("mega api folder listing complete direct_files=0 elapsed=%s", time.Since(started).Round(time.Millisecond))
		return listing{}, true, ErrNoLinks
	}
	logx.Download("mega api folder listing complete direct_files=%d elapsed=%s", len(l.files), time.Since(started).Round(time.Millisecond))
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
