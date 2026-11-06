// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/sysmem"
)

const (
	ingestBulkRows  = 256 // five-column rows use 1280 bind variables on both supported SQLite drivers
	ingestBulkBytes = 4 << 20
)

// ingestBulk retains at most one writer batch. Flush decomposes the remainder
// into power-of-two statements, so only nine statement shapes are prepared.
type ingestBulk struct {
	ctx   context.Context
	tx    *sql.Tx
	head  string
	cols  int
	stmts map[int]*sql.Stmt
	args  []any
	rows  int
}

func newIngestBulk(ctx context.Context, tx *sql.Tx, head string, cols int) *ingestBulk {
	return &ingestBulk{ctx: ctx, tx: tx, head: head, cols: cols, stmts: make(map[int]*sql.Stmt)}
}

func (b *ingestBulk) add(args ...any) {
	b.args = append(b.args, args...)
	b.rows++
}

func (b *ingestBulk) flush() error {
	for _, n := range [...]int{256, 128, 64, 32, 16, 8, 4, 2, 1} {
		for b.rows >= n {
			if err := b.exec(n, b.args[:n*b.cols]); err != nil {
				return err
			}
			clear(b.args[:n*b.cols]) // release article bodies after SQLite has copied them
			b.args = b.args[n*b.cols:]
			b.rows -= n
		}
	}
	b.args = b.args[:0]
	return nil
}

func (b *ingestBulk) exec(n int, args []any) error {
	stmt := b.stmts[n]
	if stmt == nil {
		row := "(" + strings.TrimSuffix(strings.Repeat("?,", b.cols), ",") + ")"
		query := b.head + strings.TrimSuffix(strings.Repeat(row+",", n), ",")
		var err error
		stmt, err = b.tx.PrepareContext(b.ctx, query)
		if err != nil {
			return err
		}
		b.stmts[n] = stmt
	}
	_, err := stmt.ExecContext(b.ctx, args...)
	return err
}

func (b *ingestBulk) close() {
	for _, stmt := range b.stmts {
		_ = stmt.Close()
	}
}

type ingestRowWriter struct {
	entry, fts, article, trigram, wref, alias, link *ingestBulk
	batchBytes                                      int64
	// bodyBytes and headwordBytes are the two measured drivers of the
	// dictionary's indexes: the first is what their searchable text weighs
	// BEFORE compression (that is what an article index is built over), the second
	// is what the headword indexes scale with. Both describe the source, not
	// the prepared content, and are recorded as volatile meta.
	bodyBytes, headwordBytes int64
	// storedBodyBytes is the same body after compression: what it actually costs
	// the file. Both are needed, because a database can hold bodies that no
	// index covers, and then the index is priced against what is on disk.
	storedBodyBytes int64

	ctx   context.Context
	tx    *sql.Tx
	codec *bodyCodec      // how bodies are stored
	acct  *sysmem.Account // the window's draw on the process budget
	total int             // the source's declared entry count, for dictSize

	// The window (ingest_bulk.go's hold): every entry since the start whose
	// body is still text, in order, until the dictionary is cut. It is what the
	// dictionary is cut from, and holding it is why the first bodies are
	// uncompressed for as long as the window lasts.
	held       []heldEntry
	heldText   int64
	heldCharge int64
	dictBytes  int
}

// heldEntry is one entry waiting for the dictionary: its id, which the scan
// assigned in source order, and the prepared entry with its body still text.
type heldEntry struct {
	id int64
	e  preparedIngestEntry
}

func newIngestRowWriter(ctx context.Context, tx *sql.Tx, plan Plan, codec *bodyCodec, acct *sysmem.Account, total int) *ingestRowWriter {
	w := &ingestRowWriter{
		entry: newIngestBulk(ctx, tx, "INSERT INTO entry(id, w, wf, wc, m) VALUES", 5),
		wref:  newIngestBulk(ctx, tx, "INSERT INTO wref(w, lw, id) VALUES", 3),
		alias: newIngestBulk(ctx, tx, "INSERT INTO alias(w, wf, wc, entry_id) VALUES", 4),
		link:  newIngestBulk(ctx, tx, "INSERT INTO plink(w, target) VALUES", 2),
		ctx:   ctx,
		tx:    tx,
		codec: codec,
		acct:  acct,
		total: total,
	}
	if plan.Contains {
		w.fts = newIngestBulk(ctx, tx, "INSERT INTO entry_fts(rowid, w) VALUES", 2)
		w.trigram = newIngestBulk(ctx, tx, "INSERT INTO entry_trigram(rowid, w) VALUES", 2)
	}
	if plan.FullText {
		table := "article_fts"
		if !plan.ArticlesInside {
			table = "articles.article_fts"
		}
		w.article = newIngestBulk(ctx, tx, "INSERT INTO "+table+"(rowid, w, txt) VALUES", 3)
	}
	return w
}

// write takes one entry in source order: kept in the window while the
// dictionary is not cut, written otherwise.
//
// An entry can be prepared BEFORE the dictionary exists and reach the writer
// after it does - the preparation runs ahead of the writer, out of order - and
// such an entry is written, not held: the window belongs to the first cut, and
// holding text after it would cut a second dictionary over the first.
func (w *ingestRowWriter) write(id int64, e preparedIngestEntry) error {
	if e.pending && w.codec.enc.Load() == nil {
		return w.hold(id, e)
	}
	return w.put(id, e)
}

// recordOverhead is what one held entry costs besides its strings: the struct,
// its slot in held and the alias slice header.
const recordOverhead = 192

