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
	"sync/atomic"

	"github.com/klauspost/compress/zstd"

	"github.com/DMSHLabs/DictShelf/internal/sysmem"
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

// zstdLevel is where the size/speed balance is struck, on every platform.
//
// The old answer was DEFLATE at level 6 on a desktop and level 1 on a phone,
// because compressing every article of a large dictionary is the longest
// sustained CPU burn this program ever does, and sustained CPU is precisely
// what drains a battery and heats a device into thermal throttling (D64).
//
// zstd's cheapest tier answers both halves at once: with the database's own
// dictionary it writes about three quarters of what DEFLATE level 6 wrote,
// while using less CPU than DEFLATE level 6 - so the desktop gains in size and
// the phone gains in heat, and one level serves both. Read-side is unaffected:
// decoding does not depend on the level a frame was written at, so databases
// stay interchangeable between platforms and builds.
const zstdLevel = 1

// The smallest bodies worth compressing: a zstd row pays one marker byte and a
// frame header of a few bytes, and a body shorter than that stays a literal -
// unless its first byte is a marker, which is the one case that must compress
// whatever its length (bodyEncoder.reserved).
const zstdMinLen = 16

// deflateMinLen is DEFLATE's threshold, from when DEFLATE was the default.
// DEFLATE survives as the fallback for a body zstd must not take - one too
// large to be decoded back (maxBody) - and such a body is past this length
// long before the fallback is reached.
const deflateMinLen = 120

// dictWindow is how much body text the writer holds before it cuts the
// dictionary. A variable so a test can cut early, since a real ingest would
// have to produce 16 MiB to exercise the early cut.
var dictWindow int64 = 16 << 20

// dictSliceLen is the length of one dictionary slice.
const dictSliceLen = 1024

// dictCap is the largest dictionary. The fast zstd tiers gain nothing past
// 256 KiB: what they can use is the history, and a bigger sample buys no
// matches they would find (D191).
const dictCap = 256 << 10

// bodyCodec is one ingest's writer. It has no encoder until the dictionary is
// cut from the first bodies, and the writer holds the rows until then
// (ingest_bulk.go) - the dictionary has to exist before the first frame, and
// it can only be cut from text that has not been compressed yet.
type bodyCodec struct {
	enc atomic.Pointer[bodyEncoder]
}

func newBodyCodec() *bodyCodec { return &bodyCodec{} }

// schema is the user_version of the database this codec writes: zstd rows and
// their dictionary are what makes a database schema 2.
func (c *bodyCodec) schema() int { return textSchemaMax }

// encoders is how many bodies the zstd encoder compresses at once. More
// encoders mean more match tables, each built from the dictionary; past the
// machine's own parallelism they would only take memory.
func encoders() int { return min(max(runtime.GOMAXPROCS(0)-1, 1), 8) }

// start creates the zstd encoder for dict, charging its memory to acct, and
// publishes it: from here on encode compresses.
func (c *bodyCodec) start(dict []byte, acct *sysmem.Account) error {
	n := encoders()
	// Per encoder: the history (dictionary plus a block) and the match tables,
	// which the encoder builds from the dictionary for every frame.
	per := int64(2*len(dict)) + 1<<20
	if !acct.TryAcquire(int64(n) * per) {
		n = 1
		acct.TryAcquire(per)
	}
	opts := []zstd.EOption{
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(zstdLevel)),
		zstd.WithEncoderConcurrency(n),
		zstd.WithEncoderCRC(false),
		zstd.WithSingleSegment(true),
	}
	if len(dict) > 0 {
		opts = append(opts, zstd.WithEncoderDictRaw(0, dict))
	}
	z, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		return err
	}
	c.enc.Store(newBodyEncoder(z))
	return nil
}

// close releases the zstd encoder, if one was made.
func (c *bodyCodec) close() {
	if e := c.enc.Load(); e != nil {
		e.zstd.Close()
	}
}

// bodyEncoder turns one article's HTML into what is stored. Safe for
// concurrent use: EncodeAll takes one of the encoder's states and gives it
// back.
type bodyEncoder struct {
	zstd  *zstd.Encoder
	flate *sync.Pool // *flate.Writer, for a body zstd must not take
}

