// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestAssetTokenUnescapesURLPathExactlyOnce(t *testing.T) {
	s := New(t.TempDir())
	url := "_assets_/hash/East_Side_School%E2%80%93Old_Hammond_High_School%252C_Louisiana.jpg"
	name := "_assets_/hash/East_Side_School–Old_Hammond_High_School%2C_Louisiana.jpg"
	a, err := s.Add(Provenance{DictionaryID: "source", Headword: "Hammond"}, "<img>", []AssetInput{{
		URL: url, Name: name, Data: []byte("image"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := s.AssetToken(1, url); !ok || got != a.Assets[name].Token {
		t.Fatalf("AssetToken(%q) = %q, %t; want %q", url, got, ok, a.Assets[name].Token)
	}
	tooEncoded := strings.Replace(url, "%252C", "%25252C", 1)
	if got, ok := s.AssetToken(1, tooEncoded); ok {
		t.Fatalf("AssetToken double-encoded URL = %q, %t; want no match", got, ok)
	}
}

func TestSnapshotPinsObjectsUntilClosed(t *testing.T) {
	s := New(t.TempDir())
	article, err := s.Add(Provenance{DictionaryID: "source", Headword: "word"}, "saved body", []AssetInput{{
		URL: "photo.png", Name: "photo.png", MIME: "image/png", Data: []byte("saved image"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	oldRevision := snapshot.Revision()
	if err := s.Remove(article.ID); err != nil {
		t.Fatal(err)
	}
	if body, err := snapshot.Body(article); err != nil || body != "saved body" {
		t.Fatalf("pinned body = %q, %v", body, err)
	}
	rc, _, err := snapshot.OpenAsset(article.Assets["photo.png"].Token)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil || string(got) != "saved image" {
		t.Fatalf("pinned asset = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(s.Objects(), article.BodyHash)); err != nil {
		t.Fatalf("pinned body was reclaimed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.root, "versions", oldRevision, "manifest.json")); err != nil {
		t.Fatalf("pinned manifest was reclaimed: %v", err)
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	s.Collect()
	for _, name := range []string{article.BodyHash, article.Assets["photo.png"].Hash} {
		if _, err := os.Stat(filepath.Join(s.Objects(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("unreferenced object %s remains: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.root, "versions", oldRevision)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unreferenced version remains: %v", err)
	}
}

func TestSeparateStoresSerializeWriters(t *testing.T) {
	root := t.TempDir()
	first, second := New(root), New(root)
	if err := first.Ensure(); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, s := range []*Store{first, second} {
		go func(i int, s *Store) {
			<-start
			_, err := s.Add(Provenance{DictionaryID: fmt.Sprintf("source-%d", i), Headword: fmt.Sprintf("word-%d", i)}, "body", nil)
			results <- err
		}(i, s)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	articles, err := first.Manifest()
	if err != nil || len(articles) != 2 {
		t.Fatalf("concurrent publications retained %d articles: %v", len(articles), err)
	}
}

func TestWriterProcessHelper(t *testing.T) {
	root := os.Getenv("WUDICT_FAVORITES_WRITER_HELPER")
	if root == "" {
		return
	}
	i := os.Getenv("WUDICT_FAVORITES_WRITER_ID")
	_, err := New(root).Add(Provenance{DictionaryID: "child-" + i, Headword: "word-" + i}, "body", nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSeparateProcessesSerializeWriters(t *testing.T) {
	root := t.TempDir()
	if err := New(root).Ensure(); err != nil {
		t.Fatal(err)
	}
	cmds := make([]*exec.Cmd, 2)
	for i := range cmds {
		cmd := exec.Command(os.Args[0], "-test.run=^TestWriterProcessHelper$")
		cmd.Env = append(os.Environ(), "WUDICT_FAVORITES_WRITER_HELPER="+root, fmt.Sprintf("WUDICT_FAVORITES_WRITER_ID=%d", i))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds[i] = cmd
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("writer process: %v", err)
		}
	}
	articles, err := New(root).Manifest()
	if err != nil || len(articles) != 2 {
		t.Fatalf("process publications retained %d articles: %v", len(articles), err)
	}
}

func TestStoreCacheFollowsAnotherWritersRevision(t *testing.T) {
	root := t.TempDir()
	first, second := New(root), New(root)
	if err := first.Ensure(); err != nil {
		t.Fatal(err)
	}
	if second.Has("source", "word") {
		t.Fatal("empty store unexpectedly contains the word")
	}
	if _, err := first.Add(Provenance{DictionaryID: "source", Headword: "word"}, "body", nil); err != nil {
		t.Fatal(err)
	}
	if !second.Has("source", "word") {
		t.Fatal("cached store did not observe another writer's revision")
	}
}

func TestSnapshotProcessHelper(t *testing.T) {
	root := os.Getenv("WUDICT_FAVORITES_SNAPSHOT_HELPER")
	if root == "" {
		return
	}
	snapshot, err := New(root).OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	articles := snapshot.Articles()
	if len(articles) == 0 {
		t.Fatal("helper snapshot is empty")
	}
	if err := os.WriteFile(os.Getenv("WUDICT_FAVORITES_SNAPSHOT_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Getenv("WUDICT_FAVORITES_SNAPSHOT_RELEASE")); err == nil {
			body, err := snapshot.Body(articles[0])
			if err != nil || body != "pinned body" {
				t.Fatalf("body after publication = %q, %v", body, err)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for parent")
}

func TestOtherProcessSnapshotPinsObjects(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	article, err := s.Add(Provenance{DictionaryID: "source", Headword: "word"}, "pinned body", nil)
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "ready")
	release := filepath.Join(root, "release")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotProcessHelper$")
	cmd.Env = append(os.Environ(), "WUDICT_FAVORITES_SNAPSHOT_HELPER="+root,
		"WUDICT_FAVORITES_SNAPSHOT_READY="+ready, "WUDICT_FAVORITES_SNAPSHOT_RELEASE="+release)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile(release, []byte("release"), 0o600)
		_ = cmd.Wait()
	}()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatal("snapshot helper did not become ready")
	}
	if err := s.Remove(article.ID); err != nil {
		t.Fatal(err)
	}
	s.Collect()
	if _, err := os.Stat(filepath.Join(s.Objects(), article.BodyHash)); err != nil {
		t.Fatalf("other process's pinned object was reclaimed: %v", err)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	s.Collect()
	if _, err := os.Stat(filepath.Join(s.Objects(), article.BodyHash)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unleased object remains after helper exit: %v", err)
	}
}
