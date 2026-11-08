// Copyright (C) 2026 glowinthedark
// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// Removing a prepared dictionary (D63). The file manager is no answer on
// Android, where the library lives in app-private external storage that no
// file manager may open - so the app is the only process on the device that
// can free those bytes.
//
// This is the whole file-deleting surface of that feature, kept in one place
// and behind one guard: **the folder's parent must be the library root**.
// Everything directly under the db dir is ours by construction (D20: one
// dictionary is one folder), and nothing else may be passed here - the caller
// resolves a dictionary to a folder, this refuses anything that is not one.

// RemovePrepared deletes one prepared dictionary's folder and reports the
// bytes it freed.
//
// Guards, in order: the path must resolve to a direct child of the library
// root; it must be a real directory, not a symlink (which would let a link
// planted in the library delete a tree outside it); and it must not be the
// library root itself. The folder's contents are not inspected - a folder with
// no readable text.db is exactly the incomplete/interrupted case `clean`
// reports, and refusing to remove it would leave the one thing a user most
// wants gone.
func RemovePrepared(dir string) (int64, error) {
	if dir == "" {
		return 0, fmt.Errorf("no folder given")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	abs = filepath.Clean(abs)

	root := DefaultDBDir()
	if dict.SameDir(abs, root) {
		return 0, fmt.Errorf("%s is the library itself, not a dictionary in it", abs)
	}
	// Compared canonically on the PARENT: the folder is about to be deleted,
	// so resolving symlinks through it is both pointless and racy, while its
	// parent must genuinely be the library however it was spelled (~, a
	// symlinked home, a case-variant volume).
	if !dict.SameDir(filepath.Dir(abs), root) {
		return 0, fmt.Errorf("%s is not in the library (%s)", abs, root)
	}
	// Lstat, not Stat: a symlink named like a dictionary folder must be
	// rejected rather than followed, because os.RemoveAll on the link would
	// delete the link but a caller could not tell that from success - and the
	// library never contains one.
	fi, err := os.Lstat(abs)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		return 0, fmt.Errorf("%s is not a prepared dictionary folder", abs)
	}

	n := TreeSize(abs)
	// Moved out of the way first, then deleted (removalInfix, dmsh_ingest_sweep.go).
	// Deleting a folder file by file can fail in the middle, which leaves a
	// dictionary with its text.db gone and its caches not - a dictionary that can
	// neither be used nor removed cleanly. A MOVE either happens or does not, so:
	//
	//   - a folder that cannot be moved (something inside it is held, which on
	//     Windows blocks a rename exactly as it blocks an unlink) is left exactly
	//     as it was, and the error says which folder it is;
	//   - a folder that was moved but not deleted leaves its contents under a
	//     debris name, which nothing lists, `clean` reports and the startup sweep
	//     removes.
	//
	// Either way the slot the library knows is never half-deleted.
	debris, err := renameOutOfTheWay(abs)
	if err != nil {
		return 0, fmt.Errorf("removing %s: %w", abs, err)
	}
	// removeFolder, not os.RemoveAll: Windows refuses to delete a file anything
	// holds open, and holds are transient there far more often than they mean
	// anything (removeall_windows.go).
	if err := removeFolder(debris); err != nil {
		return 0, fmt.Errorf("removing %s: %w", abs, err)
	}
	return n, nil
}

// TreeSize sums the regular files under path (path itself, if it is a file).
// Unreadable subtrees are skipped: this figure is reported to a user as "space
// freed", so an incomplete answer is better than no answer.
func TreeSize(path string) int64 {
	fi, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !fi.IsDir() {
		return fi.Size()
	}
	var n int64
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Mode().IsRegular() {
			n += info.Size()
		}
		return nil
	})
	return n
}

// renameOutOfTheWay moves a prepared folder to its debris name beside itself and
// returns the new path, or the folder it could not move. The rename is the half
// of a removal that cannot be interrupted half way (removalInfix).
func renameOutOfTheWay(abs string) (string, error) {
	to := removalDebrisName(abs)
	if err := moveFolder(abs, to); err != nil {
		return "", err
	}
	return to, nil
}
