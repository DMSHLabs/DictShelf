# Windows verification recipes

This machine's recipes for building, testing, previewing and poking the app,
moved out of `HANDOFF.md` on 2026-09-26. Scope: Windows x64, Git Bash, no
`node`, and no `gcc` **on PATH** — a mingw-w64 GCC does exist here, inside Qt
(build-windows.cmd finds it by itself, and it is what made cgo and `-race`
possible). Commands and traps, not the history — the sessions that produced
them are in `docs/handoff-archive.md`.

## Windows verification recipes (this machine)

- **2026-09-29 localization baseline:** `TestSetupFlow` also fails on unchanged
  `e938e17` (`server_test.go:574`, "app page not served after setup"). Its
  assertion searches served HTML for the old `design tokens` comment after CSS
  extraction. Reproduced using a Go overlay of unchanged HEAD without switching
  the working branch. Together with the nine failures below, the full suite
  currently reports ten failing top-level tests. Localization checks are listed
  in `docs/I18N.md`.

- Real dictionaries for manual checks: `test_data/` (three `.dsl.dz` — Asperger
  En-En 6.8k entries, Oxford En-Ru 35.8k, Zimmerman Ru-En 15.9k, ~5.4 MB, now
  git-ignored). Point a throwaway `DICT_DIR` at it to see the UI with real
  sizes, chips and index estimates instead of stub dictionaries.
- **Isolate the LIBRARY too, not just the config: pass `-db-dir` (or set an
  isolated `USERPROFILE`).** `-config /tmp/foo/wudict.toml` moves only the
  config and `style/` — `DB_DIR` still defaults to the REAL
  `~/.wudict/db`, so the first search in that preview PREPARES the `test_data`
  dictionaries into the user's own library. Measured 2026-09-28: one preview
  session added five entries and ~150 MB (`Webster's Unabridged 3 … (dsl)`
  alone is 100 MB), and they stay in this machine's "Previously imported
  dictionaries" list (the phone has its own library — untouched). Add
  `-db-dir /tmp/foo/db` (or export a throwaway `USERPROFILE`) and nothing of
  the user's is touched; a run that forgot is cleaned up from the setup page's
  library list, not by hand.
- **Kill EVERY `wudict.exe` before starting a preview server** —
  `taskkill /F /IM wudict.exe`. A stale instance keeps port 6899 and the new one
  exits with "stop the running instance first", so the browser goes on being
  served by the OLD binary's embedded assets (the log looks fine: the config
  summary is printed before the bind). Cost an hour of chasing a CSS change that
  was never served; `tasklist //FI "IMAGENAME eq wudict.exe"` should show one
  process after a restart. The page also caches its CSS by the `?v=` hash, so
  the browser must be reloaded after the server is really new.
- Go: `go build ./...`; targeted `go test ./internal/<pkg> -run '...' -count=1`.
- **`build-windows.cmd [debug|release] [purego]`** — the fork's desktop build
  (added 2026-09-28, beside `build-android.cmd`): `wudict.exe` in the repo root,
  upstream's product (wuDict, port 6888, its own config and library), NOT the
  Android app. **Every machine path it needs is in one block at the top of the
  file** — `GCC_PATH` (pinned to Qt's mingw-w64 GCC; empty falls back to `%CC%`,
  then `gcc.exe` on PATH, then `GCC_ROOTS`), `GCC_ROOTS`, `ISCC_PATH` — and each
  of those respects a value inherited from the environment, the way
  build-android.cmd respects `ANDROID_HOME`. Flavour is chosen for you: cgo
  (`-tags sqlite_fts5`, CGO_ENABLED=1) when a C compiler can be found, the
  pure-Go one when it cannot (`purego` forces the second). On this machine the
  Qt GCC 13.1.0 is found, so the default build here IS the cgo one — what
  `make build` and the CI cgo job ship. `cl.exe` is unusable: Go's cgo drives a
  gcc-style compiler, not MSVC. Verified 2026-09-28: both flavours build, run,
  and the cgo exe ingests a `test_data` `.dsl.dz` and answers `fts` (mattn +
  FTS5 inside the artifact; tags, CGO flag and driver are read back out of it
  with `go version -m`); its imports are KERNEL32 and msvcrt only, so no mingw
  DLL has to sit beside it.
