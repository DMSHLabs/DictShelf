// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DMSHLabs/DictShelf/internal/fsx"
	"github.com/DMSHLabs/DictShelf/internal/logx"
)

const (
	SettingsFile        = "settings.json"
	SearchHistoryFile   = "history.json"
	userStorageVersion  = 1
	maxUserStorageBytes = 2 << 20
)

type userStorageDocument struct {
	Version    int               `json:"version"`
	AppVersion string            `json:"appVersion,omitempty"`
	Values     map[string]string `json:"values"`
}

var userStorageHistoryKeys = map[string]bool{
	"wudict_search_history":    true,
	"wudict_wildcards_history": true,
}

var userStorageTransientKeys = map[string]bool{
	"wudict_order":                  true,
	"wudict_native_storage_seed_v1": true,
}

// userStorageMu serializes settings/history snapshots and the first-run
// import. The settings file is written last during import and acts as its
// completion marker, so an interrupted two-file write can safely be retried.
var userStorageMu sync.Mutex

func (s *Server) userStoragePaths() (string, string) {
	if s.User == "" {
		return "", ""
	}
	return filepath.Join(string(s.User), SettingsFile), filepath.Join(string(s.User), SearchHistoryFile)
}

func (s *Server) userStorageExists() (bool, error) {
	settingsPath, _ := s.userStoragePaths()
	if settingsPath == "" {
		return false, nil
	}
	_, err := os.Stat(settingsPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *Server) userStorageFilesExist() (bool, error) {
	settingsPath, historyPath := s.userStoragePaths()
	if settingsPath == "" {
		return false, nil
	}
	for _, path := range []string{settingsPath, historyPath} {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

// userStorageSnapshot reads the versioned documents and combines them into
// the legacy key/value shape consumed by the current UI during migration.
func (s *Server) userStorageSnapshot() (map[string]string, error) {
	settingsPath, historyPath := s.userStoragePaths()
	userStorageMu.Lock()
	defer userStorageMu.Unlock()
	settings, err := readUserStorage(settingsPath)
	if err != nil {
		return nil, err
	}
	history, err := readUserStorage(historyPath)
	if err != nil {
		return nil, err
	}
	for key, value := range history {
		settings[key] = value
	}
	if s.reg != nil && s.reg.prefs != nil {
		if mode, ok := s.reg.prefs.indexingMode(); ok {
			settings["wudict_indexing_mode"] = mode
		}
	}
	return settings, nil
}

func (s *Server) userStorageBootstrap() (string, string) {
	exists, _ := s.userStorageFilesExist()
	values, err := s.userStorageSnapshot()
	blocked := err != nil
	if err != nil {
		logx.Warn("settings files could not be read; preserving current WebView storage: %v", err)
		values = map[string]string{}
		exists = false
	}
	data, _ := json.Marshal(struct {
		Exists  bool              `json:"exists"`
		Blocked bool              `json:"blocked"`
		Values  map[string]string `json:"values"`
	}{Exists: exists, Blocked: blocked, Values: values})
	script := `<script>(function(){
var started=performance.now();
window.__wudictStorageBootstrapStartMs=started;
if(window.__wudictMigrationExporting)return;
var data=` + string(data) + `,store=window.localStorage;
var androidStorage=!!(window.wudictAndroidGo&&typeof window.wudictAndroidGo.userStorage==="function");
function callAndroidStorage(method,argument){var result=JSON.parse(window.wudictAndroidGo.userStorage(method,argument||""));if(!result||!result.ok)throw new Error(result&&result.error||"settings bridge failed");return result.value}
function storageRows(){var rows=[];for(var i=0;i<store.length;i++){var key=store.key(i);rows.push([key,store.getItem(key)])}return rows}
if(androidStorage&&!data.blocked){try{var state=callAndroidStorage("storageState","");if(state&&state.exists){data.values=callAndroidStorage("storageRead","");data.exists=true}else{callAndroidStorage("storageImport",JSON.stringify(storageRows()));data.values=callAndroidStorage("storageRead","");data.exists=true}}catch(error){data.blocked=true;console.error("Could not migrate Android settings",error)}}
if(data.exists){
  for(var i=store.length-1;i>=0;i--){var k=store.key(i);if(k&&k.indexOf("wudict_")===0)store.removeItem(k)}
  Object.keys(data.values||{}).forEach(function(k){store.setItem(k,data.values[k])});
}
window.__wudictStorageBootstrapMs=Math.round(performance.now()-started);
window.__wudictStorageRestored=data.exists;
window.__wudictSettingsReady=false;
var p=Storage.prototype,put=p.setItem,del=p.removeItem,clear=p.clear,t;
function persist(){if(!window.__wudictSettingsReady||data.blocked)return;clearTimeout(t);t=setTimeout(function(){try{var raw=JSON.stringify(storageRows());if(androidStorage){callAndroidStorage("storageWrite",raw);return}if(typeof window.wudictNativeCall==="function")window.wudictNativeCall("storageWrite",raw).catch(function(e){console.error("Could not save settings",e)});else fetch("/api/user-storage",{method:"PUT",headers:{"Content-Type":"application/json"},body:raw}).then(function(r){if(!r.ok)throw new Error("HTTP "+r.status)}).catch(function(e){console.error("Could not save settings",e)})}catch(e){console.error("Could not save settings",e)}},200)}
p.setItem=function(k,v){put.call(this,k,v);if(this===store)persist()};
p.removeItem=function(k){del.call(this,k);if(this===store)persist()};
p.clear=function(){clear.call(this);if(this===store)persist()};
window.__wudictSettingsReady=true;
if(!data.exists&&!data.blocked)persist();
if(typeof window.wudictStorageSeeded==="function")window.wudictStorageSeeded().catch(function(e){console.error("Could not mark settings migration",e)})
})();</script>`
	return script, assetTag([]byte(script))
}

func (s *Server) renderUserUI(page, language string) []byte {
	return s.renderUserUIChoice(page, language, language)
}

func (s *Server) renderUserUIChoice(page, language, choice string) []byte {
	storageScript, _ := s.userStorageBootstrap()
	page = strings.ReplaceAll(page, "{{USERSTORAGE}}", storageScript)
	return []byte(renderUIChoice(page, language, choice))
}

func (s *Server) handleUserStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		values, err := s.userStorageSnapshot()
		if err != nil {
			httpErr(w, http.StatusInternalServerError, "could not read settings: %v", err)
			return
		}
		writeJSON(w, values)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<20))
	if err != nil {
		httpErr(w, http.StatusRequestEntityTooLarge, "settings snapshot is too large")
		return
	}
	if err := s.saveUserStorage(string(body)); err != nil {
		httpErr(w, http.StatusBadRequest, "could not save settings: %v", err)
		return
	}
	writeJSON(w, map[string]bool{"saved": true})
}

