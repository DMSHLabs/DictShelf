// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"embed"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed web/paper_01.jpg web/paper_02.jpg web/paper_03.jpg
var desktopBackgrounds embed.FS

// Android installs these same backgrounds in style/assets. Keep existing
// files, including replacements made by the reader, when installing on Windows.
func (s *Server) InstallDesktopBackgrounds() {
	dir := s.userFilesDir()
	if runtime.GOOS != "windows" || dir == "" {
		return
	}
	for _, name := range [...]string{"paper_01.jpg", "paper_02.jpg", "paper_03.jpg"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			continue
		}
		data, err := desktopBackgrounds.ReadFile("web/" + name)
		if err != nil || os.MkdirAll(dir, 0700) != nil {
			continue
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			continue
		}
		if _, err = f.Write(data); err != nil {
			f.Close()
			os.Remove(path)
			continue
		}
		f.Close()
	}
}