- **The Windows installer (`release`) builds — with Inno Setup 7, pinned at the
  top of the script.** Neither compiler on this machine is in the uninstall
  registry, so `tools\make-installer.ps1 -Locate` finds neither of them, and
  `ISCC_PATH` in `build-windows.cmd` is pinned to
  `D:\ProgSoft\InnoSetup7\ISCC.exe`. Verified 2026-09-28: `build-windows.cmd
  release` builds the cgo `wudict.exe` and then the setup program,
  `dist\wudict-windows-x64-setup-0.0.0.exe` (7.3 MB, ProductName wuDict,
  FileDescription "wuDict Setup"); Inno Setup 7 still accepts the legacy
  `/D<name>=<value>` defines `make-installer.ps1` passes it. The **5** that is
  also on this machine (`D:\ProgSoft\InnoSetup5\`) cannot read the script at all
  — no `x64compatible`, no `PrivilegesRequiredOverridesAllowed` — and fails
  without a usable error: no text a redirect can capture, and either a failure
  or a success that wrote no file. Do not point `ISCC_PATH` at it.
- The setup file is named `…-setup-0.0.0.exe` and Windows shows version 0.0.0,
  because the numeric-version rule both packagers use (`tools\make-installer.ps1`
  and `tools\version.sh`) wants `v1.2.3`, and fork tags are `wudict2-v…`. What
  the wizard and the installed-programs entry read is the real tag
  (`wudict2-v0.5.0-2-gfffb163-dirty` at that date). Making the name right means
  either stripping the fork prefix when stamping the binary or teaching those
  two tools a numeric override — neither is done.
- Android Java: from `android/`,
  `ANDROID_HOME="$LOCALAPPDATA/Android/Sdk" ./gradlew.bat :app:compileFossDebugJavaWithJavac --offline`.
  No ANDROID_HOME in the bash environment by default.
- No `node` in this shell: JS syntax checks by hand/`vm`. **`-race` runs here
  now** (2026-09-28): it needs cgo, and a compiler exists — point CC at it the
  way `build-windows.cmd` does, e.g.
  `CC="C:/Qt/Tools/mingw1310_64/bin/gcc.exe" CGO_ENABLED=1 go test -race …`,
  which passed for `internal/lang`, `facet`, `hilite` and `artmark` in about
  eight seconds together.
- `gofmt -l` flags nearly every tracked Go file — CRLF working-copy noise,
  not real. `git diff --check` is the meaningful check.
- **The Android emulator (AVD `Small`) is x86_64 and cannot run the arm64 Go
  binary** — measured 2026-09-24 on `sdk_gphone16k_x86_64`, Android 17/API 37,
  16 KiB pages. ARM translation IS present (`ro.dalvik.vm.native.bridge =
  libndk_translation.so`, and a trivial arm64 C binary execs and returns its
  exit code), but EVERY Go binary — down to a `CGO_ENABLED=0` hello-world —
  dies with SIGSEGV inside the translated code (tombstone:
  `ndk_translation_program_runner_binfmt_misc_arm64`, guest arch arm64, `pc`
  in the guest image). That is why the app's Java half starts and the exec'd
  server never does. Arm64-v8a system images are not the answer either: on an
  x86 host they run under full QEMU emulation, slower than the translation
  they would replace.
- **`build-android.cmd debug intel` builds for the emulator.** It cross-builds
  the server for `GOARCH=amd64` too and ships both ABIs in
  `wudict2-android-arm64-x86_64-foss-debug.apk`. The token may be written in
  either position (`intel debug` too, a bare `intel` means debug) and `release
  intel` is refused. The x86_64 lib lands in `android/app/src/emuX86/jniLibs/`
  — a default source dir for NO source set — and only `-PemuX86=1` (which
  build.gradle wires to the DEBUG source set alone) makes AGP read it, so a
  release APK is arm64 whatever is on disk. Verified 2026-09-24: both ABIs in
  the APK, the x86_64 one taken back OUT of the APK runs on the emulator and
  prints its version stamp, plain `debug`/`release` stay arm64-only, and both
  refusals fire.
- **The NDK recipe for a second ABI**: the Windows NDK has no Unix-style
  `x86_64-linux-android26-clang` wrapper, so name the target on `clang.exe`
  itself — `CC="$NDK_BIN/clang.exe --target=x86_64-linux-android26"`, `CXX`
  likewise, `CGO_ENABLED=1 GOOS=android GOARCH=amd64`, same `-tags sqlite_fts5
  -trimpath` and the same ldflags including
  `-extldflags=-Wl,-z,max-page-size=16384`.
- **`build-android.cmd` needs `ANDROID_HOME` DELETED, not defaulted** (found
  2026-09-28). This shell exports `ANDROID_HOME=%LOCALAPPDATA%\Android\Sdk` —
  the cmd-style literal, unexpanded — and the script only falls back to
  `%LOCALAPPDATA%\Android\Sdk` when the variable is *undefined*, so it fails
  with "Android SDK directory does not exist: %LOCALAPPDATA%\Android\Sdk".
  Passing a real path works (`env ANDROID_HOME='C:\Users\shepe\AppData\Local\Android\Sdk'
  cmd //c build-android.cmd debug intel`), and `set`-ing it inside the `cmd //c`
  string does NOT — MSYS mangles the nested quotes. `gradlew` is unaffected:
  it takes `ANDROID_HOME="$LOCALAPPDATA/Android/Sdk"` inline.
- **Driving the app on the emulator** (2026-09-28, AVD `Small`, `debug intel`
  APK, `emulator-5554`; `android_ui_tap`/`swipe`/`type_text` all work there,
  unlike the MIUI phone). Three traps, each cost a round:
  - `uiautomator` sees the WebView as ONE node — every tap has to come from a
    screenshot's coordinates, so take a screenshot, read it, then tap.
  - **`BACK` finishes the activity once the keyboard is down.** Pressing it to
    hide the keyboard after `ENTER` in a field closed the app twice; prefer
    tapping inside the page to blur, or accept the keyboard in the screenshot.
  - **Typing into a number field APPENDS unless the field is cleared** — the tap
    puts a caret, not a selection, so "7002" landed after the existing "7001"
    and the page rightly refused `70027001`. Clear with
    `adb shell input keyevent 123` (MOVE_END) then `67` (DEL) per digit.
  - The app's own preferences are readable on a debug build:
    `adb shell run-as com.dmshepeta.wudict2.debug cat shared_prefs/shell.xml`
    — that is how a write from the page was confirmed, and how the dead
    `effective_*` keys were seen leaving the file.
  - `am start -n <appId>/com.legbehindneck.wudict.SettingsActivity` opens the
    shell settings screen (exported, floating, `taskAffinity=""`), which is the
    only practical way to reach it without the launcher's long-press menu.
- Known Windows test failures: **none, in a checkout made after the 2026-10-02
  merge.** What used to be listed here is accounted for:
  - Upstream fixed `internal/server`'s TestRescanSeesEditedSource — the
    `rename … Access is denied` reported from this machine; `releaseSuperseded`
    closes the backends a rescan retired, before an open re-prepares in place —
    and `internal/format/wmd`'s TestPlainMarkdown, whose reader now closes.
  - The wmd/cli failures that compared text against LF expectations
    (TestExamples, TestGuideIsCleanMarkdown, TestSourceFiles,
    TestWriteSpecExamples, TestDumpMarkdownRoundTrip, TestDumpMarkdownGzip)
    were CRLF-worktree artefacts, and upstream's own `.gitattributes`
    (`* text=auto eol=lf`) is the fix. A worktree checked out before that file
    arrived keeps its CRLF files and still fails them; a fresh one does not —
    verified at `23670f3`, where `file` reports the spec examples as LF and
    `internal/cli`, `internal/format/wmd` and `internal/howto` all pass. If
    they appear again, re-checkout rather than debug them.
  - The three setup-page probes were fixed on this side (2026-10-02): every page
    ships the whole i18n catalog, so setup.html's sentences are in the app
    page's body too, and the app page has no inline stylesheet to find "design
    tokens" in. TestSetupFlow, TestConfigEndpointAndSetupPage and
    TestSetupConsentFlow probe markup now (`<title>Edit Folders</title>`,
    `id="panel"`).
  - TestOpenAPICoversEveryRoute passes here and still FAILS on upstream
    `master`, because the fork's own `web/openapi.yaml` is what covers the
    missing routes. Do not "fix" the fork's copy against that failure.
  - The 09-23, 09-26 and 09-29 merges fixed the rest of what used to be listed
    here — TestAndroidAliases(×2), TestDamagedTextResource…,
    TestIntakeUploadAndInstall, TestRescanRecoversFromDeletedPreparedFolder,
    TestResourceAndIndex, TestResourceOverrideFromLibraryFolder,
    TestSetupMultipleFolders and `internal/format/dsl`'s
    TestMediaSourcesEveryZipSpelling all pass on the tag-less/pure-Go build now.
  - Flaky everywhere: TestFailedDemandIsRetried (TempDir cleanup races the
    ingest goroutine; failed 4/5 on clean HEAD once).
  - `internal/intake`: TestJobDisposesSource and
    TestSpooledSourceIsAlwaysRemoved USED to fail here (dispose ran before
    the archive reader closed, so Windows kept the "delete the source"
    file); fixed in the second-tier batch — if they come back, look at the
    archive Close ordering in `install()`.
  - Seven server tests used to fail on Windows at the rename step before
    item 2 (`2675563`) fixed it — if they regress, look at
    `internal/server/registry_windows.go`.
