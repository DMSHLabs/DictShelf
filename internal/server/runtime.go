// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"path/filepath"

	"github.com/wuweidict/wudict/internal/config"
	dslformat "github.com/wuweidict/wudict/internal/format/dsl"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/morph"
	"github.com/wuweidict/wudict/internal/search"
	"github.com/wuweidict/wudict/internal/store"
)

// Runtime is the application state shared by the HTTP adapter and native
// platform bridges. It owns one registry and one Server for the lifetime of
// the application process.
type Runtime struct {
	Server   *Server
	Registry *Registry
	User     UserDir
}

// NewRuntime creates the application services without starting a listener.
// Callers may expose Server through HTTP, a native bridge, or both; those
// transports must share this Runtime rather than opening a second registry.
// ConfigureLibrary and RecoverLibrary run first, in that order.
func NewRuntime(cfg config.Config, configPath, token, version string) (*Runtime, error) {
	if n := config.MaxProcs(); n > 0 {
		SetActiveProcs(n)
		search.SetWorkers(n)
		logx.V("parallelism: GOMAXPROCS=%d, search fan-out=%d", n, n)
	}
	SetIndexWorkers(cfg.IndexWorkers)
	userPath := configPath
	if userPath == "" {
		userPath = config.HomeConfig()
	}
	userDir := ""
	if userPath != "" {
		userDir = filepath.Dir(userPath)
	}
	user := UserDir(userDir)
	prefs := LoadPrefs(user.State())
	dslRepairs := true
	if saved, ok := prefs.dslTitleRepairs(); ok {
		dslRepairs = saved
	}
	dslformat.SetTitleRepairs(dslRepairs)
	reg, err := NewRegistry(cfg.DictDirs, cfg.UseCached,
		WithPrefs(prefs))
	if err != nil {
		return nil, err
	}
	reg.SetPreviewBudget(cfg.PreviewMemory)
	reg.SetSearchBudget(cfg.SearchMemory)
	s := New(reg)
	s.UseUserGroups()
	s.ConfigPath = configPath
	s.User = user
	s.InstallDesktopBackgrounds()
	s.Morph = morph.New(cfg.MorphCache, cfg.LemmaDir)
	store.SetCompressBodies(!cfg.NoCompress)
	if cfg.MemoryLimit > 0 {
		SetMemoryLimit(cfg.MemoryLimit)
		logx.V("memory limit: %d MB", cfg.MemoryLimit>>20)
	}
	s.Version = version
	s.DictDirOrigin = cfg.Origin("DICT_DIR")
	s.DictDirEditable = cfg.EditableInFile("DICT_DIR")
	s.ConfigProblems = cfg.Problems
	s.Effective = cfg.Effective()
	s.LemmaDir, s.LemmaURL = cfg.LemmaDir, cfg.LemmaURL
	s.ImportKeep = cfg.ImportKeep
	s.ImportURLHosts = cfg.ImportURLHosts
	s.ImportInsecure = cfg.ImportInsecure
	s.AllowRemoteDelete = cfg.AllowRemoteDelete
	s.BrowserExtensions = cfg.BrowserExtensions
	s.WebOrigins = cfg.WebOrigins
	s.TrustedHosts = cfg.TrustedHosts
	s.AuthToken = token
	return &Runtime{Server: s, Registry: reg, User: user}, nil
}
