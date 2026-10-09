// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Exit is a desktop action on this machine. A loopback socket alone is not
// enough: a browser pointed at a re-bound name can also reach that socket.
func localExitHost(host string) bool {
	name := hostName(host)
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleDesktopExit(w http.ResponseWriter, r *http.Request) {
	if s.DesktopExit == nil {
		httpErr(w, http.StatusNotFound, "desktop Exit is unavailable")
		return
	}
	if !isLoopback(r) || !localExitHost(r.Host) {
		httpErr(w, http.StatusForbidden, "Exit is available only on this machine")
		return
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if err != nil || origin.Scheme != scheme || !strings.EqualFold(origin.Host, r.Host) ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.User != nil {
		httpErr(w, http.StatusForbidden, "Exit requires this page's origin")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
	go s.DesktopExit()
}
