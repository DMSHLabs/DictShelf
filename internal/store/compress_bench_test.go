// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"os"
	"strconv"
	"testing"
)

// BenchmarkArticleRead measures the READ side of compress.go: opening articles
// from a prepared database that is already on disk. It is a MEASUREMENT TOOL,
// not a test - it asserts nothing about time and is gated on a path, so an
// ordinary `go test` skips it:
//
//	WUDICT_BENCH_TEXTDB=<prepared text.db> go test -tags sqlite_fts5,sqlite_dbstat \
//	    -run '^$' -bench BenchmarkArticleRead -benchtime 2000x ./internal/store/
//
// The headwords come from the database itself - every WUDICT_BENCH_STRIDE-th
// entry, which is 97 by default - so the same source gives the same words on
// both sides of a codec change, and the two runs read the same articles. The
// reported bodyB/op is the decoded HTML per article, and MB/s is the rate at
// which that text is produced; both make the comparison a property of the
// codec rather than of the word list.
func BenchmarkArticleRead(b *testing.B) {
	path := os.Getenv("WUDICT_BENCH_TEXTDB")
	if path == "" {
		b.Skip("set WUDICT_BENCH_TEXTDB=<prepared text.db> to run this measurement")
	}
	s, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	stride := 97
	if n, err := strconv.Atoi(os.Getenv("WUDICT_BENCH_STRIDE")); err == nil && n > 0 {
		stride = n
	}
	rows, err := s.db.Query("SELECT w FROM entry WHERE id % ? = 0 ORDER BY id LIMIT 2000", stride)
	if err != nil {
		b.Fatal(err)
	}
	var words []string
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			rows.Close()
			b.Fatal(err)
		}
		words = append(words, w)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		b.Fatal(err)
	}
	if len(words) == 0 {
		b.Fatal("the database has no headwords to read")
	}

	var bodyBytes int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := s.Exact(words[i%len(words)], 1)
		if err != nil {
			b.Fatal(err)
		}
		if len(res) != 1 {
			b.Fatalf("%q: %d results, want 1", words[i%len(words)], len(res))
		}
		bodyBytes += int64(len(res[0].Body))
	}
	b.StopTimer()
	b.ReportMetric(float64(bodyBytes)/b.Elapsed().Seconds()/1e6, "MB/s")
	b.ReportMetric(float64(bodyBytes)/float64(b.N), "bodyB/op")
}

// BenchmarkBodyDecode is BenchmarkArticleRead's codec half: the same rows are
// fetched once (untimed), and the loop decodes only, so the number is what the
// stored form costs and not what SQLite costs. It is what answers "zstd
// decompresses about four times faster" without the row fetch hiding it.
//
//	WUDICT_BENCH_TEXTDB=<prepared text.db> go test -tags sqlite_fts5,sqlite_dbstat \
//	    -run '^$' -bench BenchmarkBodyDecode -benchtime 2000x ./internal/store/
func BenchmarkBodyDecode(b *testing.B) {
	path := os.Getenv("WUDICT_BENCH_TEXTDB")
	if path == "" {
		b.Skip("set WUDICT_BENCH_TEXTDB=<prepared text.db> to run this measurement")
	}
	s, err := Open(path)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()

	stride := 97
	if n, err := strconv.Atoi(os.Getenv("WUDICT_BENCH_STRIDE")); err == nil && n > 0 {
		stride = n
	}
	rows, err := s.db.Query("SELECT m FROM entry WHERE id % ? = 0 ORDER BY id LIMIT 2000", stride)
	if err != nil {
		b.Fatal(err)
	}
	var raw [][]byte
	for rows.Next() {
		var m []byte
		if err := rows.Scan(&m); err != nil {
			rows.Close()
			b.Fatal(err)
		}
		raw = append(raw, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		b.Fatal(err)
	}
	if len(raw) == 0 {
		b.Fatal("the database has no bodies to decode")
	}

	var bodyBytes int64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bodyBytes += int64(len(s.body.decode(raw[i%len(raw)])))
	}
	b.StopTimer()
	b.ReportMetric(float64(bodyBytes)/b.Elapsed().Seconds()/1e6, "MB/s")
	b.ReportMetric(float64(bodyBytes)/float64(b.N), "bodyB/op")
}
