// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"testing"
)

func TestAcceptsGzipReadsTheHeaderAsAList(t *testing.T) {
	cases := map[string]bool{
		"":                  false,
		"gzip":              true,
		"gzip, deflate, br": true,
		"br;q=1, gzip;q=0":  true,
		"deflate":           false,
		"x-gzip":            false,
	}
	for header, want := range cases {
		request := httptest.NewRequest("GET", "/", nil)
		request.Header.Set("Accept-Encoding", header)
		if got := acceptsGzip(request); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

func TestRemotePeerExcludesLoopbackAndEmpty(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:6888": false,
		"[::1]:6888":     false,
		"":               false,
		"192.0.2.5:5123": true,
		"203.0.113.9:80": true,
	}
	for address, want := range cases {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = address
		if got := remotePeer(request); got != want {
			t.Errorf("remotePeer(%q) = %v, want %v", address, got, want)
		}
	}
}

func TestGzippedPageRoundTripsAndIsKept(t *testing.T) {
	page := bytes.Repeat([]byte("<p>alpha beta</p>"), 2000)
	first := gzippedPage(`"abcd1234"`, page)
	second := gzippedPage(`"abcd1234"`, page)
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("the page did not compress")
	}
	if &first[0] != &second[0] {
		t.Fatal("the same ETag was compressed twice")
	}
	reader, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, page) {
		t.Fatal("the compressed page does not decode to the page")
	}
	if got := gzipPageETag(`"abcd1234"`); got != `"abcd1234-gzip"` {
		t.Fatalf("gzipPageETag = %s, want a quoted tag", got)
	}
}

func TestCompressStreamLeavesLoopbackAlone(t *testing.T) {
	local := httptest.NewRequest("GET", "/api/search", nil)
	local.RemoteAddr = "127.0.0.1:5123"
	local.Header.Set("Accept-Encoding", "gzip")
	localRecorder := httptest.NewRecorder()
	localWriter, finishLocal := compressStream(localRecorder, local)
	finishLocal()
	if localWriter != localRecorder {
		t.Fatal("a loopback stream was compressed")
	}

	remote := httptest.NewRequest("GET", "/api/search", nil)
	remote.RemoteAddr = "192.0.2.5:5123"
	remote.Header.Set("Accept-Encoding", "gzip")
	remoteRecorder := httptest.NewRecorder()
	writer, finish := compressStream(remoteRecorder, remote)
	if _, err := writer.Write([]byte("{\"t\":\"hit\"}\n")); err != nil {
		t.Fatal(err)
	}
	writer.(interface{ Flush() }).Flush()
	finish()
	if got := remoteRecorder.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	reader, err := gzip.NewReader(remoteRecorder.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "{\"t\":\"hit\"}\n" {
		t.Fatalf("decoded stream = %q", decoded)
	}
}
