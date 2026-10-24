// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
	"github.com/wuweidict/wudict/internal/sysmem"
)

// The text a Reader reads, a range at a time, so that it never holds a whole
// file. A plain file that needs no R2.1 repair is read where it is; anything
// else - a compressed file, or one with a BOM, CR line ends, invalid UTF-8 or
// NUL - is decoded once: into memory while the preparation budget grants it
// (holder), past that into a temporary file, removed on Close. Where no
// temporary file can be made, the decoded text stays in memory, within
// memoryRoom.
//
// One streaming pass over the decoded text also collects what cutting it into
// chunks needs: the start and end of every line that could begin an entry.

// cand is a line that could start an entry: `##` + WS or end of line, at
// column 0. at is its first byte, end the byte after its LF (or the end of the
// text).
type cand struct{ at, end int }

type source struct {
	ra    io.ReaderAt
	size  int
	cands []cand
	head  []byte // lines 1 and 2, for R3.1
	close func() error
	drop  func() // lets go of text held in memory, once nothing reads it; nil for none
}

// release lets go of the text held in memory, if any: after the last body
// has been read nothing reads the text again.
func (s *source) release() {
	if s.drop != nil {
		s.drop()
		s.drop = nil
	}
}

// read returns a copy of text[lo:hi].
func (s *source) read(lo, hi int) ([]byte, error) {
	b := make([]byte, hi-lo)
	if n, err := s.ra.ReadAt(b, int64(lo)); n < len(b) {
		return nil, fmt.Errorf("wudict markdown: reading the text at %d: %w", lo, err)
	}
	return b, nil
}

// headMax bounds what is kept of lines 1-2.
const headMax = 64 << 10

// scanner collects a source's metadata from its decoded text, a block at a
// time. Candidates are found with bytes.Index on "\n##", so the text is read
// at memory speed rather than a byte at a time; a candidate cut by a block
// boundary is carried as the length of the `#` run its line has so far.
type scanner struct {
	off     int  // offset of the next byte
	pending bool // the line starting at lineAt is `#`*hashes so far: it may become a candidate
	lineAt  int
	hashes  int  // 0-2
	open    bool // the last candidate still waits for its line end
	lfs     int  // line ends seen, for head
	s       *source
}

// newScanner starts at the start of a text, which is the start of a line.
func newScanner(s *source) *scanner { return &scanner{s: s, pending: true} }

var hashLine = []byte("\n##")

