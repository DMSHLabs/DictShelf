// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "github.com/DMSHLabs/DictShelf/internal/search"

// applyNearWordsPref hands the saved "near" window to the search package. The
// resolution of "nobody chose one" and of an out-of-range number happens in
// ftsq (ResolveNear), which the archive reader and the page's hint go through
// as well, so there is one number and not three. It is applied where the
// parallelism pref is applied: once at startup, and again whenever the settings
// are saved - the reader changes this while a search may be running, and the
// next search must be the one that uses it.
func (s *Server) applyNearWordsPref() {
	if s == nil || s.reg == nil {
		return
	}
	var n int
	if ui := s.reg.prefs.UI(); ui != nil {
		n = ui.NearWords
	}
	search.SetNearWords(n)
}
