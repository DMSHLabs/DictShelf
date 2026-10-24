// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/util"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/sysmem"
)

// readConfig is how a text is read: the sizes of the phases' pieces, the
// workers, the lane, and whether every candidate line is a cut chosen ahead.
type readConfig struct {
	chunk, unit, workers int
	urgent               bool
	every                bool
	toggle               bool // the lane flips at every unit
}

func (c readConfig) String() string {
	return fmt.Sprintf("chunk %d unit %d workers %d urgent %v every %v toggle %v", c.chunk, c.unit, c.workers, c.urgent, c.every, c.toggle)
}

type readResult struct {
	meta    dict.Meta
	entries []dict.Entry
	warns   []string
	err     error
}

func readWith(text []byte, c readConfig) readResult {
	savedChunk, savedUnit, savedWorkers, savedEvery, savedLane := chunkSize, unitSize, workersMax, cutEvery, urgentNow
	defer func() {
		chunkSize, unitSize, workersMax, cutEvery, urgentNow = savedChunk, savedUnit, savedWorkers, savedEvery, savedLane
	}()
	chunkSize, unitSize, workersMax, cutEvery = c.chunk, c.unit, c.workers, c.every
	urgent := c.urgent
	if c.toggle {
		var n atomic.Int64
		urgentNow = func(*Reader) bool { return n.Add(1)%2 == 0 }
	} else {
		urgentNow = func(*Reader) bool { return urgent }
	}
	r, err := readSource(sourceFromBytes(decode(text)), "")
	if err != nil {
		return readResult{err: err}
	}
	var out readResult
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			out.err = err
			break
		}
		out.entries = append(out.entries, e)
	}
	out.meta = r.Meta()
	out.warns, _ = r.Warnings()
	return out
}

var parallelConfigs = []readConfig{
	{chunk: 1, unit: 1, workers: 3, urgent: true},
	{chunk: 1, unit: 1, workers: 8, urgent: true, every: true},
	{chunk: 64, unit: 64, workers: 3, urgent: true},
	{chunk: 64, unit: 1, workers: 1, urgent: true, every: true},
	{chunk: 1, unit: 64, workers: 3, urgent: false, every: true},
	{chunk: 1, unit: 1, workers: 3, urgent: true, toggle: true},
	{chunk: 1 << 30, unit: 1, workers: 8, urgent: true, toggle: true},
}

func sameRead(a, b readResult) bool {
	return (a.err == nil) == (b.err == nil) && reflect.DeepEqual(a.meta, b.meta) &&
		reflect.DeepEqual(a.entries, b.entries) && reflect.DeepEqual(a.warns, b.warns)
}

// TestParallelIsInvisible: reading in parallel, in the smallest pieces, with
// every cut chosen ahead rejected and repaired, on a lane that flips at every
// unit, gives exactly what reading the whole file at once on one core gives.
func TestParallelIsInvisible(t *testing.T) {
	docs := map[string]string{}
	for k, v := range chunkDocs {
		docs[k] = v
	}
	for _, mode := range []string{"clean", "html"} {
		b, err := os.ReadFile("../../../docs/wudict-markdown/examples/" + mode + Ext)
		if err != nil {
			t.Fatal(err)
		}
		docs["spec example "+mode] = string(b)
	}
	delete(docs, "spec examples")
	for name, text := range docs {
		t.Run(name, func(t *testing.T) {
			want := readWith([]byte(text), readConfig{chunk: 1 << 30, unit: 1 << 30, workers: 1})
			if want.err != nil {
				t.Fatal(want.err)
			}
			for _, c := range parallelConfigs {
				if got := readWith([]byte(text), c); !sameRead(want, got) {
					t.Errorf("%v reads differently:\nwant %+v\n     %+v\n     %q\ngot  %+v %v\n     %+v\n     %q", c,
						want.meta, want.entries, want.warns, got.meta, got.err, got.entries, got.warns)
				}
			}
		})
	}
}

// FuzzParallel: no input reads differently in parallel pieces than whole on
// one core, or panics the reader.
func FuzzParallel(f *testing.F) {
	for _, s := range chunkDocs {
		if s != "" {
			f.Add([]byte(s))
		}
	}
	f.Add([]byte("# D\nwudict: 1\n\n## a\n\n```\n## b\n```\n## c\n## d\n\n[x]: y\n\nx [x]\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		want := readWith(data, readConfig{chunk: 1 << 30, unit: 1 << 30, workers: 1})
		for _, c := range parallelConfigs {
			if got := readWith(data, c); !sameRead(want, got) {
				t.Fatalf("%v reads differently:\nwant %v %+v %+v\ngot  %v %+v %+v", c, want.err, want.meta, want.entries, got.err, got.meta, got.entries)
			}
		}
	})
}

