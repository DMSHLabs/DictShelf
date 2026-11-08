// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// DesktopArticleFindNavigation makes the same navigation operation available
// to WebViews whose secure origin has no HTTP API fallback.
func (s *Server) DesktopArticleFindNavigation(ctx context.Context, query string) (any, error) {
	if _, err := url.ParseQuery(query); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/search?"+query, nil)
	if err != nil {
		return nil, err
	}
	response := newDesktopResourceResponse(nil)
	s.handleArticleFindNavigation(response, req)
	var body any
	if err := json.Unmarshal(response.body.Bytes(), &body); err != nil {
		return nil, err
	}
	return map[string]any{"status": response.status, "body": body}, nil
}
