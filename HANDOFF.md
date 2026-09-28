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

**Rechecked 2026-09-27 (second release session):** `dev` is at `bd17906`, clean
and pushed, carrying the annotated tag `wudict2-v0.4.0` — the release cut this
session ("Release wudict2-v0.4.0" in `docs/handoff-archive.md`). It ships the
Status bar (item 5 in the ☰ panel paragraph below, committed as `155e30d`)
together with the ☰ panel work of the earlier releases. `master` still has not
moved since `5f0ad02`, so no upstream work is in it. On this branch
`test_data/` and `android/app/src/emuX86/jniLibs/` are ignored rather than
untracked, so `git status` is genuinely empty.

**Uncommitted on `dev` (2026-09-27/28, third session): the `Show info messages`
row, the page's top edge, the panel/status-bar moves, the notes' ink, and the
preset's paper.** The working tree carries five things, nothing committed. This
session's earlier work went in as `bef262d`, whose message landed as one
2785-character subject line — the intended subject is `feat(ui): the notes
switch, the flush header, and the scope chip`, and git-cliff reads only the
first line.

1. The switch that turns the app's transient notes off — MainActivity's
   "Starting wuDict2…", the page's waiting art and its "N of M ready" counter,
   and the lookup popup's "Looking up …". A plain `ShellPrefs` boolean
   (`INFO_MESSAGES`, ON by default), read by three Activities and delivered to
   the page on the URL beside `shell_bg` (`shell_quiet=0|1`); the page's half
   is in `index.html`'s head block and its boot/status guards. **The morph note
   is NOT silenced** — the switch covered it at first and the reader asked for
   it back ("this message must stay"): it says the answers are for a different
   form of the word, which is read rather than skimmed. **A phone has to
   confirm the row, the empty first frame and the popup without its line.**
   Full statement, reasons
   and measurements: `docs/ANDROID-UI-HANDOFF.md` → "Decisions to preserve" →
   "The information messages are one switch".
2. The page's top edge: `body{padding-top:var(--barh)}` — one rule at every
   width, replacing a fixed `4.236em` above 600px and a `--barh + --sp-2` copy
   below it — plus `#out>:first-child>details.dict{margin-top:0}`, which gives
   the first section up its `--sp-2` so its header is flush against the bar.
   Between them they remove the empty band between the search bar and the first
   dictionary header (34px at 1100px, 20px on a phone) and fix 601–800px, where
   the fixed value was 11px too small for a two-row bar and the first card
   began above the bar's lower edge. The header is now in the same place in
   every state — at the bar's edge while the bar is visible, at the window's
   top edge once it auto-hides. `details.dict>summary` no longer transitions
   `top`: the bar's box changes size in a single frame (the shell re-publishes
   the top inset on every frame of a system-bar swipe, and a two-row wrap or a
   growing `<select>` does the same), and a .28s offset arrived after the edge
   it aimed at — which is the header that was seen tucked under the bar while
   scrolling up. Same doc, bullet "The first section's header is flush against
   the bar, in every state".
3. Three UI moves of the reader's: the "Open first" pair is out of the panel
   (the wiring removed, the machinery — `orderFirst`, `applyOpenOrder`,
   `fastFirst`, the server's `UIPrefs.FastFirst` — deliberately left in place
   for upstream merges, and the stored value is no longer read); the dictionary
   chip moved WHOLE from the search field to the status bar, between the arrows
   and Examples (chip + its real `<select>`, so the shell's picker wiring is
   untouched) — and it TRAVELS: back into the field whenever that bar is hidden,
   which is the empty start and any time the bar is switched off, with "All" in
   the field and the group's own name in the bar; and its place in the panel's
   Results section is taken by a labelled drop-down of the reader's groups
   (label "Dictionaries", in the reading strip's own 96px column), whose choice
   goes through the same `wudictPickerGroupChanged` the native window's spinner
   calls. The chip itself now names the group in force instead of saying "All"
   (which is what removed the old `SCOPE_SHORT` map), and the read is guarded
   because `syncChips` runs before `group-editor.js` declares `userGroups` — an
   unguarded read there kills the rest of the page script (seen on the
   emulator). **Verified on the x86_64 emulator** (`build-android.cmd debug
   intel`, installed over the debug package that was already there): the panel
   drop-down and its label, the group list, the re-run it triggers, the chip
   reading "Test" then "All Dictionaries" in the bar, and the chip sitting in
   the field on the empty start. The row's own spacing came from the reader's
   phone afterwards (third round): the two ends are 10px gutters instead of
   full squares, the chip has a floor of one square of air on EACH side
   (`.rpad.floor` carrying the auto margins, so the two gaps are equal and
   neither can collapse — the phone had the chip pressed against ▼), and the
   fold's handle shortens to "Ex" below 420px, the width at which the full word
   stops fitting beside "All Dictionaries". Same doc, bullet "Open first is
   gone, the dictionary chip lives in the status bar…".
4. The notes' ink: `.scopenote` (the scope, widen and morph notes) is
   `--fg-soft` now, not `--fg-faint` — the reader's "these messages are too
   pale", and `--fg-faint` is 2.9:1 against the light paper.
5. **The preset's paper reaches the windows the app page does not paint**
   (2026-09-28, from the reader: in Warm the Folders/Lemmatization/Browse
   pages were white while `Edit dictionary groups` was warm). A preset layer
   reaches one document, so the colour now travels to the host: manifest →
   `/api/presets` (resolved per theme) → the appearance bridge → `ShellPrefs`
   (`preset_paper`) → `pageBg`, plus a new `shell_paper` parameter and the
   hook's fourth argument for the three pages. **The app page only FORWARDS
   it** (`shell_bg` stays the reader's colour), and the wallpaper is
   untouched. With it: the two pages' checkboxes are DRAWN now (the app's own
   square — they were platform widgets that ignored the paper entirely), and
   the four page rules that kept a platform box's `width:auto` had to drop it.
   Verified in Chromium on a throwaway server plus `go test`, `go vet`, the
   Java compile; **not on a phone**. Statement, the naming of every window
   (report them by these names) and a night-preset defect found on the way:
   `docs/ANDROID-UI-HANDOFF.md`.

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

- **A preset's night half never reaches the page** (found and measured
  2026-09-28, not fixed — it changes what two shipped looks do): the night
  halves declare `:root` (0,1,0) and `app.css`'s dark palette is 0,1,1, so
  True black and Warm dark attach and change nothing (theme pinned dark,
  `--bg` stays `#191a1c`). Day halves are fine: a tie, the layer later. Fix
  shape in the area doc's "Unfinished work".
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
