// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

const indexLogLimit = logx.SystemLogLimit

var indexLogID atomic.Uint64

func init()               { logx.SetSystemLogRoot(DefaultDBDir) }
func indexLogDir() string { return filepath.Clean(DefaultDBDir()) + ".logs" }
func IndexDiagnostic(format string, args ...any) {
	logx.SystemDiagnosticIf(ingestParallelismDiagnostics.Load(), format, args...)
}
func IndexLog() ([]byte, error) { return logx.SystemSnapshot() }
func ClearIndexLog() error      { return logx.ClearSystemLog() }

type indexTrace struct {
	id          string
	dictionary  string
	format      string
	plan        Plan
	mode        indexconst.Mode
	entries     int
	stage       string
	start, last time.Time
	done, total int
}

func newIndexTrace(src, target string, plan Plan) *indexTrace {
	return newIndexTraceContext(context.Background(), src, target, plan)
}

func newIndexTraceContext(ctx context.Context, src, target string, plan Plan) *indexTrace {
	x := &indexTrace{id: fmt.Sprintf("%d-%d", os.Getpid(), indexLogID.Add(1)), start: time.Now(), plan: plan, mode: indexconst.IngestMode(ctx)}
	IndexDiagnostic("job=%s start source=%q target=%q headwords=true contains=%v fulltext=%v runtime=%s platform=%s/%s ingest_version=%d", x.id, src, target, plan.Contains, plan.FullText, runtime.Version(), runtime.GOOS, runtime.GOARCH, IngestVersion)
	x.phase("open database")
	return x
}

func (x *indexTrace) phase(stage string) {
	x.stage = stage
	IndexDiagnostic("job=%s stage=%q elapsed=%s", x.id, stage, time.Since(x.start).Round(time.Millisecond))
}

func (x *indexTrace) progress(done, total int) {
	x.done, x.total = done, total
	if time.Since(x.last) < 10*time.Second {
		return
	}
	x.last = time.Now()
	IndexDiagnostic("job=%s stage=%q progress=%d/%d elapsed=%s", x.id, x.stage, done, total, time.Since(x.start).Round(time.Millisecond))
	logIngestParallelism("progress", x.dictionary, x.id)
}

func (x *indexTrace) finish(err error) {
	elapsed := time.Since(x.start)
	status := "success"
	if err != nil {
		status = "error"
	}
	IndexDiagnostic("job=%s finish stage=%q progress=%d/%d elapsed=%s elapsed_ms=%d dictionary=%q format=%q mode=%s status=%s entries=%d contains=%v fulltext=%v error=%q",
		x.id, x.stage, x.done, x.total, elapsed.Round(time.Millisecond), elapsed.Milliseconds(),
		x.dictionary, x.format, indexconst.ModeString(x.mode), status, x.entries, x.plan.Contains, x.plan.FullText, fmt.Sprint(err))
	logIngestParallelism("finish", x.dictionary, x.id)
}
