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

	"github.com/wuweidict/wudict/internal/dict"
)

const (
	ingestQueueEntries         = 16
	ingestQueueBytes           = 64 << 20
	ingestMinWeight            = 16 << 10
	ingestWorkersPerDictionary = 8
)

type ingestAlias struct {
	word, folded, caseKey string
}

type preparedIngestEntry struct {
	sequence, weight                 int64
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
	entry            dict.Entry
	plan             Plan
	ctx              context.Context
}

type ingestSession struct {
	ctx                   context.Context
	cancel                context.CancelFunc
	queue                 []*ingestWork
	results               chan preparedIngestEntry
	changed               chan struct{}
	outstanding, inFlight int
	active                bool
}

// The pool owns only CPU preparation. Each session keeps its own source reader
// and SQLite writer. Workers select dictionary queues in round-robin order.
type ingestPool struct {
	mu               sync.Mutex
	sessions         []*ingestSession
	cursor           int
	running, workers int
	usedBytes        int64
	changed          chan struct{}
	started          bool
}

var sharedIngestPool ingestPool

func (p *ingestPool) begin(parent context.Context) *ingestSession {
	ctx, cancel := context.WithCancel(parent)
	s := &ingestSession{
		ctx: ctx, cancel: cancel, active: true,
		results: make(chan preparedIngestEntry, ingestQueueEntries),
		changed: make(chan struct{}),
	}
	p.mu.Lock()
	if !p.started {
		p.changed = make(chan struct{})
		p.started = true
	}
	wanted := min(8, max(0, runtime.GOMAXPROCS(0)-1))
	for p.workers < wanted {
		p.workers++
		go p.worker()
	}
	p.sessions = append(p.sessions, s)
	p.signalLocked(s)
	p.mu.Unlock()
	return s
}

func (p *ingestPool) submit(s *ingestSession, item *ingestWork) error {
	for {
		p.mu.Lock()
		if err := s.ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		// One article larger than the budget may run alone. Otherwise both
		// the dictionary queue and the shared working set remain bounded.
		fits := p.usedBytes == 0 || item.weight <= ingestQueueBytes-p.usedBytes
		if s.outstanding < ingestQueueEntries && fits {
			s.queue = append(s.queue, item)
			s.outstanding++
			p.usedBytes += item.weight
			p.signalLocked(s)
			p.mu.Unlock()
			return nil
		}
		changed := p.changed
		p.mu.Unlock()
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-changed:
		}
	}
}

func (p *ingestPool) worker() {
	for {
		s, item := p.next()
		result := prepareIngestEntry(item)
		p.mu.Lock()
		s.inFlight--
		p.running--
		p.signalLocked(s)
		p.mu.Unlock()
		// At most ingestQueueEntries results can be outstanding per session.
		s.results <- result
	}
}

func (p *ingestPool) next() (*ingestSession, *ingestWork) {
	for {
		p.mu.Lock()
		if p.running < p.workerLimitLocked() {
			for offset := 0; offset < len(p.sessions); offset++ {
				index := (p.cursor + offset) % len(p.sessions)
				s := p.sessions[index]
				if len(s.queue) == 0 || s.inFlight >= ingestWorkersPerDictionary {
					continue
				}
				item := s.queue[0]
				s.queue[0] = nil
				s.queue = s.queue[1:]
				s.inFlight++
				p.running++
				p.cursor = (index + 1) % len(p.sessions)
				p.signalLocked(s)
				p.mu.Unlock()
				return s, item
			}
		}
		changed := p.changed
		p.mu.Unlock()
		<-changed
	}
}

// Existing INDEX_WORKERS controls how many dictionaries index concurrently.
// Reserve a reader and writer lane for each active ingest before adding CPU
// workers; a saturated pool lets the writer prepare its own queued entries.
func (p *ingestPool) workerLimitLocked() int {
	return min(p.workers, max(0, runtime.GOMAXPROCS(0)-2*len(p.sessions)))
}

func (p *ingestPool) takeInline(s *ingestSession) *ingestWork {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(s.queue) == 0 || p.running < p.workerLimitLocked() && s.inFlight < ingestWorkersPerDictionary {
		return nil
	}
	item := s.queue[0]
	s.queue[0] = nil
	s.queue = s.queue[1:]
	p.signalLocked(s)
	return item
}

