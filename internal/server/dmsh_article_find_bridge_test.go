package server

import (
	"context"
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
