// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later
package dsl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/wuweidict/wudict/internal/dict"
	"github.com/wuweidict/wudict/internal/store"
	"os"
	"path/filepath"
)

// Generate legacy receipts only for migration/reference regression fixtures.
func gdComparison(p string) (string, error) {
	p = dict.CanonPath(p)
	stamp, err := gdStamp(p)
	if err != nil {
		return "", err
	}
	s := gdSource{Source: p, Stamp: stamp}
	if ab, ok := dict.AbbrevCompanion(p); ok {
		s.Abbrev, err = gdStamp(ab)
		if err != nil {
			return "", err
		}
	}
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	id := sha256.Sum256([]byte(p))
	name := store.FolderName(p) + " GD-" + hex.EncodeToString(id[:6]) + ".dslgd"
	dir := filepath.Join(store.DefaultDBDir(), ".dsl-gd")
	target := filepath.Join(dir, name)
	if _, err := os.Stat(target + ".disabled"); err == nil {
		return "", nil
	}
	if previous, err := os.ReadFile(target); err == nil && bytes.Equal(previous, data) {
		return target, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".comparison-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", err
	}
	return target, nil
}
