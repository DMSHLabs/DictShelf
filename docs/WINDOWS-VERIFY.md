# Windows verification recipes

This machine's recipes for building, testing, previewing and poking the app,
moved out of `HANDOFF.md` on 2026-09-26. Scope: Windows x64, Git Bash, and no
`gcc` or `make` **on PATH** — a mingw-w64 GCC and GNU Make do exist here, inside
Qt (build-windows.cmd finds the GCC by itself, and it is what made cgo and
`-race` possible; `make` has to be called as `mingw32-make.exe`). The tool paths
themselves are in `thirdpartytools.md`, together with the local override files
`build-windows.local.bat` / `build-android.local.bat` that hold the values.
`node` IS installed now (`C:\Program Files\nodejs`, v24.21.0) — earlier notes
here said otherwise; nothing in the build needs it, and the checks that do are
listed in `translation.md`. Commands and traps, not the history — the sessions
that produced them are in `docs/handoff-archive.md`.

## Windows verification recipes (this machine)

- **2026-10-14 GUI builds:** `build-windows.cmd` with no argument uses
  `-H windowsgui` (PE subsystem 2). `build-windows.cmd cli` and `server` use
  the console subsystem (3). The CLI prints help without arguments; the server
  starts HTTP by default. GUI launch detection reads
  the current executable's PE subsystem when there is no console, so the
  GUI artifact selects WebView2 even when started from cmd. Redirected
  `--version` remains usable by the build verification. Restart the app to
  load a rebuilt executable.

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
  isolated `USERPROFILE`).** `-config /tmp/foo/dictshelf.toml` moves only the
  config and `style/` — `DB_DIR` still defaults to the REAL
  `~/.dmshlabs/dictshelf/db`, so the first search in that preview PREPARES the `test_data`
  dictionaries into the user's own library. Measured 2026-09-28: one preview
  session added five entries and ~150 MB (`Webster's Unabridged 3 … (dsl)`
  alone is 100 MB), and they stay in this machine's "Previously imported
  dictionaries" list (the phone has its own library — untouched). Add
  `-db-dir /tmp/foo/db` (or export a throwaway `USERPROFILE`) and nothing of
  the user's is touched; a run that forgot is cleaned up from the setup page's
  library list, not by hand.
- **Kill EVERY `DictShelf.exe` before starting a preview server** —
  `taskkill /F /IM DictShelf.exe`. A stale instance keeps port 6899 and the new one
  exits with "stop the running instance first", so the browser goes on being
  served by the OLD binary's embedded assets (the log looks fine: the config
  summary is printed before the bind). Cost an hour of chasing a CSS change that
  was never served; `tasklist //FI "IMAGENAME eq DictShelf.exe"` should show one
  process after a restart. The page also caches its CSS by the `?v=` hash, so
  the browser must be reloaded after the server is really new.
- Go: `CGO_ENABLED=1 go build -tags sqlite_fts5,sqlite_dbstat ./...`; targeted
  `CGO_ENABLED=1 go test -tags sqlite_fts5,sqlite_dbstat ./internal/<pkg> -run
  '...' -count=1`. The cgo driver needs `CC` (MinGW from `GCC_PATH` in
  `build-windows.local.bat`). **A build with no tags, or with `-tags
  sqlite_fts5` and `CGO_ENABLED=0`, fails on purpose** — it used to fall
  silently to the pure-Go driver, which had dbstat and the cgo one does not
  unless `sqlite_dbstat` is passed, so the same command could test a different
  binary from the one shipped (`internal/store/driver_missing.go`). There is no
  second driver any more: `-tags purego` was removed on 2026-11-03.
