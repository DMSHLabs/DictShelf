// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// The archive's own full-text index, read in place. It lives in this file
// rather than in zim.go for the fork's usual reason: zim.go is upstream's, and
// every line added to a shared file is merge-conflict surface on the next sync.
// Nothing here changes how a dict.Dictionary behaves - Exact and Prefix are
// untouched - so a sync resolves by taking upstream's side.

package zim

import (
	"fmt"
	"io"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/format/zim/glass"
)

// XapianIndex returns the archive's own full-text index - the X/fulltext/xapian
// entry - as a seekable window onto the archive file, with its length, so a
// search reads the 518 MB (or 3.18 GB) index in place instead of copying it.
// Kiwix writes this entry uncompressed for exactly that reason; a compressed
// cluster is refused rather than decompressed whole, because the point of a
// window is that the bytes are already addressable.
//
// dict.ErrNotFound reports an archive that carries no index at all - a ZIM
// built without --indexing - which is what tells a caller it has to build one
// of its own. What this returns is Kiwix's index, built with the stemmer and
// stopwords of the archive's own language, so it is only usable by a reader
// that reproduces them (see internal/format/zim/glass).
func (d *Dict) XapianIndex() (io.ReaderAt, int64, error) {
	i, de, ok := d.c.find('X', "fulltext/xapian")
	if !ok {
		return nil, 0, dict.ErrNotFound
	}
	if de.isRedirect() {
		var err error
		if _, de, err = d.c.resolve(i); err != nil {
			return nil, 0, err
		}
	}
	cd, lo, hi, err := d.c.blobSpan(de.cluster, de.blob)
	if err != nil {
		return nil, 0, err
	}
	if !cd.direct {
		return nil, 0, fmt.Errorf("fulltext/xapian is stored in a compressed cluster, so it cannot be read in place")
	}
	n := int64(hi - lo)
	return io.NewSectionReader(d.c.f, cd.body+int64(lo), n), n, nil
}

// OwnIndexMetaKey is the meta key a prepared database records when its archive
// carries its own full-text index. It is what tells the product that full text
// is answered from the archive, so no index of ours is built for this archive -
// and it is deliberately a fact about the SOURCE, recorded when the database
// was built, because the alternative is opening a 48 GB archive every time a
// dictionary row is drawn. internal/store reads the same key by this spelling
// (store.OwnFullText); the two have to agree, which is why it is exported from
// one side and named here.
const OwnIndexMetaKey = "own_ft_index"

// HasFullTextIndex reports whether this archive carries its own full-text index
// in a form a reader can address - the question an ingest asks before choosing
// what to build, and the one the search asks before promising full text.
//
// It is cheap by design: one directory lookup and one byte of the cluster's
// info field, no decompression and no table read. An index stored inside a
// COMPRESSED cluster answers "no", because reading it would mean decompressing
// up to 3 GB before the first term could be looked up - an archive like that
// gets an index of ours instead, which is the useful outcome rather than an
// error nobody can act on.
func (d *Dict) HasFullTextIndex() bool {
	i, de, ok := d.c.find('X', "fulltext/xapian")
	if !ok {
		return false
	}
	if de.isRedirect() {
		var err error
		if i, de, err = d.c.resolve(i); err != nil {
			return false
		}
	}
	start, end := d.c.extent(de.cluster)
	if end <= start || end > uint64(d.c.size) {
		return false
	}
	var info [1]byte
	if _, err := d.c.f.ReadAt(info[:], int64(start)); err != nil {
		return false
	}
	return info[0]&0x0F <= 1 // stored, not compressed: a reader can address it
}

// HasOwnFullTextIndex is HasFullTextIndex for an archive that is not open yet:
// it opens the container, asks, and closes it. Opening reads the header, the
// MIME list and the directory pointers, not the articles.
func HasOwnFullTextIndex(path string) (bool, error) {
	d, err := Open(path)
	if err != nil {
		return false, err
	}
	defer d.Close()
	return d.HasFullTextIndex(), nil
}

// FullText answers a full-text query from the archive's own index, with the
// same shape of result Exact returns: the entry's headword and its article,
// converted by this package's own article pipeline, so a caller cannot tell a
// Kiwix hit from a headword hit.
//
// What it is for: a Kiwix archive carries a full-text index of every article -
// 518 MB where an index of ours would be 1.29 GiB, on the same archive - and
// reading it means a ZIM can be searched without building anything. What it is
// not: a replacement for Exact or Prefix. The index answers "articles
// mentioning these words", knows nothing about headwords, and ranks by nothing
// more than how often the words occur, so a caller that wants a dictionary-like
// answer still asks the headword side.
//
// The errors are distinguishable: dict.ErrNotFound when the archive has no
// index, dict.ErrUnsupported wrapped around glass.ErrNoStemmer when it has one
// in a language this reader cannot stem, and a plain error when the index is
// unreadable.
func (d *Dict) FullText(query string, limit int) ([]dict.Result, error) {
	window, size, err := d.XapianIndex()
	if err != nil {
		return nil, err
	}
	db, err := glass.OpenSource(window, size)
	if err != nil {
		return nil, err
	}
	res, err := db.Search(query)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", dict.ErrUnsupported, err)
	}
	if len(res.Hits) == 0 {
		return nil, nil
	}
	// The index stores the article's path and nothing else, so the hit has to
	// be found back in the archive's own directory. A path the archive does not
	// hold is skipped: the index can name an article that has since been
	// dropped, and one stale entry is not a failed search.
	// No sorting here: the reader ranks by Xapian's own BM25, so its order is
	// the order (glass/bm25.go). Re-sorting by how often the words occur is
	// what put a German list article above the Jazzmusiker article itself.
	hits := res.Hits
	// Only the most frequent hits are worth resolving: a common word has over a
	// million of them, each one a B-tree descent into the directory, and the
	// answer is the first few. The margin covers the paths that turn out to be
	// unresolvable.
	want := limit
	if want <= 0 {
		want = 100
	}
	candidates := hits
	if n := want*10 + 50; n < len(candidates) {
		candidates = candidates[:n]
	}
	idxs := make([]int, 0, want)
	seen := map[int]bool{}
	for _, h := range candidates {
		path, err := db.DocPath(h.DocID)
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(path, "C/")
		i, de, ok := d.c.find(d.c.contentNS(), name)
		if !ok || !d.c.isArticle(i, de) || seen[i] {
			continue
		}
		seen[i] = true
		idxs = append(idxs, i)
		if len(idxs) >= want {
			break
		}
	}
	return d.results(idxs, 0)
}
