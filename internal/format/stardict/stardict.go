// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package stardict is the direct backend + ingest reader for StarDict
// dictionaries: NAME.ifo (metadata), NAME.idx[.gz|.dz] (sorted headword
// index), NAME.dict[.dz] (article data), optional NAME.syn (synonyms)
// and res/ dir or res.zip (resources). Layout ported from
// pyglossary/plugins/stardict/reader.py.
package stardict

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/artmark"
	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/htmlref"
	"github.com/wuweidict/wudict/internal/limits"
	"github.com/wuweidict/wudict/internal/resource"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 2

func init() {
	dict.RegisterFormat(".ifo", func(path string) (dict.Dictionary, error) { return Open(path) })
	dict.RegisterReader(".ifo", func(path string) (dict.Reader, error) { return NewReader(path) })
	dict.RegisterReaderVersion("stardict", ReaderVersion)
	dict.RegisterProber(".ifo", probe)
	// O8: `res/` and res.zip are found from the path; nothing lives inside the
	// .dict that a resource needs, so no Fetcher is registered.
	resource.Register("stardict", resource.Provider{Sources: MediaSources})
}

type idxEntry struct {
	word   string
	offset uint64
	size   uint32
}

// articleSource serves (offset,size) ranges of the .dict data.
type articleSource interface {
	readRange(offset int64, size int) ([]byte, error)
}

// plainDict serves ranges of an uncompressed .dict. size is the file's real
// length: the (offset,size) pair comes from the .idx, where size is a u32, so
// a corrupt index can ask for 4 GiB per lookup. Checking the range first turns
// that into an error on one article instead of an allocation the process may
// not survive - and this is the LOOKUP path, reached per query.
type plainDict struct {
	f    *os.File
	size int64
}

func (p plainDict) readRange(offset int64, size int) ([]byte, error) {
	if offset < 0 || size < 0 || offset > p.size || int64(size) > p.size-offset {
		return nil, fmt.Errorf("record range out of bounds (offset %d, %d bytes, file %d)", offset, size, p.size)
	}
	out := make([]byte, size)
	_, err := p.f.ReadAt(out, offset)
	return out, err
}

// Dict is one opened StarDict dictionary. Safe for concurrent readers.
type Dict struct {
	meta      dict.Meta
	ifo       map[string]string
	sameType  string
	entries   []idxEntry
	synonyms  map[int][]string // idx entry -> synonym words (from .syn)
	exactOnce sync.Once        // exactIdx is built on first exact lookup only
	exactIdx  map[string][]int // headword or synonym -> entry indexes
	foldOnce  sync.Once        // foldIdx is built on first accent-folded lookup only
	foldIdx   map[string][]int
	data      articleSource
	dictFile  *os.File

	basePath string // path without .ifo
	resOnce  sync.Once
	res      []resource.Source
	resMu    sync.Mutex // guards res against a concurrent Close
}

// Open opens NAME.ifo and its companion files.
func Open(ifoPath string) (*Dict, error) {
	ifo, fields, err := parseIfo(ifoPath)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSuffix(ifoPath, filepath.Ext(ifoPath))
	d := &Dict{
		ifo:      ifo,
		sameType: ifo["sametypesequence"],
		basePath: base,
		synonyms: map[int][]string{},
	}

	offBits := 32
	if ifo["idxoffsetbits"] == "64" {
		offBits = 64
	}
	idxData, err := readCompanion(base, ".idx")
	if err != nil {
		return nil, fmt.Errorf("stardict %s: %w", ifoPath, err)
	}
	wc, _ := strconv.Atoi(strings.TrimSpace(ifo["wordcount"]))
	if d.entries, err = parseIdx(idxData, offBits, wc); err != nil {
		return nil, fmt.Errorf("stardict %s: %w", ifoPath, err)
	}

	if synData, err := readCompanion(base, ".syn"); err == nil {
		parseSyn(synData, len(d.entries), d.synonyms)
	}

	// No headword index is built at open - both the raw (ensureExact) and the
	// accent-folded (ensureFold) indexes are built lazily on first use, so an
	// open only for resources or the ingest scan builds nothing. Runtime
	// readers (GoldenDict, aard2) keep no in-memory headword index at all; our
	// ingested SQLite path is that on-disk index.
	if err := d.openDictData(base); err != nil {
		return nil, fmt.Errorf("stardict %s: %w", ifoPath, err)
	}

	name := strings.TrimSpace(ifo["bookname"])
	if name == "" {
		name = filepath.Base(base)
	}
	d.meta = dict.Meta{
		Name:        name,
		Format:      "stardict",
		Path:        ifoPath,
		Description: strings.TrimSpace(ifo["description"]),
		EntryCount:  len(d.entries),
		Header:      fields,
	}
	return d, nil
}