// topLevel is the shape of a parse's top-level blocks.
func topLevel(doc ast.Node) []string {
	var out []string
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		s := fmt.Sprintf("%v@%d", n.Kind(), n.Pos())
		if h, ok := n.(*ast.Heading); ok {
			s += fmt.Sprintf("h%d/%d", h.Level, h.HeadingKind)
		}
		if bl, ok := n.(ast.BlockNode); ok {
			s += fmt.Sprintf("x%d", len(bl.Source()))
		}
		out = append(out, s)
	}
	return out
}

// TestBlockParserShape: the parser without inline parsers builds the full
// parser's top-level blocks.
func TestBlockParserShape(t *testing.T) {
	for name, text := range chunkDocs {
		src := decode([]byte(text))
		if got, want := topLevel(blockParser.Parse(src)), topLevel(mdParser.Parse(src)); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: block parser %v\nfull parser %v", name, got, want)
		}
	}
}

// TestHeadingText: a heading's text read the way phase 1 reads it - its
// content when it holds no inline syntax, else its line again, alone - is
// what the full parse of the file gives it in place.
func TestHeadingText(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	const alphabet = "ab  \t#*_`[]!<>&\\é;x:y(" // inline syntax, closing sequences, WS
	var lines []string
	for range 4000 {
		b := make([]byte, 1+rnd.Intn(12))
		for i := range b {
			b[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		lines = append(lines, strings.Repeat(" ", rnd.Intn(4))+"## "+string(b))
	}
	doc := "# D\nwudict: 1\n\n" + strings.Join(lines, "\n\nbody\n\n") + "\n\nbody\n\n[x]: entry://x\n"
	src := []byte(doc)
	full := mdParser.Parse(src)
	var want []string
	for n := full.FirstChild(); n != nil; n = n.NextSibling() {
		if isH2(n) {
			want = append(want, headingText(n, src))
		}
	}
	r := &Reader{src: sourceFromBytes(src), defs: nil}
	c, err := r.scanChunk(0, -1)
	if err != nil || !c.ok {
		t.Fatal(err)
	}
	r.defs = map[string]parser.LinkDefinition{}
	for _, d := range c.defs {
		r.defs[util.ToLinkReference(d.Label())] = d
	}
	if err := r.readHeadings([]pchunk{c}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range c.groups {
		for _, h := range g.heads {
			got = append(got, h.text)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%d headings, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("heading %q: text %q, want %q", lines[i], got[i], want[i])
		}
	}
}

// TestWrongStartFaults: a chunk read from a start inside a block - a cut chosen
// ahead that the parse then rejects - may fail (the parser panics on some
// text read from the middle of a block); the repair reads the file from a
// right start, and what the wrong one did is not the file's.
func TestWrongStartFaults(t *testing.T) {
	doc := "# D\nwudict: 1\n\n## a\n\n```\n## inside one\n## inside two\n```\n\n## b\n\nbody\n"
	want := readWith([]byte(doc), readConfig{chunk: 1 << 30, unit: 1 << 30, workers: 1})
	if want.err != nil {
		t.Fatal(want.err)
	}
	inside := strings.Index(doc, "## inside one")
	saved := scanFault
	defer func() { scanFault = saved }()
	scanFault = func(start int) error {
		if start == 0 {
			// The validator reads the first chunk; while it waits here the
			// workers read the chunks ahead, those from wrong starts included.
			time.Sleep(100 * time.Millisecond)
		}
		if start >= inside && start < strings.Index(doc, "```\n\n## b") {
			return errors.New("parse from a wrong start")
		}
		return nil
	}
	for _, c := range []readConfig{
		{chunk: 1, unit: 1, workers: 3, urgent: true, every: true},
		{chunk: 1, unit: 1, workers: 1, urgent: false, every: true},
		{chunk: 1, unit: 1, workers: 3, urgent: true, every: true, toggle: true},
	} {
		if got := readWith([]byte(doc), c); !sameRead(want, got) {
			t.Errorf("%v: %v %+v, want %+v", c, got.err, got.entries, want.entries)
		}
	}
}

// fillPool draws everything the preparation budget has left, as other
// preparations would, and returns what gives it back.
func fillPool() func() {
	var a sysmem.Account
	for n := sysmem.Budget(); n > 0; n /= 2 {
		for a.TryAcquire(n) {
		}
	}
	return a.Close
}

// TestReadNoRoom: with no budget left, both phases read on one core, the
// same as ever.
func TestReadNoRoom(t *testing.T) {
	text := []byte(chunkDocs["blocks at cuts"])
	want := readWith(text, readConfig{chunk: 1 << 30, unit: 1 << 30, workers: 1})
	release := fillPool()
	defer release()
	for _, c := range parallelConfigs {
		if got := readWith(text, c); !sameRead(want, got) {
			t.Errorf("%v with no room reads differently: %v", c, got.err)
		}
	}
}
