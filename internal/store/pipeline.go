// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"database/sql"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/sysmem"
)

// How the entries of a Reader reach the database (D182). One goroutine, the
// caller's, writes: SQLite has one writer. While a person waits on the
// preparation (dict.Urgent) the work before the write - compressing bodies,
// stripping them for full text - runs on encoder goroutines, fed in order by
// a producer goroutine that owns r.Next; otherwise the writer does all of it
// itself on one core, the shape INDEX_WORKERS promises a background
// preparation. The two switch at batch boundaries, either way, and exactly
// one goroutine calls r.Next at any time. Both write the same rows in the
// same order, so the prepared content is the same.

// bulkRows is how many rows one INSERT statement writes.
const bulkRows = 256

// batchBytes bounds the article text of one pipeline batch.
const batchBytes = 4 << 20

// urgentAt reports whether the preparation of src is urgent now. A variable
// so the tests can switch lanes mid-run.
var urgentAt = dict.Urgent

// record is one entry as it is written: an article, or a redirect (link set,
// nothing else).
type record struct {
	hw      string
	aliases []string
	body    []byte // encodeBody of the article's HTML
	txt     string // its text for full-text search, "" without
	link    string // a redirect's target
}

// encodeEntry is the work before the write; ok is false for an entry that is
// not written (no headword).
func encodeEntry(e dict.Entry, plan Plan) (rec record, ok bool, err error) {
	if len(e.Headwords) == 0 {
		return record{}, false, nil
	}
	hw := strings.TrimSpace(e.Headwords[0])
	if hw == "" {
		return record{}, false, nil
	}
	if e.LinkTo != "" {
		return record{hw: hw, link: e.LinkTo}, true, nil
	}
	body, err := NormalizeBody(e)
	if err != nil {
		return record{}, false, err
	}
	rec = record{hw: hw, body: encodeBody(body)}
	if plan.FullText {
		rec.txt = StripHTML(body)
	}
	for _, extra := range e.Headwords[1:] {
		if extra = strings.TrimSpace(extra); extra != "" && extra != hw {
			rec.aliases = append(rec.aliases, extra)
		}
	}
	return rec, true, nil
}

// bulk inserts the rows of one table bulkRows at a time, holding the rest
// until it has that many; flush writes what is left 64, 16, 4 and 1 at a time.
type bulk struct {
	tx    *sql.Tx
	head  string // "INSERT INTO t(a, b) VALUES"
	cols  int
	stmts map[int]*sql.Stmt
	args  []any
}

func newBulk(tx *sql.Tx, head string, cols int) *bulk {
	return &bulk{tx: tx, head: head, cols: cols, stmts: map[int]*sql.Stmt{}}
}

func (b *bulk) add(args ...any) error {
	b.args = append(b.args, args...)
	if len(b.args) < bulkRows*b.cols {
		return nil
	}
	err := b.exec(bulkRows, b.args)
	clear(b.args)
	b.args = b.args[:0]
	return err
}

func (b *bulk) flush() error {
	rest := b.args
	for _, n := range [...]int{64, 16, 4, 1} {
		for len(rest) >= n*b.cols {
			if err := b.exec(n, rest[:n*b.cols]); err != nil {
				return err
			}
			rest = rest[n*b.cols:]
		}
	}
	b.args = nil
	return nil
}

func (b *bulk) exec(n int, args []any) error {
	st, ok := b.stmts[n]
	if !ok {
		row := "(" + strings.TrimSuffix(strings.Repeat("?,", b.cols), ",") + ")"
		var err error
		if st, err = b.tx.Prepare(b.head + strings.TrimSuffix(strings.Repeat(row+",", n), ",")); err != nil {
			return err
		}
		b.stmts[n] = st
	}
	_, err := st.Exec(args...)
	return err
}

func (b *bulk) close() {
	for _, st := range b.stmts {
		st.Close()
	}
}