// probe reads bookname/wordcount from the tiny .ifo header only (no .idx
// load, no fold-maps) for the cheap dictionary-list path. Name mirrors
// Open's derivation so both report the same display name.
func probe(ifoPath string) (dict.Meta, error) {
	ifo, _, err := parseIfo(ifoPath)
	if err != nil {
		return dict.Meta{}, err
	}
	name := strings.TrimSpace(ifo["bookname"])
	if name == "" {
		name = filepath.Base(strings.TrimSuffix(ifoPath, filepath.Ext(ifoPath)))
	}
	n, _ := strconv.Atoi(strings.TrimSpace(ifo["wordcount"]))
	return dict.Meta{
		Name:        name,
		Format:      "stardict",
		Path:        ifoPath,
		Description: strings.TrimSpace(ifo["description"]),
		EntryCount:  n,
	}, nil
}

func (d *Dict) openDictData(base string) error {
	if f, err := os.Open(base + ".dict"); err == nil {
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		d.dictFile = f
		d.data = plainDict{f: f, size: st.Size()}
		return nil
	}
	f, err := os.Open(base + ".dict.dz")
	if err != nil {
		return fmt.Errorf("neither .dict nor .dict.dz found")
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	dz, err := newDzReader(f, st.Size())
	if err != nil {
		f.Close()
		return fmt.Errorf(".dict.dz: %w", err)
	}
	d.dictFile = f
	d.data = dz
	return nil
}

// parseIfo reads the .ifo into a lookup map and, for Meta.Header, the same
// lines in file order minus bookname and description, which Meta carries under
// their own names.
func parseIfo(path string) (map[string]string, []dict.Field, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "StarDict's dict ifo file") {
		return nil, nil, fmt.Errorf("%s: not a StarDict .ifo file", path)
	}
	m := map[string]string{}
	var fields []dict.Field
	for _, ln := range lines[1:] {
		if k, v, ok := strings.Cut(ln, "="); ok {
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			m[k] = v
			if k != "" && v != "" && k != "bookname" && k != "description" {
				fields = append(fields, dict.Field{Name: k, Value: v})
			}
		}
	}
	return m, fields, nil
}

// compressedSuffixes are the spellings a StarDict companion may be compressed
// under, in probe order. ".dz" is dictzip - a gzip member carrying a random-
// access chunk table in FEXTRA - so compress/gzip reads it like any other gzip
// stream, and only the .dict needs the chunk table at all. Tools that dictzip a
// whole dictionary leave ".idx.dz" and ".syn.dz" behind; reading only ".gz"
// declared those dictionaries index-less while the index sat right there.
var compressedSuffixes = []string{".gz", ".dz"}

// readCompanion reads base+ext, or its gzip/dictzip spelling, whole.
func readCompanion(base, ext string) ([]byte, error) {
	if data, err := os.ReadFile(base + ext); err == nil {
		return data, nil
	}
	for _, z := range compressedSuffixes {
		f, err := os.Open(base + ext + z)
		if err != nil {
			continue
		}
		data, err := readGzAll(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(base+ext+z), err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found (nor %s%s or %s%s)",
		filepath.Base(base+ext), filepath.Base(base+ext), compressedSuffixes[0],
		filepath.Base(base+ext), compressedSuffixes[1])
}

// maxIndexBytes and maxIndexRatio bound one decompressed companion. An .idx
// is loaded whole by design and runs to tens of MB, and to hundreds for a
// Wikipedia-scale dictionary (~24 bytes per headword) - but a gzip bomb, a few
// KB that inflate to gigabytes, must cost one refused dictionary, not the
// process. So the ceiling is the larger of a fixed floor and a multiple of
// the compressed size: an index deflates 3-5x, a bomb ~1000x.
//
// The floor is MAX_STARDICT_INDEX (limits.Get().StarDictIndex).
const maxIndexRatio = 16

// readGzAll decompresses f whole, keeping what it got when the stream is
// damaged at its TAIL. gzip verifies a CRC and a length that sit after the last
// byte of data, so a file whose final bytes are malformed - the same class of
// writer fault that skews a dictzip chunk table - yields every good record and
// then an error. Discarding all of it over the last record loses the
// dictionary; a header-level failure still does, because nothing decompressed.
func readGzAll(f *os.File) ([]byte, error) {
	limit := limits.Get().StarDictIndex
	if st, err := f.Stat(); err == nil {
		limit = max(limit, maxIndexRatio*st.Size())
	}
	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return readGzBounded(gr, int(min(limit, math.MaxInt-1)))
}

// readGzBounded is readGzAll's body with the ceiling made a parameter, so a
// test can exercise the bomb path without materializing 256 MiB.
func readGzBounded(gr io.Reader, max int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(gr, int64(max)+1))
	if err != nil && len(data) > 0 &&
		(errors.Is(err, gzip.ErrChecksum) || errors.Is(err, io.ErrUnexpectedEOF)) {
		return data, nil
	}
	if len(data) > max {
		return nil, limits.Exceeded("the unpacked index", "MAX_STARDICT_INDEX", int64(max))
	}
	return data, err
}

