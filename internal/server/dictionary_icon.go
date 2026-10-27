// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	lsdformat "github.com/wuweidict/wudict/internal/format/lsd"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/store"
	_ "image/gif"
	_ "image/jpeg"
)

const maxDictionaryIconBytes = 2 << 20
const maxDictionaryIconCacheBytes = 3 << 20
const dictionaryIconMaxDimension = 128
const maxDictionaryIconSourcePixels = 16 << 20
const dictionaryIconCacheName = "icon.cache"
const dictionaryIconCacheVersion = 2

const defaultDictionaryIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect x="4" y="5" width="22" height="23" rx="3" fill="#eee7d8" stroke="#806f52" stroke-width="1.5"/><path d="M9 5v23m17-18h3v18H9" fill="none" stroke="#806f52" stroke-width="1.5"/><path d="M13 12h8m-8 4h8m-8 4h6" stroke="#806f52" stroke-linecap="round" stroke-width="1.5"/></svg>`

var dictionaryIconExtensions = map[string]string{
	".bmp": "image/bmp", ".gif": "image/gif", ".ico": "image/vnd.microsoft.icon",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jfif": "image/jpeg", ".jpe": "image/jpeg", ".pjpeg": "image/jpeg",
	".png": "image/png", ".apng": "image/apng",
	".webp": "image/webp", ".avif": "image/avif", ".tif": "image/tiff", ".tiff": "image/tiff",
	".svg": "image/svg+xml",
}

type dictionaryIconCache struct {
	Version       int    `json:"version"`
	IndexUUID     string `json:"index_uuid"`
	Status        string `json:"status"`
	MIME          string `json:"mime,omitempty"`
	ExternalPath  string `json:"external_path,omitempty"`
	ExternalStamp string `json:"external_stamp,omitempty"`
	Body          []byte `json:"body,omitempty"`
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
	body, mime, ready := s.reg.cachedDictionaryIcon(e)
	if !ready {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("pending"))
		return
	}
	if len(body) == 0 {
		body, mime = []byte(defaultDictionaryIcon), "image/svg+xml"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// cachedDictionaryIcon keeps dictionary probing off WebView2's resource
// callback, which runs on the UI thread. ZIM icon discovery can open a large
// source file and take minutes on slow storage; the first request starts that
// work in the background and returns 202 so the UI can retry without waiting.
func (r *Registry) cachedDictionaryIcon(e *entry) ([]byte, string, bool) {
	gen := e.gen.Load()
	e.iconMu.Lock()
	if e.iconGen != gen {
		e.iconGen = gen
		e.iconLoading = false
		e.iconReady = false
		e.iconBody = nil
		e.iconMIME = ""
	}
	if e.iconReady {
		body, mime := e.iconBody, e.iconMIME
		e.iconMu.Unlock()
		return body, mime, true
	}
	start := !e.iconLoading
	if start {
		e.iconLoading = true
	}
	e.iconMu.Unlock()
	if start {
		go r.runDictionaryIconLoad(e, gen)
	}
	return nil, "", false
}

func (r *Registry) queueDictionaryIconRefresh(e *entry) {
	if e == nil {
		return
	}
	gen := e.gen.Load()
	e.iconMu.Lock()
	if e.iconGen != gen {
		e.iconGen = gen
		e.iconReady = false
		e.iconBody = nil
		e.iconMIME = ""
	}
	if e.iconLoading {
		e.iconMu.Unlock()
		return
	}
	e.iconLoading = true
	e.iconMu.Unlock()
	go r.runDictionaryIconLoad(e, gen)
}

func (r *Registry) runDictionaryIconLoad(e *entry, gen uint64) {
	release := acquire(currentIndexLimit())
	defer release()
	e.ingestMu.Lock()
	defer e.ingestMu.Unlock()

	var body []byte
	var mime string
	defer func() {
		if recovered := recover(); recovered != nil {
			logx.Warn("dictionary icon lookup failed for %s: %v", e.ID, recovered)
		}
		e.iconMu.Lock()
		if e.iconGen == gen && e.gen.Load() == gen {
			e.iconBody, e.iconMIME = body, mime
			e.iconReady = true
			e.iconLoading = false
		}
		e.iconMu.Unlock()
	}()
	body, mime = r.dictionaryIcon(e, gen)
}

func (r *Registry) refreshDictionaryIcons() {
	entries := r.all()
	if len(entries) == 0 {
		return
	}
	workers := cap(currentIndexLimit())
	if workers < 1 {
		workers = 1
	}
	if workers > len(entries) {
		workers = len(entries)
	}
	queue := make(chan *entry, len(entries))
	for _, e := range entries {
		queue <- e
	}
	close(queue)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for e := range queue {
				gen := e.gen.Load()
				e.iconMu.Lock()
				if e.iconGen != gen {
					e.iconGen = gen
					e.iconReady = false
					e.iconBody = nil
					e.iconMIME = ""
					e.iconLoading = false
				}
				if e.iconLoading {
					e.iconMu.Unlock()
					continue
				}
				e.iconLoading = true
				e.iconMu.Unlock()
				r.runDictionaryIconLoad(e, gen)
			}
		}()
	}
	wg.Wait()
}

func (r *Registry) dictionaryIcon(e *entry, gen uint64) ([]byte, string) {
	prepared, dir, source, ok := preparedIconInfo(e)
	if !ok {
		return nil, ""
	}
	cachePath := filepath.Join(dir, dictionaryIconCacheName)
	cached, cacheErr := readDictionaryIconCache(cachePath)
	var priorExternal string
	if cacheErr == nil {
		priorExternal = cached.ExternalPath
	}
	external := currentDictionaryIconFile(source, priorExternal)
	externalStamp := dictionaryIconFileStamp(external)
	if cacheErr == nil && dictionaryIconCacheMatches(cached, prepared, external, externalStamp) {
		if cached.Status == "found" {
			return cached.Body, cached.MIME
		}
		return nil, ""
	}

	var body []byte
	var mime string
	body, mime = sourceDictionaryIcon(source, external)
	probeComplete := true
	if len(body) == 0 && strings.EqualFold(filepath.Ext(source), ".lsd") {
		body, mime, _ = lsdformat.ReadIcon(source)
	}
	isZIM := strings.EqualFold(filepath.Ext(source), ".zim") || strings.EqualFold(filepath.Ext(source), ".zimaa")
	if len(body) == 0 && isZIM && source != "" {
		if d, err := dict.Open(source); err == nil {
			body, mime = iconFromDictionary(d)
			_ = d.Close()
		} else {
			probeComplete = false
		}
	}
	if !probeComplete {
		if cacheErr == nil && cached.Status == "found" {
			return cached.Body, cached.MIME
		}
		return nil, ""
	}

	cache := dictionaryIconCache{
		Version: dictionaryIconCacheVersion, IndexUUID: prepared.Meta["dict_uuid"],
		Status: "absent", ExternalPath: external, ExternalStamp: externalStamp,
	}
	if len(body) > 0 {
		if body, mime, err := shrinkDictionaryIcon(body, mime); err == nil && len(body) > 0 {
			cache.Status, cache.MIME, cache.Body = "found", mime, body
		} else if err != nil {
			logx.Warn("could not prepare dictionary icon for %s: %v", e.ID, err)
		}
	}
	if e.gen.Load() != gen {
		return nil, ""
	}
	latest, _, _, stillPrepared := preparedIconInfo(e)
	if !stillPrepared || latest.Meta["dict_uuid"] != cache.IndexUUID {
		return nil, ""
	}
	data, err := json.Marshal(cache)
	if err != nil {
		logx.Warn("could not encode dictionary icon cache for %s: %v", e.ID, err)
		return nil, ""
	}
	if err := fsx.WriteAtomic(cachePath, data, 0o600); err != nil {
		logx.Warn("could not save dictionary icon cache for %s: %v", e.ID, err)
		return nil, ""
	}
	if err := store.WriteInfo(dir); err != nil {
		logx.Warn("could not refresh prepared dictionary receipt for %s: %v", e.ID, err)
	}
	if cache.Status == "found" {
		return cache.Body, cache.MIME
	}
	return nil, ""
}

func sourceDictionaryIcon(source, external string) ([]byte, string) {
	if external != "" {
		if body, mime, err := readDictionaryIcon(external); err == nil {
			return body, mime
		}
	}
	if strings.EqualFold(filepath.Ext(source), ".lsd") && source != "" {
		if body, mime, err := lsdformat.ReadIcon(source); err == nil {
			return body, normalizedIconMIME(mime, "")
		}
	}
	return nil, ""
}

// dictionaryIcon remains a thin prepared-index-only helper for package
// callers; HTTP serving always uses the asynchronous cache path above.
func (s *Server) dictionaryIcon(e *entry) ([]byte, string) {
	if s.reg == nil {
		return nil, ""
	}
	return s.reg.dictionaryIcon(e, e.gen.Load())
}

func preparedIconInfo(e *entry) (store.Prepared, string, string, bool) {
	var prepared store.Prepared
	if store.IsTextDB(e.Path) {
		prepared = store.Inspect(e.Path)
		if !prepared.Usable() {
			return store.Prepared{}, "", "", false
		}
	} else {
		var ok bool
		prepared, ok = store.FindPrepared(e.Path)
		if !ok {
			return store.Prepared{}, "", "", false
		}
	}
	source := e.Path
	if store.IsTextDB(e.Path) || !fsx.FileExists(source) {
		source = prepared.Meta["source_path"]
	}
	return prepared, filepath.Dir(prepared.TextDB), source, true
}

func readDictionaryIconCache(path string) (dictionaryIconCache, error) {
	data, err := fsx.ReadBounded(path, maxDictionaryIconCacheBytes)
	if err != nil {
		return dictionaryIconCache{}, err
	}
	var cache dictionaryIconCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return dictionaryIconCache{}, err
	}
	if cache.Version != dictionaryIconCacheVersion ||
		(cache.Status != "found" && cache.Status != "absent") ||
		len(cache.Body) > maxDictionaryIconBytes ||
		(cache.Status == "found" && (len(cache.Body) == 0 || normalizedIconMIME(cache.MIME, "") == "")) ||
		(cache.Status == "absent" && len(cache.Body) != 0) {
		return dictionaryIconCache{}, fmt.Errorf("invalid dictionary icon cache")
	}
	return cache, nil
}

func dictionaryIconCacheMatches(cache dictionaryIconCache, prepared store.Prepared, external, stamp string) bool {
	return cache.Version == dictionaryIconCacheVersion &&
		cache.IndexUUID != "" && cache.IndexUUID == prepared.Meta["dict_uuid"] &&
		cache.ExternalPath == external && cache.ExternalStamp == stamp
}

func currentDictionaryIconFile(source, previous string) string {
	if source != "" && fsx.FileExists(source) {
		return dictionaryIconFile(source)
	}
	if previous != "" {
		if fi, err := os.Stat(previous); err == nil && fi.Mode().IsRegular() {
			return previous
		}
	}
	return ""
}

func dictionaryIconFileStamp(path string) string {
	if path == "" {
		return ""
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return strconv.FormatInt(fi.Size(), 10) + ":" + strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}

func shrinkDictionaryIcon(body []byte, mime string) ([]byte, string, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		// Vector and browser-native formats already scale to the view's CSS
		// dimensions; their source size is bounded before it reaches here.
		return body, mime, nil
	}
	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > maxDictionaryIconSourcePixels {
		return nil, "", fmt.Errorf("dictionary icon dimensions are too large: %dx%d", config.Width, config.Height)
	}
	if config.Width <= dictionaryIconMaxDimension && config.Height <= dictionaryIconMaxDimension {
		return body, mime, nil
	}
	decoded, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 || (width <= dictionaryIconMaxDimension && height <= dictionaryIconMaxDimension) {
		return body, mime, nil
	}
	scale := float64(dictionaryIconMaxDimension) / float64(max(width, height))
	targetW := max(1, int(float64(width)*scale))
	targetH := max(1, int(float64(height)*scale))
	resized := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := bounds.Min.Y + min(height-1, y*height/targetH)
		for x := 0; x < targetW; x++ {
			sx := bounds.Min.X + min(width-1, x*width/targetW)
			resized.Set(x, y, color.NRGBAModel.Convert(decoded.At(sx, sy)))
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, resized); err != nil {
		return nil, "", err
	}
	if out.Len() > maxDictionaryIconBytes {
		return nil, "", fmt.Errorf("resized dictionary icon exceeds %d bytes", maxDictionaryIconBytes)
	}
	return out.Bytes(), "image/png", nil
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