- **`build-windows.cmd [cli|server|installer] [--channel ...] [--update-icon]`** —
  the fork's Windows builds. The PE resource object `dictshelf_windows_amd64.syso`
  is generated on every build and holds the six icons (from
  `packaging\windows\dictshelf.rc`) together with the version block, because the
  Go linker accepts only one `.rsrc` section. Pass `--update-icon` after changing
  `packaging\windows\dictshelf.ico` to refresh the icon half alone. Before replacing a
  running output executable, the script force-stops it and waits up to 60 seconds
  for Windows to release the file. The default command builds listenerless WebView
  `DictShelf.exe`, `cli` builds console `DictShelf-cli.exe`, `server` builds console HTTP-server
  `DictShelf-server.exe`, and `installer` packages the existing desktop exe.
  Machine-specific overrides belong in the ignored `build-windows.local.bat`
  or the environment: `GCC_PATH` (empty falls back to `%CC%`, then `gcc.exe`
  on PATH, then `GCC_ROOTS`) and `ISCC_PATH`. GitHub Actions uses the
  `windows-2022` image's MinGW-w64 GCC from `C:\mingw64\bin` and passes its
  absolute path to the script. Windows
  desktop always requires cgo (`-tags sqlite_fts5,sqlite_dbstat`, CGO_ENABLED=1);
  there is no other flavour. If no compatible GCC is found, the
  script exits before replacing
  the existing executable. This machine's ignored local settings select Qt GCC 13.1.0, which
  produces what `make build` and the CI cgo job ship. `cl.exe` is unusable: Go's
  cgo drives a gcc-style compiler, not MSVC. Verified 2026-09-28: the cgo exe
  builds, runs, ingests a `test_data` `.dsl.dz` and answers `fts` (mattn +
  FTS5 inside the artifact; tags, CGO flag and driver are read back out of it
  with `go version -m`); its imports are KERNEL32 and msvcrt only, so no mingw
  DLL has to sit beside it.
- `DictShelf.exe` now hosts its WebView2 window on a locked Windows UI
  thread when launched from Explorer or Start; terminal launches retain the
  browser. It keeps a separate profile under `%LOCALAPPDATA%\dictshelf\WebView2`
  and falls back to the browser when the Runtime is missing. Closing the
  window leaves the tray server running; Settings → Exit and tray Quit stop it.
  Recheck a rebuilt release installer on a desktop before claiming visual
  verification.
- **An unkeyed desktop launch opened File Explorer at "This PC" instead of a
  browser — measured and fixed 2026-10-07.** With no access key configured (the
  default) `keyURL` answers `""` on purpose — it also feeds the startup banner,
  which prints nothing for it — and the two browser-opening call sites in
  `internal/cli/cli.go` handed that empty address straight to the opener.
  Windows makes it loud: `rundll32 url.dll,FileProtocolHandler ""` is not a
  no-op, it opens This PC. `browseURL` now falls back to the plain server URL,
  with `internal/cli/browser_test.go` guarding it. Verified on the desktop
  build: the launch leaves the Explorer window count at 0 and opens the setup
  page in Edge (window title "Edit Folders"). The other platforms have the same
  defect with `xdg-open ""` / `open ""`, where the browser never opens at all.
