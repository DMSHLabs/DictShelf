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

// ExportMDX saves the pinned collection as an MDX and, when needed, a companion
// MDD. Existing files are never replaced. Both unfinished files are removed on
// failure or cancellation.
func (s *Store) ExportMDX(ctx context.Context, folder string) (mdxPath, mddPath string, err error) {
	snap, err := s.OpenSnapshot()
	if err != nil {
		return "", "", err
	}
	defer snap.Close()
	articles := snap.Articles()
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
	name := snap.Name()
	about, err := s.About()
	if err != nil {
		return "", "", err
	}
	mdxEntries := make([]mdictwrite.Entry, 0, len(articles))
	mddEntries := make([]mdictwrite.Entry, 0)
	for _, article := range articles {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		article := article
		assets := map[string]string{}
		for _, asset := range article.Assets {
			asset := asset
			if !validObjectHash(asset.Hash) {
				return "", "", fmt.Errorf("invalid resource in %q", article.Headword)
			}
			base := path.Base(strings.ReplaceAll(asset.Name, "\\", "/"))
			if base == "" || base == "." || base == "/" {
				base = "resource"
			}
			base = strings.ReplaceAll(base, "\\", "_")
			rel := "_favourites/" + article.ID + "/" + base
			assets[asset.URL] = rel
			object := filepath.Join(s.Objects(), asset.Hash)
			st, statErr := os.Stat(object)
			if statErr != nil {
				return "", "", statErr
			}
			mddEntries = append(mddEntries, mdictwrite.Entry{Key: "\\" + strings.ReplaceAll(rel, "/", "\\"), Size: st.Size(), Open: func() (io.ReadCloser, error) {
				f, openErr := os.Open(object)
				if openErr != nil {
					return nil, openErr
				}
				return contextReader{ctx, f}, nil
			}})
		}
		body, bodyErr := snap.Body(article)
		if bodyErr != nil {
			return "", "", bodyErr
		}
		rewrite := func(body string) string {
			return (htmlref.Rewriter{URL: func(ref htmlref.Ref) string {
				if rel, ok := assets[ref.URL]; ok {
					return rel
				}
				return ref.URL
			}}).Rewrite(body)
		}
		size := len(rewrite(body)) + 1
		mdxEntries = append(mdxEntries, mdictwrite.Entry{Key: article.Headword, Size: int64(size), Open: func() (io.ReadCloser, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			body, err := snap.Body(article)
			if err != nil {
				return nil, err
			}
			return contextReader{ctx, io.NopCloser(bytes.NewReader(append([]byte(rewrite(body)), 0)))}, nil
		}})
	}
	// Reserve a pair under the same base name. The exclusive create protects
	// older exports even when another export starts in the same folder.
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
				os.Remove(mdxPath)
				continue
			}
			if err != nil {
				mdxFile.Close()
				os.Remove(mdxPath)
				return "", "", err
			}
		}
		break
	}
	if mdxFile == nil {
		return "", "", errors.New("no available export filename")
	}
	defer func() {
		mdxFile.Close()
		if mddFile != nil {
			mddFile.Close()
		}
		if err != nil {
			os.Remove(mdxPath)
			if mddFile != nil {
				os.Remove(mddPath)
			}
		}
	}()
	if err = mdictwrite.Write(mdxFile, name, about, false, mdxEntries); err != nil {
		return "", "", err
	}
	if err = mdxFile.Sync(); err != nil {
		return "", "", err
	}
	if mddFile != nil {
		if err = mdictwrite.Write(mddFile, name, about, true, mddEntries); err != nil {
			return "", "", err
		}
		if err = mddFile.Sync(); err != nil {
			return "", "", err
		}
	} else {
		mddPath = ""
	}
	return mdxPath, mddPath, nil
}

func safeExportName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "Favourites"
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	return name
}
