# Agent handoff — current state

Read whole before planning; keep under ~300 lines. Historical narratives belong in `docs/handoff-archive.md` (grep only; indexed by date) or commit messages. `git show 51c0737:HANDOFF.md` holds the pre-split file. Windows recipes and known baseline failures: `docs/WINDOWS-VERIFY.md`.
## Branch state (verify with git before trusting)
**Current DSL work, 2026-10-02:** user-selected `GD_DSL` at `074cf1e` (Enhancer work committed by user); clean at start. Uncommitted here: persistent Original/GD/Both selection in Dictionary settings, shared per DSL family, default both; bulk affects currently registered DSL sources only. Both cards/indexes remain, unavailable variants excluded from picker/groups/search (including explicit API scopes), no rebuild/deletion on selection. Last surviving copy remains available; source path moves reset selection. Inactive original DSL metadata reads headers instead of auto-preparing; missing active indexes use existing preparation. API/persistence/cache-only tests, targeted prefs/groups/I18n/appearance tests with race, Node localization/card checks, DSL/dict/store, build/vet pass; Chromium EN/RU 320/390/1100px verifies bulk/checks/reload/results/failure rollback and no overflow. Isolated fixture preview http://127.0.0.1:6908, reports/screenshots results/enhancer-preview; reproduction tools/dslcompare/verify_modes.cjs. No commit/APK/device/full-suite run. Prior v4 parser/Enhancer facts: production cleanup/styles ON, GDOptions{} OFF for oracle; native bold/italic/metadata preserved, generated wu-xonly excludes mixed translations, Examples commands unchanged/no GD article button. Five embedded fonts/scoped CSS v4, stylesheet rewrite exemption, existing pre-v4 GD indexes need rebuild; ordinary parser unchanged. Prior key/tree corpus 28 files/3,259,925 cards passes; Stage 1 HTML sample 27,002 cards (2,335 media excluded), only spacing differences. Native scanner/index-byte and full visual GD parity NOT proven. Details: docs/DSL.md section 11 and tools/dslcompare/README.md; full-suite Windows baseline failures docs/WINDOWS-VERIFY.md.

**Rechecked 2026-09-30 (README sync from upstream):** `dev` is at `17910d0`, the merge of upstream `master` (`db28e39`) into it. Upstream's two commits touch only `README.md` — wording, `<kbd>` tags, `[!TIP]`/`[!WARNING]` admonitions, and clarifications (lemmatization and drag-n-drop on the setup page, `NO_COMPRESS`, portable mode) — so no code changed and nothing needed building. One conflict, the Tips block: upstream restyled it as a `[!TIP]` list with a single cycling theme glyph, where this fork's line reads `☀☾` because its theme control is Auto plus a switch. Resolution took upstream's list and markup with this fork's glyph and its "never remembered" wording, and put back the "audio plays on click" tip upstream's rewrite had dropped. Four typos that pass introduced were fixed here rather than carried — `(morophology)`, "cmd or cmd/PowerShell", "makes the app UI to disappear", "apend" in the README, plus the broken `</kbd<` in the new tip line — and are reported upstream together with the Play description's "uDict is free software". `README.md` never names the fork's own build system, so nothing was owed there. Not pushed.

