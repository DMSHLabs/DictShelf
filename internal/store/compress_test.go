// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// writeRawTextDB writes the smallest database Open accepts, with each body
// stored exactly as the caller hands it over: the schema (user_version) and
// the bytes of `entry.m` are what these tests are about, so nothing between
// them is allowed to be a store function. dict, when non-empty, is written to
// body_dict id 0 - the shape a zstd row is decoded with.
func writeRawTextDB(t *testing.T, path string, schema int, dict []byte, bodies map[string][]byte) {
	t.Helper()
	db, err := sql.Open(driverName, dsnIngest(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(fmt.Sprintf(`
		PRAGMA user_version = %d;
		CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT);
		CREATE TABLE entry(id INTEGER PRIMARY KEY, w TEXT NOT NULL, wf TEXT NOT NULL, wc TEXT NOT NULL, m TEXT NOT NULL);
		CREATE TABLE browse_anchor(ordinal INTEGER PRIMARY KEY, entry_id INTEGER NOT NULL, w TEXT NOT NULL);
		CREATE TABLE alias(w TEXT NOT NULL, wf TEXT NOT NULL, wc TEXT NOT NULL, entry_id INTEGER NOT NULL REFERENCES entry(id));
	`, schema)); err != nil {
		t.Fatal(err)
	}
	if dict != nil {
		if _, err := db.Exec(`CREATE TABLE body_dict(id INTEGER PRIMARY KEY, data BLOB NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO body_dict(id, data) VALUES(0, ?)`, dict); err != nil {
			t.Fatal(err)
		}
	}
	for _, kv := range [][2]string{
		{"name", "Raw"}, {"format", "mdx"}, {"dict_uuid", "0123456789abcdef"},
		{"ingest_version", fmt.Sprint(IngestVersion)}, {"entry_count", fmt.Sprint(len(bodies))},
	} {
		if _, err := db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)`, kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	words := make([]string, 0, len(bodies))
	for w := range bodies {
		words = append(words, w)
	}
	// map order is random; a stable id order keeps a failure readable
	for i := 0; i < len(words); i++ {
		for j := i + 1; j < len(words); j++ {
			if words[j] < words[i] {
				words[i], words[j] = words[j], words[i]
			}
		}
	}
	for i, w := range words {
		if _, err := db.Exec(`INSERT INTO entry(id, w, wf, wc, m) VALUES(?, ?, ?, ?, ?)`,
			i+1, w, strings.ToLower(w), w, bodies[w]); err != nil {
			t.Fatal(err)
		}
	}
}

// zstdRow builds what a schema-2 database stores for s: a zstd frame without
// its 4-byte magic, written with dict when there is one. It uses the zstd
// library directly, not the store's encoder, so the test pins the FORMAT
// rather than whatever our own code happens to write today.
func zstdRow(t *testing.T, dict []byte, s string) []byte {
	t.Helper()
	opts := []zstd.EOption{
		zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(1)),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(false),
		zstd.WithSingleSegment(true),
	}
	if len(dict) > 0 {
		opts = append(opts, zstd.WithEncoderDictRaw(0, dict))
	}
	z, err := zstd.NewWriter(nil, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	frame := z.EncodeAll([]byte(s), nil)
	if len(frame) < len(zstdMagic) || !bytes.Equal(frame[:len(zstdMagic)], zstdMagic) {
		t.Fatalf("zstd frame does not open with the magic: % x", frame)
	}
	return append([]byte{markZstd}, frame[len(zstdMagic):]...)
}

// bodyOf reads one entry straight out of an opened store, through every layer
// a reader would go through.
func bodyOf(t *testing.T, path, word string) string {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	defer s.Close()
	res, err := s.Exact(word, 1)
	if err != nil {
		t.Fatalf("Exact(%s): %v", word, err)
	}
	if len(res) != 1 {
		t.Fatalf("Exact(%s) returned %d results, want 1", word, len(res))
	}
	return res[0].Body
}

// A database written before zstd existed - schema 1, DEFLATE and literal rows -
// must open and read exactly as it did. This is the whole reason the schema is
// a range and the markers are read per schema.
func TestSchemaOneDatabaseStillOpens(t *testing.T) {
	long := "<p>" + strings.Repeat("texto comprimido de ejemplo ", 20) + "</p>"
	short := "<p>corto</p>" // below the threshold: stored literally
	path := filepath.Join(t.TempDir(), "old.text.db")
	writeRawTextDB(t, path, 1, nil, map[string][]byte{
		"largo": encodeBody(long), // marker 0x00: raw DEFLATE
		"corto": []byte(short),    // no marker at all
	})
	if got := bodyOf(t, path, "largo"); got != long {
		t.Errorf("deflate body from a schema-1 database = %q, want %q", got, long)
	}
	if got := bodyOf(t, path, "corto"); got != short {
		t.Errorf("literal body from a schema-1 database = %q, want %q", got, short)
	}
	// And the same file is not mistaken for the newer schema.
	if p := Inspect(path); !p.Usable() || p.Schema != 1 {
		t.Errorf("schema-1 database: usable=%v schema=%d, want true and 1", p.Usable(), p.Schema)
	}
}

// A schema-1 database is full of literal text whose first byte may be any
// byte. 0x01-0x08 are markers in schema 2 and NOT in schema 1, so those rows
// read back byte for byte.
func TestMarkerBytesAreLiteralInSchemaOne(t *testing.T) {
	for b := byte(0x01); b <= markReserved; b++ {
		want := string([]byte{b}) + "<p>plain text that happens to start with " + fmt.Sprint(b) + "</p>"
		path := filepath.Join(t.TempDir(), "markers.text.db")
		writeRawTextDB(t, path, 1, nil, map[string][]byte{"w": []byte(want)})
		if got := bodyOf(t, path, "w"); got != want {
			t.Fatalf("byte 0x%02x: schema-1 body = %q, want %q", b, got, want)
		}
	}
}

// One database may hold rows of both kinds at once - a rebuild converts body
// by body - and every row must read as the text the source had.
func TestMixedBodiesInOneDatabaseReadTheSame(t *testing.T) {
	long := "<p>" + strings.Repeat("shared markup of an article ", 40) + "</p>"
	other := "<p>" + strings.Repeat("otra entrada con texto repetido ", 30) + "</p>"
	short := "<p>x</p>"
	// A dictionary cut from the bodies themselves, in the shape the writer
	// cuts one: fixed-length slices.
	dict := []byte(long[0:min(len(long), 256)])
	path := filepath.Join(t.TempDir(), "mixed.text.db")
	writeRawTextDB(t, path, 2, dict, map[string][]byte{
		"vieja": encodeBody(long),        // a row the rebuild has not reached
		"nueva": zstdRow(t, dict, other), // a row the rebuild wrote
		"corta": []byte(short),           // short enough to stay literal
	})
	for word, want := range map[string]string{"vieja": long, "nueva": other, "corta": short} {
		if got := bodyOf(t, path, word); got != want {
			t.Errorf("%s = %q, want %q", word, got, want)
		}
	}
}

// 0x02-0x08 are reserved in schema 2: a decoder must not serve them as text,
// and 0x01 without a dictionary must not be served as text either.
func TestReservedMarkersAreNotTextInSchemaTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reserved.text.db")
	writeRawTextDB(t, path, 2, nil, map[string][]byte{
		"rezagada": append([]byte{0x02}, "<p>reserved</p>"...),
	})
	if got := bodyOf(t, path, "rezagada"); got != "" {
		t.Errorf("reserved marker served as text: %q", got)
	}
}

// A zstd row whose frame is damaged degrades to nothing rather than taking the
// query down, exactly as a truncated DEFLATE row degrades to what inflated.
func TestDamagedZstdRowDegrades(t *testing.T) {
	dict := []byte(strings.Repeat("<p>common</p>", 32))
	row := zstdRow(t, dict, "<p>"+strings.Repeat("body ", 50)+"</p>")
	row[len(row)-1] ^= 0xff // a corrupt tail
	path := filepath.Join(t.TempDir(), "damaged.text.db")
	writeRawTextDB(t, path, 2, dict, map[string][]byte{"roto": row})
	if got := bodyOf(t, path, "roto"); got != "" {
		t.Errorf("a damaged zstd row returned %q, want the empty degrade", got)
	}
}

// The schema is a range and nothing outside it is a wudict database: the
// message a user sees must name the range, not one number.
func TestSchemaOutsideTheRangeIsRefused(t *testing.T) {
	for _, schema := range []int{0, 3, 99} {
		path := filepath.Join(t.TempDir(), "foreign.text.db")
		writeRawTextDB(t, path, schema, nil, map[string][]byte{"w": []byte("<p>x</p>")})
		s, err := Open(path)
		if err == nil {
			s.Close()
			t.Fatalf("schema %d opened, want refusal", schema)
		}
		if !strings.Contains(err.Error(), "not a wudict database") ||
			!strings.Contains(err.Error(), "want 1 to 2") {
			t.Errorf("schema %d: error %q does not name the accepted range", schema, err)
		}
	}
}
