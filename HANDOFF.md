# Agent handoff — current state

Read this whole file at the start of a session: it is the CURRENT state, and it
is kept short on purpose — aim for under ~300 lines. When it grows past that,
move the finished narrative out instead of letting it accumulate.

Where everything else lives:
- The per-session narrative — what each past session changed, measured and left
  — is in `docs/handoff-archive.md`. Read it by grep, never whole: an index at
  its top lists every section with its date and the line numbers it had in the
  pre-split `HANDOFF.md`. The commit messages carry the same narratives
  independently, so `git log`/`git show` is a second copy, and
  `git show 51c0737:HANDOFF.md` is that file as it stood on 2026-09-26.
- This machine's build, test and emulator recipes, and the tests that fail on
  clean HEAD, are in `docs/WINDOWS-VERIFY.md`.

## Branch state (verify with git before trusting)

**Rechecked 2026-09-28 (the release was REPLACED):** `dev` is at `373273e`,
pushed, and the annotated tag `wudict2-v0.5.0` was MOVED onto it (`577c586` →
`373273e`) so the released v0.5.0 carries the night-preset fix instead of a new
tag following it. The release page was updated in place — new body, new APK
(versionCode 389, sha256 `1e8f0de9…`), the old asset deleted. The released build
is therefore the third session's five changes (the `Show info messages` row, the
page's top edge, the panel/status-bar moves, the notes' ink, the preset's paper
— `bef262d`, `7eb0e40`, `7be896e`, `3bfe24d`) PLUS the night-preset fix below.
`master` still has not moved since `5f0ad02`, so no upstream work is in it. On
this branch `test_data/` and `android/app/src/emuX86/jniLibs/` are ignored rather
than untracked, so `git status` is genuinely empty.

**The third session's five changes (2026-09-27/28; committed `bef262d`,
`7eb0e40`, `7be896e`, `3bfe24d`; shipped in v0.5.0) — the `Show info messages`
row, the page's top edge, the panel/status-bar moves, the notes' ink and the
preset's paper — are written up in `docs/handoff-archive.md` as "The third
session: five changes…". The live rules they produced are in
`docs/ANDROID-UI-HANDOFF.md` → "Decisions to preserve".**

**Uncommitted on `fonts-colour` (branch cut by the reader, 2026-09-28): GREY
MEANS DISABLED, and the old hierarchy is a PRESET.** The greys were doing double
duty as a hierarchy, so the panel's labels, heads, hints and controls read as
switched off in every look. The ~116 declarations that carried them now read
`--label` / `--label-quiet`, both defaulting to `var(--fg)`, and the greys are
kept only where they ARE a state (`details.noindex`, `.group-control-disabled`,
`.rmgo.busy`, a locked capability chip, a disabled checkbox's fill, the Folders
page's `.row.no`, the field's ⌕/spinner). **The way back is
`presets/labels/quiet_labels_app.css`** — its own manifest group, one file for
both themes, and it reaches the three standalone pages through the new
`"pages": true` manifest field (they alias `--fg-soft`/`--fg-faint` to their own
`--soft`/`--faint`, so one file serves all four documents). Verified in Chromium
(four documents, on/off, day/night, a paper tone) plus three new Go tests, and
**on the reader's phone** ("на телефоне отлично работает"). Live rule: the area
doc → "GREY MEANS DISABLED — the app's text is `--fg`".

**The night presets' palette fix (`6d7dc6a`, in the replaced v0.5.0)** is
written up in the area doc → "An app half must outrank the palette of the theme
it is attached in"; guarded by `TestPresetAppHalvesOutrankTheirPalette`,
verified on the reader's phone.