- **The Windows installer (`installer`) packages the existing desktop exe — with Inno Setup 7, pinned at the
  top of the script.** Neither compiler on this machine is in the uninstall
  registry, so `tools\make-installer.ps1 -Locate` finds neither of them, and
  `ISCC_PATH` in `build-windows.cmd` is pinned to
  `D:\ProgSoft\InnoSetup7\ISCC.exe`. Verified 2026-09-28: `build-windows.cmd
  installer` packages the cgo `DictShelf.exe` and builds the setup program,
  `dist\dictshelf-windows-x64-setup-0.0.0.exe` (7.3 MB, ProductName DictShelf,
  FileDescription "DictShelf Setup"); Inno Setup 7 still accepts the legacy
  `/D<name>=<value>` defines `make-installer.ps1` passes it. The **5** that is
  also on this machine (`D:\ProgSoft\InnoSetup5\`) cannot read the script at all
  — no `x64compatible`, no `PrivilegesRequiredOverridesAllowed` — and fails
  without a usable error: no text a redirect can capture, and either a failure
  or a success that wrote no file. Do not point `ISCC_PATH` at it.
- The Windows installer's numeric version recognises both `v1.2.3` and this
  fork's `wudict2-v1.2.3` tags. Earlier setup files named `0.0.0` predate
  this fix; unversioned builds still use that fallback.
- Android Java: from `android/`,
  `ANDROID_HOME="$LOCALAPPDATA/Android/Sdk" ./gradlew.bat :app:compileFossDebugJavaWithJavac --offline`.
  No ANDROID_HOME in the bash environment by default.
- **Checking the page in a browser.** `node` IS in this shell now (`C:\Program Files\nodejs\node`,
  v24.21.0 — earlier notes here said "no node"; nothing in the build needs it), and Playwright is
  installed OUTSIDE the repository at `/d/Temp/pw` (`NODE_PATH=/d/Temp/pw/node_modules`, launch with
  `channel: 'msedge'`). Use it for scripted checks; for reading the page state, the ZCode in-app
  browser (the `browser-use` skill's `control-browser`) is quicker. Either way, against a local
  preview: build the server with `CGO_ENABLED=1 go build -tags
  sqlite_fts5,sqlite_dbstat -o <tmp>/DictShelf.exe .`,
  start it with isolated dirs and no auth —
  `DictShelf.exe serve -dict-dir <test_data> -db-dir <tmp>/lib -config <tmp>/dictshelf.toml
  -port 6925 -auth off -no-browser -no-tray` (the `-config` file must EXIST,
  even empty) — then load `http://127.0.0.1:6925/` and read the page's own state with `evaluate`
  (tokens, `localStorage`, element `hidden` flags) while installing `window.__errs` listeners to
  catch page errors. `?dict=<id>` opens a named dictionary straight away. This is how v0.10.0's Fonts
  tokens, search-mode persistence, group-editor create/hint/filter paths and a real search were
  verified with no console errors; a device run is still owed. **Articles are drawn in a Shadow DOM or
  an iframe, so `querySelectorAll('mark')` from the page context finds no highlights** — check
  highlighting in the server's own answer (`mode=phrase&hl=1`, look for the marks in `Body`), or walk
  the shadow roots. That trap has already cost one wrong conclusion. Two more: this shell's `grep` is
  `ugrep`, whose `-c`/`-a` do not count binary matches — count strings in a
  packaged `.so` with `python3` (`data.count(b"...")`); and `go build -o
  /tmp/x.exe` writes where MSYS/Win32 disagree about `/tmp` — pass
  `"$(cygpath -w /tmp/x.exe)"`.
- **Starting the server against the user's OWN library** (the one with the archives and the built
  indexes) needs the mutex out of the way, so run it with a throwaway `USERPROFILE`; add
  `-db-dir C:\Users\shepe\.dmshlabs\dictshelf\db -config C:\Users\shepe\.dmshlabs\dictshelf\dictshelf.toml`
  and, as above, `-use-cached -auth off -no-browser -no-tray`. The first request after a start no
  longer opens the whole library, but it is still a cold read. Library contents worth knowing when
  picking a case: `Викитека` (our index built, 3.17 GB), `Викисловарь` (ours, 1.29 GB), and
  `Википедия` ×2 / `Wikipedia` / `Wiktionary` with no index of ours — those last are where the
  archive-answered plate and the "declined reading" behaviour are checked.
- **`-race` runs here now** (2026-09-28): it needs cgo, and a compiler exists —
  point CC at it the way `build-windows.cmd` does, e.g.
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
  `dictshelf-android-arm64-v8a-x86_64-foss-debug.apk`. The token may be written in
  either position (`intel debug` too, a bare `intel` means debug). The x86_64 lib
  lands in `android/app/src/emuX86/jniLibs/` — a default source dir for NO source
  set — and only `-PemuX86=1`, which the script passes, makes AGP read it, so a
  stray emulator `.so` in the tree cannot reach a shipping build. Verified
  2026-09-24: both ABIs in the APK, the x86_64 one taken back OUT of the APK runs
  on the emulator and prints its version stamp, and plain `debug`/`release` stay
  arm64-only.
- **`intel release` is allowed since 2026-11-03** (it used to be refused): a
  signing key is configured for it, `build.gradle` reads `src/emuX86/jniLibs`
  under `-PemuX86=1` for the release variant too, and the artifact is renamed
  `dictshelf-android-arm64-v8a-x86_64-foss-emu.apk`. The `-emu` suffix is the
  warning: an emulator ABI is never published, and the closing summary says so.
  Both AVDs here are x86_64 (see the note above on why an arm64-only APK cannot
  run there).
