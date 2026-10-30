// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build android && cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>
void wudictNotifyBusy(int busy);
*/
import "C"

import (
	"encoding/json"

	"github.com/DMSHLabs/DictShelf/internal/config"
	"github.com/DMSHLabs/DictShelf/internal/server"
)

//export WudictGoBridgeProtocol
func WudictGoBridgeProtocol() C.int { return 1 }

//export WudictGoBridgeInstallBusyObserver
func WudictGoBridgeInstallBusyObserver() {
	server.SetBusyObserver(func(busy bool) {
		if busy {
			C.wudictNotifyBusy(1)
		} else {
			C.wudictNotifyBusy(0)
		}
	})
}

type configSummary struct {
	DictDirs    int  `json:"dictDirs"`
	DBMatches   bool `json:"dbMatches"`
	SourceFound bool `json:"sourceFound"`
	Error       bool `json:"error"`
}

//export WudictGoBridgeConfigSummary
func WudictGoBridgeConfigSummary(path, flagsJSON *C.char) *C.char {
	flags := map[string]string{}
	if err := json.Unmarshal([]byte(C.GoString(flagsJSON)), &flags); err != nil {
		return C.CString(`{"error":"invalid config overrides"}`)
	}
	cfg, err := config.Load(C.GoString(path), flags)
	summary := configSummary{DictDirs: len(cfg.DictDirs), DBMatches: cfg.DBDir == flags["DB_DIR"], SourceFound: cfg.Source != "", Error: err != nil}
	data, marshalErr := json.Marshal(summary)
	if marshalErr != nil {
		return C.CString(`{"error":"cannot encode config summary"}`)
	}
	return C.CString(string(data))
}

func main() {}
