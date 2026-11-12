// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/DMSHLabs/DictShelf/internal/config"
	"github.com/DMSHLabs/DictShelf/internal/favorites"
	dslformat "github.com/DMSHLabs/DictShelf/internal/format/dsl"
	"github.com/DMSHLabs/DictShelf/internal/indexconst"
	"github.com/DMSHLabs/DictShelf/internal/logx"
	"github.com/DMSHLabs/DictShelf/internal/morph"
	"github.com/DMSHLabs/DictShelf/internal/search"
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
	started := time.Now()
	markStartup := func(stage string, since time.Time) {
		logx.SystemStartupTiming("stage=%s duration_ms=%d elapsed_ms=%d", stage,
			time.Since(since).Milliseconds(), time.Since(started).Milliseconds())
	}
	stageStarted := time.Now()
	if configPath != "" {
		if absolute, err := filepath.Abs(configPath); err == nil {
			configPath = absolute
		}
	}
	if n := config.MaxProcs(); n > 0 {
		searchWorkersBaseline = n
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
	if err := prefs.recordAppVersion(version); err != nil {
		logx.Warn("could not record app version in %s: %v", StateFile, err)
	}
	if err := config.RecordAppVersion(userPath, version); err != nil {
		logx.Warn("could not record app version in %s: %v", config.Name, err)
	}
	mode, modeSaved := prefs.indexingMode()
	modeSource := "state"
	if !modeSaved {
		mode = defaultIndexingModeFor(runtime.GOOS)
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
	markStartup("runtime_user_state", stageStarted)
	stageStarted = time.Now()
	name := "Favourites"
	if prefs.ResolvedLanguage("") == "ru" {
		name = "Избранное"
	}
	favSets, err := favorites.OpenCollections(user.Favorites(), name)
	if err != nil {
		return nil, fmt.Errorf("provision favourites collections: %w", err)
	}
	collections, err := favSets.List()
	if err != nil {
		return nil, fmt.Errorf("list favourites collections: %w", err)
	}
	var builtins []Builtin
	for _, collection := range collections {
		set, ok := favSets.Store(collection.ID)
		if !ok {
			return nil, fmt.Errorf("favourites collection %q is unavailable", collection.ID)
		}
		builtins = append(builtins, Builtin{ID: collection.ID, Path: set.Path()})
	}
	markStartup("runtime_favourites", stageStarted)
	stageStarted = time.Now()
	reg, err := NewRegistry(cfg.DictDirs, cfg.UseCached,
		WithPrefs(prefs), WithBuiltin(builtins...))
	if err != nil {
		return nil, err
	}
	markStartup("runtime_registry", stageStarted)
	stageStarted = time.Now()
	reg.SetPreviewBudget(cfg.PreviewMemory)
	s := New(reg)
	s.favSets = favSets
	s.favourites, _ = favSets.Store(favorites.ID)
	s.UseUserGroups()
	s.ConfigPath = configPath
	s.User = user
	s.wildcardPages.setLimit(resultCacheLimitBytes(reg.prefs.UI()))
	s.wildcardPages.init(s.wildcardCacheDir())
	s.applySearchParallelismPrefs()
	s.applyNearWordsPref()
	s.InstallDesktopBackgrounds()
	s.Morph = morph.New(cfg.MorphCache, cfg.LemmaDir)
	if cfg.MemoryLimit > 0 {
		SetMemoryLimit(cfg.MemoryLimit)
		logx.V("memory limit: %d MB", cfg.MemoryLimit>>20)
	}
	s.Version = version
	if err := s.recordUserStorageAppVersion(); err != nil {
		logx.Warn("could not record app version in %s and %s: %v", SettingsFile, SearchHistoryFile, err)
	}
	markStartup("runtime_services", stageStarted)
	s.DictDirOrigin = cfg.Origin("DICT_DIR")
	s.DictDirEditable = cfg.EditableInFile("DICT_DIR")
	s.ConfigProblems = cfg.Problems
	s.Effective = cfg.Effective()
	s.LemmaDir, s.LemmaURL = cfg.LemmaDir, cfg.LemmaURL
	s.ImportKeep = cfg.ImportKeep
	s.ImportURLHosts = cfg.ImportURLHosts
	s.ImportInsecure = cfg.ImportInsecure
	s.AllowRemoteDelete = cfg.AllowRemoteDelete
	s.FindTrace = cfg.FindTrace
	s.BrowserExtensions = cfg.BrowserExtensions
	s.WebOrigins = cfg.WebOrigins
	s.TrustedHosts = cfg.TrustedHosts
	s.AuthToken = token
	return &Runtime{Server: s, Registry: reg, User: user}, nil
}
