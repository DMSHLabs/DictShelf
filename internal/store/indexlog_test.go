// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"github.com/wuweidict/wudict/internal/dict"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexLogFailureAndRotation(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	trace := newIndexTrace("broken.dsl", "text.db", Plan{Contains: true, FullText: true})
	trace.phase("optimize contains index")
	trace.progress(42, 100)
	trace.finish(errors.New("disk full"))
	b, err := IndexLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"broken.dsl", "contains=true fulltext=true", "optimize contains index", "42/100", "disk full"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q in %s", want, b)
		}
	}
	path := filepath.Join(indexLogDir(), "system.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", indexLogLimit)), 0o600); err != nil {
		t.Fatal(err)
	}
	IndexDiagnostic("after rotation")
	b, err = IndexLog()
	if err != nil || !strings.HasSuffix(string(b), "after rotation\n") || len(b) < indexLogLimit {
		t.Fatalf("rotation lost diagnostics: %v", err)
	}
}

func TestIndexLogIngestRecordsStagesWithoutArticle(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	r := &fakeReader{meta: dict.Meta{Name: "Example", Format: "mdx", Path: "example.mdx"}, entries: []dict.Entry{h("alpha", "PRIVATE ARTICLE BODY")}}
	_, err := IngestPlan(r, filepath.Join(t.TempDir(), "text.db"), Plan{Contains: true, FullText: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := IndexLog()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dictionary=\"Example\"", "headword index", "optimize fulltext index", "optimize contains index", "save database", "finish"} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(string(b), "PRIVATE ARTICLE BODY") {
		t.Fatal("article leaked into diagnostics")
	}
}

type diagnosticBrokenReader struct{ fakeReader }

func (r *diagnosticBrokenReader) Next() (dict.Entry, error) {
	return dict.Entry{}, errors.New("corrupt dictionary block")
}

func TestIndexLogRecordsReaderFailure(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	r := &diagnosticBrokenReader{fakeReader: fakeReader{meta: dict.Meta{Name: "Broken", Format: "mdx", Path: "broken.mdx"}}}
	_, err := IngestPlan(r, filepath.Join(t.TempDir(), "text.db"), Plan{}, nil)
	if err == nil {
		t.Fatal("expected reader failure")
	}
	b, readErr := IndexLog()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(b), "corrupt dictionary block") || !strings.Contains(string(b), "finish stage=\"scan articles") {
		t.Fatalf("missing failure details: %s", b)
	}
}

type diagnosticPanicReader struct{ fakeReader }

func (r *diagnosticPanicReader) Next() (dict.Entry, error) { panic("broken decoder") }

func TestIndexLogRecordsAndPropagatesPanic(t *testing.T) {
	t.Setenv("WUDICT_DB_DIR", t.TempDir())
	r := &diagnosticPanicReader{}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed")
			}
		}()
		_, _ = IngestPlan(r, filepath.Join(t.TempDir(), "text.db"), Plan{}, nil)
	}()
	b, err := IndexLog()
	if err != nil || !strings.Contains(string(b), "panic: broken decoder") {
		t.Fatalf("panic not recorded: %v %s", err, b)
	}
}
