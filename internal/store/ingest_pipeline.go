// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/sysmem"
)

const (
	ingestQueueEntries = 16
	ingestMinWeight    = 16 << 10
	ingestWorkEntries  = 256
	ingestWorkBytes    = 4 << 20
)

type ingestAlias struct {
	word, folded, caseKey string
}

type preparedIngestEntry struct {
	headword, folded, caseKey, lower string
	body                             []byte
	text                             string
	aliases                          []ingestAlias
	subEntry                         bool
	err                              error
	panicValue                       any
}

type ingestWork struct {
	sequence, weight int64
	entries          []dict.Entry
	plan             Plan
	ctx              context.Context
}

type preparedIngestBatch struct {
	sequence, weight int64
	entries          []preparedIngestEntry
}

type ingestSession struct {
	mu                    sync.Mutex
	ctx                   context.Context
	cancel                context.CancelFunc
	queue                 []*ingestWork
	results               chan preparedIngestBatch
	changed               chan struct{}
	outstanding, inFlight int
	active                bool
}

// The pool owns only CPU preparation. Each session keeps its own source reader
// and SQLite writer. Workers select dictionary queues in round-robin order.
type ingestPool struct {
	registryMu    sync.Mutex                       // begins and ends publish immutable session snapshots
	sessions      atomic.Pointer[[]*ingestSession] // readers never hold the registry lock
	cursor        atomic.Uint64
	running       atomic.Int32
	inlineRunning atomic.Int32
	workers       atomic.Int32
	wakeMu        sync.Mutex
	wake          chan struct{}
	usedBytes     atomic.Int64
	memCapacity   atomic.Int64
}

var sharedIngestPool = ingestPool{wake: make(chan struct{})}

func (p *ingestPool) begin(parent context.Context) *ingestSession {
	ctx, cancel := context.WithCancel(parent)
	s := &ingestSession{
		ctx: ctx, cancel: cancel, active: true,
		results: make(chan preparedIngestBatch, ingestQueueEntries),
		changed: make(chan struct{}),
	}
	budget := sysmem.Budget()
	p.memCapacity.Store(budget)
	p.registryMu.Lock()
	// Spawn for the machine, even if Android is backgrounded at start. The
	// active limit below follows GOMAXPROCS when the app returns to foreground.
	wanted := indexconst.IngestWorkerLimit(runtime.NumCPU(), 1)
	for int(p.workers.Load()) < wanted {
		p.workers.Add(1)
		go p.worker()
	}
	current := p.sessions.Load()
	var next []*ingestSession
	if current != nil {
		next = append(next, (*current)...)
	}
	next = append(next, s)
	p.sessions.Store(&next)
	p.registryMu.Unlock()
	p.signalWorkers()
	return s
}

func (p *ingestPool) submit(s *ingestSession, item *ingestWork) error {
	for {
		s.mu.Lock()
		if err := s.ctx.Err(); err != nil {
			s.mu.Unlock()
			return err
		}
		// One oversized batch may run alone; all other reservations fit the
		// process-wide budget measured when preparation begins.
		if s.outstanding < ingestQueueEntries && p.reserve(item.weight) {
			s.queue = append(s.queue, item)
			s.outstanding++
			p.signalSessionLocked(s)
			s.mu.Unlock()
			p.signalWorkers()
			return nil
		}
		queueChanged := s.changed
		queueFull := s.outstanding >= ingestQueueEntries
		s.mu.Unlock()
		if queueFull {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case <-queueChanged:
			}
		} else {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case <-time.After(2 * time.Millisecond):
			}
		}
	}
}

func (p *ingestPool) reserve(weight int64) bool {
	// Reserving a whole batch by CAS keeps other dictionary queues independent.
	for {
		used := p.usedBytes.Load()
		if used != 0 && weight > p.memCapacity.Load()-used {
			return false
		}
		if p.usedBytes.CompareAndSwap(used, used+weight) {
			return true
		}
	}
}

func (p *ingestPool) worker() {
	for {
		s, item := p.next()
		result := prepareIngestBatch(item)
		s.mu.Lock()
		s.inFlight--
		s.mu.Unlock()
		p.running.Add(-1)
		p.signalWorkers()
		// At most ingestQueueEntries results can be outstanding per session.
		s.results <- result
	}
}