**Current translation slice, 2026-09-30:** `translation` at `45e99df`, clean at start; category localization and README sync are committed. Known legacy server errors are now translated at web display boundaries, uncommitted. API responses and job behavior unchanged; unknown diagnostics remain literal. Targeted I18n/AppearanceContract and Node rendering/fallback checks passed; no APK/device check. Rare errors and device cases: `translation_todo.md`; adapter rules: `translation.md`. Earlier snapshots below are historical.
**Rechecked 2026-09-30 (release):** published this branch, at the user's request, as a **preview release** — `wudict2-v0.6.0-ru.1`, annotated tag on `01a1438` (the changelog commit), APK from `build-android.cmd release`: `versionName='wudict2-v0.6.0-ru.1'`, versionCode 426, `locales: '--_--' 'ru'`, `web/i18n/ru.json` and Russian strings confirmed inside the packaged `libwudict.so`, sha256 `d1af995b935fcefbcdf69cff0bdf38b02fef85dcb6dc46fe2f0ae84833b56935`, the same signing certificate as every earlier release. Marked **pre-release** deliberately, so `latest` stays the stable v0.5.0. Not merged into `dev`; the build carries `dev` as well, which is why its notes also describe the unreleased `dev` work (the upstream catch-up and the fork work since v0.5.0). Checked before publishing: `TestI18n` and `TestAppearanceContract` green, `git diff --check` clean; `make i18n-check-js` NOT run — there is no Node on this machine. Narrative: `docs/handoff-archive.md` → "Release wudict2-v0.6.0-ru.1"; translation state stays in [translation_todo.md](translation_todo.md). Documentation audit, 2026-09-30: current branch `translation_layout_fix`, HEAD `ace52db`; only these documentation edits are pending. Both translation documents are now in English; the audit clarified static/dynamic key limitations, initialization order, native picker text sources, the remaining CSS toolbar exception, and verification boundaries. No application code, build, or device checks in this audit.
**Rechecked 2026-09-30 (second pass, layout fix):** branch `translation_layout_fix` at `ba4d021` (`transalation docs`), clean at session start; the whole translation series is committed by the user. **Uncommitted here: the Settings drawer's row labels are now a MEASURED column per section.** `panelRowColumns()` (`internal/server/web/index.html`, called from `showPanel` and on a window resize while the drawer is open) measures each section's labels with a `Range` and publishes the longest as `--rlcol` on that section; the Appearance rows' label rule and the reading strip's `.seg>span` take it as their `flex-basis`, so labels are one line and the controls of a section begin at one x — 92px/86px in Russian and 61px/93px in English. Three rules ride with it: a row wraps only when its OWN content does not fit; a row that cannot hold the column (the Presets row, whose control is a menu plus three 32px actions) keeps its own label width instead of losing its line; and the strip's segs stretch to their row, without which the column never reaches labels that live inside a shrink-to-fit seg. The `#panel:lang(ru)` mid-word wrap hack is gone from `i18n.css`, and the Presets row's three icons travel as one `iconrow` flex item so a wrap cannot part them. The reader's two reports were the Russian drawer breaking "Оформлен|ия"/"Компактн|о" inside the old 60px column and clipping "Читать вслух" inside the strip's 96px. Verified in Chromium at 320/360/375/393/412px in RU and EN (alignment holds from 375px up in Russian, 360px in English; below that the fallback rule keeps the rows on one line where it can) and under a simulated 1.4× Android text zoom; no label clipped and no horizontal overflow anywhere. `go build ./...`, `go vet`, the targeted I18n/AppearanceContract/asset tests and `git diff --check` pass. **NOT verified on a device** — the reader's own phone pass is owed. Rules and measurements: `docs/ANDROID-UI-HANDOFF.md` → "The drawer's rows are read down ONE column per section"; translation rule: [translation.md](translation.md) → "Label widths and layout"; the device checklist item is in [translation_todo.md](translation_todo.md). No branch switch, commit or publication; no APK built or installed.

