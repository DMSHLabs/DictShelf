# Workspace dialog refactor

## Why

Edit Filters, Edit Folders and Lemmatization currently load separate HTML pages in one reusable iframe. Opening or switching between them navigates that iframe and can show the previous page while the next one loads. Dictionary Settings, Dictionary Groups and Rescan Folders are already part of the main document, so they open without that page-navigation delay.

## Goal

Make the three editors open as dialogs from the already loaded application document. Opening or switching dialogs should not navigate an iframe or briefly display another editor. Preserve their current behavior, layout, scrolling, translations, close handling, folder picker and progress reporting.

Keep each editor maintainable in separate source files for its markup, styles and behavior where practical. Assemble/embed these resources as part of the application build; do not fetch editor pages when the user opens a dialog. Reuse the existing Go bridge and keep platform-specific behavior behind the current platform adapters so the same UI works on Windows, Android, Linux and macOS.

## Order of work

1. Finish and verify the ongoing Windows server-removal work first. Keep this refactor separate so regressions remain easy to identify.
2. Use Edit Filters as a pilot. Move its markup, styles and JavaScript into modular source files that are included in the main document at startup, then open it as an in-document dialog.
3. Verify its loading, reading and saving of `groups.ini`, translations, appearance presets, close behavior and browser fallback. Check Windows and Android first; check Linux and macOS on their supported builds.
4. Apply the proven structure to Edit Folders and Lemmatization, preserving their native folder picker and shared progress behavior.
5. Remove the old iframe route and dialog-only page branches once all three editors use the in-document implementation and the supported platforms are verified.

## Completion checks

- Opening any of the three editors does not navigate an iframe or show stale editor content.
- Dialogs appear immediately after activation; no page-load wait is visible.
- Existing save, reset, validation, indexing, folder selection, progress, scrolling, close and translation behavior is preserved.
- The implementation builds for Windows, Android, Linux and macOS, with platform-specific checks recorded where available.

## Settings and history storage

This storage change is required before the first wuDict2 release so existing users can carry their preferences forward. Keep it separate from the Windows server-removal work because it affects persistent data and all supported platforms. The versioned files, shared HTTP persistence adapter, and Windows first-run import from the previous WebView origin are implemented; explicit settings export/import and runtime verification of the migration branches remain.

### Decisions

- WebView is a presentation layer, not the authoritative store for user settings. Settings must be portable independently of the WebView profile and origin.
- Keep related JSON files together beside the app configuration, using the existing config/data directory selection (including portable mode).
- Keep `state.json` for library and dictionary state: dictionary order, groups and other state tied to the configured collection.
- Add `settings.json` for user preferences such as theme, interface layout, search behavior and reading options.
- Add `history.json` for recent searches, including wildcard searches, and future kinds of search history. Keep history separate from settings so users can export/import either independently.
- Browse caches are derived data, not history or settings; they can be rebuilt and should not be included in settings export.
- Add explicit settings export/import. Decide separately whether history export/import is also exposed; the formats should be versioned so future changes can migrate safely.

### Migration and implementation notes

- Audit persistent `localStorage` keys and classify them as settings, history, disposable cache, diagnostics or transient UI state before moving them. Do not move cache or temporary UI state into the settings file by default.
- Migrate existing user preferences and search history from WebView storage into the new files before removing their browser-storage reads/writes. Avoid losing existing data during upgrades.
- Once file-backed settings/history are loaded and saved through shared Go operations, remove the WebView-origin storage copy used during first launch if no remaining WebView-only data needs it.
- Keep browser, Windows, Android, Linux and macOS on the same file-backed Go operations; transport adapters may differ, but the persisted format and behavior should not.
