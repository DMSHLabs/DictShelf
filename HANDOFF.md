# Agent handoff — current state

Current checkout: branch `DictShelf` at `9ebe8c34`, merging `dev` at `f7bda1e3` (`show/hide icons`). The `DictShelf` remote was deleted at the user request. The merge is in progress and is not committed. DictShelf identity and icon assets remain in this branch; LSD embedded-icon lookup and `_abrev.lsd` companion handling are part of the current implementation.
The incoming changes add saved dictionary-icon visibility, safer Edit Folders startup behavior, and removal of empty known MDX ad placeholders during prepared indexing (Reader version 2). Go tests/build pass; Windows UI and MDX reindex behavior still need runtime confirmation.

## Product identity

- Product: DictShelf; publisher: DMSH Labs.
- Android application ID, Java package and Gradle namespace: `com.dmshlabs.dictshelf`. The external lookup component is `com.dmshlabs.dictshelf.lookup.LookupActivity`. JNI exports use the new Java package.
- Windows desktop, CLI and server artifacts: `DictShelf.exe`, `DictShelf-cli.exe`, `DictShelf-server.exe`. The installer has its own GUID, install directory, ProgID, shortcuts and WebView2 profile, so the prior app can remain installed.
- macOS bundle ID and launchd label: `com.dmshlabs.dictshelf`; Linux user unit: `dictshelf.service`.
- Desktop settings, cache and library default root: `~/.dmshlabs/dictshelf`; config file: `dictshelf.toml`. macOS GUI logs are under `~/Library/Logs/DMSH Labs/DictShelf/`.
- Startup no longer offers the old Windows automatic data migration. Old installations and their files remain untouched. Explicit settings import/export is the agreed transfer method but is not implemented yet.
- Go module path, Go package names, environment prefix, DB-format prefix and Android native library names remain technical `wudict` identifiers.

## Verification in this identity change

- `go test ./...` passed after the final path and localization changes.
- `build-android.cmd debug intel` built the FOSS debug APK with arm64 and x86_64. The APK reports `com.dmshlabs.dictshelf.debug`; FOSS and Play Java compilation passed. No APK was installed.
- `build-windows.cmd` built and ran `DictShelf.exe`; `build-windows.cmd installer` compiled `dist/dictshelf-windows-x64-setup-1.01.0.exe`. The installer offers `%LOCALAPPDATA%\Programs\DMSH Labs\DictShelf` for per-user installation.
- `git diff --check` passed. Linux/macOS artifacts were reviewed statically but cannot be built on this Windows host.
- System Log export `Build:` versions replace the legacy `wudict2-` tag prefix with `DictShelf-`; targeted `internal/server` tests pass.
- Linux/macOS artifacts were reviewed statically but cannot be built on this Windows host.

## Architecture and open checks

- Android runs the Go server in-process through JNI and opens no HTTP listener. On this machine, the x86_64 emulator cannot run translated arm64 Go binaries; use `build-android.cmd debug intel` for device/emulator builds.
- Windows listenerless desktop uses WebView2 and a direct native Go bridge. The browser and explicit server modes still use HTTP. The current secure desktop origin is `https://dictshelf.localhost`.
- Browse and Wildcards retain SQLite-backed paged result caches; merged Wildcards maintains source positions and per-view cache identities. Rebuilt runtime behavior on Windows/Android still needs manual checks.
- Remaining broad UI checks are listed in the old session notes; prioritize Windows startup/list/Exit/tray, System Log export, settings and resource loading, and Android smoke testing after rebuilding.
- The portable Windows write-path audit still has unresolved confinement gaps; see `docs/PORTABLE-WRITES-AUDIT.md`.
- Manual runtime checks remain: install and launch on Windows and Android, verify fresh separate data roots and resource loading, and confirm previous wuDict2 installations/data remain untouched. Do not install APKs or commit unless the user asks.
- Use `build-windows.cmd` for Windows, `build-android.cmd debug intel` for Android, and the recipes in `docs/WINDOWS-VERIFY.md`. Read the relevant `docs/SPEC.md` section before implementation.
