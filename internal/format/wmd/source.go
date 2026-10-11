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
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"unicode/utf8"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/fsx"
)

// The text a Reader reads, a range at a time, so that it never holds a whole
// file. A plain file that needs no R2.1 repair is read where it is; anything
// else - a compressed file, or one with a BOM, CR line ends, invalid UTF-8 or
// NUL - is decoded once into a temporary file, removed on Close. Where no
// temporary file can be made, the decoded text is held in memory instead,
// within memoryRoom.
//
// One streaming pass over the decoded text also collects what cutting it into
// chunks needs: the start and end of every line that could begin an entry, and
// whether a reference definition can occur at all.

// cand is a line that could start an entry: `##` + WS or end of line, at
// column 0. at is its first byte, end the byte after its LF (or the end of the
// text).
type cand struct{ at, end int }

type source struct {
	ra    io.ReaderAt
	size  int
	cands []cand
	defs  bool   // the text holds "]:": a reference definition is possible
	head  []byte // lines 1 and 2, for R3.1
	close func() error
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
// time.
type scanner struct {
	off    int  // offset of the next byte
	col    int  // column of the next byte, counted to 3
	hashes int  // `#` at the start of the current line, or -1
	lineAt int  // offset of the current line's start
	open   bool // the last candidate still waits for its line end
	prev   byte
	lfs    int // line ends seen, for head
	s      *source
}

func newScanner(s *source) *scanner { return &scanner{s: s} }

func (sc *scanner) Write(p []byte) (int, error) {
	s := sc.s
	for _, c := range p {
		if sc.lfs < 2 && len(s.head) < headMax {
			s.head = append(s.head, c)
		}
		if sc.prev == ']' && c == ':' {
			s.defs = true
		}
		switch sc.col {
		case 0:
			sc.lineAt = sc.off
			if c == '#' {
				sc.hashes = 1
			} else {
				sc.hashes = -1
			}
		case 1:
			if sc.hashes == 1 && c == '#' {
				sc.hashes = 2
			} else {
				sc.hashes = -1
			}
		case 2:
			if sc.hashes == 2 && (c == ' ' || c == '\t' || c == '\n') {
				s.cands = append(s.cands, cand{at: sc.lineAt})
				sc.open = true
			}
			sc.hashes = -1
		}
		sc.off++
		if c == '\n' {
			if sc.open {
				s.cands[len(s.cands)-1].end = sc.off
				sc.open = false
			}
			sc.lfs++
			sc.col = 0
		} else if sc.col < 3 {
			sc.col++
		}
		sc.prev = c
	}
	return len(p), nil
}

// finish closes the scan at the end of the text.
func (sc *scanner) finish() {
	s := sc.s
	if sc.col == 2 && sc.hashes == 2 { // a last line that is exactly `##`
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
// temporary file can be made: half the Go memory limit where one is set
// (Android), else maxMarkdown.
func memoryRoom() int64 {
	if l := debug.SetMemoryLimit(-1); l < math.MaxInt64 {
		return l / 2
	}
	return maxMarkdown
}

// openSource opens path's decoded text (see the file comment).
func openSource(path string) (*source, error) {
	defer dict.ClearActivity(path)
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
	tmp, terr := os.CreateTemp("", "wudict-md-*.txt")
	if terr != nil {
		// No temporary file: the decoded text is held in memory.
		room := memoryRoom()
		var b bytes.Buffer
		if _, err := decodeStream(in, io.MultiWriter(&b, sc, report), min(limit, room)); err != nil {
			if errors.Is(err, errOverLimit) && room < limit {
				return nil, &dict.NoRoomError{What: "to unpack it", Need: room, Free: room}
			}
			return nil, streamErr(err, limit, packed)
		}
		sc.finish()
		s.ra, s.close = bytes.NewReader(b.Bytes()), func() error { return nil }
		return s, nil
	}
	bw := bufio.NewWriterSize(&roomWriter{w: tmp, dir: filepath.Dir(tmp.Name())}, 256<<10)
	_, err = decodeStream(in, io.MultiWriter(bw, sc, report), limit)
	if err == nil {
		err = bw.Flush()
	}
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, streamErr(err, limit, packed)
	}
	sc.finish()
	s.ra = tmp
	s.close = func() error {
		tmp.Close()
		return os.Remove(tmp.Name())
	}
	return s, nil
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