**Rechecked 2026-09-30:** user-selected `translation` at `04b8211` (Translation 5), clean before this session. Earlier translation slices are committed by the user; remaining identified UI messages and article-frame controls are now translated, uncommitted here; no branch switch, commit or publication. `master` remains upstream-only. The user will build/install/check this slice in the emulator themselves; no APK was built or installed this session.
**Localization:** manual Language → English/Русский → apply and reload; English default, no system-locale detection. Browse and native Android resources are translated. This slice adds the Settings drawer, appearance subjects/options, built-in look/layer display names, CSS editor hints and related dialogs/messages. Dictionaries/search/speech stay unchanged; main `html lang="en"` preserves word segmentation, with selected `lang` scoped to UI containers. Names/IDs in stored looks, groups and the preset manifest remain unchanged. Translation rules: [translation.md](translation.md); remaining work/device checks: [translation_todo.md](translation_todo.md). Earlier verification history: `docs/I18N.md`.
Verified this slice: targeted Go localization/appearance/preset tests and Node syntax/catalog/plural/name-preservation checks pass; git diff --check clean. Visual/device checks are left to the user, including the CSS toolbar at narrow widths (the Russian labels of the drawer that wrapped mid-word were fixed on 2026-09-30 — see the branch state above). Dictionary Settings and Dictionary Groups dialogs are now translated too: cards, index/rebuild/removal messages, bulk FTS confirmation, group hints and known validation errors. Node renders a dictionary card to check escaping and stable action IDs; targeted Go/JS checks pass. Folders and Lemmas pages are now translated too, including import/download states and folder intros; targeted Go/Node checks pass. Temporary Language buttons on Browse/setup removed by user request; the Settings entry remains. Device checks left to the user. Search/results UI is now translated; Go localization/appearance and Node result-rendering/plural checks pass, preserving article content and action IDs. Device verification is left to the user. System Settings and speech UI are translated too; Go/Node checks pass, including unchanged override payloads and speech-language decisions. Android row labels use existing localized resources; generated voice labels use UI locale without changing voice IDs. Device checks left to user. Identified remaining save/audio/style messages and iframe subentry controls are translated; Go/Node checks pass, including escaping of iframe UI labels with article HTML unchanged. Remaining translation: technical error details (metadata/filter labels completed in the category-localization pass above); user device audit may find further literals. Full suite was not rerun; its ten known baseline failures are in `docs/WINDOWS-VERIFY.md`. Crash fix, 2026-09-30: emulator AndroidRuntime records BadTokenException/token null in DictionaryPicker.show. UiLanguage.context now uses ContextThemeWrapper with locale override, retaining Activity window services; FOSS/Play Java compilation passes. Both picker openings still need user verification in a rebuilt APK. Prior native slice: FOSS debug installed with data preserved, Russian native settings fit; exhaustive import/notification/physical-phone checks remain.
Earlier release/branch snapshots and picker, label-register, appearance-test and `master_build` notes moved intact to `docs/handoff-archive.md` → "Pre-assessment branch snapshots (2026-09-29)"; live UI rules and device checks remain in `docs/ANDROID-UI-HANDOFF.md`.

**Rechecked 2026-09-29:** `dev` is at `f09ee61`, the merge of upstream `master`
(`d3f3936`) into it; `dev2` (`932e833`) is an ancestor of `dev`, so `dev` is the
one full line. Upstream's fifteen commits brought **wumark** — a whole new
dictionary format (`internal/format/wmd` with goldmark/v2, the `dump --md`
writer, `docs/WUDICT-MARKDOWN.md` and its examples, format detection and
companions) — plus the `bword` → `entry` link canonicalization fix and the
Android share/reader lookup intents. Eight files conflicted; the merge message
carries the resolutions, and three of them are standing fork decisions worth
knowing without reading the code:

- **The wudict howto is not wired.** `internal/cli/cli.go` deliberately leaves
  `HowtoDir` empty: it neither installs nor registers the guide, because a
  listed built-in would make the registry non-empty on a first run with no
  dictionaries of the reader's own and `/` would open the guide instead of the
  setup page. That first run is the fork's, so the guide stays out of the
  registry and out of the picker; `internal/howto`, its `/api/howto` endpoints,
  the guide file and its tests all remain in the tree, unwired. `setup.html`'s
  "bring back the howto" row and the `d.builtin` branch in `askRemoval` are
  kept whole but can never fire (`HowtoRemoved` is false while `HowtoDir` is
  empty).
