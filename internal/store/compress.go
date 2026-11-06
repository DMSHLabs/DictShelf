// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"compress/flate"
	"database/sql"
	"errors"
	"io"
	"runtime"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// Article bodies dominate a prepared dictionary: measured on a 40k-entry
// dictionary, the HTML in `entry.m` was 69 MB of a 72 MB database - the search
// indexes themselves were 2 MB. Stored verbatim, that text makes a text.db
// several times the size of the file it came from.
//
// So bodies are compressed per row. Per row, rather than in shared blocks like
// MDX/SLOB do, because a row must stay independently readable without a block
// cache; the cost is a worse ratio on very short articles, which is why a body
// is only stored compressed when compression actually wins.
//
// The first byte of a stored body says what it is:
//
//	0x00           raw DEFLATE (RFC 1951)
//	0x01           a zstd frame without its 4-byte magic, decoded with the
//	               database's raw dictionary (table body_dict, id 0) - or with
//	               none when the table holds no row
//	0x02-0x08      reserved
//	anything else  literal text
//
// 0x01-0x08 are markers only in a schema-2 database, the one that stores zstd
// rows; in schema 1 they are literal text. That is what lets a reader
// understand both without a flag of its own: the SCHEMA (PRAGMA user_version)
// decides how the first byte is read. And a body whose first byte is a marker
// byte of its database's schema is never stored literally - it is compressed -
// so a literal never looks like a marker.
const (
	markDeflate  = 0x00
	markZstd     = 0x01
	markReserved = 0x08 // the last reserved byte
)

// The schemas a text.db may have (PRAGMA user_version). 1 is DEFLATE and
// literal rows; 2 adds zstd rows and body_dict. The sidecar files - media.db,
// the link cache - carry their own version and stay at 1 whatever the text.db
// beside them is.
const (
	textSchemaMin = 1
	textSchemaMax = 2
	sidecarSchema = 1
)

// textSchemaOK reports whether this build reads a text.db of schema v.
func textSchemaOK(v int) bool { return v >= textSchemaMin && v <= textSchemaMax }

// compressLevel is where the size/speed balance is struck. Level 6 is the
// usual answer and the right one on a desktop, where an ingest is a minute of
// a machine that is plugged in.
//
// On a phone it is the wrong answer. Compressing every article of a large
// dictionary at level 6 is the longest sustained CPU burn this program ever
// does, and sustained CPU is precisely what drains a battery and heats a
// device into thermal throttling. Level 1 costs roughly a tenth of the CPU for
// article HTML - highly repetitive markup, which deflate's cheapest settings
// already handle well - in exchange for a database on the order of a tenth
// larger. Storage is the resource a phone has most of. (D64)
//
// Read-side is unaffected: DEFLATE decoding does not depend on the level a
// stream was written at, so databases stay interchangeable between platforms.
var compressLevel = func() int {
	if runtime.GOOS == "android" {
		return 1
	}
	return 6
}()

var flateWriters = sync.Pool{New: func() any {
	w, _ := flate.NewWriter(io.Discard, compressLevel)
	return w
}}

// encodeBody returns what to store for one article body.
func encodeBody(s string) []byte {
	if len(s) < 120 { // shorter than the deflate overhead pays back
		return []byte(s)
	}
	var buf bytes.Buffer
	buf.WriteByte(markDeflate)
	w := flateWriters.Get().(*flate.Writer)
	defer flateWriters.Put(w)
	w.Reset(&buf)
	if _, err := io.WriteString(w, s); err != nil {
		return []byte(s)
	}
	if err := w.Close(); err != nil {
		return []byte(s)
	}
	if buf.Len() >= len(s) { // incompressible: keep it readable
		return []byte(s)
	}
	return buf.Bytes()
}

// zstdMagic opens every zstd frame; a stored row carries the frame without it.
var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

// maxBody bounds one decoded zstd body, like the other decompression caps.
const maxBody = 64 << 20