// writer is the single SQLite writer: ids in reader order, every table
// through its bulk.
type writer struct {
	entry, fts, alias, link *bulk
	progress                Progress
	total                   int
	id, subEntries          int64
	linkCount               int64
}

func newWriter(tx *sql.Tx, progress Progress, total int) *writer {
	return &writer{
		entry:    newBulk(tx, "INSERT INTO entry(id, w, m) VALUES", 3),
		fts:      newBulk(tx, "INSERT INTO entry_fts(rowid, w, txt) VALUES", 3),
		alias:    newBulk(tx, "INSERT INTO alias(w, entry_id) VALUES", 2),
		link:     newBulk(tx, "INSERT INTO plink(w, target, lt) VALUES", 3),
		progress: progress,
		total:    total,
	}
}

func (w *writer) write(rec record) error {
	if rec.link != "" {
		w.linkCount++
		return w.link.add(rec.hw, rec.link, strings.ToLower(rec.link))
	}
	w.id++
	if err := w.entry.add(w.id, rec.hw, rec.body); err != nil {
		return err
	}
	if err := w.fts.add(w.id, rec.hw, rec.txt); err != nil {
		return err
	}
	if isSubEntry(rec.hw) {
		w.subEntries++
	}
	for _, a := range rec.aliases {
		if err := w.alias.add(a, w.id); err != nil {
			return err
		}
	}
	if w.progress != nil && w.id%batchSize == 0 {
		w.progress(clampDone(int(w.id), w.total), w.total)
	}
	return nil
}

// flush writes every row still held, before anything reads the tables.
func (w *writer) flush() error {
	for _, b := range []*bulk{w.entry, w.fts, w.alias, w.link} {
		if err := b.flush(); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) close() {
	for _, b := range []*bulk{w.entry, w.fts, w.alias, w.link} {
		b.close()
	}
}

// fill writes every entry of r through w, on the lane src's urgency chooses
// at each batch boundary. It returns only once no goroutine it started is
// left, so the caller may close r.
func fill(r dict.Reader, src, path string, plan Plan, w *writer) error {
	for n := 0; ; n++ {
		if n%bulkRows == 0 && urgentAt(src) {
			eof, err := pipe(r, src, path, plan, w)
			if err != nil || eof {
				return err
			}
			// Handed back: the writer owns r.Next again and reads a whole
			// batch on one core before it asks again, so lanes that flip
			// at every boundary still move.
		}
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		rec, ok, err := encodeEntry(e, plan)
		if err != nil {
			return err
		}
		if ok {
			if err := w.write(rec); err != nil {
				return err
			}
		}
	}
}

// batch is a run of entries moving through the pipeline.
type batch struct {
	entries []dict.Entry
	recs    []record
	weight  int64 // its draw on the preparation budget
	err     error // a Next or encode error, or a panic
	eof     bool  // the reader ended after entries
	inline  bool  // the budget had no room for it: the writer encodes it
	handoff bool  // the lane ends: the writer takes r.Next back
	done    chan struct{}
}

// pipe runs the pipeline until the reader ends (eof), the lane ends (the
// preparation is no longer urgent: the writer takes r.Next back), or an error,
// and returns after joining every goroutine it started - the producer's
// r.Next in progress included. Batches draw on an account of their own, so the
// first one in flight always runs whatever else the preparation holds.
func pipe(r dict.Reader, src, path string, plan Plan, w *writer) (eof bool, err error) {
	var acct sysmem.Account
	defer acct.Close()
	encoders := min(max(runtime.GOMAXPROCS(0)-1, 1), 8)
	order := make(chan *batch, 2*encoders+2) // every batch, in reader order
	jobs := make(chan *batch, 2*encoders+2)  // the batches with entries to encode
	stop := make(chan struct{})
	released := make(chan struct{}, 1) // the writer gave weight back
	var wg sync.WaitGroup
	defer func() {
		close(stop)
		wg.Wait()
	}()

	wg.Add(1)
	go func() { // the producer: the only caller of r.Next while it runs
		defer wg.Done()
		defer close(order)
		defer close(jobs)
		for {
			b := &batch{done: make(chan struct{})}
			if !urgentAt(src) {
				b.handoff = true
				close(b.done)
				send(order, b, stop)
				return
			}
			size := 0
			for len(b.entries) < bulkRows && size < batchBytes {
				e, err := safeNext(r, path)
				if errors.Is(err, io.EOF) {
					b.eof = true
					break
				}
				if err != nil {
					b.err = err
					break
				}
				b.entries = append(b.entries, e)
				size += len(e.Body)
			}
			if b.err != nil || len(b.entries) == 0 {
				close(b.done)
				send(order, b, stop)
				return
			}
			// Up to compressed and stripped copies beside the text, twice over
			// for the collector's headroom.
			w := int64(size) * 4
			if plan.FullText {
				w = int64(size) * 6
			}
			for !acct.TryAcquire(w) {
				if acct.Held() == 0 {
					// The budget has no room even for one batch in flight: this
					// one goes to the writer to encode, and the lane ends.
					b.inline = true
					close(b.done)
					if send(order, b, stop) {
						send(order, &batch{handoff: true, done: closed}, stop)
					}
					return
				}
				select { // weight comes back as the writer finishes a batch
				case <-released:
				case <-stop:
					return
				}
			}
			b.weight = w
			if !send(order, b, stop) {
				acct.Release(b.weight)
				return
			}
			if !send(jobs, b, stop) {
				return
			}
			if b.eof {
				return
			}
		}
	}()

	for range encoders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := range jobs {
				encodeBatch(b, plan, path)
				close(b.done)
			}
		}()
	}

	for b := range order {
		<-b.done
		if b.err != nil {
			return false, b.err
		}
		if b.inline {
			encodeBatch(b, plan, path)
			if b.err != nil {
				return false, b.err
			}
		}
		for _, rec := range b.recs {
			if err := w.write(rec); err != nil {
				return false, err
			}
		}
		acct.Release(b.weight)
		select {
		case released <- struct{}{}:
		default:
		}
		if b.eof || b.handoff {
			return b.eof, nil
		}
	}
	return false, errors.New("ingest: the producer stopped without an end")
}