func (p *ingestPool) next() (*ingestSession, *ingestWork) {
	for {
		wake := p.workerWake()
		sessions := p.sessions.Load()
		contended := false
		if sessions != nil && len(*sessions) > 0 && p.reserveWorker(len(*sessions)) {
			start := int(p.cursor.Add(1)-1) % len(*sessions)
			for offset := 0; offset < len(*sessions); offset++ {
				index := (start + offset) % len(*sessions)
				s := (*sessions)[index]
				if !s.mu.TryLock() {
					contended = true
					continue
				}
				if len(s.queue) == 0 {
					s.mu.Unlock()
					continue
				}
				item := s.queue[0]
				s.queue[0] = nil
				s.queue = s.queue[1:]
				s.inFlight++
				s.mu.Unlock()
				return s, item
			}
			p.running.Add(-1)
		}
		if contended {
			select {
			case <-wake:
			case <-time.After(time.Millisecond):
			}
		} else {
			<-wake
		}
	}
}

func (p *ingestPool) reserveWorker(sessions int) bool {
	limit := p.workerLimit(sessions)
	for {
		running := p.running.Load()
		if int(running) >= limit {
			return false
		}
		if p.running.CompareAndSwap(running, running+1) {
			return true
		}
	}
}

// Reserve one logical processor for the rest of the app and a reader and
// writer lane for each active ingest. The remaining processors are shared.
func (p *ingestPool) workerLimit(sessions int) int {
	logical := min(runtime.NumCPU(), runtime.GOMAXPROCS(0))
	return min(int(p.workers.Load()), indexconst.IngestWorkerLimit(logical, sessions))
}

func (p *ingestPool) takeInline(s *ingestSession) *ingestWork {
	sessions := p.sessions.Load()
	processorsAvailable := sessions != nil && int(p.running.Load()) < p.workerLimit(len(*sessions))
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 || processorsAvailable {
		return nil
	}
	item := s.queue[0]
	s.queue[0] = nil
	s.queue = s.queue[1:]
	return item
}

func (p *ingestPool) takeReadyOrInline(s *ingestSession) (preparedIngestBatch, bool) {
	// A ready batch keeps the SQLite writer writing. Inline preparation is
	// needed only when the shared processors cannot take more queued work.
	select {
	case result := <-s.results:
		return result, true
	default:
	}
	if item := p.takeInline(s); item != nil {
		p.inlineRunning.Add(1)
		defer p.inlineRunning.Add(-1)
		return prepareIngestBatch(item), true
	}
	return preparedIngestBatch{}, false
}

func (p *ingestPool) release(s *ingestSession, result preparedIngestBatch) {
	s.mu.Lock()
	if s.outstanding == 0 {
		panic("store: invalid ingest reservation")
	}
	s.outstanding--
	p.signalSessionLocked(s)
	s.mu.Unlock()
	if p.usedBytes.Add(-result.weight) < 0 {
		panic("store: invalid ingest reservation weight")
	}
	p.signalWorkers()
}

func (p *ingestPool) state(s *ingestSession) (int, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outstanding, s.changed
}

func (p *ingestPool) abortQueued(s *ingestSession) {
	s.mu.Lock()
	var released int64
	for _, item := range s.queue {
		released += item.weight
		s.outstanding--
	}
	s.queue = nil
	p.signalSessionLocked(s)
	s.mu.Unlock()
	if p.usedBytes.Add(-released) < 0 {
		panic("store: invalid queued ingest reservation")
	}
}

func (p *ingestPool) end(s *ingestSession) {
	s.cancel()
	s.mu.Lock()
	if s.outstanding != 0 || s.inFlight != 0 || len(s.queue) != 0 {
		panic("store: ingest session still owns work")
	}
	s.active = false
	s.mu.Unlock()
	p.registryMu.Lock()
	defer p.registryMu.Unlock()
	current := p.sessions.Load()
	next := make([]*ingestSession, 0, len(*current)-1)
	for _, existing := range *current {
		if existing != s {
			next = append(next, existing)
		}
	}
	p.sessions.Store(&next)
	p.signalWorkers()
}