func isCandWS(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

func (sc *scanner) Write(p []byte) (int, error) {
	base := sc.off
	sc.off += len(p)
	sc.head(p)
	if sc.open {
		k := bytes.IndexByte(p, '\n')
		if k < 0 {
			return len(p), nil
		}
		sc.closeCand(base + k + 1)
	}
	// The line that started before p, if it may still be a candidate.
	if sc.pending {
		j := 0
		for ; sc.hashes < 2 && j < len(p) && p[j] == '#'; j++ {
			sc.hashes++
		}
		switch {
		case j == len(p) && sc.hashes <= 2:
			return len(p), nil // p ended inside the `#` run: still undecided
		case sc.hashes == 2 && isCandWS(p[j]):
			sc.addCand(sc.lineAt, p, base, j)
		}
		sc.pending = false
	}
	for i := 0; ; {
		k := bytes.Index(p[i:], hashLine)
		if k < 0 {
			break
		}
		at := i + k + 1 // the line start, in p
		if at+2 == len(p) {
			sc.pending, sc.lineAt, sc.hashes = true, base+at, 2 // `##` ends p: decided by the next byte
			return len(p), nil
		}
		if !isCandWS(p[at+2]) {
			i = at
			continue
		}
		if i = sc.addCand(base+at, p, base, at+2); i < 0 {
			return len(p), nil // the candidate's line runs past p
		}
	}
	// The last line of p may become a candidate in the next block.
	if k := bytes.LastIndexByte(p, '\n'); k >= 0 {
		if t := p[k+1:]; len(t) < 2 && (len(t) == 0 || t[0] == '#') {
			sc.pending, sc.lineAt, sc.hashes = true, base+k+1, len(t)
		}
	}
	return len(p), nil
}

// addCand records the candidate line starting at at, whose WS is p[ws], and
// returns where in p its line end is, or -1 when the line runs past p.
func (sc *scanner) addCand(at int, p []byte, base, ws int) int {
	sc.s.cands = append(sc.s.cands, cand{at: at})
	sc.open = true
	e := bytes.IndexByte(p[ws:], '\n')
	if e < 0 {
		return -1
	}
	sc.closeCand(base + ws + e + 1)
	return ws + e
}

func (sc *scanner) closeCand(end int) {
	sc.s.cands[len(sc.s.cands)-1].end = end
	sc.open = false
}

// head keeps lines 1-2, at most headMax bytes of them.
func (sc *scanner) head(p []byte) {
	s := sc.s
	for sc.lfs < 2 && len(p) > 0 && len(s.head) < headMax {
		n := len(p)
		k := bytes.IndexByte(p, '\n')
		if k >= 0 {
			n = k + 1
		}
		n = min(n, headMax-len(s.head))
		s.head = append(s.head, p[:n]...)
		if k >= 0 && n == k+1 {
			sc.lfs++
		}
		p = p[n:]
	}
}

// finish closes the scan at the end of the text.
func (sc *scanner) finish() {
	s := sc.s
	if sc.pending && sc.hashes == 2 { // a last line that is exactly `##`
		s.cands = append(s.cands, cand{at: sc.lineAt})
		sc.open = true
	}
	if sc.open {
		s.cands[len(s.cands)-1].end = sc.off
	}
	s.size = sc.off
}

// sourceFromBytes is a source over decoded text in memory.
func sourceFromBytes(text []byte) *source {
	s := &source{ra: bytes.NewReader(text), close: func() error { return nil }}
	sc := newScanner(s)
	sc.Write(text)
	sc.finish()
	return s
}

// errOverLimit: decodeStream read past its limit. The caller says what the
// limit was (overErr, or a *dict.NoRoomError for memory).
var errOverLimit = errors.New("over the limit")

// decodeStream copies r to w applying R2.1, a block at a time: a UTF-8
// sequence or a CR LF cut by a block boundary is carried to the next block.
// It reports whether anything was changed, and fails with errOverLimit past
// limit bytes read.
func decodeStream(r io.Reader, w io.Writer, limit int64) (changed bool, err error) {
	lr := &io.LimitedReader{R: r, N: limit + 1}
	br := bufio.NewReaderSize(lr, 256<<10)
	buf := make([]byte, 0, 256<<10+4)
	first := true
	for {
		// Fill the block; only io.EOF ends the text - a truncated gzip
		// stream reports io.ErrUnexpectedEOF, which is an error.
		var rerr error
		for len(buf) < cap(buf)-4 && rerr == nil {
			var n int
			n, rerr = br.Read(buf[len(buf) : cap(buf)-4])
			buf = buf[:len(buf)+n]
		}
		eof := rerr == io.EOF
		if rerr != nil && !eof {
			return changed, rerr
		}
		if lr.N <= 0 {
			return changed, errOverLimit
		}
		if first {
			if t := trimBOM(buf); len(t) < len(buf) {
				buf, changed = t, true
			}
			first = false
		}
		// What may continue in the next block: an incomplete UTF-8 sequence,
		// or a CR whose LF has not been read yet.
		keep := 0
		if !eof {
			keep = tailCarry(buf)
		}
		out, ch := decodeBlock(buf[:len(buf)-keep])
		changed = changed || ch
		if _, err := w.Write(out); err != nil {
			return changed, err
		}
		carry := append([]byte(nil), buf[len(buf)-keep:]...)
		buf = append(buf[:0], carry...)
		if eof {
			return changed, nil
		}
	}
}

// tailCarry is how many bytes at the end of b may belong with the next block.
func tailCarry(b []byte) int {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return 1
	}
	for k := 1; k <= 3 && k <= len(b); k++ {
		c := b[len(b)-k]
		if c < utf8.RuneSelf {
			return 0
		}
		if utf8.RuneStart(c) {
			if !utf8.FullRune(b[len(b)-k:]) {
				return k
			}
			return 0
		}
	}
	return 0
}

// unpackReserve is the free disk space unpacking leaves alone: the decoded
// text stops before the disk holding the temporary folder falls under it, so
// a file that would fill the disk fails instead of filling it.
const unpackReserve = 1 << 30

// roomStep is how many bytes of decoded text are written between two looks
// at the free space.
const roomStep = 64 << 20

// freeSpace is fsx.FreeSpace; a variable for the tests.
var freeSpace = fsx.FreeSpace

// roomWriter writes the decoded text to its temporary file while the disk
// keeps unpackReserve free: before each roomStep bytes, it looks whether they
// fit above the reserve. Where the free space cannot be read, it writes on,
// and a full disk reports itself.
type roomWriter struct {
	w       io.Writer
	dir     string
	n, next int64 // bytes written; when to look again
}

