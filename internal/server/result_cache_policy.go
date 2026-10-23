// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"runtime"

	"github.com/wuweidict/wudict/internal/indexconst"
)

func defaultResultCacheLimitGB() int {
	if runtime.GOOS == "android" {
		return indexconst.ResultCacheAndroidDefaultGB
	}
	return indexconst.ResultCacheDesktopDefaultGB
}

func resultCacheLimitBytes(ui *UIPrefs) int64 {
	if ui != nil && ui.ResultCacheNoLimit {
		return 0
	}
	gb := defaultResultCacheLimitGB()
	if ui != nil && ui.ResultCacheLimitGB > 0 {
		gb = min(ui.ResultCacheLimitGB, 1<<20)
	}
	return int64(gb) << 30
}

func (s *Server) uiPrefsSnapshot() *UIPrefs {
	ui := s.reg.prefs.UI()
	if ui == nil {
		ui = &UIPrefs{}
	}
	if ui.ResultCacheLimitGB < 1 {
		ui.ResultCacheLimitGB = defaultResultCacheLimitGB()
	}
	if ui.CombinedDictionaryLimit < 1 {
		ui.CombinedDictionaryLimit = indexconst.CombinedDictionaryLimitDefault
	} else {
		ui.CombinedDictionaryLimit = min(ui.CombinedDictionaryLimit, indexconst.CombinedDictionaryLimitMax)
	}
	return ui
}

func (s *Server) applyResultCachePrefs() {
	if s == nil {
		return
	}
	s.wildcardPages.setLimit(resultCacheLimitBytes(s.reg.prefs.UI()))
}
