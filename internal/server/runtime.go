// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"path/filepath"

	"github.com/wuweidict/wudict/internal/config"
	dslformat "github.com/wuweidict/wudict/internal/format/dsl"
	"github.com/wuweidict/wudict/internal/indexconst"
	"github.com/wuweidict/wudict/internal/logx"
	"github.com/wuweidict/wudict/internal/morph"
	"github.com/wuweidict/wudict/internal/search"
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
	if configPath != "" {
		if absolute, err := filepath.Abs(configPath); err == nil {
			configPath = absolute
		}
	}
	if n := config.MaxProcs(); n > 0 {
		SetActiveProcs(n)
		search.SetWorkers(n)
		logx.V("parallelism: GOMAXPROCS=%d, search fan-out=%d", n, n)
	}
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
	mode, modeSaved := prefs.indexingMode()
	modeSource := "state"
	if !modeSaved {
		mode = "pipeline"
		modeSource = "default"
		if userDir != "" {
			legacySettings, readErr := readUserStorage(filepath.Join(userDir, SettingsFile))
			if readErr != nil {
				logx.Warn("could not restore indexing mode from %s: %v", SettingsFile, readErr)
			} else if legacyMode := legacySettings["wudict_indexing_mode"]; validIndexingMode(legacyMode) {
				mode = legacyMode
				modeSource = "settings"
			}
		}
		if err := prefs.setIndexingMode(mode); err != nil {
			logx.Warn("could not persist indexing mode: %v", err)
		}
	}
	if _, busy := indexconst.SetMode(mode); busy {
		return nil, fmt.Errorf("cannot restore indexing mode while dictionary indexing is active")
	}
	logx.System("indexing mode restored mode=%s source=%s", mode, modeSource)
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
	s.wildcardPages.setLimit(resultCacheLimitBytes(reg.prefs.UI()))
	s.wildcardPages.init(s.wildcardCacheDir())
	s.InstallDesktopBackgrounds()
	s.Morph = morph.New(cfg.MorphCache, cfg.LemmaDir)
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