- **The Go bridge alone can be compile-checked without Gradle.** That is what
  settles "does `android/nativebridge` still build" after a Go-side change -
  asked on 2026-11-04, after the `MAX_*` wiring added `limits.Set` to
  `WudictGoBridgeRuntimeStart`. From Git Bash, with the NDK prebuilt for THIS
  host:

      NDK="$LOCALAPPDATA/Android/Sdk/ndk/30.0.16248370/toolchains/llvm/prebuilt/windows-x86_64/bin"
      CC="$NDK/aarch64-linux-android26-clang" CXX="$NDK/aarch64-linux-android26-clang++" \
        CGO_ENABLED=1 GOOS=android GOARCH=arm64 go build -buildmode=c-shared \
        -tags sqlite_fts5,sqlite_dbstat -trimpath -o "$TEMP/libwudictbridge.so" ./android/nativebridge

  Verified 2026-11-04: exit 0, `libwudictbridge.so` 37 787 560 bytes. `make
  android-go` runs the same command and does **not** work here: `Makefile:174`
  computes `NDK_HOST` as `linux-x86_64` for every host that is not Darwin, and
  the NDK installed here ships `windows-x86_64` only, so cgo reports the
  compiler missing and make exits 1. Use the command above, or
  `build-android.cmd` for a whole APK.