- **The share/reader intents are ported into the fork's structures.** The
  toggle is a fourth `sys-row` in the web System pane (`#sysReader`, with its
  state sync and listener in index.html) and travels over the existing
  `wudict:system` bridge — `Shell.systemState` puts `reader`, the set handler
  writes `ShellPrefs.READER`. Upstream's row in `SettingsActivity` is NOT
  taken: this fork draws those rows in the page (the file's own D100 note).
  The manifest filters, `LookupActivity.fromReader` and `Intake` merged clean.
- **`docs/OPEN.md` numbering:** upstream's O11 (wumark bundle) and O12
  (admonitions) keep their numbers; the fork's O11 (single-dictionary scope,
  CLOSED) is now **O13**, and the five references in `docs/handoff-archive.md`
  were updated with it.

Verified: `go build`/`go vet` clean (the new goldmark dependency downloads
fine); `go test ./...` leaves nine failures against `dev`'s two — the seven new
ones are upstream's own Windows problems in the wumark packages, all of them
reproducing on clean `master`, and six disappear in an LF checkout
(`docs/WINDOWS-VERIFY.md` carries the list). The page was also checked live: a
throwaway server
(isolated `USERPROFILE`, port 6902, pure-Go build) served the merged page, the
scope picker listed only the reader's dictionary (no guide), and the panel
counted "1 folder · 1 dictionary"; a second server on an EMPTY dictionary
folder served the SETUP page for `/`, which is the first-run order this fork
keeps. The System pane itself is shell-only, so the new row is verified in the
page's source (and by the Android string-reference check), not in a desktop
browser.

**The shell's own settings now live in the app page: the `System Settings` window
(2026-09-28; the first half is committed as `ae0958f` "Move system to
settings", the rest is uncommitted).** The ☰ drawer gained a `System` section —
a door row, drawn only where the shell answers — opening `#sysSettings`, a
window of the app's own family carrying the shell screen's rows (three lookups,
info messages, access key, Clear browser cache above Advanced, the Advanced
override rows, Restore defaults) over a new `wudict:system` prompt
(`Shell.systemState`, plus `ShellPrefs.setOverrideChecked`, which the shell
screen shares). `SettingsActivity` keeps only the **Server port** row (the one
row that can lock the app out — a port that will not bind means no page, and
`Restore defaults` is all-or-nothing and points at 6889, which may itself be
the busy one), a duplicated `Restore defaults`, the restart footer and the
signpost; 18 strings and the effective-values cache went with the other rows.
Three rules hold the two views together: every write goes through
`setOverrideChecked`; `ServerProcess.port` answers "the server we can reach"
(the live child's port while one is up, the configured one otherwise), so a
stored port change is inert until the next start and no window knocks at a port
nobody bound; and a FAILED start is retried on a focus gain
(`ServerProcess.failed` + `MainActivity.retryServer`). Verified on the EMULATOR
(AVD `Small`, `debug intel`, 2026-09-28) with the real bridge, including the
busy-port walk end to end: adbd held 5555, the app failed with the child's own
diagnosis, the native row took 6901, and the retry brought the app up on it.
That pass found four bugs, all fixed: a byte count read as megabytes in the
memory fields (every size row now states its unit), the prompt bridge falling
through to the WebView's own dialog after a port row changed
(`Shell.notePageUrl`/`ownPage`), the stale note reading the state from before
the write, and the native footer probing the configured port. Live rules, the
evidence and what is owed on the phone: `docs/ANDROID-UI-HANDOFF.md` → "The
System Settings window: current work"; emulator/Bash recipes:
`docs/WINDOWS-VERIFY.md`.

**The 09-26 upstream sync (into `dev2`) moved to the archive:
`docs/handoff-archive.md` → "The 09-26 upstream sync into `dev2`".** It holds
the five upstream commits it carried, the two conflicts, the port shape to
reuse on the NEXT sync (take OURS everywhere, then port upstream's deltas —
CSS into `app.css`, JS/markup into the page — diffing against a regenerated
`merge-work/index.full.html`), the new known Windows failure it inherited
(`TestRescanSeesEditedSource`, identical on clean `master`), and the
`merge-work/inline.go` fix. `master_build` (below) is what that sync feeds.

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