func newBodyEncoder(z *zstd.Encoder) *bodyEncoder {
	return &bodyEncoder{zstd: z, flate: &sync.Pool{New: func() any {
		w, _ := flate.NewWriter(io.Discard, 1)
		return w
	}}}
}

// reserved reports whether s would read as a marker in this schema. Such a
// body is never stored literally, whatever its length or the setting, so that
// "a literal never looks like a marker" holds by construction.
func (e *bodyEncoder) reserved(s string) bool {
	return len(s) > 0 && s[0] <= markReserved
}

// encode returns what to store for s: compressed when that is smaller, or when
// s must not be stored literally; s itself otherwise.
func (e *bodyEncoder) encode(s string) []byte {
	must := e.reserved(s)
	// A body the decoder could not read back (maxBody) is stored as DEFLATE,
	// which has no such cap.
	if (must || len(s) >= zstdMinLen) && len(s) <= maxBody {
		// Two allocations a row, as DEFLATE had: the text as a slice, and one
		// buffer the frame is written into straight after the marker, its
		// magic then dropped in place. A row that stays literal returns the
		// text slice it already has.
		src := []byte(s)
		out := e.zstd.EncodeAll(src, append(make([]byte, 0, len(s)/2+64), markZstd))
		if len(out) > 1+len(zstdMagic) && bytes.Equal(out[1:1+len(zstdMagic)], zstdMagic) {
			out = out[:1+copy(out[1:], out[1+len(zstdMagic):])]
			if must || len(out) < len(s) {
				return out
			}
			return src
		}
		// not a standard frame: never stored, and DEFLATE covers a must
	}
	// DEFLATE's own threshold is kept where it was, for the one body to which
	// it still applies: too large to be decoded back through zstd. Such a body
	// is far past it, and a body that must be compressed ignores it anyway.
	if must || (len(s) >= deflateMinLen && len(s) > maxBody) {
		if out, ok := e.deflate(s); ok && (must || len(out) < len(s)) {
			return out
		}
	}
	return []byte(s)
}

// deflate is the fallback: the old row format, kept because a database written
// by it must stay readable and a body too large for maxBody has to go
// somewhere. Against a bytes.Buffer its two error paths cannot be taken - the
// writer has nowhere to fail - so a body that must be compressed always leaves
// here compressed.
func (e *bodyEncoder) deflate(s string) ([]byte, bool) {
	var buf bytes.Buffer
	buf.WriteByte(markDeflate)
	w := e.flate.Get().(*flate.Writer)
	defer e.flate.Put(w)
	w.Reset(&buf)
	if _, err := io.WriteString(w, s); err != nil {
		return nil, false
	}
	if err := w.Close(); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// sampleDict is the raw dictionary: n = size/chunk slices of chunk bytes, the
// i-th taken from body i*len/n at a spread offset. A short body contributes
// itself whole, so a dictionary of short articles comes out smaller than size.
func sampleDict(bodies []string, size, chunk int) []byte {
	n := size / chunk
	out := make([]byte, 0, size)
	for i := 0; i < n && len(bodies) > 0; i++ {
		b := bodies[i*len(bodies)/n]
		off := 0
		if len(b) > chunk {
			off = (i * 7919) % (len(b) - chunk)
		}
		out = append(out, b[off:min(len(b), off+chunk)]...)
	}
	return out[:min(len(out), size)]
}

// dictSize is 2% of the text, at least 8 KiB, at most dictCap. text is the body
// text held; when the source did not end inside the window (eof false), the
// total is estimated from the source's entry count and the mean article, and a
// source that declares no count gets the cap.
func dictSize(text int64, articles, entries int, eof bool) int {
	total := text
	if !eof {
		if entries <= 0 || articles <= 0 {
			return dictCap
		}
		if est := text / int64(articles) * int64(entries); est > total {
			total = est
		}
	}
	return int(min(max(total/50, 8<<10), int64(dictCap)))
}

// bodyEncodingLabel is the meta `body_encoding`: how the rows were written, for
// people reading the meta. The read path detects the form per row and does not
// consult this - which is why the label names the level as well, and why a
// database that recorded an older one still reads.
const bodyEncodingLabel = "zstd-1"

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