// hold keeps an entry whose body is still text, charging it to the ingest's
// account twice over for the collector's headroom, plus the record itself. A
// redirect never comes here: it is written at once and takes no entry id, so
// holding it would only keep a dictionary of redirects in memory until the
// end. The window is cut once it holds dictWindow of article text, or as soon
// as the budget refuses more - after at least one entry, so a preparation the
// budget has no room for still gets a dictionary.
func (w *ingestRowWriter) hold(id int64, e preparedIngestEntry) error {
	n := 2*int64(len(e.raw)+len(e.text)+len(e.headword)) + recordOverhead
	for _, a := range e.aliases {
		n += 2 * int64(len(a.word))
	}
	if !w.acct.TryAcquire(n) {
		if len(w.held) > 0 {
			if err := w.cut(false); err != nil {
				return err
			}
			return w.put(id, e)
		}
		n = 0
	}
	w.heldCharge += n
	w.held = append(w.held, heldEntry{id: id, e: e})
	w.heldText += int64(len(e.raw))
	if w.heldText >= dictWindow {
		return w.cut(false)
	}
	return nil
}

// cut makes the dictionary from the window's articles, starts the encoder,
// stores the dictionary, and writes the window in order. eof says the source
// ended inside the window, so its text - and therefore the dictionary's size -
// is known exactly.
func (w *ingestRowWriter) cut(eof bool) error {
	bodies := make([]string, 0, len(w.held))
	for _, h := range w.held {
		bodies = append(bodies, h.e.raw)
	}
	dict := sampleDict(bodies, dictSize(w.heldText, len(w.held), w.total, eof), dictSliceLen)
	if err := w.codec.start(dict, w.acct); err != nil {
		return err
	}
	if len(dict) > 0 {
		if _, err := w.tx.ExecContext(w.ctx, "INSERT INTO body_dict(id, data) VALUES(0, ?)", dict); err != nil {
			return err
		}
	}
	w.dictBytes = len(dict)
	encodeHeld(w.held, w.codec.enc.Load())
	for _, h := range w.held {
		if err := w.put(h.id, h.e); err != nil {
			return err
		}
	}
	w.held = nil
	w.acct.Release(w.heldCharge)
	w.heldCharge = 0
	return nil
}

// encodeHeld encodes the window on the encoder's own goroutine count. The
// window is the whole corpus whenever the source ended inside it - every small
// dictionary - and encoding that on the writer's one goroutine left a small
// ingest slower than the DEFLATE it replaced, on a machine with twenty-four
// cores and one of them working.
func encodeHeld(held []heldEntry, enc *bodyEncoder) {
	var next atomic.Int64
	var wg sync.WaitGroup
	for range encoders() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= len(held) {
					return
				}
				if held[i].e.pending {
					held[i].e.body = enc.encode(held[i].e.raw)
					held[i].e.raw, held[i].e.pending = "", false
				}
			}
		}()
	}
	wg.Wait()
}

// finish is the last call: it cuts the dictionary if the source ended inside
// the window, then writes every row still buffered. A MID-scan flush must not
// cut: the dictionary is cut from the first window of article text, not from
// the first batch, and a dictionary small enough to end inside the window is
// exactly the case where that difference shows.
func (w *ingestRowWriter) finish() error {
	if w.codec.enc.Load() == nil {
		if err := w.cut(true); err != nil {
			return err
		}
	}
	return w.flush()
}

// put writes one entry, encoding its body first if it left the window before
// the encoder existed.
func (w *ingestRowWriter) put(id int64, e preparedIngestEntry) error {
	if e.pending {
		e.body = w.codec.enc.Load().encode(e.raw)
		e.raw, e.pending = "", false
	}
	w.entry.add(id, e.headword, e.folded, e.caseKey, e.body)
	w.wref.add(e.headword, e.lower, id)
	w.bodyBytes += e.bodyBytes
	w.storedBodyBytes += int64(len(e.body))
	w.headwordBytes += int64(len(e.headword))
	w.batchBytes += int64(len(e.headword))*4 + int64(len(e.folded)+len(e.caseKey)+len(e.lower)+len(e.body))
	if w.fts != nil {
		w.fts.add(id, e.headword)
		w.trigram.add(id, e.folded)
		w.batchBytes += int64(len(e.headword) + len(e.folded))
	}
	if w.article != nil {
		w.article.add(id, e.headword, e.text)
		w.batchBytes += int64(len(e.headword) + len(e.text))
	}
	for _, alias := range e.aliases {
		w.alias.add(alias.word, alias.folded, alias.caseKey, id)
		w.batchBytes += int64(len(alias.word) + len(alias.folded) + len(alias.caseKey))
	}
	return w.flushIfFull()
}

func (w *ingestRowWriter) redirect(headword, target string) error {
	w.link.add(headword, target)
	w.batchBytes += int64(len(headword) + len(target))
	return w.flushIfFull()
}

func (w *ingestRowWriter) flushIfFull() error {
	if w.entry.rows < ingestBulkRows && w.link.rows < ingestBulkRows && w.alias.rows < ingestBulkRows && w.batchBytes < ingestBulkBytes {
		return nil
	}
	return w.flush()
}

// Entry rows go first because alias has a foreign key to entry. All tables
// are flushed before redirect resolution or transaction commit reads them.
// This never cuts the dictionary (see finish): a flush happens mid-scan, and
// the window is a fixed amount of article TEXT from the start, not a batch.
func (w *ingestRowWriter) flush() error {
	for _, b := range []*ingestBulk{w.entry, w.fts, w.article, w.trigram, w.wref, w.alias, w.link} {
		if b != nil {
			if err := b.flush(); err != nil {
				return err
			}
		}
	}
	w.batchBytes = 0
	return nil
}

func (w *ingestRowWriter) close() {
	for _, b := range []*ingestBulk{w.entry, w.fts, w.article, w.trigram, w.wref, w.alias, w.link} {
		if b != nil {
			b.close()
		}
	}
}
