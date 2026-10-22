// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package lsd contains the reader for compiled Lingvo LSD dictionaries.
//
// The decoder is kept separate from internal/format/dsl: LSD is a binary
// container, while DSL is the text article language both formats may feed to
// the common ingest pipeline.
package lsd
