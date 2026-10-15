# Agent handoff — current state
Checkout: branch `dev2` at `5a156d6`; merge in progress from `portable` at `dfcba90`.

## Current work
The merge keeps the Windows desktop build as one GUI `wuDict2.exe`; `build-windows.cmd console` builds the same executable with a console subsystem for diagnostics. Windows tray launch detection uses the executable subsystem and falls back to the browser for console runs. The native WebView bridge shares Go services for search, dictionary listing, indexing, reindexing, ingest and progress; HTTP remains available to the server and browser UI.

Windows WebView2 marks every document at creation, including pages reached through in-app navigation, so desktop layout and local appearance storage stay active across pages. The `← Words` return link appears in the top bar before `wuDict2`; the duplicate status-bar link stays hidden. Dictionary selection uses the same in-page group, dictionary and search/filter dialog in browser, Windows WebView2 and Android WebView; Android's old native dictionary dialog and prompt route are removed.

In Dictionary Groups, the member and available-dictionary panels stay visible. When either panel is active, they split the available height evenly if each half fits ten rows; otherwise the active panel grows toward ten rows while preserving one row in the other panel.

The Android WebView marks itself explicitly before calling the paper appearance bridge. The page sends `field:"paper"` only for Android; Windows appearance requests use the page's local desktop store. The Windows WebView2 host also intercepts `wudict:appearance` prompts from restored older pages and routes them to the desktop appearance store before a native JavaScript dialog appears.

Windows portable mode uses `Data` beside the executable when it contains `Dictionaries` and `wudict2.toml`. It stores the database and lemmas under `Data`, resolves relative paths there, and rewrites their configured paths on startup. A legacy `wudict.toml` is copied to `wudict2.toml` when needed. Copied prepared indexes are relinked to dictionary copies by filename, size and source fingerprint, so moving a portable folder does not require rebuilding them. Invalid portable data reports the problem and falls back to the regular user data folder. Windows uses `wudict2.toml`; other platforms retain `wudict.toml`.

Windows migration from `~/.wudict` to `~/.wuDict2` carries wuDict2's prepared database, logs, lemmas, styles and settings, and rewrites paths in the imported config. Upstream-only index data is not imported.

Newly added dictionaries always queue the mandatory headword index, even before optional index defaults are saved. Saved defaults select only Contains and full-text. Search does not build a missing index; it skips that dictionary. The focused tests cover both rules.

Fork-specific dictionary groups, mask search, migration and Android identity remain enabled. Upstream how-to and wildcard features remain unwired as recorded in `AGENTS.md`.

## Verification
The portable merge preview passes `go build ./...` and `build-windows.cmd`; `internal/config` and `internal/cli` tests pass. Store tests fail in `TestPrefix` and `TestDefaultPlanOmitsTrigram`, and the server suite retains known asynchronous-index, DSL-backend and fixture-cleanup failures. Portable startup and index relinking have not been exercised at runtime. The Windows appearance popup fix was confirmed by the user. Android picker behavior remains unverified on a device.

## Remaining manual check
On Windows, verify portable startup with prepared dictionaries and confirm the indexes are reused after copying `Data`. Check the shared dictionary picker on Android.
