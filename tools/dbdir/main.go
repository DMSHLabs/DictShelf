// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Command dbdir prints the library folder (DB_DIR) as wudict resolves it -
// environment, wudict.toml, else the default - so a Makefile target acts on
// the folder the app uses rather than on a copy of the default.
package main

import (
	"fmt"
	"os"

	"github.com/wuweidict/wudict/internal/config"
	"github.com/wuweidict/wudict/internal/store"
)

func main() {
	cfg, err := config.Load("", nil)
	if err != nil {
		// Never a guess: the caller deletes what this prints.
		fmt.Fprintln(os.Stderr, "dbdir:", err)
		os.Exit(1)
	}
	store.SetDBDir(cfg.DBDir)
	fmt.Println(store.DefaultDBDir())
}
