// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"compress/flate"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/sysmem"
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

// deflateRow builds what a schema-1 database - and a schema-2 one a rebuild
// has not reached - stores for s: the marker and a raw DEFLATE stream. Like
// zstdRow, it is built with the standard library so the test pins the format.
func deflateRow(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteByte(markDeflate)
	w, err := flate.NewWriter(&buf, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
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
		"largo": deflateRow(t, long), // marker 0x00: raw DEFLATE
		"corto": []byte(short),       // no marker at all
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
		"vieja": deflateRow(t, long),     // a row the rebuild has not reached
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

// The dictionary is 2% of the text, inside its floor and its cap; a source
// that did not end inside the window is estimated from its declared count.
func TestDictSizeIsTwoPercentWithinItsBounds(t *testing.T) {
	cases := []struct {
		text              int64
		articles, entries int
		eof               bool
		want              int
	}{
		{16 << 20, 100, 100, true, dictCap},   // 2% is above the cap
		{1 << 20, 10, 10, true, 1 << 20 / 50}, // 2% between the bounds
		{1000, 10, 10, true, 8 << 10},         // below the floor
		// Estimated from the count, and the mean article is integer division:
		// 1 MiB / 10 is 104857 bytes, so 100 of them are 10 485 700, not
		// 10 485 760.
		{1 << 20, 10, 100, false, 104857 * 100 / 50},
		{1 << 20, 0, 0, false, dictCap},      // nothing declared: the cap
		{4096, 4, 4, true, 8 << 10},          // a small database still gets the floor
		{16 << 20, 100, 100, false, dictCap}, // a filled window and the cap agree
		// …and an estimate far past the cap is still cut down to it.
		{10 << 20, 100, 1000, false, dictCap},
	}
	for _, c := range cases {
		got := dictSize(c.text, c.articles, c.entries, c.eof)
		if got != c.want {
			t.Errorf("dictSize(%d, %d, %d, %v) = %d, want %d", c.text, c.articles, c.entries, c.eof, got, c.want)
		}
	}
}

// The dictionary is n slices of chunk bytes, spread over the bodies, and never
// longer than the size asked for.
func TestSampleDictSpreadsEvenSlices(t *testing.T) {
	bodies := make([]string, 8)
	for i := range bodies {
		bodies[i] = strings.Repeat(string(rune('a'+i)), 4096)
	}
	dict := sampleDict(bodies, 4096, dictSliceLen)
	if len(dict) != 4096 {
		t.Fatalf("dictionary is %d bytes, want 4096", len(dict))
	}
	// Four slices, and each comes from a different body: that is what makes a
	// sample of the corpus rather than of the first article.
	seen := map[byte]bool{}
	for i := 0; i < 4; i++ {
		seen[dict[i*dictSliceLen]] = true
	}
	if len(seen) != 4 {
		t.Errorf("the slices came from %d bodies, want 4", len(seen))
	}
	// A short body contributes itself whole, so a dictionary of short articles
	// is shorter than the size asked for - it is never padded up to it.
	short := []string{"<p>a</p>", "<p>b</p>"}
	if got := sampleDict(short, 8192, dictSliceLen); len(got) > 8192 {
		t.Errorf("a dictionary of short bodies is %d bytes, larger than the size asked for", len(got))
	}
	if got := sampleDict(nil, 8192, dictSliceLen); len(got) != 0 {
		t.Errorf("a dictionary of no bodies is %d bytes, want 0", len(got))
	}
}

// A row the encoder writes is the format the decoder reads: zstd, no magic in
// the row, the dictionary in body_dict - and it is actually smaller.
func TestZstdRowIsAFrameWithoutItsMagic(t *testing.T) {
	dict := []byte(strings.Repeat("<p>shared markup</p>", 32))
	codec := newBodyCodec()
	if err := codec.start(dict, &sysmem.Account{}); err != nil {
		t.Fatal(err)
	}
	defer codec.close()
	body := "<p>" + strings.Repeat("artículo de ejemplo ", 40) + "</p>"
	stored := codec.enc.Load().encode(body)
	if stored[0] != markZstd {
		t.Fatalf("stored row starts with 0x%02x, want the zstd marker", stored[0])
	}
	if bytes.HasPrefix(stored[1:], zstdMagic) {
		t.Error("the row carries the zstd magic; it must not")
	}
	if len(stored) >= len(body) {
		t.Errorf("a repetitive body was not compressed: %d → %d", len(body), len(stored))
	}
}

// An ingest writes the newest schema, its dictionary, and the meta that says
// so - and every article still reads back as the source had it.
func TestIngestWritesSchemaTwoAndItsDictionary(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	body := func(w string) string {
		return "<p>" + strings.Repeat(w+" definición de ejemplo con texto repetido. ", 12) + "</p>"
	}
	var entries []dict.Entry
	for i := 0; i < 40; i++ {
		w := fmt.Sprintf("palabra%02d", i)
		entries = append(entries, h(w, body(w)))
	}
	path := filepath.Join(t.TempDir(), TextDBName)
	r := &fakeReader{meta: dict.Meta{Name: "Zstd", Format: "mdx", Path: "/x.mdx"}, entries: entries}
	if _, err := IngestPlan(r, path, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	m, schema, err := ReadMetaSchema(path)
	if err != nil {
		t.Fatal(err)
	}
	if schema != textSchemaMax {
		t.Errorf("ingest wrote schema %d, want %d", schema, textSchemaMax)
	}
	if m["body_encoding"] != bodyEncodingLabel {
		t.Errorf("body_encoding = %q, want %q", m["body_encoding"], bodyEncodingLabel)
	}
	if m["body_dict_bytes"] == "" || m["body_dict_bytes"] == "0" {
		t.Errorf("body_dict_bytes = %q, want a dictionary", m["body_dict_bytes"])
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var dictLen int
	if err := s.db.QueryRow("SELECT length(data) FROM body_dict WHERE id = 0").Scan(&dictLen); err != nil {
		t.Fatalf("no dictionary row: %v", err)
	}
	if dictLen == 0 {
		t.Error("the dictionary row is empty")
	}
	for i := 0; i < 40; i++ {
		w := fmt.Sprintf("palabra%02d", i)
		res, err := s.Exact(w, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Body != body(w) {
			t.Errorf("%s: body did not survive the zstd round trip", w)
		}
	}
}

// The window cuts where the text reaches it, not at a batch boundary: with the
// window forced small, a later entry must still encode against the SAME
// dictionary - one cut, one body_dict row, and every body readable.
func TestTheWindowCutsOnceAndEverythingReads(t *testing.T) {
	old := dictWindow
	dictWindow = 4096
	defer func() { dictWindow = old }()
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	body := func(w string) string {
		return "<p>" + strings.Repeat(w+" texto repetido de ejemplo ", 40) + "</p>"
	}
	var entries []dict.Entry
	for i := 0; i < 200; i++ {
		w := fmt.Sprintf("w%03d", i)
		entries = append(entries, h(w, body(w)))
	}
	path := filepath.Join(t.TempDir(), TextDBName)
	r := &fakeReader{meta: dict.Meta{Name: "Window", Format: "mdx", Path: "/w.mdx"}, entries: entries}
	if _, err := IngestPlan(r, path, Plan{FullText: true}, nil); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var rows int
	if err := s.db.QueryRow("SELECT count(*) FROM body_dict").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("body_dict holds %d rows, want exactly 1", rows)
	}
	for i := 0; i < 200; i++ {
		w := fmt.Sprintf("w%03d", i)
		res, err := s.Exact(w, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 1 || res[0].Body != body(w) {
			t.Fatalf("%s: body did not survive the window cut", w)
		}
	}
}
