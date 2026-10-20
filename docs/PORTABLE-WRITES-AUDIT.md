# Portable write-path audit

Date: 2026-10-20. Reviewed `dev2` at `722daad4` (`Portable`).
Scope: Windows desktop startup, application-owned persistent state, temporary files, native WebView response transport, SQLite, and dictionary import/removal. Static source audit; no application launch or filesystem trace was performed. The initial audit changed no runtime source; the subsequent GUI-log fix is marked below. Go build and Windows cgo GUI build pass for that fix; its GUI path has not been checked by a filesystem trace.

## Result

The reviewed portable application does **not** write exclusively inside its executable directory. The GUI log finding was fixed after this audit: it now writes `wudict.log` under the active data root's `db.logs` (portable Data, otherwise WUDICT_HOME or `~/.wuDict2`). Several active code paths still use the system temporary directory. Allowing external dictionary roots also permits automatic cleanup and imports outside the portable tree.

## Confirmed automatic or feature-triggered writes outside the portable tree

| Trigger | Destination | Evidence |
| --- | --- | --- |
| Normal GUI startup — fixed | `<active data root>/db.logs/wudict.log` | `internal/cli/cli.go`, `openHeadlessLog` now selects WUDICT_PORTABLE_HOME, WUDICT_HOME, then `~/.wuDict2`. LOCALAPPDATA is no longer used for this log. |
| A Windows WebView response exceeding 8 MiB | System temp, `wudict2-webview-<pid>-*.tmp` | `internal/server/desktop_resource.go`, `desktopMemoryThreshold` and `desktopResourceResponse.Write`; `third_party/go-webview2/webview.go`, response spill callback; `third_party/go-webview2/pkg/edge/filestream_windows.go`, `NewFileStream` uses `os.CreateTemp("", ...)`. Normal release removes the file; creation still occurs outside. |
| Opening compressed or repaired wudict markdown | System temp, `wudict-md-*.txt` | `internal/format/wmd/source.go`, `openSource` decoding path calls `os.CreateTemp("", ...)`. Ordinary unchanged plain files are read in place. |
| External Speex decoding | System temp, `wudict-spx*` | `internal/server/server.go`, `externalSpxToWav` calls `os.MkdirTemp("", ...)`. Conditional: the normal cgo build uses the in-process decoder unless external decoding is selected. |
| Index preparation, especially large temporary tables/sorts | SQLite-selected Windows temp directory | `internal/store/ingest.go` explicitly sets `PRAGMA temp_store = FILE`. No application assignment to `sqlite3_temp_directory` or portable temp environment was found. The pinned mattn SQLite amalgamation's Windows `winGetTempname` uses `GetTempPathW` absent an override. This is a native SQLite path, so changing only Go `CreateTemp` callers would not fix it. |
| Startup with external dictionary roots containing abandoned imports | External `<dictionary root>/.wudict-intake` | `internal/server/dmsh_runtime_library.go`, `RecoverLibrary` calls `intake.Sweep(cfg.DictDirs)`; `internal/intake/stage.go`, `Sweep` removes stale staging trees. External dictionaries are therefore not strictly read-only even at startup. |
| Import when the first configured dictionary root is external | External dictionary root, downloads/spool/staging and installed dictionary files | `internal/server/intake.go`, `importDir` returns `s.reg.Dirs()[0]`; URL/path/chunk imports use that destination. `internal/intake/stage.go` deliberately places staging/spool inside the destination. There is no portable containment check here. |

## Paths already directed inside Data in the normal validated launch

- Configuration: `Data/wudict2.toml`; startup selects `WUDICT_HOME` and `CONFIG_PATH` there.
- State, groups, interface settings, search history, styles, appearance presets, looks and uploaded style assets: derived from `Server.User`, the active configuration directory (`internal/server/runtime.go`, `userdir.go`, `userstorage.go`, `style.go`, `userfiles.go`, `prefs.go`, `dmsh_groups.go`, `looks.go`, `presets.go`).
- Prepared dictionary library: `Data/db`; lemma installs and their partial downloads: `Data/lemmas`. Startup normalization sets these paths and validates their lexical containment.
- System logs and Go/native crash reports/dumps: `Data/db.logs`, derived from the selected library (`internal/logx/system.go`, `crash.go`, `crash_native_windows.go`, `internal/store/indexlog.go`). The separate GUI log now also lives in the active data root's db.logs.
- Wildcards cache: `Data/cache/wildcards` because `Server.User` is set (`internal/server/wildcard_cache.go`). Its system-cache/temp fallback applies when User is absent.
- Decoded Speex cache: `Data/db/spxcache`; the separate external-decoder working directory remains outside.
- WebView user-data profile: `Data/WebView2` (`internal/cli/desktop_window_windows.go`). This verifies the profile path supplied by the app, not every write performed internally by the OS or WebView runtime.
- The process lock is a Windows named mutex, not a file or registry value. The explicit Windows registry access reviewed reads the system theme with QUERY_VALUE.

## Additional ways containment can be lost

1. `prepareWindowsDataRoot` handles an existing `WUDICT_HOME` before portable detection. An inherited external override can select ordinary data despite Data beside the executable.
2. Invalid portable data produces a warning and falls back to `~/.wuDict2`; startup does not fail closed. The app can then write normal per-user data.
3. `validateWindowsPortableConfig` loads without CLI flag values; `cmdServe` subsequently applies `-db-dir`. The effective library path is not rechecked for portable containment after that override.
4. `windowsPathWithin` checks lexical paths, without resolving junctions/symlinks. A Data/db or other writable subdirectory redirected outside can pass lexical checks and write elsewhere.
5. Explicit operations may write outside: exporting headwords, creating another portable copy, CLI dump/resource output and deleting original dictionary files. These must have a defined exception policy if “only inside” is intended as a strict rule. Legacy `.dslgd` removal also writes a `.disabled` marker beside the source.

## Required changes for application-owned automatic writes to stay inside

- Completed: route the GUI log into the active data root's db.logs.
- Establish Data/Temp before opening runtime services. Route Go temporary files, COM response spools and native SQLite temporary files there, including child-process temp configuration.
- Treat external dictionary roots as read-only in portable mode: do not sweep them; choose an internal import destination independently of the first search root.
- Validate the final effective writable paths after flags/environment; resolve junctions. Decide whether invalid portable data must stop instead of switching to ordinary mode.
- Keep user-selected export destinations separate from application-owned state; define permitted exceptions explicitly.

A subsequent isolated Windows filesystem trace is needed to verify native WebView/SQLite behavior and the final implementation. No claim is made that Windows itself leaves no traces (for example crash handling, antivirus or OS execution history).