func (rw *roomWriter) Write(p []byte) (int, error) {
	if rw.n >= rw.next {
		if free, ok := freeSpace(rw.dir); ok && free < unpackReserve+roomStep {
			// The temporary file is removed on this error, so what it holds
			// is free again: Need and Free both count it.
			return 0, &dict.NoRoomError{What: "to unpack it", Dir: rw.dir,
				Need: rw.n + unpackReserve + roomStep, Free: free + rw.n}
		}
		rw.next = rw.n + roomStep
	}
	n, err := rw.w.Write(p)
	rw.n += int64(n)
	return n, err
}

// reportStep is how many bytes of unpacked text pass between two reports of
// how far unpacking has come.
const reportStep = 8 << 20

// unpackReport publishes how far unpacking has come (dict.SetActivity), for
// a caller waiting on an open to show. store's ingest, which reads the text
// it unpacks, clears it.
type unpackReport struct {
	path           string
	n, next, total int64 // bytes so far; when to report again; unpackEstimate
}

func (u *unpackReport) Write(p []byte) (int, error) {
	u.n += int64(len(p))
	if u.n >= u.next {
		if u.total < u.n {
			u.total = 0 // the estimate was wrong: say only what is known
		}
		dict.SetActivity(u.path, dict.Activity{Phase: dict.PhaseUnpacking, Done: u.n, Total: u.total})
		u.next = u.n + reportStep
	}
	return len(p), nil
}

// unpackEstimate is about how many bytes a compressed file of size bytes
// unpacks to, for showing progress and never for a decision (D168). The gzip
// trailer holds the last member's size modulo 4 GiB, and is random bytes in a
// truncated file. No text unpacks to less than its compressed size minus
// gzip's overhead (headers - a dictzip chunk table is up to 128 KiB - and
// deflate's worst case of 5 bytes per 16 KiB), so a trailer under that has
// wrapped and is lifted by 4 GiB until it is not: the least size the trailer
// allows. It can still be short by multiples of 4 GiB, which the caller sees
// when the text passes it. 0 when nothing plausible is known: no trailer, or
// an estimate past limit, which the unpacking will not reach.
func unpackEstimate(f io.ReaderAt, size int64, limit int64) int64 {
	if size < 18 { // a gzip header and trailer alone
		return 0
	}
	var b [4]byte
	if _, err := f.ReadAt(b[:], size-4); err != nil {
		return 0
	}
	est := int64(binary.LittleEndian.Uint32(b[:]))
	if least := size - 1<<20 - size/1000; est > 0 && est < least {
		est += (least - est + 1<<32 - 1) >> 32 << 32
	}
	if est <= 0 || est > limit {
		return 0
	}
	return est
}

// memoryRoom is how much decoded text may be held in memory, where no
// temporary file can be made: half the configured Go memory limit where one
// is set (Android) - never a ceiling raised in an emergency - else
// maxMarkdown.
func memoryRoom() int64 {
	if l := sysmem.ConfiguredLimit(); l > 0 {
		return l / 2
	}
	return maxMarkdown
}

// openSource opens path's decoded text (see the file comment).
func openSource(path string) (*source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var in io.Reader = f
	var report io.Writer = io.Discard
	limit, packed := int64(textMax), int64(0)
	if compressed(path) {
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		zr, err := gzip.NewReader(bufio.NewReader(f)) // multistream: concatenated members read as one
		if err != nil {
			f.Close()
			return nil, formatErr("not a gzip stream: %v", err)
		}
		packed = max(fi.Size(), 1) // a plain file is the one with packed == 0
		in, limit = zr, unpackBound(fi.Size())
		report = &unpackReport{path: path, total: unpackEstimate(f, fi.Size(), limit)}
	} else {
		// A plain file is used in place when it needs no repair.
		s := &source{}
		sc := newScanner(s)
		changed, err := decodeStream(f, sc, limit)
		if err != nil {
			f.Close()
			if errors.Is(err, errOverLimit) {
				return nil, overErr(limit, 0)
			}
			return nil, err
		}
		if !changed {
			sc.finish()
			s.ra, s.close = f, f.Close
			return s, nil
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	defer f.Close()

	s := &source{}
	sc := newScanner(s)
	h := &holder{charge: textRoom()}
	_, err = decodeStream(in, io.MultiWriter(h, sc, report), limit)
	if err == nil {
		err = h.flush()
	}
	if err != nil {
		h.close()
		return nil, streamErr(err, limit, packed)
	}
	sc.finish()
	s.ra, s.close = h, h.close
	if h.file == nil {
		s.drop = h.drop
	}
	return s, nil
}

// textBlock is the size of the blocks decoded text is held in.
const textBlock = 4 << 20

// textRoom is what decoded text held in memory may draw on the preparation
// budget, at twice its size: a quarter of the budget, so a preparation keeps
// room to parse in parallel (D182, 7A). A variable so the tests can send
// every text to a temporary file.
var textRoom = func() int64 { return sysmem.Budget() / 4 }

// holder keeps decoded text in memory, in blocks of up to textBlock - every
// block but the last full, the last grown by doubling from 64 KiB, so a small
// text holds little - while the budget grants each byte of capacity twice
// over (the collector's headroom) within charge; past that it moves the text
// to a temporary file and writes the rest there. Where no temporary file can
// be made, the text stays in memory, within memoryRoom.
type holder struct {
	acct   sysmem.Account
	charge int64 // what the blocks may draw
	blocks [][]byte
	size   int64
	file   *os.File
	w      *bufio.Writer
	stuck  bool // no temporary file could be made
}

// firstBlock is the capacity the first block starts at.
const firstBlock = 64 << 10

func (h *holder) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if h.file != nil {
			k, err := h.w.Write(p)
			return n - len(p) + k, err
		}
		if len(h.blocks) == 0 || len(h.blocks[len(h.blocks)-1]) == textBlock {
			if !h.grow(nil, firstBlock) {
				if err := h.spill(); err != nil {
					return n - len(p), err
				}
				continue
			}
		}
		last := &h.blocks[len(h.blocks)-1]
		if len(*last) == cap(*last) && !h.grow(last, min(2*cap(*last), textBlock)) {
			if err := h.spill(); err != nil {
				return n - len(p), err
			}
			continue
		}
		k := min(len(p), cap(*last)-len(*last))
		*last = append(*last, p[:k]...)
		h.size += int64(k)
		p = p[k:]
	}
	return n, nil
}

