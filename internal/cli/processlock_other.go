// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package cli

import (
	"errors"

	"github.com/DMSHLabs/DictShelf/internal/server"
)

func acquireProcessLock() (func(), error) { return nil, nil }

func prepareWindowsDataRoot() error { return nil }

func legacyMigrationStatus() server.FirstRunMigration { return server.FirstRunMigration{} }

func applyLegacyMigration(string, *server.Registry, func(), string) error {
	return errors.New("Windows data migration is unavailable on this platform")
}