// closed is a done channel already closed, for batches nothing encodes.
var closed = func() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}()

// send delivers b unless stop closes first.
func send(ch chan<- *batch, b *batch, stop <-chan struct{}) bool {
	select {
	case ch <- b:
		return true
	case <-stop:
		return false
	}
}

// safeNext is r.Next on a goroutine no caller can recover: a panic in a
// format's reader becomes the error dict.Open would have made of it.
func safeNext(r dict.Reader, path string) (e dict.Entry, err error) {
	defer func() {
		if p := recover(); p != nil {
			e, err = dict.Entry{}, dict.PanicError(path, p)
		}
	}()
	return r.Next()
}

// encodeBatch fills b.recs, or b.err.
func encodeBatch(b *batch, plan Plan, path string) {
	defer func() {
		if p := recover(); p != nil {
			b.recs, b.err = nil, dict.PanicError(path, p)
		}
	}()
	b.recs = make([]record, 0, len(b.entries))
	for _, e := range b.entries {
		rec, ok, err := encodeEntry(e, plan)
		if err != nil {
			b.err = err
			return
		}
		if ok {
			b.recs = append(b.recs, rec)
		}
	}
	b.entries = nil
}

// fillWref writes the redirect scratch table from the headword index, which
// covers (w, id) and never reaches the article pages; rows stream through,
// never collected.
func fillWref(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT id, w FROM entry INDEXED BY idx_entry_w")
	if err != nil {
		return err
	}
	defer rows.Close()
	b := newBulk(tx, "INSERT INTO wref(w, lw, id) VALUES", 3)
	defer b.close()
	for rows.Next() {
		var id int64
		var w string
		if err := rows.Scan(&id, &w); err != nil {
			return err
		}
		if err := b.add(w, strings.ToLower(w), id); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return b.flush()
}