func (p *ingestPool) signalSessionLocked(s *ingestSession) {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (p *ingestPool) signalWorkers() {
	p.wakeMu.Lock()
	close(p.wake)
	p.wake = make(chan struct{})
	p.wakeMu.Unlock()
}

func (p *ingestPool) workerWake() <-chan struct{} {
	p.wakeMu.Lock()
	wake := p.wake
	p.wakeMu.Unlock()
	return wake
}

func prepareIngestBatch(item *ingestWork) preparedIngestBatch {
	result := preparedIngestBatch{sequence: item.sequence, weight: item.weight, entries: make([]preparedIngestEntry, 0, len(item.entries))}
	for _, entry := range item.entries {
		result.entries = append(result.entries, prepareIngestEntry(entry, item.plan, item.ctx))
	}
	return result
}

func prepareIngestEntry(e dict.Entry, plan Plan, ctx context.Context) (result preparedIngestEntry) {
	defer func() {
		if value := recover(); value != nil {
			result.panicValue = value
		}
	}()
	if err := ctx.Err(); err != nil {
		result.err = err
		return result
	}
	result.headword = strings.TrimSpace(e.Headwords[0])
	body, err := NormalizeBody(e)
	if err != nil {
		result.err = err
		return result
	}
	result.folded = dict.Fold(result.headword)
	result.caseKey = dict.CaseKey(result.headword)
	result.lower = strings.ToLower(result.headword)
	result.body = encodeBody(body)
	if plan.FullText {
		result.text = StripHTML(body)
	}
	result.subEntry = isSubEntry(result.headword)
	for _, extra := range e.Headwords[1:] {
		extra = strings.TrimSpace(extra)
		if extra != "" && extra != result.headword {
			result.aliases = append(result.aliases, ingestAlias{extra, dict.Fold(extra), dict.CaseKey(extra)})
		}
	}
	return result
}

func ingestWeight(e dict.Entry, plan Plan) int64 {
	// The reservation covers the queued source, normalized HTML, compressed
	// body and (when enabled) the extracted article text. Text escaping can
	// expand a source substantially, so prefer a conservative estimate.
	factor := int64(8)
	if plan.FullText {
		factor = 12
	}
	if int64(len(e.Body)) > math.MaxInt64/factor {
		return math.MaxInt64
	}
	weight := int64(len(e.Body)) * factor
	for _, word := range e.Headwords {
		if int64(len(word)) > (math.MaxInt64-weight)/3 {
			return math.MaxInt64
		}
		weight += int64(len(word)) * 3
	}
	return max(weight, ingestMinWeight)
}

type ingestRedirect struct{ headword, target string }
type ingestReaderResult struct {
	err        error
	panicValue any
}

func readIngestEntries(s *ingestSession, r dict.Reader, plan Plan, redirects chan<- ingestRedirect, done chan<- ingestReaderResult) {
	var outcome ingestReaderResult
	defer func() {
		if value := recover(); value != nil {
			outcome.panicValue = value
		}
		close(redirects)
		done <- outcome
	}()
	var sequence int64
	var entries []dict.Entry
	var weight, sourceBytes int64
	submit := func() error {
		if len(entries) == 0 {
			return nil
		}
		item := &ingestWork{sequence: sequence - int64(len(entries)) + 1, weight: weight, entries: entries, plan: plan, ctx: s.ctx}
		if err := sharedIngestPool.submit(s, item); err != nil {
			return err
		}
		entries = nil
		weight, sourceBytes = 0, 0
		return nil
	}
	for {
		if err := s.ctx.Err(); err != nil {
			outcome.err = err
			return
		}
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
			outcome.err = submit()
			return
		}
		if err != nil {
			outcome.err = err
			return
		}
		if err := s.ctx.Err(); err != nil {
			outcome.err = err
			return
		}
		if len(e.Headwords) == 0 {
			continue
		}
		hw := strings.TrimSpace(e.Headwords[0])
		if hw == "" {
			continue
		}
		if e.LinkTo != "" {
			select {
			case redirects <- ingestRedirect{strings.Clone(hw), strings.Clone(e.LinkTo)}:
			case <-s.ctx.Done():
				outcome.err = s.ctx.Err()
				return
			}
			continue
		}
		// Readers are sequential and may reuse backing storage after Next.
		// Own queued strings before asking the reader for another entry.
		e.Body = strings.Clone(e.Body)
		e.Headwords = append([]string(nil), e.Headwords...)
		for i := range e.Headwords {
			e.Headwords[i] = strings.Clone(e.Headwords[i])
		}
		if len(entries) == ingestWorkEntries || len(entries) > 0 && int64(len(e.Body)) > ingestWorkBytes-sourceBytes {
			if err := submit(); err != nil {
				outcome.err = err
				return
			}
		}
		sequence++
		entries = append(entries, e)
		entryWeight := ingestWeight(e, plan)
		if entryWeight > math.MaxInt64-weight {
			weight = math.MaxInt64
		} else {
			weight += entryWeight
		}
		sourceBytes += int64(len(e.Body))
		if len(entries) == ingestWorkEntries || sourceBytes >= ingestWorkBytes {
			if err := submit(); err != nil {
				outcome.err = err
				return
			}
		}
	}
}

