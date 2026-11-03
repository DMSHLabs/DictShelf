// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"strconv"
	"strings"
	"testing"
)

// What the two-parse reader of 96e9a334 is worth, as a number rather than a
// claim. These are synthetic dictionaries, not real ones: this machine has no
// .wmd dictionary to measure (docs/WINDOWS-verify.md, "ZIM archives on this
// machine", names the fixtures that do exist, and wmd is not among them), so
// upstream's before/after - a 17 GB OED 4:44 to 35 s - is not reproduced here.
//
// perfDoc builds n entries; refs makes each body reference two definitions by
// label; defs appends that many link reference definitions after the entries;
// blank puts an empty line between them, so each definition is its own
// paragraph. blank matters: goldmark v2.1.5's linkReferenceParagraphTransformer
// is quadratic in the definitions of ONE paragraph (WUDICT-MARKDOWN.md R9.3,
// "40,000 reference definitions in one entry in 0.5 s"), in the reader this
// fork replaced and in this one alike, and a paragraph holding thousands of
// them swamps everything else. The benchmarks below take the blank-line shape
// on purpose, so what they measure is the reader's own quadratic - every parse
// being given every definition - which is the one refLabels removes.
func perfDoc(n int, refs bool, defs int, blank bool) []byte {
	var b strings.Builder
	b.WriteString("# Perf\nwudict: 1\n\n")
	for i := 0; i < n; i++ {
		b.WriteString("## word")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n\nbody of word")
		b.WriteString(strconv.Itoa(i))
		if refs {
			b.WriteString(" references [w")
			b.WriteString(strconv.Itoa(i))
			b.WriteString("] and [w")
			b.WriteString(strconv.Itoa((i + 1) % n))
			b.WriteString("]")
		}
		b.WriteString(" and some more ordinary text to fill it out a little.\n\n")
	}
	for i := 0; i < defs; i++ {
		b.WriteString("[w")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("]: entry://w")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n")
		if blank {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

// benchChunk64k reads text and renders every entry, in 64 KiB chunks - the
// default 1 MiB scaled down so a run finishes in seconds.
func benchChunk64k(b *testing.B, text []byte) {
	saved := chunkSize
	defer func() { chunkSize = saved }()
	chunkSize = 1 << 16
	text = decode(text)
	b.SetBytes(int64(len(text)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := read(text)
		if err != nil {
			b.Fatal(err)
		}
		n := 0
		for {
			if _, err := r.Next(); err != nil {
				break
			}
			n++
		}
		if n != 20000 {
			b.Fatalf("%d entries", n)
		}
		r.Close()
	}
}

// BenchmarkReadPlain: 20 000 entries, no references, no definitions - the
// baseline. Measured 2026-11-04: 51 ms before this port, 22 ms after.
func BenchmarkReadPlain(b *testing.B) { benchChunk64k(b, perfDoc(20000, false, 0, false)) }

// BenchmarkReadUnreferencedDefs: the same entries with 20 000 definitions
// nothing references: what giving every parse every definition costs, in the
// shape where nothing else does. Measured 2026-11-04: 207 ms before, 56 ms
// after.
func BenchmarkReadUnreferencedDefs(b *testing.B) {
	benchChunk64k(b, perfDoc(20000, false, 20000, true))
}

// BenchmarkReadReferencedDefs: the entries reference the definitions, as a
// real dictionary does. Measured 2026-11-04: 319 ms before, 73 ms after.
func BenchmarkReadReferencedDefs(b *testing.B) {
	benchChunk64k(b, perfDoc(20000, true, 20000, true))
}
