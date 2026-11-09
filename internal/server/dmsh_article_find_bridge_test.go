package server

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopArticleFindNavigationDispatch(t *testing.T) {
	server := &Server{}
	result, err := server.DesktopJSONContext(context.Background(), "", "articleFindNavigation", "cacheAction=navClose&cacheID=abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	response := result.(map[string]any)
	if response["status"] != 200 {
		t.Fatalf("bridge status = %v; want 200", response["status"])
	}
}

// The bridge has no Host on its request, so the page cache's origin must come
// from the address its page requests use - not from the page's own origin,
// which is a different host in the desktop window.
func TestDesktopNavigationUsesTheBridgesOrigin(t *testing.T) {
	dir := t.TempDir()
	source := "#NAME \"Bridge\"\n\nalpha\n\talpha beta\n"
	if err := os.WriteFile(filepath.Join(dir, "bridge.dsl"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	isolatedDBDir(t)
	registry, err := NewRegistry([]string{dir}, false, WithComparisons(false))
	if err != nil {
		t.Fatal(err)
	}
	waitNewDictionaryIndexing(t, registry)
	closeBackends(t, registry)
	entries := registry.all()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want one", len(entries))
	}
	server := New(registry)
	server.User = UserDir(t.TempDir())
	query := url.Values{
		"cacheAction": {"navStart"}, "cacheID": {"abcdef0123456789"},
		"dict": {entries[0].ID}, "q": {"zzzznomatch"}, "mode": {"fts"}, "resultMode": {"phrase"},
		"cacheCount": {"1"}, "cacheOrders": {"0"}, "pageSize": {"20"}, "firstSize": {"5"}, "rungs": {"phrase"},
		"origin": {"https://dictshelf.localhost"},
	}
	if _, err := server.DesktopArticleFindNavigation(context.Background(), "http://127.0.0.1:6888", query.Encode()); err != nil {
		t.Fatal(err)
	}
	server.articleFindNavigationMu.Lock()
	session := server.articleFindNavigationSessions["abcdef0123456789"]
	server.articleFindNavigationMu.Unlock()
	if session == nil {
		t.Fatal("the bridge did not start a session")
	}
	if session.origin != "http://127.0.0.1:6888" {
		t.Fatalf("session origin = %q; want the bridge address", session.origin)
	}
}
