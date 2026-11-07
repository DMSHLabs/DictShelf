// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package favorites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/DMSHLabs/DictShelf/internal/htmlref"
	"github.com/DMSHLabs/DictShelf/internal/mdictwrite"
)

type contextReader struct {
	context.Context
	io.ReadCloser
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.Context.Err(); err != nil {
		return 0, err
	}
	return r.ReadCloser.Read(p)
}

// ExportMDX writes the saved articles and resources from the collection's
// prepared SQLite databases. Existing files are never replaced. Both unfinished
// files are removed on failure or cancellation.
//
// title names the dictionary inside the export and on its filename; empty takes
// the collection's own name. progress, when given, is called with the units done
// and the total: two per article, because preparing an entry and writing it both
// cost the same read of the body.
func (s *Store) ExportMDX(ctx context.Context, folder, title string, progress func(done, total int)) (mdxPath, mddPath string, err error) {
	articles, err := s.Manifest()
	if err != nil {
		return "", "", err
	}
	if len(articles) == 0 {
		return "", "", errors.New("the favourites collection is empty")
	}
	info, err := os.Stat(folder)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", errors.New("export path is not a folder")
	}
	name := strings.TrimSpace(title)
	if name == "" {
		name, err = s.Name()
		if err != nil {
			return "", "", err
		}
	}
	about, err := s.About()
	if err != nil {
		return "", "", err
	}
	var progressDone atomic.Int64
	total := int64(2 * len(articles))
	report := func() {
		if progress != nil {
			progress(int(progressDone.Load()), int(total))
		}
	}
	step := func() {
		progressDone.Add(1)
		report()
	}
	mdxEntries := make([]mdictwrite.Entry, 0, len(articles))
	mddEntries := make([]mdictwrite.Entry, 0)
	for _, article := range articles {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		article := article
		assetPaths := map[string]string{}
		for _, asset := range article.Assets {
			asset := asset
			base := path.Base(strings.ReplaceAll(asset.Name, "\\", "/"))
			if base == "" || base == "." || base == "/" {
				base = "resource"
			}
			rel := "_favourites/" + article.ID + "/" + path.Base(asset.Token) + path.Ext(base)
			assetPaths[asset.URL] = rel
			size, err := s.AssetSize(asset.Token)
			if err != nil {
				return "", "", err
			}
			token := asset.Token
			mddEntries = append(mddEntries, mdictwrite.Entry{Key: "\\" + strings.ReplaceAll(rel, "/", "\\"), Size: size, Open: func() (io.ReadCloser, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				reader, _, err := s.OpenAsset(token)
				if err != nil {
					return nil, err
				}
				return contextReader{Context: ctx, ReadCloser: reader}, nil
			}})
		}
		body, err := s.Body(article)
		if err != nil {
			return "", "", err
		}
		rewrite := func(body string) string {
			return (htmlref.Rewriter{URL: func(ref htmlref.Ref) string {
				if rel, ok := assetPaths[ref.URL]; ok {
					return rel
				}
				return ref.URL
			}}).Rewrite(body)
		}
		value := append([]byte(rewrite(body)), 0)
		mdxEntries = append(mdxEntries, mdictwrite.Entry{Key: article.Headword, Size: int64(len(value)), Open: func() (io.ReadCloser, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			step()
			return contextReader{Context: ctx, ReadCloser: io.NopCloser(bytes.NewReader(value))}, nil
		}})
		step()
	}
	var mdxFile, mddFile *os.File
	for n := 1; n <= 10000; n++ {
		base := safeExportName(name)
		if n > 1 {
			base = fmt.Sprintf("%s (%d)", base, n)
		}
		mdxPath = filepath.Join(folder, base+".mdx")
		mddPath = filepath.Join(folder, base+".mdd")
		if _, statErr := os.Stat(mddPath); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", "", statErr
		}
		mdxFile, err = os.OpenFile(mdxPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if len(mddEntries) > 0 {
			mddFile, err = os.OpenFile(mddPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if errors.Is(err, os.ErrExist) {
				mdxFile.Close()
				_ = os.Remove(mdxPath)
				mdxFile = nil
				continue
			}
			if err != nil {
				mdxFile.Close()
				_ = os.Remove(mdxPath)
				return "", "", err
			}
		}
		break
	}
	if mdxFile == nil {
		return "", "", errors.New("no available export filename")
	}
	defer func() {
		_ = mdxFile.Close()
		if mddFile != nil {
			_ = mddFile.Close()
		}
		if err != nil {
			_ = os.Remove(mdxPath)
			if mddFile != nil {
				_ = os.Remove(mddPath)
			}
			mdxPath, mddPath = "", ""
		}
	}()
	if err = mdictwrite.Write(mdxFile, name, about, false, mdxEntries); err != nil {
		return
	}
	if err = mdxFile.Sync(); err != nil {
		return
	}
	if mddFile != nil {
		if err = mdictwrite.Write(mddFile, name, about, true, mddEntries); err != nil {
			return
		}
		if err = mddFile.Sync(); err != nil {
			return
		}
	} else {
		mddPath = ""
	}
	if err = ctx.Err(); err != nil {
		return
	}
	return mdxPath, mddPath, nil
}

func safeExportName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "Favourites"
	}
	first := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if first == "CON" || first == "PRN" || first == "AUX" || first == "NUL" || (len(first) == 4 && (strings.HasPrefix(first, "COM") || strings.HasPrefix(first, "LPT")) && first[3] >= '1' && first[3] <= '9') {
		name = "_" + name
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	return name
}