// parseIdx decodes the sorted index: word\0 + offset (32/64-bit BE) +
// size (u32 BE) per entry. hint is the .ifo wordcount, used only to size the
// result once - bounded by the most records data can hold, so a lying header
// cannot turn it into an allocation.
func parseIdx(data []byte, offBits, hint int) ([]idxEntry, error) {
	offSize := 4
	if offBits == 64 {
		offSize = 8
	}
	out := make([]idxEntry, 0, max(0, min(hint, len(data)/(offSize+5))))
	pos := 0
	for pos < len(data) {
		nul := bytes.IndexByte(data[pos:], 0)
		if nul < 0 {
			break
		}
		word := string(data[pos : pos+nul])
		pos += nul + 1
		if pos+offSize+4 > len(data) {
			// A record cut in half at the end of the file: keep the ones
			// already parsed. Returning nothing turned a single truncated
			// trailing entry into a dictionary with no index at all.
			if len(out) > 0 {
				return out, nil
			}
			return nil, fmt.Errorf("truncated idx at %q", word)
		}
		var off uint64
		if offBits == 64 {
			off = binary.BigEndian.Uint64(data[pos:])
		} else {
			off = uint64(binary.BigEndian.Uint32(data[pos:]))
		}
		size := binary.BigEndian.Uint32(data[pos+offSize:])
		pos += offSize + 4
		out = append(out, idxEntry{word: word, offset: off, size: size})
	}
	return out, nil
}

// parseSyn decodes NAME.syn: word\0 + u32 BE index into the idx entries.
func parseSyn(data []byte, entryCount int, into map[int][]string) {
	pos := 0
	for pos < len(data) {
		nul := bytes.IndexByte(data[pos:], 0)
		if nul < 0 || pos+nul+5 > len(data) {
			break
		}
		word := string(data[pos : pos+nul])
		idx := int(binary.BigEndian.Uint32(data[pos+nul+1:]))
		pos += nul + 5
		if idx >= 0 && idx < entryCount {
			into[idx] = append(into[idx], word)
		}
	}
}

func (d *Dict) Meta() dict.Meta { return d.meta }
func (d *Dict) Caps() dict.Caps { return dict.Caps{Exact: true, Prefix: true} }

func (d *Dict) Close() error {
	// Do, not a flag: a Resource call racing Close must find the sources
	// already built and closed, never build a fresh set nothing will close.
	d.resOnce.Do(func() {})
	d.resMu.Lock()
	for _, src := range d.res {
		src.Close()
	}
	d.res = nil
	d.resMu.Unlock()
	if d.dictFile != nil {
		return d.dictFile.Close()
	}
	return nil
}

func (d *Dict) Exact(word string, limit int) ([]dict.Result, error) {
	word = strings.TrimSpace(word)
	d.ensureExact()
	idxs := d.exactIdx[word]
	if len(idxs) == 0 {
		d.ensureFold()
		idxs = d.foldIdx[fold(word)]
	}
	return d.results(idxs, limit)
}

// ensureExact builds the raw headword+synonym index on first lookup, so opens
// that only serve resources or feed the ingest scan never build it.
func (d *Dict) ensureExact() {
	d.exactOnce.Do(func() {
		idx := make(map[string][]int, len(d.entries))
		for i, e := range d.entries {
			idx[e.word] = append(idx[e.word], i)
		}
		for i, words := range d.synonyms {
			for _, w := range words {
				idx[w] = append(idx[w], i)
			}
		}
		d.exactIdx = idx
	})
}

