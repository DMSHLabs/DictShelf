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
// to WebViews whose secure origin has no HTTP API fallback. baseURL is the
// address the bridge's page requests use, so the session's own page loads key
// the page cache exactly as the reader's copies do - the page's own origin is a
// different host on this transport.
func (s *Server) DesktopArticleFindNavigation(ctx context.Context, baseURL, query string) (any, error) {
	values, err := url.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	if base, ok := desktopOrigin(baseURL); ok {
		values.Set("origin", base.Scheme+"://"+base.Host)
		query = values.Encode()
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
