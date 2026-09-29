// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"unicode/utf8"
)

// The text a Reader reads, a range at a time, so that it never holds a whole
// file. A plain file that needs no R2.1 repair is read where it is; anything
// else - a compressed file, or one with a BOM, CR line ends, invalid UTF-8 or
// NUL - is decoded once into a temporary file, removed on Close. Where no
// temporary file can be made, the decoded text is held in memory instead.
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

// decodeStream copies r to w applying R2.1, a block at a time: a UTF-8
// sequence or a CR LF cut by a block boundary is carried to the next block.
// It reports whether anything was changed, and fails past limit bytes read.
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
			return changed, formatErr("decompressed markdown over %d bytes", limit)
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

// openSource opens path's decoded text (see the file comment).
func openSource(path string) (*source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var in io.Reader = f
	limit := int64(1 << 62) // a plain file is not bounded; this only keeps limit+1 finite
	if compressed(path) {
		zr, err := gzip.NewReader(bufio.NewReader(f)) // multistream: concatenated members read as one
		if err != nil {
			f.Close()
			return nil, formatErr("not a gzip stream: %v", err)
		}
		in, limit = zr, maxMarkdown
	} else {
		// A plain file is used in place when it needs no repair.
		s := &source{}
		sc := newScanner(s)
		changed, err := decodeStream(f, sc, limit)
		if err != nil {
			f.Close()
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
		var b bytes.Buffer
		if _, err := decodeStream(in, io.MultiWriter(&b, sc), limit); err != nil {
			return nil, streamErr(err)
		}
		sc.finish()
		s.ra, s.close = bytes.NewReader(b.Bytes()), func() error { return nil }
		return s, nil
	}
	bw := bufio.NewWriterSize(tmp, 256<<10)
	_, err = decodeStream(in, io.MultiWriter(bw, sc), limit)
	if err == nil {
		err = bw.Flush()
	}
	if err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return nil, streamErr(err)
	}
	sc.finish()
	s.ra = tmp
	s.close = func() error {
		tmp.Close()
		return os.Remove(tmp.Name())
	}
	return s, nil
}

// streamErr reports a decoding failure as E-format; a failure of the file
// system (a full disk) stays what it is.
func streamErr(err error) error {
	var e *Error
	var pe *fs.PathError
	switch {
	case errors.As(err, &e), errors.As(err, &pe):
		return err
	}
	return formatErr("cannot be decompressed: %v", err)
}