// scanIngestEntries is the only SQLite writer. Results may finish out of
// order, but article IDs, aliases and progress are written in source order.
func scanIngestEntries(ctx context.Context, r dict.Reader, plan Plan, total int, progress Progress, rows *ingestRowWriter, trace *indexTrace) (id, subEntries, linkCount int64, err error) {
	s := sharedIngestPool.begin(ctx)
	logIngestParallelism("start", trace.dictionary, trace.id)
	redirects := make(chan ingestRedirect, ingestQueueEntries)
	done := make(chan ingestReaderResult, 1)
	go readIngestEntries(s, r, plan, redirects, done)
	pending := make(map[int64]preparedIngestBatch)
	readerDone := false
	defer func() {
		s.cancel()
		sharedIngestPool.abortQueued(s)
		for _, result := range pending {
			sharedIngestPool.release(s, result)
		}
		for {
			outstanding, _ := sharedIngestPool.state(s)
			if readerDone && outstanding == 0 {
				break
			}
			select {
			case result := <-s.results:
				sharedIngestPool.release(s, result)
			case <-done:
				readerDone = true
				done = nil
			}
		}
		sharedIngestPool.end(s)
	}()
	var next int64 = 1
	for {
		outstanding, changed := sharedIngestPool.state(s)
		if readerDone && redirects == nil && outstanding == 0 {
			return id, subEntries, linkCount, nil
		}
		if err := ctx.Err(); err != nil {
			return id, subEntries, linkCount, err
		}
		result, ready := sharedIngestPool.takeReadyOrInline(s)
		if !ready {
			select {
			case result = <-s.results:
			case redirect, ok := <-redirects:
				if !ok {
					redirects = nil
					continue
				}
				if err := rows.redirect(redirect.headword, redirect.target); err != nil {
					return id, subEntries, linkCount, err
				}
				linkCount++
				continue
			case outcome := <-done:
				readerDone = true
				done = nil
				if outcome.panicValue != nil {
					panic(outcome.panicValue)
				}
				if outcome.err != nil {
					return id, subEntries, linkCount, outcome.err
				}
				continue
			case <-changed:
				continue
			case <-ctx.Done():
				return id, subEntries, linkCount, ctx.Err()
			}
		}
		pending[result.sequence] = result
		for {
			ready, ok := pending[next]
			if !ok {
				break
			}
			for _, entry := range ready.entries {
				if entry.panicValue != nil {
					panic(entry.panicValue)
				}
				if entry.err != nil {
					return id, subEntries, linkCount, entry.err
				}
				if err := rows.write(next, entry); err != nil {
					return id, subEntries, linkCount, err
				}
				id = next
				if entry.subEntry {
					subEntries++
				}
				if progress != nil && id%batchSize == 0 {
					if err := rows.flush(); err != nil {
						return id, subEntries, linkCount, err
					}
					progress(clampDone(int(id), total), total)
				}
				next++
			}
			delete(pending, ready.sequence)
			sharedIngestPool.release(s, ready)
		}
	}
}
