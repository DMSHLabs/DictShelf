// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/DMSHLabs/DictShelf/internal/store"
)

func TestStoreUsesTextAndMediaDatabasesAsCanonicalStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "fav")
	set := New(root)
	if err := set.EnsureNamed("Saved words"); err != nil {
		t.Fatal(err)
	}

	dictionary, err := store.Open(set.Path())
	if err != nil {
		t.Fatal(err)
	}
	caps := dictionary.Caps()
	if !caps.Contains || !caps.FTS {
		t.Fatalf("empty favourites indexes = %+v, want Contains and full-text enabled", caps)
	}
	dictionary.Close()

	article, err := set.Add(Provenance{DictionaryID: "source", DictionaryName: "Source", Headword: "apple"},
		`<p>fruit</p><img src="apple.png">`, []AssetInput{{URL: "apple.png", Name: "apple.png", MIME: "image/png", Data: []byte("image-bytes")}})
	if err != nil {
		t.Fatal(err)
	}
	if article.EntryID != 1 || article.BodySize == 0 {
		t.Fatalf("stored article = %+v", article)
	}
	if !set.Has("source", "apple") {
		t.Fatal("stored article metadata is not searchable by source")
	}
	body, err := set.Body(article)
	if err != nil || body != `<p>fruit</p><img src="apple.png">` {
		t.Fatalf("stored body = %q, %v", body, err)
	}
	token, ok := set.AssetToken(article.EntryID, "apple.png")
	if !ok {
		t.Fatal("stored image resource token is missing")
	}
	resource, mime, err := set.OpenAsset(token)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resource)
	resource.Close()
	if err != nil || mime != "image/png" || string(data) != "image-bytes" {
		t.Fatalf("stored resource = %q (%q), %v", data, mime, err)
	}

	for _, name := range []string{store.TextDBName, store.MediaDBName} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("canonical %s is missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "objects")); !os.IsNotExist(err) {
		t.Fatalf("obsolete objects directory status = %v", err)
	}

	if err := set.Remove(article.ID); err != nil {
		t.Fatal(err)
	}
	articles, err := set.Manifest()
	if err != nil || len(articles) != 0 {
		t.Fatalf("articles after removal = %+v, %v", articles, err)
	}
	if _, err := os.Stat(filepath.Join(root, store.MediaDBName)); !os.IsNotExist(err) {
		t.Fatalf("empty collection retained media.db: %v", err)
	}
}