Branch `master_build` (`0cc878c`, pushed): upstream `master` + the fork's
build system only — `build-android.cmd` adapted to master's
version-suffixed APK names (script computes `APK_VERSION` from git
describe, same sanitization as build.gradle's `apkVersion`), and
build.gradle carries the fork's `-PemuX86` debug-ABI support
(src/emuX86/jniLibs, debug-only). App identity stays upstream
(`com.legbehindneck.wudict`, port 6888), so builds from this branch are
the ORIGINAL product and install beside wuDict2. Verified:
`build-android.cmd debug intel` produced
`wudict-android-arm64-x86_64-foss-debug-<ver>.apk` with both ABIs.
Release signing reads the same untracked `build-android.local.bat` when
the MAIN checkout is switched to `master_build` (a worktree has no
local.bat — copy it there first). After each upstream `master` sync,
repeat this small overlay (two files) or merge `master` into
`master_build` when the naming/emuX86 code still applies.

**Rechecked 2026-09-26:** `dev2` is at `f5b95d8`, the merge of upstream `master`
(`5f0ad02`) into it — five upstream commits: the index-version subsystem
(`0688c89`: `dict.ReaderVersion`, `store/fingerprint.go`, `store/stale.go`, the
`reindex` CLI and `/api/reindex`, the panel's Rebuild line, `ingest` defaulting
to the configured DICT_DIR), Browse triggering indexing (`95f0238`), and three
UI fixes (`78deb59` Group by, `17e730b` read-aloud icon, `fa83443` double
scrollbars). Two conflicts; both resolutions are in the merge message. The page
port is the shape to remember:

- index.html's 963-line "conflict" was upstream's inline stylesheet against our
  two `<link>`s (upstream has no `app.css` at all), and the remaining six hunks
  are the D149 picker-sections UI this fork never took — the same call as the
  09-25 sync. So: take OURS everywhere, then port upstream's deltas, CSS into
  `app.css` and JS/markup into the page. Ported: `.seg` scoped to `.bar` (top
  and phone query), the one-scrollbar lock (`html:has(#panel.show)` plus
  `--wd-sbw` measured in `showPanel`), `overscroll-behavior:contain`,
  `#speakBtn[aria-pressed="false"] .wave`, and the Rebuild line's markup with
  upstream's reindex JS verbatim — `renderReindex()` is called at the end of
  `renderPanel`, since the fork has no `syncPanelHeader`. Skipped: the
  `.chips`/`aria-pressed` rules and the `renderGroupSeg`/`CHECK_SVG` rework.
- Verified in a browser, not just by build: a throwaway server (isolated
  `USERPROFILE`, port 6899, pure-Go build) served the merged page, `loadDicts`
  filled the scope picker, and the new feature ran end to end — editing the
  source and pressing Rescan made the panel show "1 dictionary was prepared by
  an older version [Rebuild]", and Rebuild took `/api/reindex` to
  `{"done":1,"total":1}` and cleared the line.
- **`TestRescanSeesEditedSource` is a new known Windows failure** (upstream's own
  new test; it fails identically on clean `master`, so it is inherited rather
  than introduced). The live repro is "edit a dictionary source → Rescan
  folders → the page re-opens it": the implicit prepare cannot rename over
  text.db and the card shows `Access is denied`. The Rebuild button then fixes
  it, so the blast radius is that one path. Only the tag-less/pure-Go build
  could be tested here (no gcc) — the shipped cgo build is unverified.
- `merge-work/inline.go` needed a fix to run at all: its `pick.js` reference no
  longer matched the page (`?v={{PICKJS}}` arrived in the 09-23 merge). Fixed in
  the main checkout, and `index.full.html` regenerated from the merged page.

**The ☰ panel's 2026-09-26/27 changes (committed `5dc4d99` and `a808a64`,
shipped in v0.3.0):** four changes to the ☰ panel, all verified on a throwaway
server (127.0.0.1:6899, `test_data`, Chromium at 1100 and 390/360/320px) plus
`go build ./...`, `go vet`, the prefs/looks/presets/asset server tests and a
clean-first-run check. (1) The `Sort dictionaries` pair is gone with everything behind it — the
picker lists the reader's own arrangement and no other (`refreshDictUI` →
`orderedDicts()`; no `sortOwn`, no `sortMine` in the prefs body, no
`UIPrefs.SortMine`) — and a fresh `state.json` still carries no `fastFirst`, so
**Open first is My order by default**. (2) `Read aloud` is a label + `On|Off`
pair like the rows beside it (the label keeps its colour — the pair states the
state — and only the speaker's waves hide while it is off); the three rows'
labels and both button columns now line up (`.facts .seg>span` was shrinking to
each row's own text); and **Compact** is promoted onto the panel under Font
weight, a pair reading the same payload the sheet's checkbox does, with the
sheet keeping its row and the two verified to follow each other both ways.
(3) `.sect` draws the drawer's section hairlines from the ink instead of
borrowing `--line`, which is shared with every card border — from a phone
report that they were invisible in both of the reader's looks; `--line` itself
is untouched. (4) A second layer moved onto the panel the same way: **Examples
`Show|Hide`**, which folds the lines that hold nothing but examples. The work is
split — `web/examples.js` marks each shadow article's example-only `<p>` with
`wu-xonly` as the DOM appears (no state, no library rebuild, `artmark.Version`
untouched) and the layer's article half is the one rule that hides them — so the
switch is a plain preset (`style/presets.json`, `hide_examples`) and folding
back needs no sweep. (5) The **Status bar** (`.sbar`, the user's design): a
fixed bottom bar carrying `▲` / `Examples` / `▼` — the jumps between
dictionaries (the picker's own jump, walked, NOT wrapping) and a quick handle on
the fold — with `Status bar: Show|Hide` as a row in the drawer, beside the
controls it duplicates. It wears the bottom inset the shell already publishes,
so **no Java change was needed**; its row is a square of air, the two arrows, a
square, then `Examples` against the right edge and a closing square (the
squares are measured from the buttons, `--sbar-sq`), and the wallpaper layer
paints it exactly as it paints the top bar (both `background_image_app*.css`
files grew a `.sbar` selector — the day half mirrored to the bar's top
hairline). Each end of the walk is spent rather than wrapped (▲ dimmed at the
first dictionary, ▼ at the last, the way the font steppers dim at their bounds),
and each jump re-aligns its landing for a moment, so a section that is still
growing cannot leave its header halfway down the screen. Two traps are written
down in the area doc because each cost a round here: `barH()` reads 0 while the
top bar is auto-hidden, and the page settles after a landing, so geometry alone
cannot say where the reader is. Owed on a phone: the three Results
rows, Compact's effect on a real narrow screen, the Examples fold in a real
entry, the Status bar end to end (jumps, chip, and the sheet covering it), the
hairlines in daylight, and the picker's order. Rules, measurements
and the CSS traps are in `docs/ANDROID-UI-HANDOFF.md` → "Decisions to
preserve".

Facts that are not derivable from the code (still standing):
- **The fork gained a Windows desktop build on 2026-09-28: `build-windows.cmd`**
  (uncommitted at the time of writing, in the working tree). It builds
  upstream's product — `wudict.exe`, wuDict, port 6888, its own config and
  library — not the Android app, in the cgo flavour when it can find a C
  compiler; on this machine it finds Qt's mingw-w64 GCC
  (`C:\Qt\Tools\mingw1310_64\bin\gcc.exe`) on its own, which is also the first
  time cgo — and with it `-race` — is available here. All its machine paths sit
  in one block at the top of the file (`GCC_PATH`, `GCC_ROOTS`, `ISCC_PATH`).
  `release` also builds the installer, verified 2026-09-28 against the Inno
  Setup 7 pinned in `ISCC_PATH` (`D:\ProgSoft\InnoSetup7`; neither Inno Setup
  copy on this machine is in the uninstall registry, so nothing is found
  automatically — and the 5 that sits there too cannot read the .iss at all).
  That installer's numeric version reads 0.0.0, since fork tags are `wudict2-v…`
  and upstream's parser wants `v1.2.3`. Recipes: `docs/WINDOWS-VERIFY.md`.
- **The setup page's 📁 is now a host capability, not our prompt.** The page
  calls `wudictPickFolder` and shows the button only while the host has set
  `data-folder-picker` (foss `Storage.java`, reached through `MainActivity` →
  `Intake` → `Storage`); the Play flavour never defines the hook, so 📁 stays
  hidden there. `Shell.java`'s `wudict:folder-picker` prompt path (`onJsPrompt`
  → `settleDir`) is now unreachable — deliberately left in place; delete it
  when someone is next in that file. `showDirectoryPicker` went with it: in a
  browser it can name only the folder, never the path the server needs.
- **Double tap and double click run through three places that must stay in
  step**: `pick.js` (master's version — it fixes a `lang="en_US"` that threw
  out of the whole hit test, and bounds its scan around the tap), `frame.js`'s
  iframe handler and index.html's shadow-root handler. Both handlers try the
  selection first and fall back to the word, and the two are NOT the same
  message: a selection is prose and goes by `pick` to `lookupSelection`, a
  segmenter word is already exact and goes by `pickword` to `searchFor`. Keep
  that split — `lookupSelection`'s trailing-digit trim turns "CO2" into "CO"
  and "1984" into nothing.

## Where the live knowledge lives, by area

This file carries state and the rules that span the tree. The per-area rules —
most of them decisions that were expensive to reach — are written down
elsewhere, and a session must read the named place BEFORE changing that area.

- **Appearance, the Settings panel, the search bar, the setup/browse/lemmas
  pages** → `docs/ANDROID-UI-HANDOFF.md`: "Decisions to preserve" carries the
  live rules, "Unfinished work" and "Unverified assumptions" carry what is owed.
- **Presets and saved appearances** → the same list, plus two archive sections:
  `docs/handoff-archive.md` → "A preset's theme is its FILE NAME" and "The
  Presets row's button, and the editor's file line". Four invariants bite here:
  a preset's theme comes from its FILE NAME and never from a guard inside its
  CSS; the conflict rule between presets is an INTERSECTION, not equality; app
  halves are injected UNDER the reader's own stylesheet while the article halves
  are composed by the page; and the editor's App/Article boxes hold ONLY the
  reader's own CSS. The four are reminders, not the source: the full statements,
  and the bugs each one came from, are in the two archive sections named here.
- **Android shell ↔ page channels, the folder picker, double tap** → the facts
  above; fork identity, startup and builds → `docs/ANDROID-FORK.md`.
- **The server, the store and the formats** → the architecture facts below.
- **Anything historical** → `git log`, or `docs/handoff-archive.md` by grep.

## What remains from the review (with the reasons for leaving each)

- **Ingest is not cancellable** — deferred on purpose. Plumbing ctx through
  `IngestPlan`/`IngestMedia`/Progress touches signatures shared with the CLI,
  the server and format self-prepare: a wide diff in upstream-shared code for
  a need nobody has voiced (one ingest runs at a time, intake jobs cancel
  fine). Do it only if a "cancel indexing" button becomes a requirement.
- **`setFeatures` stale backend**: when the rebuild succeeds but the media
  step fails, the entry keeps the old backend and `revalidate` (path-string
  compare) never swaps it until eviction. Fix is ~5 lines (reopen on the
  media error path, reporting both errors); left because the window is
  narrow and the failure already surfaces to the user.
- **Recover guards for ad-hoc goroutines** — partially moot, and that is
  why it was not done wholesale: `handleDicts`/`Warm` workers call
  `dict.Open`/`Probe`, which recover internally. The genuinely bare spots
  are `registry.go` `recordLinks` (container parsing) and the `lemmas.go`
  install goroutine (download + archive extraction); wrap those two first
  if any panic ever escapes.
- **Lemma catalogue fetch holds `catMu` across the network** — every
  `/api/lemmas` poll queues behind a hung fetch. Rare (needs a stalled
  connection during an install); fix shape is serve-stale + singleflight.
- **Deliberately deferred in item 5**: `upgraded.linked` (links-locator
  media fetch buffers whole resource; Seek semantics make streaming harder
  there) and `.spx` transcoding (clips sit orders of magnitude below the
  4 MiB streaming threshold).
- **Fixed since this list was first written** (so nobody re-reports them):
  zim `Close()`/`c.zr` race, `rows.Err()` in `Keywords`/`Media.Names`,
  per-call `strings.NewReplacer` in stardict/slob/bgl, dead record-range
  tree in gomdict, per-call `http.Client` in intake fetch/probe, O(n²)
  name scans in `plainArchive`, and the intake dispose ordering
  (`install()` now closes the archive before the tail removes the source —
  on Windows the remove over the open reader silently kept the file; the
  two intake tests that caught it are green again).

## Architecture facts this session leaned on

- Android shell ↔ page channels: `wudict://` navigations caught in
  `Shell.openExternal`; `window.prompt('wudict:…')` answered from
  `Shell.windows().onJsPrompt` (the dictionary picker — the setup page's folder
  button is no longer a prompt consumer: it navigates to `wudict://folder`
  through `wudictPickFolder`, see the facts above);
  `window.wudictNativeShell=1` injected by `Shell.applyBackground` marks a
  page that will be answered; and `Shell.shellQuery` puts the shell's own
  page parameters on the URL — `shell_bg` (the reader's window colour),
  `shell_paper` (the paper an enabled preset paints, which the app page only
  forwards), `shell_image`, `shell_quiet` — which the page reads in its head
  block and remembers in `sessionStorage`, because each of them has to be
  true at FIRST paint (the injected channel only runs at
  `onPageFinished`). Web assets stay upstream-shared; Android behavior is
  injected, not compiled in (D54).
- Registry entry backends: `upgraded{Store over text.db + lazy src}`,
  `native` (no source), direct preview. `dsl.Dict` and `bgl.Dict` embed
  `*store.Store` themselves (auto-prepare formats) — they hold text.db too,
  which is why `releasePrepared` closes ANY serving backend, not just
  `*upgraded` (Windows-only; `registry_other.go` is a no-op).
- `entry.rebuilding` bars `open()` with `errReindexing` for the length of an
  ingest that renames over the prepared DB; defers in setFeatures /
  ensureBaseIndex / reabsorbAbbrev own the un-set on every error path.
- Allocation ceilings convention: 256 MiB (`maxLZOBlock`, zim
  `maxClusterBytes`, slob `maxItemBytes`, stardict `maxIndexBytes`);
  bgl blocks 64 MiB. `readAllBounded` pattern per package.
- `entry.preparedDB` (no source-changed check) is for PATH lookups
  (`serveOverride`); `preparedTextDB` (with the SQLite-verified check) stays
  for open decisions and the panel's dict rows.
- `store.Library()` reads `info.txt` receipts first (`receiptMeta`);
  `WriteInfo` writes a machine-readable `contains = 0|1` line; receipts
  predating it fall back to `ReadMeta` until re-ingest.
- `Media.Resource`: length-first probe, whole-row read under
  `blobStreamMin` (4 MiB), `blobReader` (substr chunks over the read-only
  pool) above; Range requests then read only the requested bytes.
- `search.runQuery`: queries run in an inner goroutine; on ctx death they
  are abandoned (never interrupted — dict interfaces take no ctx), bounded
  to one goroutine per wedged backend. A finished query outranks a
  cancellation landing in the same tick (re-check under the ctx branch).
- `config.SaveKeyRaw`: the WHOLE read-modify-write holds `saveMu`; the
  write is temp-in-same-dir + Sync + rename (`writeFileAtomic`). Nested
  locking would deadlock — `writeFileAtomic` takes no lock itself.
- Merge workflow aid (NOT in the repo; excluded via `.git/info/exclude`):
  `merge-work/index.full.html` — the fork's index.html with all extracted
  assets (app.css, history, pick.js, group editor) inlined back, regenerated
  by `go run merge-work/inline.go`. On an upstream sync, diff the new
  upstream index.html against it: hunks outside fork-customized regions
  port mechanically, hunks inside them are manual ports (to index.html or
  to the external asset that feature now lives in). Procedure in
  `merge-work/README.txt`; regenerate before every merge — a stale full
  page hides exactly the changes being merged.

## Not verified on a device

Everything above is build- and test-verified on Windows x64 and
cross-compiled for linux/arm64. No device (Android phone) has run the
folder-picker UI, the rename fix under a real rebuild, or streamed media
over the Range path. The Android-side checklist lives in
`docs/ANDROID-UI-HANDOFF.md`.