// importLegacyStorage moves durable localStorage values into independent
// settings and history files. Existing settings always win; the old profile
// is never allowed to overwrite data already adopted by this installation.
func (s *Server) importLegacyStorage(raw string) error {
	return s.importLegacyStorageContext(context.Background(), raw)
}

func (s *Server) importLegacyStorageContext(ctx context.Context, raw string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(raw) > 8<<20 {
		return errors.New("legacy storage export is too large")
	}
	var rows [][]string
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return fmt.Errorf("invalid legacy storage export: %w", err)
	}
	settingsPath, historyPath := s.userStoragePaths()
	if settingsPath == "" {
		return errors.New("settings folder is unavailable")
	}
	userStorageMu.Lock()
	defer userStorageMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(settingsPath); err == nil {
		if _, readErr := readUserStorage(settingsPath); readErr != nil {
			return fmt.Errorf("could not read existing %s without replacing it: %w", SettingsFile, readErr)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("could not inspect existing %s without replacing it: %w", SettingsFile, err)
	}

	settings := make(map[string]string)
	history := make(map[string]string)
	for _, row := range rows {
		if len(row) != 2 || !strings.HasPrefix(row[0], "wudict_") || userStorageTransientKeys[row[0]] {
			continue
		}
		if userStorageHistoryKeys[row[0]] {
			history[row[0]] = row[1]
		} else {
			settings[row[0]] = row[1]
		}
	}
	if err := writeUserStorage(historyPath, history, s.Version); err != nil {
		return fmt.Errorf("could not save %s: %w", SearchHistoryFile, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := writeUserStorage(settingsPath, settings, s.Version); err != nil {
		return fmt.Errorf("could not save %s: %w", SettingsFile, err)
	}
	return nil
}

// saveUserStorage persists a complete durable localStorage snapshot. History
// entries remain in their own file. Ephemeral display state and migration
// markers are omitted; diagnostic preferences are durable across restarts.
func (s *Server) saveUserStorage(raw string) error {
	if len(raw) > 8<<20 {
		return errors.New("settings snapshot is too large")
	}
	var rows [][]string
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return fmt.Errorf("invalid settings snapshot: %w", err)
	}
	settingsPath, historyPath := s.userStoragePaths()
	if settingsPath == "" {
		return errors.New("settings folder is unavailable")
	}
	userStorageMu.Lock()
	defer userStorageMu.Unlock()
	for _, path := range []string{settingsPath, historyPath} {
		if _, err := os.Stat(path); err == nil {
			if _, err := readUserStorage(path); err != nil {
				return fmt.Errorf("refusing to replace unreadable settings/history file: %w", err)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("could not inspect settings/history file: %w", err)
		}
	}
	settings := make(map[string]string)
	history := make(map[string]string)
	for _, row := range rows {
		if len(row) != 2 || !strings.HasPrefix(row[0], "wudict_") || userStorageTransientKeys[row[0]] {
			continue
		}
		if userStorageHistoryKeys[row[0]] {
			history[row[0]] = row[1]
		} else {
			settings[row[0]] = row[1]
		}
	}
	if err := writeUserStorage(historyPath, history, s.Version); err != nil {
		return fmt.Errorf("could not save %s: %w", SearchHistoryFile, err)
	}
	if err := writeUserStorage(settingsPath, settings, s.Version); err != nil {
		return fmt.Errorf("could not save %s: %w", SettingsFile, err)
	}
	return nil
}

func readUserStorage(path string) (map[string]string, error) {
	values := make(map[string]string)
	if path == "" {
		return values, nil
	}
	b, err := fsx.ReadBounded(path, maxUserStorageBytes)
	if errors.Is(err, fsx.ErrNotRegular) || errors.Is(err, fsx.ErrTooLarge) {
		return nil, fmt.Errorf("invalid settings file: %w", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}
	var document userStorageDocument
	if err := json.Unmarshal(b, &document); err != nil {
		return nil, fmt.Errorf("invalid settings file: %w", err)
	}
	if document.Version != userStorageVersion {
		return nil, fmt.Errorf("unsupported settings file version %d", document.Version)
	}
	for key, value := range document.Values {
		if strings.HasPrefix(key, "wudict_") && !userStorageTransientKeys[key] {
			values[key] = value
		}
	}
	return values, nil
}

func writeUserStorage(path string, values map[string]string, appVersion string) error {
	if path == "" {
		return errors.New("settings folder is unavailable")
	}
	document := userStorageDocument{Version: userStorageVersion, AppVersion: appVersion, Values: values}
	b, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(b)) > maxUserStorageBytes {
		return errors.New("settings exceed the file size limit")
	}
	return fsx.WriteAtomic(path, b, 0o600)
}

// recordUserStorageAppVersion updates existing snapshots without creating
// settings.json before Android's legacy-localStorage import can complete.
func (s *Server) recordUserStorageAppVersion() error {
	if s.Version == "" {
		return nil
	}
	settingsPath, historyPath := s.userStoragePaths()
	userStorageMu.Lock()
	defer userStorageMu.Unlock()
	for _, path := range []string{settingsPath, historyPath} {
		if path == "" {
			continue
		}
		data, err := fsx.ReadBounded(path, maxUserStorageBytes)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var document userStorageDocument
		if err := json.Unmarshal(data, &document); err != nil {
			return fmt.Errorf("invalid settings file %s: %w", filepath.Base(path), err)
		}
		if document.Version != userStorageVersion {
			return fmt.Errorf("unsupported settings file version %d in %s", document.Version, filepath.Base(path))
		}
		if document.AppVersion == s.Version {
			continue
		}
		document.AppVersion = s.Version
		updated, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return err
		}
		if int64(len(updated)) > maxUserStorageBytes {
			return fmt.Errorf("%s exceeds the file size limit", filepath.Base(path))
		}
		if err := fsx.WriteAtomic(path, updated, 0o600); err != nil {
			return err
		}
	}
	return nil
}