func (p *ingestPool) release(s *ingestSession, result preparedIngestEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.outstanding == 0 || result.weight > p.usedBytes {
		panic("store: invalid ingest reservation")
	}
	s.outstanding--
	p.usedBytes -= result.weight
	p.signalLocked(s)
}

func (p *ingestPool) state(s *ingestSession) (int, <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return s.outstanding, s.changed
}

func (p *ingestPool) abortQueued(s *ingestSession) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range s.queue {
		p.usedBytes -= item.weight
		s.outstanding--
	}
	s.queue = nil
	p.signalLocked(s)
}

func (p *ingestPool) end(s *ingestSession) {
	s.cancel()
	p.mu.Lock()
	defer p.mu.Unlock()
	if s.outstanding != 0 || s.inFlight != 0 || len(s.queue) != 0 {
		panic("store: ingest session still owns work")
	}
	s.active = false
	for i, existing := range p.sessions {
		if existing != s {
			continue
		}
		p.sessions = append(p.sessions[:i], p.sessions[i+1:]...)
		if len(p.sessions) == 0 {
			p.cursor = 0
		} else if i < p.cursor {
			p.cursor--
		} else if p.cursor >= len(p.sessions) {
			p.cursor = 0
		}
		break
	}
	p.signalLocked(nil)
}

func (p *ingestPool) signalLocked(s *ingestSession) {
	close(p.changed)
	p.changed = make(chan struct{})
	if s != nil {
		close(s.changed)
		s.changed = make(chan struct{})
	}
}

func prepareIngestEntry(item *ingestWork) (result preparedIngestEntry) {
	result.sequence, result.weight = item.sequence, item.weight
	defer func() {
		if value := recover(); value != nil {
			result.panicValue = value
		}
	}()
	if err := item.ctx.Err(); err != nil {
		result.err = err
		return result
	}
	e := item.entry
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
	if item.plan.FullText {
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
	for {
		if err := s.ctx.Err(); err != nil {
			outcome.err = err
			return
		}
		e, err := r.Next()
		if errors.Is(err, io.EOF) {
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
		sequence++
		item := &ingestWork{sequence: sequence, weight: ingestWeight(e, plan), entry: e, plan: plan, ctx: s.ctx}
		if err := sharedIngestPool.submit(s, item); err != nil {
			outcome.err = err
			return
		}
	}
}

// scanIngestEntries is the only SQLite writer. Results may finish out of
// order, but article IDs, aliases and progress are written in source order.
func scanIngestEntries(ctx context.Context, r dict.Reader, plan Plan, total int, progress Progress, rows *ingestRowWriter) (id, subEntries, linkCount int64, err error) {
	s := sharedIngestPool.begin(ctx)
	redirects := make(chan ingestRedirect, ingestQueueEntries)
	done := make(chan ingestReaderResult, 1)
	go readIngestEntries(s, r, plan, redirects, done)
	pending := make(map[int64]preparedIngestEntry)
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
		var result preparedIngestEntry
		gotResult := false
		if item := sharedIngestPool.takeInline(s); item != nil {
			result = prepareIngestEntry(item)
			gotResult = true
		} else {
			select {
			case result = <-s.results:
				gotResult = true
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
		if !gotResult {
			continue
		}
		pending[result.sequence] = result
		for {
			ready, ok := pending[next]
			if !ok {
				break
			}
			if ready.panicValue != nil {
				panic(ready.panicValue)
			}
			if ready.err != nil {
				return id, subEntries, linkCount, ready.err
			}
			if err := rows.write(next, ready); err != nil {
				return id, subEntries, linkCount, err
			}
			id = next
			if ready.subEntry {
				subEntries++
			}
			delete(pending, next)
			sharedIngestPool.release(s, ready)
			if progress != nil && id%batchSize == 0 {
				if err := rows.flush(); err != nil {
					return id, subEntries, linkCount, err
				}
				progress(clampDone(int(id), total), total)
			}
			next++
		}
	}
}