// ensureFold builds the accent/case-folded headword index on first use. The
// per-headword NFD fold is the expensive part of opening, so deferring it keeps
// open fast and costs nothing for dictionaries that are only ever queried by
// exact (or ingested) headword.
func (d *Dict) ensureFold() {
	d.foldOnce.Do(func() {
		idx := make(map[string][]int, len(d.entries))
		for i, e := range d.entries {
			f := fold(e.word)
			idx[f] = append(idx[f], i)
		}
		for i, words := range d.synonyms {
			for _, w := range words {
				f := fold(w)
				idx[f] = append(idx[f], i)
			}
		}
		d.foldIdx = idx
	})
}

// Prefix returns up to limit headwords starting with word, the exact matches
// first (raw pass first, folded passes only when the raw pass is empty).
//
// An exact hit does not end the search: "starts with" must not answer a
// complete headword with that one article and drop every longer key under it,
// so the exact hit takes precedence, not the whole answer. It is seeded from the
// headword index rather than left to the scan because the scan walks the file
// in storage order and stops at limit: under a prefix with more than `limit`
// entries behind it, the word actually typed could be the one cut.
func (d *Dict) Prefix(word string, limit int) ([]dict.Result, error) {
	word = strings.TrimSpace(word)
	d.ensureExact()
	idxs := append([]int(nil), d.exactIdx[word]...)
	seen := make(map[int]bool, len(idxs))
	for _, i := range idxs {
		seen[i] = true
	}
	scan := func(useFold bool) {
		key := word
		if useFold {
			key = fold(word)
		}
		for i, e := range d.entries {
			if seen[i] {
				continue
			}
			w := e.word
			if useFold {
				w = fold(w)
			}
			if strings.HasPrefix(w, key) {
				idxs = append(idxs, i)
				if limit > 0 && len(idxs) >= limit {
					return
				}
			}
		}
	}
	scan(false)
	if len(idxs) == 0 {
		// The folded EXACT index first, and the folded scan only behind it:
		// the index is built once and answers every later keystroke, while the
		// scan re-folds every headword on each call.
		d.ensureFold()
		idxs = append(idxs, d.foldIdx[fold(word)]...)
		for _, i := range idxs {
			seen[i] = true
		}
		scan(true)
	}
	return d.results(idxs, limit)
}

// results renders the matched entries, dropping the ones whose article bytes
// cannot be read. A record range that falls outside the .dict - a truncated
// dictzip tail, a corrupt .idx row - is damage to ONE entry, and failing the
// whole lookup for it hid every other match for the same word behind an error.
// The error is reported only when it is the entire answer.
func (d *Dict) results(idxs []int, limit int) ([]dict.Result, error) {
	var out []dict.Result
	var firstErr error
	seen := map[int]bool{}
	for _, i := range idxs {
		if seen[i] {
			continue
		}
		seen[i] = true
		body, err := d.article(i)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, dict.Result{Headword: d.entries[i].word, Body: body})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// article reads and renders one record to HTML.
func (d *Dict) article(i int) (string, error) {
	e := d.entries[i]
	raw, err := d.data.readRange(int64(e.offset), int(e.size))
	if err != nil {
		return "", fmt.Errorf("record %q: %w", e.word, err)
	}
	return recordToHTML(raw, d.sameType), nil
}

func (d *Dict) Keywords(offset, n int) []string {
	lo, hi, ok := dict.KeywordRange(len(d.entries), offset, n)
	if !ok {
		return nil
	}
	out := make([]string, 0, hi-lo)
	for _, e := range d.entries[lo:hi] {
		out = append(out, e.word)
	}
	return out
}

// Resource serves files from res/ (dir) or res.zip beside the .ifo. Names are
// matched through internal/resource, so a res.zip whose entry names were
// written in a legacy code page still resolves against the UTF-8 name.
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	for _, src := range d.sources() {
		if rc, err := src.Open(name); err == nil {
			return rc, resource.MIME(name), nil
		}
	}
	return nil, "", dict.ErrNotFound
}