// grow gives block b (a new block when nil) capacity c, drawing twice the
// capacity added on the budget; false when it does not fit, or when no
// temporary file can be made and the text is past memoryRoom.
func (h *holder) grow(b *[]byte, c int) bool {
	old := 0
	if b != nil {
		old = cap(*b)
	}
	add := int64(c - old)
	if h.stuck {
		if room := memoryRoom(); h.size+add > room {
			return false
		}
	} else if 2*(h.acct.Held()/2+add) > h.charge || !h.acct.TryAcquire(2*add) {
		return false
	}
	nb := make([]byte, 0, c)
	if b == nil {
		h.blocks = append(h.blocks, nb)
		return true
	}
	*b = append(nb, *b...)
	return true
}

// spill moves the text held to a temporary file, which the rest is then
// written to; without one the text stays in memory.
func (h *holder) spill() error {
	if h.stuck {
		room := memoryRoom()
		return &dict.NoRoomError{What: "to unpack it", Need: room, Free: room}
	}
	tmp, err := os.CreateTemp("", "wudict-md-*.txt")
	if err != nil {
		h.stuck = true
		return nil
	}
	h.file = tmp
	h.w = bufio.NewWriterSize(&roomWriter{w: tmp, dir: filepath.Dir(tmp.Name())}, 256<<10)
	for _, b := range h.blocks {
		if _, err := h.w.Write(b); err != nil {
			return err
		}
	}
	h.drop()
	return nil
}

func (h *holder) flush() error {
	if h.w != nil {
		return h.w.Flush()
	}
	return nil
}

func (h *holder) ReadAt(p []byte, off int64) (int, error) {
	if h.file != nil {
		return h.file.ReadAt(p, off)
	}
	n := 0
	for n < len(p) {
		bi, bo := int(off/textBlock), int(off%textBlock)
		if bi >= len(h.blocks) || bo >= len(h.blocks[bi]) {
			return n, io.EOF
		}
		k := copy(p[n:], h.blocks[bi][bo:])
		n += k
		off += int64(k)
	}
	return n, nil
}

// drop lets go of the text held in memory and of its draw on the budget.
func (h *holder) drop() {
	h.blocks = nil
	h.acct.Close()
}

func (h *holder) close() error {
	h.drop()
	if h.file == nil {
		return nil
	}
	h.file.Close()
	return os.Remove(h.file.Name())
}

// streamErr reports a decoding failure as E-format - past limit, the one
// overErr names; a failure of the file system (a full disk) or of the room to
// unpack in stays what it is.
func streamErr(err error, limit, packed int64) error {
	var e *Error
	var pe *fs.PathError
	var re *dict.NoRoomError
	switch {
	case errors.Is(err, errOverLimit):
		return overErr(limit, packed)
	case errors.As(err, &e), errors.As(err, &pe), errors.As(err, &re):
		return err
	}
	return formatErr("cannot be decompressed: %v", err)
}
