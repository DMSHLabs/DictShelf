# Windows verification recipes

This machine's recipes for building, testing, previewing and poking the app,
moved out of `HANDOFF.md` on 2026-09-26. Scope: Windows x64, Git Bash, no `node`
and no `gcc` (what that rules out is below). Commands and traps, not the
history — the sessions that produced them are in `docs/handoff-archive.md`.

## Windows verification recipes (this machine)

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
- Android Java: from `android/`,
  `ANDROID_HOME="$LOCALAPPDATA/Android/Sdk" ./gradlew.bat :app:compileFossDebugJavaWithJavac --offline`.
  No ANDROID_HOME in the bash environment by default.
- No `node`, no `gcc` in this shell: JS syntax checks by hand/`vm`, and
  `-race` cannot run (cgo). Do not burn time on either.
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
- Known Windows test failures that fail identically on clean HEAD (do NOT
  chase them as regressions — compare against a clean checkout via
  `git worktree add /tmp/x HEAD`):
  - `internal/server`: TestSetupFlow, and TestRescanSeesEditedSource — the
    latter is new in the 09-26 upstream merge and fails identically on clean
    upstream `master` (same `rename … Access is denied`, see the 09-26 note in
    Branch state for the live reproduction and its workaround).
  - The 09-23 and 09-26 upstream merges fixed the rest of what used to be
    listed here — TestAndroidAliases(×2), TestDamagedTextResource…,
    TestIntakeUploadAndInstall, TestOpenAPICoversEveryRoute,
    TestRescanRecoversFromDeletedPreparedFolder, TestResourceAndIndex,
    TestResourceOverrideFromLibraryFolder, TestSetupMultipleFolders and
    `internal/format/dsl`'s TestMediaSourcesEveryZipSpelling all pass on the
    tag-less/pure-Go build now.
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
