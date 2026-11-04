// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/format/wmd"
	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/resource"
	"github.com/DMSHLabs/DictShelf/internal/store"
)

// `wudict dump` writes one dictionary out, as pyglossary's CSV (the default)
// or as wudict markdown (-format md, dump_md.go).
//
// The CSV is a two-column
// file whose leading rows are "#key","value" metadata and whose remaining rows
// are word, definition and - when an entry has aliases - a third column of
// alternates joined by commas. Resources go beside it in "<file>.csv_res",
// which is the directory name pyglossary's CSV writer derives and its reader
// looks for, so a dump round-trips through `pyglossary dict.csv out.xyz`
// without being told anything about it.
//
// Cross-references are written in their canonical spelling, `entry://`: a
// source's `bword:` links are respelled on the way out (htmlref.CanonRef), never
// copied, whether the body comes from a format Reader or a prepared text.db.
//
// The point is an exit door: every dictionary wudict can read becomes a format
// forty other tools can read, using their converter rather than a matrix of
// writers here.

func cmdDump(args []string) error {
	fs := flag.NewFlagSet("dump", flag.ExitOnError)
	var out, format, modeFlag, compress, resFlag string
	fs.StringVar(&out, "o", "", "output folder for the dump and its resources (created if missing)")
	fs.StringVar(&out, "output", "", "long form of -o")
	fs.StringVar(&format, "format", "csv", "csv, or md (wudict markdown)")
	fs.StringVar(&modeFlag, "mode", "html", "md only: html (each article's HTML kept) or clean (markdown only, lossy)")
	fs.StringVar(&compress, "compress", "", "md only: gz")
	fs.StringVar(&resFlag, "resources", "all", "all, text (only .css, .js and other text files), or none")
	fs.Parse(args)
	if fs.NArg() != 1 || out == "" {
		return fmt.Errorf("usage: wudict dump [-format csv|md] [-mode html|clean] [-compress gz] [-resources all|text|none] -o <outdir> <dictfile>")
	}
	scope, err := parseResScope(resFlag)
	if err != nil {
		return err
	}
	md := false
	switch strings.ToLower(format) {
	case "csv":
	case "md", "markdown":
		md = true
	default:
		return fmt.Errorf("-format %q: want csv or md", format)
	}
	mode, err := wmd.ParseMode(modeFlag)
	if err != nil {
		return fmt.Errorf("-%v", err)
	}
	gz := false
	switch strings.ToLower(compress) {
	case "":
	case "gz":
		gz = true
	default:
		return fmt.Errorf("-compress %q: want gz", compress)
	}
	modeSet := false
	fs.Visit(func(f *flag.Flag) { modeSet = modeSet || f.Name == "mode" })
	if !md && (gz || modeSet) {
		return fmt.Errorf("-mode and -compress are for -format md")
	}
	applyLibrarySettings()
	// A prepared dictionary is a folder, and the folder is the name a user
	// types for it: resolved to its text.db here, once, so every stage below
	// reads the same file.
	src := dict.MainFile(fs.Arg(0))

	base := dumpBase(src)
	if base == "" {
		return fmt.Errorf("cannot derive an output name from %q: rename the file, or dump it from its library folder", src)
	}
	path := filepath.Join(out, base+".csv")
	resDir := path + "_res" // pyglossary: the csv's own name plus "_res"
	if md {
		path = filepath.Join(out, base+wmd.Ext)
		if gz {
			path += ".gz"
		}
		resDir = filepath.Join(out, base+".wudict.files") // spec R2.3
	}

	// Said before anything is written, because after it is too late to be a
	// warning. Named files, not a general caution: "will be overwritten" is
	// only useful if you can tell whether it means yours.
	if fsx.DirExists(out) {
		if scope == resNone {
			fmt.Fprintf(os.Stderr, "%s already exists - %s will be overwritten\n", out, filepath.Base(path))
		} else {
			fmt.Fprintf(os.Stderr, "%s already exists - %s and anything already in %s/ under the same name will be overwritten\n",
				out, filepath.Base(path), filepath.Base(resDir))
		}
	}
	var n int
	var srcFormat string // the format the entries were read as
	if md {
		r, err := dumpMarkdown(src, out, path, base+".wudict", mode, gz)
		if err != nil {
			var cf *cleanFailure
			if errors.As(err, &cf) {
				printRawEntry(cf)
			}
			return err
		}
		n, srcFormat = r.articles, r.format
		dumpNote(r.empty, "article with an empty body SKIPPED")
		dumpNote(r.nameless, "entry without a headword SKIPPED")
		dumpNote(r.repaired, "name or header value repaired (control characters, invalid UTF-8)")
	} else {
		r, err := dumpEntries(src, out, path)
		if err != nil {
			return err
		}
		n, srcFormat = r.rows, r.format
		dumpNote(r.nameless, "entry without a headword SKIPPED")
	}
	size, _ := fileSize(path)
	fmt.Printf("%s → %s (%s)\n", logx.Plural(n, "entry", "entries"), path, logx.Size(size))

	files, bytes, err := dumpResources(src, srcFormat, resDir, scope)
	if err != nil {
		return err
	}
	if files > 0 {
		fmt.Printf("%s → %s (%s)\n", logx.Plural(files, "resource", "resources"), resDir, logx.Size(bytes))
	}
	return nil
}

