// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"os"
	"runtime"
	"testing"
)

func TestInstallDesktopBackgrounds(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows background installation")
	}
	s := &Server{User: UserDir(t.TempDir())}
	s.InstallDesktopBackgrounds()
	for _, name := range [...]string{"paper_01.jpg", "paper_02.jpg", "paper_03.jpg"} {
		got, err := os.ReadFile(s.userFilePath(name))
		if err != nil {
			t.Fatal(err)
		}
		want, _ := desktopBackgrounds.ReadFile("web/" + name)
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from Android asset", name)
		}
	}
	const replacement = "reader's wallpaper"
	if err := os.WriteFile(s.userFilePath("paper_01.jpg"), []byte(replacement), 0600); err != nil {
		t.Fatal(err)
	}
	s.InstallDesktopBackgrounds()
	got, err := os.ReadFile(s.userFilePath("paper_01.jpg"))
	if err != nil || string(got) != replacement {
		t.Fatalf("existing wallpaper was replaced: %q, %v", got, err)
	}
}
