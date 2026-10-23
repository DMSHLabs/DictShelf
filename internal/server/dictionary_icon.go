// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
	lsdformat "github.com/wuweidict/wudict/internal/format/lsd"
	"github.com/wuweidict/wudict/internal/store"
)

const maxDictionaryIconBytes = 2 << 20

const defaultDictionaryIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect x="4" y="5" width="22" height="23" rx="3" fill="#eee7d8" stroke="#806f52" stroke-width="1.5"/><path d="M9 5v23m17-18h3v18H9" fill="none" stroke="#806f52" stroke-width="1.5"/><path d="M13 12h8m-8 4h8m-8 4h6" stroke="#806f52" stroke-linecap="round" stroke-width="1.5"/></svg>`

var dictionaryIconExtensions = map[string]string{
	".bmp": "image/bmp", ".gif": "image/gif", ".ico": "image/vnd.microsoft.icon",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jfif": "image/jpeg", ".jpe": "image/jpeg", ".pjpeg": "image/jpeg",
	".png": "image/png", ".apng": "image/apng",
	".webp": "image/webp", ".avif": "image/avif", ".tif": "image/tiff", ".tiff": "image/tiff",
	".svg": "image/svg+xml",
}

func (s *Server) handleDictionaryIcon(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/dict-icon/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	e, err := s.reg.get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body, mime := s.dictionaryIcon(e)
	if len(body) == 0 {
		body, mime = []byte(defaultDictionaryIcon), "image/svg+xml"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

func (s *Server) dictionaryIcon(e *entry) ([]byte, string) {
	source := e.Path
	if prepared, ok := preparedInfo(source); ok && prepared.Meta != nil && prepared.Meta["source_path"] != "" {
		source = prepared.Meta["source_path"]
	}
	if path := dictionaryIconFile(source); path != "" {
		if body, mime, err := readDictionaryIcon(path); err == nil {
			return body, mime
		}
	}
	if strings.EqualFold(filepath.Ext(source), ".lsd") {
		if body, mime, err := lsdformat.ReadIcon(source); err == nil {
			return body, normalizedIconMIME(mime, "")
		}
	}

	// ZIM has a standard metadata namespace for its illustrations. Open the
	// lightweight native reader directly, even when search uses a prepared
	// title index; this does not build a headword map or index.
	if strings.EqualFold(filepath.Ext(source), ".zim") || strings.EqualFold(filepath.Ext(source), ".zimaa") {
		if d, err := dict.Open(source); err == nil {
			body, mime := iconFromDictionary(d)
			_ = d.Close()
			if len(body) != 0 {
				return body, mime
			}
		}
	}

	// A backend already opened for normal app work can also expose a
	// conventional icon resource. Do not open every large dictionary just to
	// inspect its resource table while drawing the list.
	e.dMu.RLock()
	d := e.d
	e.dMu.RUnlock()
	if d != nil {
		if body, mime := iconFromDictionary(d); len(body) != 0 {
			return body, mime
		}
	}
	return nil, ""
}

func iconFromDictionary(d dict.Dictionary) ([]byte, string) {
	if provider, ok := d.(dict.IconProvider); ok {
		if rc, mime, err := provider.Icon(); err == nil {
			body, readErr := readIcon(rc)
			if readErr == nil && len(body) != 0 {
				return body, normalizedIconMIME(mime, "")
			}
		}
	}
	lister, ok := d.(dict.ResourceLister)
	if !ok {
		return nil, ""
	}
	for _, name := range conventionalIconResources(lister.Resources()) {
		rc, mime, err := d.Resource(name)
		if err != nil {
			continue
		}
		body, err := readIcon(rc)
		if err == nil && len(body) != 0 {
			return body, normalizedIconMIME(mime, filepath.Ext(name))
		}
	}
	return nil, ""
}

func conventionalIconResources(resources []string) []string {
	wanted := map[string]bool{"icon": true, "favicon": true, "logo": true, "cover": true, "dictionary": true}
	var found []string
	for _, name := range resources {
		base := strings.TrimSuffix(filepath.Base(strings.ReplaceAll(name, "\\", "/")), filepath.Ext(name))
		if !wanted[strings.ToLower(base)] || dictionaryIconExtensions[strings.ToLower(filepath.Ext(name))] == "" {
			continue
		}
		found = append(found, name)
	}
	return found
}

func dictionaryIconFile(source string) string {
	if source == "" || store.IsTextDB(source) {
		return ""
	}
	stem := dict.Stem(source)
	dir := filepath.Dir(stem)
	base := filepath.Base(stem)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, suffix := range []string{"", "_name"} {
		for _, entry := range entries {
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if entry.IsDir() || dictionaryIconExtensions[ext] == "" ||
				!strings.EqualFold(strings.TrimSuffix(name, filepath.Ext(name)), base+suffix) {
				continue
			}
			return filepath.Join(dir, name)
		}
	}
	return ""
}

func readDictionaryIcon(path string) ([]byte, string, error) {
	mime := normalizedIconMIME("", filepath.Ext(path))
	if mime == "" {
		return nil, "", dict.ErrNotFound
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if info.Size() <= 0 || info.Size() > maxDictionaryIconBytes {
		return nil, "", fmt.Errorf("dictionary icon has invalid size %d", info.Size())
	}
	body, err := io.ReadAll(io.LimitReader(f, maxDictionaryIconBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxDictionaryIconBytes {
		return nil, "", fmt.Errorf("dictionary icon exceeds %d bytes", maxDictionaryIconBytes)
	}
	return body, mime, nil
}

func readIcon(rc io.ReadCloser) ([]byte, error) {
	if rc == nil {
		return nil, dict.ErrNotFound
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, maxDictionaryIconBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxDictionaryIconBytes {
		return nil, fmt.Errorf("dictionary icon has invalid size %d", len(body))
	}
	return body, nil
}

func normalizedIconMIME(mime, ext string) string {
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	if strings.HasPrefix(mime, "image/") {
		return mime
	}
	return dictionaryIconExtensions[strings.ToLower(ext)]
}