// dumpNote reports, when n > 0, what the dump changed on the way out.
func dumpNote(n int, what string) {
	if n > 0 {
		fmt.Fprintf(os.Stderr, "  %d %s\n", n, what)
	}
}

// resScope is which resources a dump writes (-resources).
type resScope int

const (
	resAll  resScope = iota
	resText          // stylesheets, scripts and other text an article uses directly
	resNone          // the articles only
)

func parseResScope(s string) (resScope, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "all":
		return resAll, nil
	case "text":
		return resText, nil
	case "none":
		return resNone, nil
	}
	return 0, fmt.Errorf("-resources %q: want all, text or none", s)
}

// textExts are the resources -resources text keeps: text an article loads
// as it is. Images - svg included - audio, video and fonts are media.
var textExts = map[string]bool{
	".css": true, ".js": true, ".mjs": true, ".json": true,
	".html": true, ".htm": true, ".xml": true, ".txt": true,
}

func isTextResource(name string) bool { return textExts[strings.ToLower(path.Ext(name))] }

// dumpBase names the output after the SOURCE FILE rather than after the
// dictionary's own title: the title is free text that may be empty, may repeat
// across a collection, and routinely holds characters no filesystem accepts.
// A prepared dictionary is a folder whose file is always called text.db, so
// there the folder is the name - dumping `~/.wudict/db/Oxford/text.db` writes
// Oxford.csv, not text.csv.
func dumpBase(src string) string {
	if store.IsTextDB(src) {
		b := filepath.Base(src)
		if strings.EqualFold(b, store.TextDBName) {
			return safeName(filepath.Base(filepath.Dir(src)))
		}
		return safeName(b[:len(b)-len(".text.db")])
	}
	return store.FolderName(src)
}

// csvDump is what a CSV dump wrote.
type csvDump struct {
	rows     int    // entries written
	nameless int    // entries left out for having no headword
	format   string // the format the entries were read as (dict.Meta.Format)
}

// dumpEntries writes the CSV. The ingest Reader is preferred wherever a format
// has one: it is a single sequential pass that yields each entry's aliases with
// it, where the query interface would have to be walked headword by headword
// and would report an alias as another copy of its entry. A prepared
// dictionary has no Reader - it IS the index - and is streamed from its own
// tables instead.
//
// The source is opened BEFORE the output folder is created, so a dictionary
// that cannot be read leaves nothing behind.
func dumpEntries(src, outDir, csvPath string) (r csvDump, err error) {
	var meta dict.Meta
	var each func(row func([]string, string) error) error
	var closeSrc func() error

	if store.IsTextDB(src) {
		s, err := store.Open(src)
		if err != nil {
			return r, err
		}
		meta, closeSrc = s.Meta(), s.Close
		each = func(row func([]string, string) error) error {
			return s.EachEntry(func(headword string, alts []string, body string) error {
				return row(append([]string{headword}, alts...), htmlref.CanonLinks(body))
			})
		}
	} else {
		rd, err := dict.OpenReader(src)
		if err != nil {
			return r, err
		}
		meta, closeSrc = rd.Meta(), rd.Close
		each = func(row func([]string, string) error) error { return readAll(rd, row) }
	}
	defer closeSrc()
	r.format = meta.Format

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return r, err
	}
	f, err := os.Create(csvPath)
	if err != nil {
		return r, err
	}
	defer f.Close()
	w := csv.NewWriter(f) // buffered internally

	m := newMeter("entries", meta.EntryCount, entryEvery)
	defer m.Clear()
	row := func(words []string, body string) error {
		m.Add(1)
		if len(words) == 0 || words[0] == "" {
			r.nameless++
			return nil // a row pyglossary would drop: never written
		}
		rec := []string{words[0], body}
		if len(words) > 1 {
			// Comma-joined regardless of the delimiter, which is what
			// pyglossary's reader splits this column on.
			rec = append(rec, strings.Join(words[1:], ","))
		}
		r.rows++
		return w.Write(rec)
	}

	if err := writeInfoRows(w, meta); err != nil {
		return r, err
	}
	if err := each(row); err != nil {
		return r, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return r, err
	}
	return r, f.Close()
}

