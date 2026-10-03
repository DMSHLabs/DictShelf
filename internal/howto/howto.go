// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package howto is the wudict howto: the app's own guide, written as a wudict
// markdown dictionary (docs/WUDICT-MARKDOWN.md) and built into the binary.
//
// It ships as a dictionary rather than as pages because that is the proof: the
// guide is searched, linked, browsed and styled by the same code as any
// dictionary a user adds. Every headword starts with "wudict ", so the guide
// never answers a lookup meant for the user's own dictionaries.
//
// The server lists it from a copy in the app's own folder (Install), under a
// fixed id so links such as /browse?dict=wudict-howto work on every machine.
// A user who wants to edit it puts a copy in a dictionary folder (CopyTo); that
// copy then stands in for the built-in one.
package howto

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/wuweidict/wudict/internal/fsx"
)

const (
	// FileName is the guide's file name; a dictionary folder holding a file of
	// this name has its own copy, which hides the built-in one.
	FileName = "wudict-howto.wudict.md"
	// FilesDir holds the guide's images, beside it (spec R2.3).
	FilesDir = "wudict-howto.wudict.files"
	// ID is the built-in guide's dictionary id, the same on every machine.
	ID = "wudict-howto"
	// removedMark, beside the built-in guide, records that the user removed
	// it: the app then stops writing it back at every start, until Restore.
	removedMark = "wudict-howto.removed"
)

//go:embed wudict-howto.wudict.md wudict-howto.wudict.files
var files embed.FS

// Source returns the guide's markdown.
func Source() []byte {
	b, _ := files.ReadFile(FileName)
	return b
}

// Install writes the guide and its images into dir, rewriting only a file
// whose content differs - so an upgrade that changed the guide changes the
// file, and the prepared dictionary is rebuilt from it, while an unchanged
// guide is never touched. Images the guide no longer has are removed. It
// returns the guide's path.
func Install(dir string) (string, error) {
	if err := os.MkdirAll(filepath.Join(dir, FilesDir), 0o755); err != nil {
		return "", err
	}
	want := map[string]bool{}
	err := fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		want[p] = true
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return writeIfChanged(filepath.Join(dir, filepath.FromSlash(p)), b)
	})
	if err != nil {
		return "", err
	}
	if ents, err := os.ReadDir(filepath.Join(dir, FilesDir)); err == nil {
		for _, e := range ents {
			if !e.IsDir() && !want[path.Join(FilesDir, e.Name())] {
				os.Remove(filepath.Join(dir, FilesDir, e.Name()))
			}
		}
	}
	return filepath.Join(dir, FileName), nil
}

// IsRemoved reports whether the user removed the built-in guide from dir.
func IsRemoved(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, removedMark))
	return err == nil
}

// MarkRemoved records that the user removed the built-in guide from dir, and
// deletes whatever of it is still there: the app then no longer installs it.
func MarkRemoved(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeIfChanged(filepath.Join(dir, removedMark), []byte("removed by the user; Setup brings it back\n")); err != nil {
		return err
	}
	os.Remove(filepath.Join(dir, FileName))
	return os.RemoveAll(filepath.Join(dir, FilesDir))
}

// Restore undoes MarkRemoved: the guide is installed again and stays.
func Restore(dir string) (string, error) {
	if err := fsx.RemoveIfExists(filepath.Join(dir, removedMark)); err != nil {
		return "", err
	}
	return Install(dir)
}

// CopyTo puts an editable copy of the guide and its images into a dictionary
// folder. It never overwrites: a guide already there is the user's, and
// fs.ErrExist says so.
func CopyTo(dir string) (string, error) {
	dst := filepath.Join(dir, FileName)
	if _, err := os.Lstat(dst); err == nil {
		return "", fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dir, FilesDir), 0o755); err != nil {
		return "", err
	}
	// The images first, the guide last: a scan that sees the guide sees it
	// whole.
	err := fs.WalkDir(files, FilesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		return writeIfChanged(filepath.Join(dir, filepath.FromSlash(p)), b)
	})
	if err != nil {
		return "", err
	}
	return dst, writeIfChanged(dst, Source())
}

// writeIfChanged writes b to p through a temporary file and a rename, unless
// p already holds exactly b.
func writeIfChanged(p string, b []byte) error {
	if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, b) {
		return nil
	}
	return fsx.WriteAtomic(p, b, 0o644)
}
