// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/dict"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

type resourceSource struct {
	dictionary dict.Dictionary
	reader     io.ReadCloser
	mime       string
	modTime    time.Time
	override   bool
}

// openResource resolves a user override or opens the dictionary's resource.
// It contains no HTTP request or response handling, so the browser adapter and
// the in-process WebView use the same source-selection rules.
func (s *Server) openResource(e *entry, name string) (resourceSource, error) {
	if file, modTime, ok := s.openResourceOverride(e, name); ok {
		return resourceSource{reader: file, modTime: modTime, override: true}, nil
	}
	d, err := e.open()
	if err != nil {
		return resourceSource{}, err
	}
	reader, mimeType, err := d.Resource(name)
	// Resource lookup may lazily open a prepared dictionary's source file.
	s.reg.nudge()
	if err != nil {
		return resourceSource{}, err
	}
	return resourceSource{dictionary: d, reader: reader, mime: mimeType}, nil
}

// openResourceOverride returns a seekable file from the prepared dictionary's
// library folder. Invalid or absent overrides leave the embedded source in use.
func (s *Server) openResourceOverride(e *entry, name string) (*os.File, time.Time, bool) {
	textDB, ok := e.preparedDB()
	if !ok {
		return nil, time.Time{}, false
	}
	dir := filepath.Join(filepath.Dir(textDB), "res")
	rel := path.Clean("/" + name)
	if rel == "/" {
		return nil, time.Time{}, false
	}
	rel = rel[1:]
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if !within(dir, full) {
		return nil, time.Time{}, false
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, time.Time{}, false
	}
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		_ = f.Close()
		return nil, time.Time{}, false
	}
	return f, info.ModTime(), true
}

func (s *Server) logDamagedTextResource(e *entry, id, name string) {
	if _, dup := s.nulSeen.LoadOrStore(id+"\x00"+name, true); !dup {
		logx.Warn("%s%s is damaged: it contains NUL bytes and cannot parse. "+
			"The dictionary is stored that way - wudict is serving it verbatim. "+
			"Drop a good copy at <library folder>/res/%s to override it.",
			logx.Dict(e.Path), name, name)
	}
}

func (s *Server) copyTextResource(w io.Writer, e *entry, id, name string, reader io.Reader) error {
	watch := &nulWatcher{r: reader, text: isTextResource(name)}
	_, err := io.Copy(w, watch)
	if watch.seen {
		s.logDamagedTextResource(e, id, name)
	}
	return err
}
