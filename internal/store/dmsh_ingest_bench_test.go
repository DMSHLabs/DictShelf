// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// TestIngestSplitThroughput is a MEASUREMENT TOOL, not a test: it builds one
// synthetic read-through database - the form a ZIM archive gets, bodies dropped
// and the article text indexed, which is the build whose index is a file of its
// own - and prints the phase timings, so a change to the write path can be A/B'd
// on one machine without touching a real library.
//
// It is gated twice over: it does nothing unless WUDICT_BENCH_ARTICLES is set,
// and it never asserts anything about time. Run it with the same environment on
// both sides of the change:
//
//	WUDICT_BENCH_ARTICLES=100000 go test -tags sqlite_fts5,sqlite_dbstat -run TestIngestSplitThroughput -v ./internal/store/
func TestIngestSplitThroughput(t *testing.T) {
	articles, err := strconv.Atoi(os.Getenv("WUDICT_BENCH_ARTICLES"))
	if err != nil || articles <= 0 {
		t.Skip("set WUDICT_BENCH_ARTICLES=<entries> to run this measurement")
	}
	textBytes := 1500
	if n, err := strconv.Atoi(os.Getenv("WUDICT_BENCH_TEXT")); err == nil && n > 0 {
		textBytes = n
	}
	contains := os.Getenv("WUDICT_BENCH_CONTAINS") != "0"
	out := filepath.Join(t.TempDir(), TextDBName)
	r := &benchReader{n: articles, textBytes: textBytes}

	ingestParallelismDiagnostics.Store(true)
	start := time.Now()
	if _, err := IngestPlan(r, out, Plan{FullText: true, Contains: contains}, nil); err != nil {
		t.Fatal(err)
	}
	total := time.Since(start)

	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	side, _ := ArticlesSibling(out)
	var sideSize int64
	if side != "" {
		if fi, err := os.Stat(side); err == nil {
			sideSize = fi.Size()
		}
	}
	t.Logf("BENCH total=%s articles=%d text.db=%d article_index=%d", total.Round(time.Millisecond), articles, st.Size(), sideSize)
	// Only this run's own lines: the system log is shared with whatever else
	// this machine has been doing, and its older sessions are not our business.
	log, _ := IndexLog()
	job := ""
	for _, line := range strings.Split(string(log), "\n") {
		if at := strings.Index(line, "job="); at >= 0 {
			rest := line[at:]
			if strings.Contains(rest, "start source="+"\"bench\"") {
				job = strings.Fields(rest)[0]
			}
		}
	}
	for _, line := range strings.Split(string(log), "\n") {
		if job != "" && strings.Contains(line, job) {
			t.Logf("BENCH   %s", line[strings.Index(line, "job="):])
		}
	}
}

// benchReader yields n entries whose body is about textBytes of word-like text,
// and drops the bodies the way the ZIM index reader does - so the database keeps
// the article index and no bodies, which is the shape under measurement.
type benchReader struct {
	n         int
	textBytes int
	i         int
	meta      dict.Meta
}

var benchWords = []string{
	"слово", "значение", "пример", "текст", "статья", "определение", "перевод", "форма",
	"употребление", "контекст", "источник", "ссылка", "понятие", "термин", "раздел", "список",
}

func (b *benchReader) Meta() dict.Meta {
	return dict.Meta{Name: "bench", Format: "bench", Path: "bench"}
}

func (b *benchReader) Close() error { return nil }

func (b *benchReader) DropBodies() bool { return true }

func (b *benchReader) Next() (dict.Entry, error) {
	if b.i >= b.n {
		return dict.Entry{}, io.EOF
	}
	var sb strings.Builder
	sb.Grow(b.textBytes + 64)
	fmt.Fprintf(&sb, "<p>%d.</b> ", b.i)
	for sb.Len() < b.textBytes {
		fmt.Fprintf(&sb, "%s%d %s ", benchWords[sb.Len()%len(benchWords)], b.i%97, benchWords[(sb.Len()*7)%len(benchWords)])
	}
	e := dict.Entry{
		Headwords: []string{fmt.Sprintf("word%07d", b.i)},
		Body:      sb.String(),
		Kind:      dict.BodyHTML,
	}
	b.i++
	return e, nil
}
