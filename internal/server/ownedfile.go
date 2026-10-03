// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"errors"
	"fmt"
	"io/fs"
	"sync"
	"time"

	"github.com/wuweidict/wudict/internal/facet"
	"github.com/wuweidict/wudict/internal/fsx"
)

// ownedFile is one text file the user owns beside wudict.toml - groups.ini,
// app.css, article.css, state.json - and what has to hold for every one of
// them, stated once:
//
//   - read bounded (fsx.ReadBounded): a pipe, a device or an oversize file is
//     refused, never read - a pipe would never end, under the lock every
//     reader waits on;
//   - cached by CONTENT: re-read at most once per recheck (0 is every use),
//     re-parsed only when the text changed, and two saves inside one mtime
//     tick (FAT's 2 s, a phone's FUSE storage) are still told apart;
//   - written atomically (fsx.WriteAtomic), the state written put in effect
//     without reading it back, saves serialised;
//   - an existing file that cannot be read is replaced only when the caller
//     says so: an editor could not show it, and a save from there would
//     destroy it.
//
// An empty path is a file with nowhere to live: its value is kept in memory,
// which is what a home-less environment and the tests need.
type ownedFile[T any] struct {
	name string        // for messages: "groups.ini"
	path func() string // resolved per use: the server's folder is set after New
	max  int64
	// recheck is how stale a hand edit may be on its next use: a file read
	// once per dictionary row (groups.ini) waits a second, one read once per
	// page (a stylesheet) is read every time, so a save-and-reload shows it.
	recheck time.Duration
	perm    fs.FileMode                            // for a new file
	parse   func(text string) (T, []facet.Problem) // never fails: problems say what was skipped
	absent  func() T                               // in effect with no usable file

	mu      sync.Mutex
	loaded  bool
	checked time.Time
	cur     fileState[T]
}

// fileState is the file in effect, parsed.
type fileState[T any] struct {
	val      T
	text     string
	exists   bool   // a file is there (usable or not)
	why      string // non-empty: it exists and cannot be used; absent() is in effect
	problems []facet.Problem
}

func (st fileState[T]) unusable() bool { return st.why != "" }

// errUnusable refuses a save over a file that cannot be read (replace=false).
var errUnusable = errors.New("cannot be read")

// now is the file in effect, re-read at most once per recheck.
func (f *ownedFile[T]) now() fileState[T] {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getLocked(false)
}

// fresh is now without the recheck delay: what is on disk this moment.
func (f *ownedFile[T]) fresh() fileState[T] {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getLocked(true)
}

func (f *ownedFile[T]) getLocked(force bool) fileState[T] {
	p := f.path()
	if p == "" {
		if !f.loaded {
			f.cur, f.loaded = fileState[T]{val: f.absent()}, true
		}
		return f.cur
	}
	if f.loaded && !force && time.Since(f.checked) < f.recheck {
		return f.cur
	}
	f.checked = time.Now()
	text, exists, why := f.read(p)
	if !f.loaded || exists != f.cur.exists || why != f.cur.why || text != f.cur.text {
		f.cur, f.loaded = f.state(text, exists, why), true
	}
	return f.cur
}

// read is what is on disk at p: its text, whether it exists, and why it
// cannot be used when it cannot.
func (f *ownedFile[T]) read(p string) (text string, exists bool, why string) {
	b, err := fsx.ReadBounded(p, f.max)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", false, ""
	case errors.Is(err, fsx.ErrNotRegular), errors.Is(err, fsx.ErrTooLarge):
		return "", true, fmt.Sprintf("%s is not a text file of at most %d KB", f.name, f.max>>10)
	case err != nil:
		return "", true, "cannot read " + f.name + ": " + err.Error()
	}
	return string(b), true, ""
}

func (f *ownedFile[T]) state(text string, exists bool, why string) fileState[T] {
	switch {
	case !exists:
		return fileState[T]{val: f.absent()}
	case why != "":
		return fileState[T]{val: f.absent(), exists: true, why: why, problems: []facet.Problem{{Msg: why}}}
	}
	v, probs := f.parse(text)
	return fileState[T]{val: v, text: text, exists: true, problems: probs}
}

// save writes text and puts it in effect. A file there now that cannot be
// read is replaced only with replace, else errUnusable (wrapped with why).
func (f *ownedFile[T]) save(text string, replace bool) (fileState[T], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.path()
	if p != "" {
		if _, exists, why := f.read(p); exists && why != "" && !replace {
			return f.cur, fmt.Errorf("%s - %w", why, errUnusable)
		}
		if err := fsx.WriteAtomic(p, []byte(text), f.perm); err != nil {
			return f.cur, err
		}
	}
	f.cur, f.loaded, f.checked = f.state(text, true, ""), true, time.Now()
	return f.cur, nil
}

// remove deletes the file; absent() is in effect again.
func (f *ownedFile[T]) remove() (fileState[T], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p := f.path(); p != "" {
		if err := fsx.RemoveIfExists(p); err != nil {
			return f.cur, err
		}
	}
	f.cur, f.loaded, f.checked = f.state("", false, ""), true, time.Now()
	return f.cur, nil
}
