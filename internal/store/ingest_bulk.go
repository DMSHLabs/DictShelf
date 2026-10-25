// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"strings"
)

const (
	ingestBulkRows  = 128 // five-column entry rows fit SQLite's older 999-variable limit
	ingestBulkBytes = 4 << 20
)

// ingestBulk retains at most one writer batch. Flush decomposes the remainder
// into power-of-two statements, so only eight statement shapes are prepared.
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
	for _, n := range [...]int{128, 64, 32, 16, 8, 4, 2, 1} {
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
}

func newIngestRowWriter(ctx context.Context, tx *sql.Tx, plan Plan) *ingestRowWriter {
	w := &ingestRowWriter{
		entry: newIngestBulk(ctx, tx, "INSERT INTO entry(id, w, wf, wc, m) VALUES", 5),
		wref:  newIngestBulk(ctx, tx, "INSERT INTO wref(w, lw, id) VALUES", 3),
		alias: newIngestBulk(ctx, tx, "INSERT INTO alias(w, wf, wc, entry_id) VALUES", 4),
		link:  newIngestBulk(ctx, tx, "INSERT INTO plink(w, target) VALUES", 2),
	}
	if plan.Contains {
		w.fts = newIngestBulk(ctx, tx, "INSERT INTO entry_fts(rowid, w) VALUES", 2)
		w.trigram = newIngestBulk(ctx, tx, "INSERT INTO entry_trigram(rowid, w) VALUES", 2)
	}
	if plan.FullText {
		w.article = newIngestBulk(ctx, tx, "INSERT INTO article_fts(rowid, w, txt) VALUES", 3)
	}
	return w
}

func (w *ingestRowWriter) write(id int64, e preparedIngestEntry) error {
	w.entry.add(id, e.headword, e.folded, e.caseKey, e.body)
	w.wref.add(e.headword, e.lower, id)
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
