// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package wmd

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/yuin/goldmark/v2/parser"

	"github.com/wuweidict/wudict/internal/sysmem"
)

// Phase 2 (see read.go): the bodies, a unit of whole heading groups at a
// time, rendered in file order - on worker goroutines that run ahead of Next
// while a person waits on the preparation, by Next itself otherwise. Next
// reads the lane at every unit, both ways.

// unit is a run of whole heading groups, parsed and rendered at once.
type unit struct {
	start, end int
	ats        []int // the start of each group's first heading line, as phase 1 found it
	render     []int // the groups whose bodies Next yields (indices into ats), in order
}

// renderer is phase 2's workers and the units handed to them.
type renderer struct {
	jobs    chan job
	results map[int]*result // the units handed out and not yet taken by Next
	next    int             // the next unit to hand out
	workers int
	acct    sysmem.Account
	stop    atomic.Bool
	wg      sync.WaitGroup
}

type job struct {
	unit int
	res  *result
}

type result struct {
	done   chan struct{}
	bodies []string
	warns  []string
	err    error
	weight int64
}

// want makes unit u's bodies the ones Next yields from.
func (r *Reader) want(u int) error {
	if u == r.cur {
		return nil
	}
	urgent := urgentNow(r)
	if urgent {
		r.startRender()
		r.handOut(u)
	}
	var res *result
	if r.rend != nil {
		if res = r.rend.results[u]; res != nil {
			delete(r.rend.results, u)
		}
	}
	var bodies, warns []string
	var err error
	if res != nil {
		<-res.done
		bodies, warns, err = res.bodies, res.warns, res.err
		r.rend.acct.Release(res.weight)
	} else {
		bodies, warns, err = r.renderUnit(u)
		if r.rend != nil {
			r.rend.next = max(r.rend.next, u+1)
		}
	}
	if err != nil {
		return err
	}
	for _, w := range warns {
		r.warn(0, "%s", w)
	}
	r.cur, r.bodies = u, bodies
	if urgent {
		r.handOut(u + 1)
	}
	return nil
}

// startRender starts the workers, once.
func (r *Reader) startRender() {
	if r.rend != nil {
		return
	}
	n := min(runtime.GOMAXPROCS(0), workersMax)
	rd := &renderer{jobs: make(chan job, 2*n), results: map[int]*result{}, workers: n}
	r.rend = rd
	for range n {
		rd.wg.Add(1)
		go func() {
			defer rd.wg.Done()
			for j := range rd.jobs {
				if !rd.stop.Load() {
					j.res.bodies, j.res.warns, j.res.err = r.renderUnit(j.unit)
				}
				close(j.res.done)
			}
		}()
	}
}

// handOut gives the workers the units with bodies from 'from' on, while the
// window (twice the workers) and the budget have room.
func (r *Reader) handOut(from int) {
	rd := r.rend
	rd.next = max(rd.next, from)
	for rd.next < len(r.units) && len(rd.results) < 2*rd.workers {
		u := rd.next
		if len(r.units[u].render) == 0 {
			rd.next++
			continue
		}
		w := unitWeight * int64(r.units[u].end-r.units[u].start)
		if !rd.acct.TryAcquire(w) {
			return // no room: Next renders the unit itself when it gets there
		}
		res := &result{done: make(chan struct{}), weight: w}
		rd.results[u] = res
		rd.jobs <- job{unit: u, res: res} // never blocks: at most 2*workers are out
		rd.next++
	}
}

// end lets go of what phase 2 holds: the workers are joined, their draw on
// the budget returned, the text held in memory dropped. Safe to call again.
func (r *Reader) end() {
	if r.ended {
		return
	}
	r.ended = true
	if rd := r.rend; rd != nil {
		rd.stop.Store(true)
		close(rd.jobs)
		rd.wg.Wait()
		rd.acct.Close()
		r.rend = nil
	}
	r.bodies = nil
	if r.src != nil {
		r.src.release()
	}
}

// renderUnit parses unit u, knowing the definitions it can reference, and
// renders the bodies Next yields from it. The parse must find the groups
// phase 1 found, where it found them: anything else is a reader fault, and
// would pair names with the wrong bodies.
func (r *Reader) renderUnit(u int) (bodies, warns []string, err error) {
	defer func() {
		// The parser is third-party code on hostile input: a panic costs the
		// file, never the process.
		if p := recover(); p != nil {
			bodies, warns, err = nil, nil, formatErr("the file could not be parsed: %v", p)
		}
	}()
	un := &r.units[u]
	text, err := r.src.read(un.start, un.end)
	if err != nil {
		return nil, nil, err
	}
	pc := parser.NewContext()
	prefill(pc, r.defs, text)
	ss := spans(mdParser.Parse(text, parser.WithContext(pc)).FirstChild(), nil)
	drift := len(ss) != len(un.ats)
	for i := 0; !drift && i < len(ss); i++ {
		drift = un.start+lineStart(text, ss[i].at.Pos()) != un.ats[i]
	}
	if drift {
		return nil, nil, fmt.Errorf("wudict markdown: the text at %d reads differently on its second parse", un.start)
	}
	for _, gi := range un.render {
		body, w := renderBody(text, ss[gi].first, ss[gi].stop)
		bodies = append(bodies, body)
		if w != "" {
			warns = append(warns, w)
		}
	}
	return bodies, warns, nil
}
