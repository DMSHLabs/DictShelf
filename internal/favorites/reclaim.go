// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var writerLocks sync.Map

func (s *Store) lockWriter() (*fileLock, error) {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return nil, err
	}
	key, err := filepath.Abs(s.root)
	if err != nil {
		key = s.root
	}
	value, _ := writerLocks.LoadOrStore(strings.ToLower(filepath.Clean(key)), &sync.Mutex{})
	local := value.(*sync.Mutex)
	local.Lock()
	lease, ok, err := openFileLock(filepath.Join(s.root, "writer.lock"), lockExclusive, true)
	if err != nil || !ok {
		local.Unlock()
		if err == nil {
			err = os.ErrInvalid
		}
		return nil, err
	}
	lease.local = local
	return lease, nil
}

// Collect reclaims versions and objects that no open snapshot can still read.
// Failures are best-effort: publication and serving the current version do not
// depend on successful cleanup, and a later collection retries them.
func (s *Store) Collect() {
	s.mu.Lock()
	defer s.mu.Unlock()
	writer, err := s.lockWriter()
	if err != nil {
		return
	}
	defer writer.Close()
	current, err := s.Current()
	if err != nil {
		return
	}
	s.reclaimLocked(current, true)
}

func (s *Store) reclaimLocked(current string, sweepObjects bool) {
	versionsDir := filepath.Join(s.root, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return
	}
	liveObjects := map[string]struct{}{}
	safeToSweep := true
	mark := func(rev string) {
		m, err := readManifest(versionsDir, rev)
		if err != nil {
			safeToSweep = false
			return
		}
		for _, a := range m.Articles {
			if !validObjectHash(a.BodyHash) {
				safeToSweep = false
			} else {
				liveObjects[a.BodyHash] = struct{}{}
			}
			for _, asset := range a.Assets {
				if !validObjectHash(asset.Hash) {
					safeToSweep = false
				} else {
					liveObjects[asset.Hash] = struct{}{}
				}
			}
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() || !validRevision(entry.Name()) {
			continue
		}
		rev := entry.Name()
		if rev == current {
			mark(rev)
			continue
		}
		dir := filepath.Join(versionsDir, rev)
		leasePath := filepath.Join(s.root, "leases", rev+".lock")
		lease, acquired, lockErr := openFileLock(leasePath, lockExclusive, false)
		if lockErr != nil || !acquired {
			// A reader owns this immutable version. Preserve its objects too.
			mark(rev)
			continue
		}
		manifestPath := filepath.Join(dir, "manifest.json")
		m, readErr := readManifest(versionsDir, rev)
		removeErr := os.RemoveAll(dir)
		_, statErr := os.Stat(manifestPath)
		_ = lease.Close()
		if statErr != nil && os.IsNotExist(statErr) {
			_ = os.Remove(leasePath)
		}
		if readErr == nil && (removeErr != nil || statErr == nil) {
			for _, a := range m.Articles {
				if !validObjectHash(a.BodyHash) {
					safeToSweep = false
				} else {
					liveObjects[a.BodyHash] = struct{}{}
				}
				for _, asset := range a.Assets {
					if !validObjectHash(asset.Hash) {
						safeToSweep = false
					} else {
						liveObjects[asset.Hash] = struct{}{}
					}
				}
			}
		} else if readErr != nil && (removeErr != nil || statErr == nil) {
			safeToSweep = false
		}
	}
	s.reclaimOrphanLeases(current)

	if !sweepObjects || !safeToSweep {
		return
	}
	objects, err := os.ReadDir(s.Objects())
	if err != nil {
		return
	}
	for _, object := range objects {
		if object.IsDir() || !validObjectHash(object.Name()) {
			continue
		}
		if _, keep := liveObjects[object.Name()]; !keep {
			_ = os.Remove(filepath.Join(s.Objects(), object.Name()))
		}
	}
}

func (s *Store) reclaimOrphanLeases(current string) {
	leasesDir := filepath.Join(s.root, "leases")
	entries, err := os.ReadDir(leasesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		rev := strings.TrimSuffix(entry.Name(), ".lock")
		if !validRevision(rev) || rev == current {
			continue
		}
		if _, err := os.Stat(filepath.Join(s.root, "versions", rev)); err == nil {
			continue
		}
		lease, acquired, err := openFileLock(filepath.Join(leasesDir, entry.Name()), lockExclusive, false)
		if err != nil || !acquired {
			continue
		}
		_ = lease.Close()
		_ = os.Remove(filepath.Join(leasesDir, entry.Name()))
	}
}

func readManifest(versionsDir, rev string) (manifest, error) {
	b, err := readManifestBytes(filepath.Join(versionsDir, rev, "manifest.json"))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{}, err
	}
	if m.Version != Version || m.Revision != rev {
		return manifest{}, os.ErrInvalid
	}
	return m, nil
}

func validRevision(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

func validObjectHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
