// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"errors"
	"io"
	"os"
	"testing"
)

func TestAddPublishesImmutableSnapshotAndResource(t *testing.T) {
	s := New(t.TempDir())
	p := Provenance{DictionaryID: "source-id", DictionaryName: "Original", Headword: "word"}
	a, err := s.Add(p, "<p>verbatim</p>", []AssetInput{{URL: "img/p.png", Name: "p.png", MIME: "image/png", Data: []byte("image-bytes")}})
	if err != nil {
		t.Fatal(err)
	}
	body, err := s.Body(a)
	if err != nil || body != "<p>verbatim</p>" {
		t.Fatalf("body = %q, %v", body, err)
	}
	assets, err := s.ResourceTokens()
	if err != nil || len(assets) != 1 || assets[0] != a.Assets["p.png"].Token {
		t.Fatalf("tokens = %v, %v", assets, err)
	}
	rc, mime, err := s.OpenAsset(assets[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(b) != "image-bytes" || mime != "image/png" {
		t.Fatalf("asset = %q, %q, %v", b, mime, err)
	}
	if _, err := s.Add(p, "duplicate", nil); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}
	if err := s.Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	articles, err := s.Manifest()
	if err != nil || len(articles) != 0 {
		t.Fatalf("manifest after remove = %d, %v", len(articles), err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("empty snapshot should remain provisioned: %v", err)
	}
}

func TestFailedAddLeavesPublishedManifestUntouched(t *testing.T) {
	s := New(t.TempDir())
	p := Provenance{DictionaryID: "source-id", DictionaryName: "Original", Headword: "word"}
	a, err := s.Add(p, "old", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(Provenance{DictionaryID: "source-id", Headword: "too-big"}, string(make([]byte, MaxArticle+1)), nil); err == nil {
		t.Fatal("oversized body accepted")
	}
	articles, err := s.Manifest()
	if err != nil || len(articles) != 1 || articles[0].ID != a.ID {
		t.Fatalf("published state changed after failure: %+v, %v", articles, err)
	}
}

func TestAssetTokensAreScopedToSnapshot(t *testing.T) {
	s := New(t.TempDir())
	asset := func(body string) []AssetInput {
		return []AssetInput{{URL: "image.png", Name: "image.png", Data: []byte(body)}}
	}
	a, err := s.Add(Provenance{DictionaryID: "one", Headword: "same"}, "one", asset("first"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Add(Provenance{DictionaryID: "two", Headword: "same"}, "two", asset("second"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Assets["image.png"].Token == b.Assets["image.png"].Token {
		t.Fatal("resource token is not article-scoped")
	}
	for token, want := range map[string]string{a.Assets["image.png"].Token: "first", b.Assets["image.png"].Token: "second"} {
		rc, _, err := s.OpenAsset(token)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil || string(got) != want {
			t.Fatalf("token %q = %q, %v, want %q", token, got, err, want)
		}
	}
}