- **Launching one**: `"%LOCALAPPDATA%\Android\Sdk\emulator\emulator" -avd <name>`
  — the AVD used for the 2026-11-03 checks is `720x1600` (a phone geometry with
  the app's own breakpoints); `Small` is the older one. Wake a black screen with
  `adb shell input keyevent KEYCODE_WAKEUP` before believing a screenshot.
  Installing over a running app (`adb install -r`) leaves the old process
  alive and can answer with a stale error - force-stop the app first. The App
  Links handoff recipe (installing, forcing the domain state, opening a real
  share link) is in `docs/ANDROID-FORK.md`.
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
    `adb shell run-as com.dmshlabs.dictshelf.debug cat shared_prefs/shell.xml`
    — that is how a write from the page was confirmed, and how the dead
    `effective_*` keys were seen leaving the file.
  - `am start -n <appId>/com.dmshlabs.dictshelf.SettingsActivity` opens the
    shell settings screen (exported, floating, `taskAffinity=""`), which is the
    only practical way to reach it without the launcher's long-press menu.
- Known Windows test failures: **none, in a checkout made after the 2026-10-02
  merge.** What used to be listed here is accounted for:
  - A later `sqlite_fts5` full-suite run on the LSD branch exposed a test-only
    global-state leak: `dmsh_portable_test.go` restored `store.DefaultDBDir()`
    as a persistent override, freezing one test's temporary path for later
    tests. Portable tests now clear the override after use, and group-rescan
    tests wait for queued dictionary indexing before deleting fixtures. The
    full cgo suite passes with `go test -tags sqlite_fts5 -trimpath ./... -count=1`.
  - `go vet -tags sqlite_fts5 -trimpath ./...` passes after the Windows crash
    callback began accepting `*windowsExceptionPointers` directly instead of
    converting its `uintptr` argument back into a pointer. Home-path test
    expectations compare `filepath.Clean` values, matching the normalization in
    `ExpandHome` and `pathAliases`; a `mingw32-make check` run from Git Bash
    passes through vet and the full cgo suite with local dictionary fixtures.
    (GNU Make is here — 4.2.1, inside Qt — but it is not on PATH as `make`, so
    `make check` typed as such is "command not found"; see `thirdpartytools.md`.)
  - The fourth upstream sync (2026-10-04) rechecked the whole suite in its fresh
    LF worktree: `go build`, `go vet`, `gofmt -l` clean and `go test -count=1
    ./...` green, 0 failures. Its one Windows-relevant change is upstream's
    `TestSamePath` fix (`internal/fsx/fsx_test.go`: `filepath.Rel` from the
    working directory fails across volumes, so the test `t.Chdir`s first — this
    machine's checkout is on D: and TempDir on C:). Trap that bit twice here: a
    smoke server started with only `--dict-dir` prepares the test dictionary
    into the REAL `~/.dmshlabs/dictshelf/db`; isolate with `--db-dir` **and** `--config`
    (or a throwaway `USERPROFILE`), or clean the leftovers out of the library
    afterwards.
  - The third upstream sync (2026-10-04) rechecked the whole suite in its fresh
    LF worktree: `go build`, `go vet` and `go test -count=1 ./...` green, 0
    failures. New trap of that round: served pages and assets now have their
    comments **stripped** (`internal/server/webasset.go`), so a test that probes
    served HTML for a comment fails by design — probe markup instead, and when a
    comment IS the subject read the raw file (`web/presets/**` via `presetFS`,
    which `handlePresetFile` serves unstripped).
  - GD_DSL integration rechecked the full suite in this checkout. The spec and
    its clean/html Markdown examples still had CRLF and were converted locally
    to LF without indexed content changes. TestParseRefBothCopies now normalizes
    CRLF before extracting JavaScript; the new DSL mode route is documented in
    OpenAPI. Go build/vet/full suite (default and sqlite_fts5/cgo) and
    server/store/DSL race tests pass.
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
    they appear again, convert the checkout rather than debug them: a plain `git checkout --force` does NOT rewrite a file whose normalised content already matches the index, so remove the tracked files and check them out again — `git ls-files | xargs rm` then `git checkout -- .` — or clone fresh.
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
    TestMediaSourcesEveryZipSpelling all pass now.
  - Flaky everywhere: TestFailedDemandIsRetried (TempDir cleanup races the
    ingest goroutine; failed 4/5 on clean HEAD once), TestPreviewEviction
    (a sweep measured against a live heap: seen once in a loaded full-suite run
    as "still 1050 bytes of preview open after the sweep", while passing 5/5 in
    isolation and in the runs either side of it — re-run before believing it),
    and TestDictionaryListSharedByHTTPAndNativeWithoutRouter (seen once in a
    full-suite run on 2026-11-01, with the three transports' rows differing by
    the estimate a background stats pass had just written; passed alone, with
    two neighbours, and in the two full runs either side of it - the fixture's
    "let self-preparing fixtures settle" first call is what it races).
    TestSingleDSLIndexRemoval used to fail here the same way (`unlinkat
    .../demo/stats.cache: ... being used by another process`); it was our own
    stats pass holding the file, and it is fixed - see
    `docs/handoff-archive.md`, "The removal claims the files" - 100 runs on
    2026-11-03 pass.
  - `internal/intake`: TestJobDisposesSource and
    TestSpooledSourceIsAlwaysRemoved USED to fail here (dispose ran before
    the archive reader closed, so Windows kept the "delete the source"
    file); fixed in the second-tier batch — if they come back, look at the
    archive Close ordering in `install()`.
  - Seven server tests used to fail on Windows at the rename step before
    item 2 (`2675563`) fixed it — if they regress, look at
    `internal/server/registry_windows.go`.
  - Four tests USED to fail in every run here and no longer do - all four pass
    alone and in the suite on 2026-11-03, and a `mingw32-make check` has exited 0
    on this machine repeatedly that day:
    - `TestConfigEndpointAndSetupPage` - the probe looked for a `<title>` the
      page had stopped carrying; the probe was corrected.
    - `TestReaderGolden` (internal/format/bgl) and `TestReaderGolden` /
      `TestGDReaderGolden` (internal/format/dsl) - the "prepared content changed
      but no version was bumped" hashes were stale in this checkout.
    - `TestNewDictionariesGetContainsIndexWithoutSavedDefaults` - a real product
      bug: the first-run default was written in two places that disagreed about
      the contains index; the queue now asks `prefs.newIndexDefaults`.
    They stayed in this list long after they were fixed, which is its own
    lesson: re-run a "known failure" before believing it.
  - `TestI18nPagesAndCache`, `TestRemoveTakesTheEmptiedFolderWithIt` and
    `TestDictsRevalidation` were reported failing during the 2026-11-03 sessions
    and **all three pass now** (re-run together on 2026-11-03, cgo +
    `sqlite_fts5,sqlite_dbstat`: `ok ... 0.804s`). `TestDictsRevalidation` was
    already known to be flaky in a loaded full-suite run; the other two were
    reported as solid failures, so the report outlived the defect. Same lesson.

## Lemma files on this machine (Russian installed 2026-11-01)

Moved here from `HANDOFF.md` on 2026-11-03: this is what is installed on this
machine, not a property of the product.

`~/.dmshlabs/dictshelf/lemmas/ru.tsv.gz` (1.9 MB) is what `internal/morph` reads: `.tsv` + optional
`.gz`, the stem resolved through `internal/lang`, so the name needs no guessing. Verified by loading it
and lemmatizing, not by reading it: человека/людей -> человек, года -> год, статьями -> статья, времени ->
время, кларнетиста -> кларнетист, джазового -> джазовый, and the ё case works both ways (идёт and идет ->
идти), which is what `yoRetry` in morph.go exists for. The built-in English still answers (running -> run).
Data: 48 213 lines, 566 275 forms. The whole retry path is covered by `TestSharedSearchMorphology` in
internal/server, which passes.

What it costs on this machine, measured: 266 ms to load and first answer, **+82 MB heap** resident
(morph.go's comment says the ru pack is 65 MB, so that upper bound is low for Russian - it matters where
the budget is 64 MB, which is Android's PREVIEW_MEMORY default). 1000 further lookups take under a
millisecond in total: the load is the cost, not the use.

Two things about the data that are not bugs, so nobody re-finds them: a word that is both a form and a
lemma keeps golem's single answer - "домашние" is a form of "домашний" AND a lemma of its own (the
plural-only noun), and since the file lists the noun line first, the answer is "домашние" itself, which
`Lemma` reports as no useful answer; and 4 610 forms are prefixed with `*` (golem's own English pack has
none), the source list's mark for "in the paradigm but not a word" - golem keeps the mark as part of the
key, so those forms simply never match, which is the same outcome by accident.

A file dropped into the folder while the app is running is picked up on the next start or by the lemma
installer in the app (`internal/server/lemmas.go` calls `Morph.Rescan`); `wudict lemmas list` cannot run
while the app is up, because every non-informational command takes the single-instance mutex.

## Measuring the ingest write path on this machine

Moved to [`MEASUREMENTS.md`](MEASUREMENTS.md) (#measuring-the-ingest-write-path-on-this-machine)
on 2026-11-03: the gated bench (`WUDICT_BENCH_ARTICLES`), the real-archive
`DictShelf ingest -fulltext -contains` runs and what their spread means are all
numbers, and numbers live in one file now.

## ZIM archives on this machine (the reader's fixtures)

`D:\Dictionaries\wuDict\Dictionaries\zim\` holds six real archives: the two
`maxi` Wikipedias (46 GiB de, 35 GiB ru), `wikipedia_ru_all_mini_2026-10.zim`
(5.7 GiB), `wikisource_ru_all_nopic_2026-10.zim` (3.6 GiB), and the two
`nopic` Wiktionaries — `wiktionary_ru_all_nopic_2026-09.zim` (2.0 GiB) and
`wiktionary_en_all_nopic_2026-08.zim` (8.7 GiB).

`internal/format/zim`'s integration test skips unless an archive is named, and
the Makefile's own default (`~/Downloads/Language/ZIM/wiktionary_ro_all_nopic_2026-07.zim`)
does not exist here. Use the Russian Wiktionary for a default run: it is the
smallest dictionary in the folder, and the test opens it, reads its meta, draws
twenty keywords from the middle of the file and finds each one by `Exact`.

    WUDICT_TEST_ZIM="D:/Dictionaries/wuDict/Dictionaries/zim/wiktionary_ru_all_nopic_2026-09.zim" \
      CGO_ENABLED=1 go test -tags sqlite_fts5,sqlite_dbstat ./internal/format/zim/ -run TestRealFile -count=1 -v

Measured 2026-11-04: `Викисловарь: 2 527 809 entries, lang "ru", 1 528 188 articles
claimed, 16 MiB preview`, and `TestRealFile` passes — the smoke test this fork
had never run against a real archive. Note what it does NOT cover: decompressing
a large stored cluster, a seek into a media resource, and the read-through
prepared form are the invariants the large `maxi` files are needed for, and they
have to be exercised by hand, not by this test.