// readAll drains a format Reader into row(), rendering each body exactly as an
// ingest would, with its cross-references in the canonical `entry://` spelling.
func readAll(r dict.Reader, row func([]string, string) error) error {
	for {
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if e.LinkTo != "" {
			// A pure redirect has no body of its own, so it becomes an
			// entry:// anchor and the pointer survives the conversion instead
			// of becoming a dangling headword.
			href := htmlref.EntryHref(e.LinkTo)
			esc := html.EscapeString(e.LinkTo)
			if err := row(e.Headwords, `<a href="`+html.EscapeString(href)+`">`+esc+`</a>`); err != nil {
				return err
			}
			continue
		}
		body, err := store.NormalizeBody(e)
		if err != nil {
			return fmt.Errorf("%s: %w", firstWord(e.Headwords), err)
		}
		if err := row(e.Headwords, htmlref.CanonLinks(body)); err != nil {
			return err
		}
	}
}

// writeInfoRows emits the "#key","value" header block. Only fields the
// dictionary actually carries are written: pyglossary reads these into the
// glossary's info, and an invented author or target language would be carried
// into every format converted from here.
func writeInfoRows(w *csv.Writer, m dict.Meta) error {
	info := [][2]string{{"name", m.Name}}
	if m.IndexLang != "" {
		// Headwords are the source side. Declared only - a language worked out
		// from a file name is the search path's guess, not the file's claim.
		info = append(info, [2]string{"sourceLang", m.IndexLang})
	}
	if m.Description != "" {
		info = append(info, [2]string{"description", m.Description})
	}
	for _, kv := range info {
		if kv[1] == "" {
			continue
		}
		if err := w.Write([]string{"#" + kv[0], kv[1]}); err != nil {
			return err
		}
	}
	return nil
}

// dumpResources unpacks every resource the dictionary holds into resDir,
// preserving the folder structure the names carry so that an article's
// `src="audio/x.mp3"` still resolves after the conversion. format is the
// format the entries were read as (openResources).
//
// A resource that cannot be read is reported and skipped rather than ending
// the dump: the names come from a container that may be truncated or lying,
// and 40,000 good files are not worth losing to one bad record.
func dumpResources(src, format, resDir string, scope resScope) (files int, written int64, err error) {
	if scope == resNone {
		return 0, 0, nil
	}
	d, err := openResources(src, format)
	if err != nil {
		return 0, 0, err
	}
	defer d.Close()
	names := d.list()
	names = resource.Filter(names) // a dump is for humans; .DS_Store is not
	if scope == resText {
		names = slices.DeleteFunc(names, func(n string) bool { return !isTextResource(n) })
	}
	if len(names) == 0 {
		return 0, 0, nil
	}
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		return 0, 0, err
	}
	var failed int
	m := newMeter("resources", len(names), resourceEvery)
	if m != nil {
		m.bytes = true
	}
	for _, name := range names {
		n, err := dumpOneResource(d, resDir, name)
		m.AddBytes(n)
		m.Add(1)
		if err != nil {
			m.Clear()
			fmt.Fprintf(os.Stderr, "  %s: %v\n", name, err)
			failed++
			continue
		}
		files++
		written += n
	}
	m.Clear()
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "  %d of %d resources could not be extracted\n", failed, len(names))
	}
	return files, written, nil
}

func dumpOneResource(d dict.ResourceOpener, resDir, name string) (int64, error) {
	dest := resFilePath(resDir, name)
	if dest == "" {
		return 0, fmt.Errorf("no usable file name")
	}
	rc, _, err := d.Resource(name)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, rc)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return n, err
}

func firstWord(words []string) string {
	if len(words) == 0 {
		return "(no headword)"
	}
	return words[0]
}

func fileSize(path string) (int64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
