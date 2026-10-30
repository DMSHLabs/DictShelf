// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package slob

import (
	"cmp"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/resource"
)

// ReaderVersion is the behaviour version of this format's Reader (see
// dict.RegisterReaderVersion). Bump it in the same commit as any change to
// what the Reader yields, and update the golden in reader_golden_test.go.
const ReaderVersion = 1

func init() {
	dict.RegisterFormat(".slob", func(path string) (dict.Dictionary, error) { return Open(path) })
	dict.RegisterReader(".slob", func(path string) (dict.Reader, error) { return NewReader(path) })
	dict.RegisterReaderVersion("slob", ReaderVersion)
}

// Dict is one opened .slob dictionary (direct backend). Lookup runs over
// an in-memory copy of the ref list, decoded on the first lookup, with
// exact/fold maps - no ICU collation needed (see docs/FORMATS.md). Safe for
// concurrent readers.
type Dict struct {
	c         *container
	meta      dict.Meta
	exactOnce sync.Once // exactIdx is built on first exact lookup only
	exactIdx  map[string][]int
	foldOnce  sync.Once // foldIdx is built on first accent-folded lookup only
	foldIdx   map[string][]int
}

func Open(path string) (*Dict, error) {
	c, err := openContainer(path)
	if err != nil {
		return nil, err
	}
	// No headword index is built at open - neither exact nor folded - and the
	// ref list itself is not read. Both indexes are built lazily on the first
	// lookup (see ensureExact/ensureFold), so opening a slob for the ingest
	// scan, a browse or a media listing never pays for a lookup index. The
	// official aard2/GoldenDict readers keep no in-memory headword index at
	// all; our ingested SQLite path is that index.
	d := &Dict{c: c}
	name := strings.TrimSpace(c.tags["label"])
	if name == "" {
		base := filepath.Base(path)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	d.meta = dict.Meta{
		Name:        name,
		Format:      "slob",
		Path:        path,
		Description: strings.TrimSpace(c.tags["copyright"]),
		EntryCount:  c.refCount,
		Header:      headerFields(c),
	}
	return d, nil
}

// headerFields is the slob's tags in file order, minus label and copyright,
// which Meta already carries as Name and Description.
func headerFields(c *container) []dict.Field {
	var out []dict.Field
	for _, k := range c.tagOrder {
		if k == "label" || k == "copyright" {
			continue
		}
		if v := strings.TrimSpace(c.tags[k]); v != "" {
			out = append(out, dict.Field{Name: k, Value: v})
		}
	}
	return out
}

func (d *Dict) Meta() dict.Meta { return d.meta }
func (d *Dict) Caps() dict.Caps { return dict.Caps{Exact: true, Prefix: true} }
func (d *Dict) Close() error    { return d.c.Close() }

// Tags exposes the raw slob tag map (label, uri, license…).
func (d *Dict) Tags() map[string]string { return d.c.tags }

func (d *Dict) Exact(word string, limit int) ([]dict.Result, error) {
	word = strings.TrimSpace(word)
	refs, err := d.c.allRefs()
	if err != nil {
		return nil, err
	}
	d.ensureExact(refs)
	idxs := d.exactIdx[word]
	if len(idxs) == 0 {
		d.ensureFold(refs)
		idxs = d.foldIdx[fold(word)]
	}
	return d.results(refs, idxs, limit)
}

// ensureExact builds the raw headword index on first lookup, so opens that only
// feed the ingest scan, a browse or a media listing never build it.
func (d *Dict) ensureExact(refs []ref) {
	d.exactOnce.Do(func() {
		idx := make(map[string][]int, len(refs))
		for i, r := range refs {
			idx[r.key] = append(idx[r.key], i)
		}
		d.exactIdx = idx
	})
}

// ensureFold builds the accent/case-folded ref index on first use, deferring
// the per-headword NFD fold that dominates open time and RAM.
func (d *Dict) ensureFold(refs []ref) {
	d.foldOnce.Do(func() {
		idx := make(map[string][]int, len(refs))
		for i, r := range refs {
			f := fold(r.key)
			idx[f] = append(idx[f], i)
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
	refs, err := d.c.allRefs()
	if err != nil {
		return nil, err
	}
	d.ensureExact(refs)
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
		for i, r := range refs {
			if seen[i] {
				continue
			}
			k := r.key
			if useFold {
				k = fold(k)
			}
			if strings.HasPrefix(k, key) {
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
		d.ensureFold(refs)
		idxs = append(idxs, d.foldIdx[fold(word)]...)
		for _, i := range idxs {
			seen[i] = true
		}
		scan(true)
	}
	return d.results(refs, idxs, limit)
}

// results renders the referenced blobs, keeping only article content
// types (resources are reachable via Resource, not search).
func (d *Dict) results(refs []ref, idxs []int, limit int) ([]dict.Result, error) {
	var out []dict.Result
	seen := map[[2]uint32]bool{} // dedupe alias refs to the same blob
	for _, i := range idxs {
		r := refs[i]
		blobKey := [2]uint32{r.bin, uint32(r.item)}
		if seen[blobKey] {
			continue
		}
		ctype, data, err := d.c.getItem(r.bin, r.item)
		if err != nil {
			return nil, err
		}
		body, ok := articleHTML(ctype, data)
		if !ok {
			continue
		}
		seen[blobKey] = true
		out = append(out, dict.Result{Headword: r.key, Body: body})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// articleHTML converts blob content to HTML when it is article-like.
func articleHTML(ctype string, data []byte) (string, bool) {
	switch {
	case strings.HasPrefix(ctype, "text/html"):
		return string(data), true
	case strings.HasPrefix(ctype, "text/plain"):
		return "<p>" + strings.ReplaceAll(htmlEscape(string(data)), "\n", "<br/>") + "</p>", true
	default:
		return "", false
	}
}

// htmlEscaper is built once: strings.NewReplacer compiles a trie, and this
// runs for the text item of every lookup.
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func htmlEscape(s string) string { return htmlEscaper.Replace(s) }

func fold(s string) string { return dict.Fold(s) }

// Keywords reads the requested range of the ref list from the file; a browse
// never decodes the whole list.
func (d *Dict) Keywords(offset, n int) []string {
	lo, hi, ok := dict.KeywordRange(d.c.refCount, offset, n)
	if !ok {
		return nil
	}
	locs, err := d.c.locate(lo, hi)
	out := make([]string, hi-lo)
	if err == nil {
		err = d.c.walk(locs, true, func(l *refLoc, r ref) error {
			out[int(l.idx)-lo] = r.key
			return nil
		})
	}
	if err != nil {
		// The signature cannot carry it, and a list with holes must not pass
		// for the page: name the failure and return nothing.
		logx.Warn("%skeywords: %v", logx.Dict(d.meta.Name), err)
		return nil
	}
	return out
}

// Resource serves any blob by its ref key (slob stores resources as
// ordinary refs with non-article content types).
func (d *Dict) Resource(name string) (io.ReadCloser, string, error) {
	// ensureExact/ensureFold, not a bare map read. Both indexes are built
	// lazily on first use, and a Dict opened only to serve files has had no
	// earlier lookup to build them - a bare read would consult two nil maps and
	// report every resource missing. That is the normal case for a PREPARED
	// dictionary: searches are answered from
	// text.db, and the slob is reopened solely as the resource fallback
	// (registry.go's upgraded.src), where no lookup ever runs.
	refs, err := d.c.allRefs()
	if err != nil {
		return nil, "", err
	}
	d.ensureExact(refs)
	idxs := d.exactIdx[name]
	if len(idxs) == 0 {
		d.ensureFold(refs)
		idxs = d.foldIdx[fold(name)]
	}
	if len(idxs) == 0 {
		return nil, "", dict.ErrNotFound
	}
	r := refs[idxs[0]]
	ctype, data, err := d.c.getItem(r.bin, r.item)
	if err != nil {
		return nil, "", err
	}
	return resource.Bytes(data), ctype, nil
}

// Resources lists ref keys whose content type is not article-like
// (images, audio, css, …), reading the ref list from the file and only the
// bin headers.
func (d *Dict) Resources() []string {
	var out []string
	locs, err := d.c.locate(0, d.c.refCount)
	if err == nil {
		err = d.c.walk(locs, true, func(_ *refLoc, r ref) error {
			ctype, err := d.c.itemContentType(r.bin, r.item)
			if err != nil || strings.HasPrefix(ctype, "text/html") || strings.HasPrefix(ctype, "text/plain") {
				return nil
			}
			out = append(out, r.key)
			return nil
		})
	}
	if err != nil {
		logx.Warn("%sresources: %v (listing the %d read so far)", logx.Dict(d.meta.Name), err, len(out))
	}
	sort.Strings(out)
	return out
}

// ---- ingest --------------------------------------------------------------

// Reader scans blobs bin-by-bin (sequential decompression - refs order
// would thrash bins) and attaches every ref key of a blob as headwords:
// first key = display headword, the rest become aliases.
//
// It holds 24 bytes per ref - its place in the list, its blob, where its
// record is - and reads a blob's keys from the file only when the blob is
// emitted, so preparing a Wikipedia-scale slob never holds its keys at once.
type Reader struct {
	d    *Dict
	locs []refLoc // every ref, ordered by blob, then by place in the list
	pos  int
	keys refWindow
}

func NewReader(path string) (*Reader, error) {
	d, err := Open(path)
	if err != nil {
		return nil, err
	}
	locs, err := d.c.locate(0, d.c.refCount)
	if err == nil {
		err = d.c.walk(locs, false, func(l *refLoc, r ref) error {
			l.blob = uint64(r.bin)<<16 | uint64(r.item)
			return nil
		})
	}
	if err != nil {
		d.Close()
		return nil, err
	}
	// By blob, so bins decompress once each; within a blob in list order, so
	// the first key - the display headword - is the one the list puts first.
	slices.SortFunc(locs, func(a, b refLoc) int {
		if c := cmp.Compare(a.blob, b.blob); c != 0 {
			return c
		}
		return cmp.Compare(a.idx, b.idx)
	})
	return &Reader{d: d, locs: locs, keys: refWindow{c: d.c, size: keyWindowBytes}}, nil
}

func (r *Reader) Meta() dict.Meta { return r.d.Meta() }

func (r *Reader) Next() (dict.Entry, error) {
	for r.pos < len(r.locs) {
		lo, blob := r.pos, r.locs[r.pos].blob
		for r.pos < len(r.locs) && r.locs[r.pos].blob == blob {
			r.pos++
		}
		bin, item := uint32(blob>>16), uint16(blob)
		bk := [2]uint32{bin, uint32(item)}
		ctype, data, err := r.d.c.getItem(bin, item)
		if err != nil {
			return dict.Entry{}, fmt.Errorf("slob ingest blob %v: %w", bk, err)
		}
		body, ok := articleHTML(ctype, data)
		if !ok {
			continue // resource blob: stays lazy in the source file
		}
		keys := make([]string, 0, r.pos-lo)
		for _, l := range r.locs[lo:r.pos] {
			rf, err := r.keys.ref(l.pos, true)
			if err != nil {
				return dict.Entry{}, fmt.Errorf("slob ingest blob %v: %w", bk, err)
			}
			// Refs differing only by fragment name the blob once.
			if !slices.Contains(keys, rf.key) {
				keys = append(keys, rf.key)
			}
		}
		return dict.Entry{Headwords: keys, Body: body, Kind: dict.BodyHTML}, nil
	}
	return dict.Entry{}, io.EOF
}

func (r *Reader) Close() error { return r.d.Close() }
