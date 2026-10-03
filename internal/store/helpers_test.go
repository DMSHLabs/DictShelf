// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"

	"github.com/wuweidict/wudict/internal/dict"
)

var bg = context.Background()

// ingestFull prepares with headwords and full text, the plan most of these
// tests assert against.
func ingestFull(r dict.Reader, dbPath string) error {
	_, err := IngestPlan(r, dbPath, Plan{FullText: true}, nil)
	return err
}
