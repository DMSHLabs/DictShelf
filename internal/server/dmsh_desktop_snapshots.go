// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import "github.com/wuweidict/wudict/internal/facet"

func (s *Server) groupsSnapshot() map[string]any {
	return groupsResponse(s.User.Groups(), s.groupsNow())
}

func groupsResponse(file string, st fileState[*facet.Rules]) map[string]any {
	problems := st.problems
	if problems == nil {
		problems = []facet.Problem{}
	}
	text := st.text
	if !st.exists {
		text = facet.DefaultText
	}
	return map[string]any{
		"text":     text,
		"custom":   st.exists,
		"unusable": st.unusable(),
		"file":     file,
		"writable": file != "",
		"problems": problems,
		"facets":   st.val.Facets(),
	}
}