// bodyDecoder reads the stored bodies of one database. The zstd decoder, and
// with it the dictionary its frames lean on, is made on the first zstd row:
// most opened dictionaries are searched for headwords and never read an
// article, and the dictionary would sit in memory for nothing. Safe for
// concurrent use - a decode holds the read lock - so a page of articles never
// builds a second decoder.
type bodyDecoder struct {
	schema int
	db     *sql.DB // where body_dict is read from; nil when there is none

	mu     sync.RWMutex
	zstd   *zstd.Decoder // nil until the first zstd row
	closed bool
}

func newBodyDecoder(schema int, db *sql.DB) *bodyDecoder {
	return &bodyDecoder{schema: schema, db: db}
}

// decode returns the article stored as raw. A row that does not decode
// degrades to what decoded, or to nothing; it never fails the query.
func (d *bodyDecoder) decode(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	switch b := raw[0]; {
	case b == markDeflate:
		return inflate(raw[1:])
	case d.schema >= 2 && b == markZstd:
		return d.unzstd(raw[1:])
	case d.schema >= 2 && b <= markReserved:
		return ""
	}
	return string(raw)
}

// unzstd decodes one frame. A decoder that could not be made is tried again by
// the next row: the failure may have been transient (a busy or interrupted
// read of body_dict).
func (d *bodyDecoder) unzstd(frame []byte) string {
	d.mu.RLock()
	if d.zstd != nil {
		defer d.mu.RUnlock()
		return d.decodeFrame(frame)
	}
	d.mu.RUnlock()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.zstd == nil && !d.closed {
		d.init()
	}
	if d.zstd == nil {
		return ""
	}
	return d.decodeFrame(frame)
}

// maxPooledFrame bounds the buffers framePool keeps: one huge article must
// not stay resident in the pool.
const maxPooledFrame = 1 << 20

// decodeFrame decodes with d.zstd, which the caller holds a lock on. The magic
// the row does not carry is put back on a pooled buffer, not on a new one.
func (d *bodyDecoder) decodeFrame(frame []byte) string {
	bp := framePool.Get().(*[]byte)
	buf := append(append((*bp)[:0], zstdMagic...), frame...)
	out, err := d.zstd.DecodeAll(buf, nil)
	if cap(buf) <= maxPooledFrame {
		*bp = buf[:0]
		framePool.Put(bp)
	}
	if err != nil {
		return ""
	}
	return string(out)
}

// init makes the zstd decoder. Called with mu held for writing.
func (d *bodyDecoder) init() {
	var dict []byte
	if d.db != nil {
		err := d.db.QueryRow("SELECT data FROM body_dict WHERE id = 0").Scan(&dict)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return
		}
	}
	opts := []zstd.DOption{
		zstd.WithDecoderConcurrency(1),
		zstd.WithDecoderMaxMemory(maxBody),
		zstd.WithDecoderMaxWindow(maxBody),
	}
	if len(dict) > 0 {
		opts = append(opts, zstd.WithDecoderDictRaw(0, dict))
	}
	z, err := zstd.NewReader(nil, opts...)
	if err != nil {
		return
	}
	d.zstd = z
}

// close releases the decoder for good: no later row makes another.
func (d *bodyDecoder) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.zstd != nil {
		d.zstd.Close()
	}
	d.zstd, d.closed = nil, true
}

var framePool = sync.Pool{New: func() any { b := make([]byte, 0, 4096); return &b }}

// flateReaders hold DEFLATE readers for reuse: a new reader per row costs
// about a tenth of the decode time.
var flateReaders sync.Pool

func inflate(b []byte) string {
	br := bytes.NewReader(b)
	r, _ := flateReaders.Get().(io.ReadCloser)
	if r == nil {
		r = flate.NewReader(br)
	} else if err := r.(flate.Resetter).Reset(br, nil); err != nil {
		r = flate.NewReader(br)
	}
	var out bytes.Buffer
	// a truncated or corrupt body must not take the whole query down; return
	// what inflated so the article degrades instead of vanishing
	_, _ = io.Copy(&out, r)
	flateReaders.Put(r)
	return out.String()
}
