// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"github.com/wuweidict/wudict/internal/logx"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"
)

const indexLogLimit = logx.SystemLogLimit

var indexLogID atomic.Uint64

func init()                                      { logx.SetSystemLogRoot(DefaultDBDir) }
func indexLogDir() string                        { return filepath.Clean(DefaultDBDir()) + ".logs" }
func IndexDiagnostic(format string, args ...any) { logx.System(format, args...) }
func IndexLog() ([]byte, error)                  { return logx.SystemSnapshot() }

type indexTrace struct {
	id          string
	stage       string
	start, last time.Time
	done, total int
}

func newIndexTrace(src, target string, plan Plan) *indexTrace {
	x := &indexTrace{id: fmt.Sprintf("%d-%d", os.Getpid(), indexLogID.Add(1)), start: time.Now()}
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
}

func (x *indexTrace) finish(err error) {
	IndexDiagnostic("job=%s finish stage=%q progress=%d/%d elapsed=%s error=%q", x.id, x.stage, x.done, x.total, time.Since(x.start).Round(time.Millisecond), fmt.Sprint(err))
}