// Resources lists res/ dir files (relative, forward-slash) and res.zip
// entries.
func (d *Dict) Resources() []string {
	var out []string
	seen := map[string]bool{}
	for _, src := range d.sources() {
		for _, n := range src.List() {
			if k := resource.Key(n); k != "" && !seen[k] {
				seen[k] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (d *Dict) sources() []resource.Source {
	d.resOnce.Do(d.loadSources)
	d.resMu.Lock()
	defer d.resMu.Unlock()
	return d.res
}

func (d *Dict) loadSources() {
	res := MediaSources(d.basePath)
	d.resMu.Lock()
	d.res = res
	d.resMu.Unlock()
}

// MediaSources builds a StarDict's resource containers from its PATH alone: the
// `res/` folder and `res.zip` that sit beside the .ifo. Nothing here needs the
// dictionary open, which is why a prepared StarDict can serve its media without
// re-reading the .idx it has already replaced. Registered as the format's O8
// provider; loadSources calls it, so the two cannot drift.
//
// Any path inside the dictionary's folder identifies it - the .ifo, or the
// basePath with its extension already dropped - since only the folder is used.
func MediaSources(anyPath string) []resource.Source {
	dir := filepath.Dir(anyPath)
	var res []resource.Source
	if r := filepath.Join(dir, "res"); resource.IsDir(r) {
		res = append(res, resource.NewDir(r))
	}
	if z, err := resource.OpenZip(filepath.Join(dir, "res.zip")); err == nil {
		res = append(res, z)
	}
	return res
}

func fold(s string) string { return dict.Fold(s) }

// ---- record rendering ----------------------------------------------------

// recordToHTML converts one .dict record to HTML. With same type sequence
// the type of each part is fixed; otherwise each part is prefixed by its
// type char. Lowercase types are text (NUL-terminated unless last),
// uppercase types carry a u32 BE size prefix.
func recordToHTML(raw []byte, sameType string) string {
	var b strings.Builder
	if sameType != "" {
		rest := raw
		for ti := 0; ti < len(sameType) && len(rest) > 0; ti++ {
			t := sameType[ti]
			last := ti == len(sameType)-1
			part, tail := cutPart(rest, t, last)
			b.WriteString(partToHTML(t, part))
			rest = tail
		}
		return b.String()
	}
	rest := raw
	for len(rest) > 0 {
		t := rest[0]
		rest = rest[1:]
		part, tail := cutPart(rest, t, false)
		b.WriteString(partToHTML(t, part))
		rest = tail
	}
	return b.String()
}

// cutPart splits one typed part off the record.
func cutPart(data []byte, t byte, last bool) (part, rest []byte) {
	if t >= 'A' && t <= 'Z' { // uppercase: u32 size prefix
		if len(data) < 4 {
			return nil, nil
		}
		n := int(binary.BigEndian.Uint32(data))
		// Negative only where int is 32-bit and the u32 wraps; on such a build
		// 4+n compares below len(data) and the slice would panic.
		if n < 0 || 4+n > len(data) {
			n = len(data) - 4
		}
		return data[4 : 4+n], data[4+n:]
	}
	if last {
		return data, nil
	}
	if i := bytes.IndexByte(data, 0); i >= 0 {
		return data[:i], data[i+1:]
	}
	return data, nil
}

func partToHTML(t byte, data []byte) string {
	switch t {
	case 'h', 'g': // HTML / Pango markup (HTML-compatible subset)
		return strings.Trim(string(data), "\x00")
	case 'm', 'l', 't', 'y': // plain text / phonetics
		s := htmlref.EscapeText(strings.Trim(string(data), "\x00"))
		return "<p>" + strings.ReplaceAll(s, "\n", "<br/>") + "</p>"
	case 'x': // XDXF markup
		return xdxfToHTML(strings.Trim(string(data), "\x00"))
	case 'r': // resource list
		return resourceList(strings.Trim(string(data), "\x00"))
	case 'W', 'P', 'X': // media / unknown binary: not renderable inline
		return ""
	default:
		s := htmlref.EscapeText(strings.Trim(string(data), "\x00"))
		return "<p>" + s + "</p>"
	}
}

// resourceList renders an 'r' part: one resource per line, "type:name", the
// name relative to the dictionary's resource storage (res/ or res.zip). img,
// snd and vdo name media and are drawn by what a browser can do with the file
// - artmark's rule, so a vdo:film.avi is a link exactly as it is in a DSL
// article; att is an attachment, always a link. A line of any other shape is
// shown as the text it is.
func resourceList(s string) string {
	var b strings.Builder
	for _, ln := range strings.Split(s, "\n") {
		if ln = strings.TrimSpace(ln); ln == "" {
			continue
		}
		typ, name, _ := strings.Cut(ln, ":")
		name = strings.TrimSpace(name)
		el := htmlref.EscapeText(ln)
		switch {
		case name == "":
		case typ == "att":
			el = artmark.Media(name, artmark.MediaFile)
		case typ == "img" || typ == "snd" || typ == "vdo":
			el = artmark.Media(name, artmark.KindOf(name))
		}
		b.WriteString("<p>" + el + "</p>")
	}
	return b.String()
}

// wordCount reports the .ifo wordcount (used by tests).
func (d *Dict) wordCount() int {
	n, _ := strconv.Atoi(d.ifo["wordcount"])
	return n
}
