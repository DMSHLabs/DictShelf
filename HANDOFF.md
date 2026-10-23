# Agent handoff — current state

Edit Folders in the embedded main-window dialog now uses the main window cached folder configuration immediately, reveals the import section as soon as the configured destination is known, and no longer writes index defaults on open. Folder validation no longer appears as a global indexing task. `node --check internal/server/web/setup.js` and `git diff --check` pass; Windows runtime confirmation is pending.
Settings → Appearance has a saved On/Off control for dictionary icons. It defaults on; the shared icon style hides current and future icons without rebuilding result lists. Go build and Windows UI verification remain pending.
MDX prepared indexes now remove empty elements with known advertising-slot IDs (including numbered `ad_contentslot_N` slots); nonempty and media-bearing slots remain. MDX Reader version is 2, so old prepared indexes are offered for Rebuild. `go build ./internal/format/mdx` and `git diff --check` pass; end-to-end Windows reindex/display verification is pending.

The Oxford Advanced Learner's Dictionary 10th source MDX now has `Title="Oxford Advanced Learner's Dictionary 10th"` instead of `OALDWU`; its UTF-16 header length and Adler-32 were updated, and the reader reopened it and looked up `dog`. The original is preserved beside it as `.mdx.title-backup.bak`; prepared data will be stale from the source size/mtime change and needs Rebuild.

Current checkout: branch `dev` at `3fc8eaff`. No
upstream is configured. The application uses one DSL reader with GoldenDict-
compatible repair and article preparation; there is no Original/GD selection
or duplicate DSL index. `docs/DSL.md` and `tools/dslcompare/README.md` describe
that current architecture. The LSD decoder and reader work is present in this
branch; upstream review and pending ZIM checks remain as recorded below. The
user-owned untracked `DictShelf.exe` was preserved. This session adds a saved
Combined list dictionary limit (default 500, maximum 5,000), a specific
over-limit message, and moves Clear browser cache beneath its Browser cache
description. Desktop System settings no longer show internal config key names.
`go build ./...`, `git diff --check`, and translation JSON parsing pass; tests
were not run, and Windows/Android settings UI verification remains pending.

Dictionary names now have a shared icon route: a supported adjacent image file
wins, then a format icon (including OpenZIM Illustration metadata). When no
icon is found the UI leaves the name unadorned. Search result headings and
Dictionary settings show available icons; browser, Windows WebView and Android
WebView serve them through their local Go resource path. `go build ./...` and
`git diff --check` pass; UI runtime checks remain pending.

Dictionary icons also appear in dictionary search, export and group lists. A
prepared legacy abbreviation glossary is excluded from the library list when
its discovered parent DSL is compressed (`.dsl.dz`) as well as when it is
plain. The `TestAbbrevCompanion` check and `go build ./...` pass; UI runtime
verification remains pending.
Dictionary Groups renders each name in its own text span beside the icon; the
list-search highlighter targets only that span. Its marks therefore remain
inline and cannot become separate flex items with gaps between letters.
The shared list search now removes old marks before applying each current
query and repaints on both input and search events (including search-field
clear actions); runtime confirmation remains pending.

The icon request is now dispatched by the Windows WebView resource adapter as
well as the HTTP and Android paths. DSL abbreviation companions accept both
`_abrv` and `_abrev` spellings (plain or dictzip): they are absorbed by the
parent glossary and excluded from discovery when that parent exists.

Android log 2026-10-23 05:35–05:37 shows the picker repeatedly spending
1.9–3.3 s in its native user-groups call, while Browse article paint takes
4.4–5.4 s: startup dependencies wait about 1.6–2.5 s, then the 383-dictionary
list takes about 2.5 s, while scoped search itself takes under 50 ms. Android
now passes the existing diagnostic flag into the Go group snapshot and logs
native-bridge stages. The picker reuses the loaded group snapshot instead of
fetching it on every open. Browse headword links carry `from` as well as `dict`;
startup skips the group read for scoped article links and defers it until after
the list/search has had two animation frames to paint. Log 2026-10-23 11:21
confirms first Browse article paint in ~0.4 s, but also shows a second search
after list completion; that retry lost the one-dictionary scope and could show
all-dictionary results. Scoped searches no longer mark themselves stale just
because the full dictionary list is still loading, and any necessary retry
preserves its scope. Latest log still shows group linking at 3.4–3.6 s for 383
dictionaries; optimizing that native snapshot is separate follow-up work.
Linked-group synchronization now reads each dictionary's metadata at most
once per snapshot, even with several linked groups. With Browse/article
diagnostics enabled, `dictionary_picker_groups` splits metadata, matching,
membership updates and total time; Android runtime timing remains pending.

The existing Browse/article navigation diagnostics checkbox failed to record
direct Browse headword clicks because those links carry `dict`, while the main
page diagnostic gate required `from`. The gate now accepts `dict` and uses it
as the article dictionary ID; the Android log confirmed `article_transition`
stages from the click through first article paint.

Wildcards' combined mode keeps the dictionary field's geometry, displays
“Combined list” / «Общий список», and removes the dropdown affordance while
disabling selection. The merge icon's tooltip describes its next action in
English and Russian. After a combined SQL-cache stream completes, a missing
visible page is read again after the stream closes, so the lookup can see the
completed cache. Android runtime verification remains pending.

Combined Wildcards top-level result rows now store their own result ordinal as
the base position, and the renderer uses that ordinal when highlighting and
scrolling. This fixes match navigation when all dictionaries are merged; the
merged SQL signature changed to v4 so stale position rows rebuild. `go build
./...`, inline JavaScript syntax and `git diff --check` pass; Android runtime
verification remains pending.

Browse and Wildcards ribbon clicks reveal a partly clipped heading and, when it fits, one neighboring heading. Clicking the first or last heading aligns to that strip edge; Browse no longer centers each selected heading or scrolls when it is already fully visible. Shared ribbon geometry checks and inline JavaScript syntax pass; Windows/Android interaction remains to be checked. Wildcards toolbar: the merge-books control is fixed to 3.2em with a larger icon; page controls show “Next page” / “Previous page” when the row has room, fall back to arrows, then hide before the page counter on narrow rows.

Browse's “Loading headwords” page indicator now waits 500 ms for both ordinary page reads and in-flight page prefetches, avoiding a flash on quick Next transitions. Runtime verification is pending.

Browse deep headword jumps spent about 450 ms in `Locate`'s SQL count even when fewer than 100 rows were counted. The query now gives SQLite an explicit lower bound at the closest 100-row anchor, and the anchor ordinal includes the anchor row; focused position/query-plan checks pass. Wildcards per-dictionary lower search retains base offsets. Combined lower search carries both source and upper-mask base positions; navigation, highlighting and Find from here now use the latter, with a new indexed cache signature. Runtime checks remain pending. Browse diagnostics split anchor lookup, bounded count, page read, HTTP response, and Android bridge timings.

Browse `*home*` with trigram enabled failed because the candidate SQL joined two tables that both expose `w`, while the shared internal-entry filter used unqualified `w`. The two joined mask-candidate queries now qualify the filter as `e.w`; runtime verification is pending.

Selecting Wildcards resets its view to separate dictionaries; explicit URL/history/article-return restoration keeps its saved view. View switches restore completed metadata and visited pages from bounded memory caches (128 entries, 30-minute TTL). The 17:58 log shows repeated cached switches in 63–77 ms without spinner; initial SQL builds remain long.

Browse/Wildcards diagnostics correlate switch and run IDs, requested query/group, FTS/trigram variant, exact browser cache key, saved cache ID, alternative index variants, session-write result, SQL page reads, build/resume, spinner and task-panel visibility. This distinguishes a new submit from a view switch and a settings-key miss from a lost cache entry. Wildcards rows snapshot FTS/trigram settings for cache identity; lower rows inherit them. Returning to completed caches reads only a missing visible page and keeps the spinner off. The merged signature validator accepts both index flags and the selected dictionary IDs; runtime lower `dog` searches succeed.

System Settings → Search now applies optional FTS/trigram indexes to both Wildcards SQL result databases. In the separate-dictionaries database, upper-mask words are indexed with their dictionary and base positions; the lower search selects FTS candidates for plain token queries, trigram candidates for masks with a 3+ literal, and scans blocks otherwise. The index combination is part of server and browser cache identity. Focused four-combination tests, index rebuild tests, Go build, JavaScript parse and diff checks pass. Runtime timing on Windows/Android is pending.

At the 2026-10-21 stage, branch `dev` was at `5bdb31cf`. Browse's match-count button,
“Find from here” action and indexed candidate search in the merged Wildcards
database are committed. Opt-in Browse diagnostics now logs
per-request headword search mode/source, first event/word and total timings,
found/scanned counts; merged search additionally logs cache hit/miss, selected
index, candidate count, and cache-build time. The existing Settings → System
Browse/Wildcards diagnostics checkbox enables it in both views. The 2026-10-21
System Log includes an earlier Wildcards run that scanned dictionary indexes
(up to 61 s) and a later run using the upper-mask SQLite cache. That cache run
still took 16.6 s on 197k rows because it emitted one event per match; batching
now emits the first result immediately and subsequent results in batches. Go
build passes; runtime timing after this fix is pending. Plain Browse `dog` in a
9.1M-entry dictionary took 11 ms; merged Wildcards narrowed `dog` to 6382 FTS
candidates and built 6249 results in 115 ms. Merged filter caches now resolve each match's base position through
the unfiltered mask's indexed result cache; their signature changed so old
incorrect positions rebuild. This addresses “Find from here” after jumping to
the middle of the combined list. Ordinary Wildcards' lower search for selective
top masks now scans completed result blocks in the wildcard SQLite cache, not
dictionary indexes, and records the same base offsets. Streaming search and
later page/position reads now require a valid wildcard SQL cache; missing,
malformed or unavailable cache data returns an error instead of falling back
to a dictionary index. Combined mode passes its prepared merged-database ID to
the lower search, validates the selected dictionary IDs from SQL metadata, and
does not reopen dictionaries on that path. The Wildcards UI cache-key version
changed so an older session cannot restore a combined row without that ID. The UI waits for upper
mask completion and shows one aggregate word-count progress with the currently
scanned block's dictionary name. Masks made only of `*` and `?` normalize to a
minimum-length mask and build the same complete 300-word SQL cache; all-star
queries no longer use lazy page-only storage. Runtime UI confirmation remains.
Earlier Browse/Wildcards, Android UI,
and task-progress work is included in the checkout. Browse and Wildcards SQL
result caches now use indexed 300-headword blocks, retain files
across idle periods/restarts, and share a five-file/age/disk-size cleanup policy.
Settings → System controls the combined cache disk cap (2 GiB default on Android,
8 GiB on desktop, or No Limit).
Old cache schemas are prepared on first open. `go build ./...`, inline JavaScript
syntax checks, catalog JSON parsing, and `git diff --check` pass; the FOSS debug
APK also builds successfully. No tests were run, and the APK was not installed.
Windows/Android UI and long-running cache behavior need runtime verification.
Merged Wildcards progress now uses one scale across building the combined SQL list
and applying its mask, reserving 100% until both caches are committed. Switching
back to per-dictionary mode reads completed SQL-cache pages without showing a
full-search task; runtime verification remains pending.
Combined Wildcards diagnostics record index combination, cache hit/build, row counts and elapsed time. Each combination stays isolated.
Android UI changes keep
the Browse A–Z heading and Find/Filter dock fixed while its dictionary list
scrolls; the Reading it word by word hint stays with the heading above its
divider, and filter mode is selected by default.
The Wildcards merge button uses a two-state SVG: several books → one book, or
one book → several books. Its title and accessible name describe the next
transition in English and Russian; it sits beside Total and block paging in
the Wildcards header. The result-count control is visibly button-shaped. The
normal per-dictionary and merged views use separate SQLite caches; after the initial merged cache is
built, toggling back and forth reuses each view. The merged list is deduplicated
and globally sorted, applies the active mask and filter, and retains source
positions for paging and “Find from here”. The selected view is kept in the
search URL and article-return link. The previous/next-match buttons now retain
the merged flag and source dictionary IDs when creating filter rows; without
those fields, merged-match requests silently queried a nonexistent dictionary.
Cache eviction links both views by dictionary group and FTS/trigram setting, including merged find caches; an active member protects the whole group.
Match pages now also carry a separate ordinal within the masked merged list:
the source position is still used to highlight the exact headword, while the
masked ordinal selects the correct base page. Older merged-find SQL caches are
rebuilt automatically if they lack this ordinal column.
`go build ./...`, inline JavaScript parsing, catalog JSON parsing and
`git diff --check` pass; the FOSS arm64 debug APK builds successfully and was
not installed. Windows/Android runtime behavior remains unverified.
The Android-only `A60` header marker is now shown only by debug builds. The
wildcards results now use a full-width unboxed surface with compact top controls.
Browse and wildcard headword searches reveal their first matching word as soon
as the stream provides it, including searches started by Apply. Wildcard SQL
cache pages reassemble variable-sized stream events into fixed-size pages
within each dictionary, preserving the group's dictionary order during parallel
search. The wildcard search match counter opens a localized Go to match dialog
and navigates to the selected result. In Wildcards search mode, right-clicking
or long-pressing a headword opens “Find from here”; the chosen headword receives
a rectangular outline and subsequent match navigation starts beside it. Opening
an article stores the clicked word and restores it to view on return via ← Word.

Browse's bottom match count is now a button that opens the existing customized
Go to match dialog. In Browse search mode, long-pressing on touch or right-clicking
a headword opens “Find from here”; it outlines the selected word and uses SQLite
NOCASE-compatible binary bounds so Next starts after the word and Previous before
it. `go build ./...`, inline JavaScript parsing, and `git diff --check` pass;
Windows/Android interaction still needs runtime verification.

Merged Wildcards search previously scanned every row in the combined SQL table
in Go and rebuilt the query result cache. The merged database now builds FTS5
and (when supported by SQLite) trigram indexes; plain headword queries and masks
with a literal run of at least three characters use SQL index candidates before
the existing exact matcher. A full scan remains for masks without a 3+ character
literal or any indexed bottom-search term. The base cache signature version
changed to rebuild older merged databases. Go build and `git diff --check` pass;
runtime speed and SQLite index behavior are not yet measured on Windows or
Android.
Windows log 2026-10-21 19:35:28 exposed a slow Wildcards lower `dog` search inside `*d*g*`: SQLite ordered the dictionary table before FTS, taking 31 s for 2,045 candidates and over a minute for 2,771. The candidate query now forces FTS first with `CROSS JOIN`; the latter query takes ~10 ms against the saved cache, but app runtime remains unverified.
The ordered SQL wildcard cache supports seeking to matches before or after a
headword position across dictionary boundaries.
The live Wildcards pane now assembles visible 300-word pages by cumulative
result offset, since stream events can be shorter than a page; treating one
event as a page shifted the outlined match onto unrelated words. A completed
visible page is reread from the SQL cache.
“Find from here” treats the outlined word as a cursor between matches: if 100
matches precede it, the counter shows 100 and the first Next/Previous goes to
match 101/100. Explicit Go to match clears that cursor. A single dictionary
also wraps match navigation at its ends.
Position lookup now uses indexed first/last positions and the starting match
ordinal of each SQL cache block; SQLite selects one block via its B-tree and Go
binary-searches that block's positions. The unreleased cache has no schema
migration; remove existing Wildcards cache files before using this change.
In-memory search positions use binary search too. Focused position,
page-boundary, and SQLite query-plan checks passed before migration removal.
Android versionCode now uses 941 as a baseline at commit `bb2a4b17` and
increments for each later commit; all flavours and build types share it. The
FOSS debug APK for arm64/x86_64 built successfully with manifest versionCode
941. Preserve all other working-tree changes. Android uses the in-process
Go/JNI bridge and opens no TCP listener.

Startup progress cleanup: successful initial dictionary-list loading now clears
its status immediately and removes only its transient loading task even when
indexing is also active. The independent indexing task remains visible with its
own progress. The deferred progress asset also initializes its modal-dialog
tracking before publishing its task API; otherwise an already-running startup
load could throw during the ready event and prevent progress rendering. Android
debug APK rebuilt and installed alongside release on the connected phone. The
WebView reaches phase `ready` with 21/21 dictionaries, an empty status row, no
visible server-work notice and zero active UI tasks; the dialog initialization
exception no longer appears. A follow-up removes the legacy startup status
counter entirely, including its unused EN/RU catalog entries: the task manager
is the only loading indicator, with a plain task label. Release app remains
installed and untouched.

Windows CI run `39398437050` passed toolchain setup but failed in `runtime/cgo`
with `cgo.exe: exit status 2`. The checked-in fix for the old local GCC path is
present. A local Go 1.26.5 cgo Windows build succeeds with Qt GCC 13.1. Both
Windows workflows now use the `windows-2022` image's `C:\mingw64\bin\gcc.exe`
instead of installing a rolling MSYS2 toolchain, and print its version/target.
The runner image documents GCC 14.2.0 and binutils 2.44. CI rerun is pending.

Windows builds use `build-windows.cmd` for the listenerless WebView app, `cli`
for the console CLI (no-argument help, no TCP listener), `server` for the
console HTTP server, and `installer` to package the already-built desktop exe.
Both Windows workflows build and upload all four artifacts.

Create portable progress: the job first counts planned source bytes using the same traversal and exclusions as copying, then reports total bytes and copied bytes. The dialog shows an indeterminate counting phase, a percentage bar/current file while copying, and 100% plus the created-path confirmation only after successful publication and job completion. The Create button stays visible and disabled after success for the completed path; manual edits and the folder picker enable it for a different path. Reopening the dialog preserves that distinction. Portable controls initialize only in the Windows shell; Android and other non-Windows shells remove the command and dialog from the document. Mocked completion/path-edit and Android-exclusion checks pass. Focused portable/catalog tests, inline JS syntax, mocked progress/completion/cancellation states, Windows cgo GUI build and git diff --check pass; Windows visual verification remains pending.

Current audit snapshot: `dev2` at `722daad4` (`Portable`), initially clean. Static Windows portable write-path audit fails confinement: WebView spill files, markdown decoding, external Speex and native SQLite temporaries can use system temp. External dictionary roots are swept at startup and the first root receives imports. WUDICT_HOME precedence, invalid-Data fallback, post-validation CLI library overrides and lexical-only path checks also prevent a strict guarantee. See [docs/PORTABLE-WRITES-AUDIT.md](docs/PORTABLE-WRITES-AUDIT.md) for evidence and the remediation scope. The subsequent GUI-log fix directs wudict.log into the active data root/db.logs (Data for portable, WUDICT_HOME or ~/.wuDict2 otherwise), independently of a custom library path. Go build, Windows cgo GUI build and git diff --check pass; no installation or commit. Other audit findings remain unresolved and filesystem tracing remains unperformed.

Previous implementation snapshot: `dev2` at `0bca0bc9`. Uncommitted Create portable work adds a Windows Settings → Tools dialog with output folder, native picker, optional dictionary copying, progress and Stop. It copies the running executable and active user data into a staged portable folder, snapshots live SQLite databases, includes external library/lemma paths, and copies complete dictionary roots (resources included), stripping their common ancestor or adding volume directories across drives. Output must be new/empty and separate from data sources. Source files retain timestamps; originals are untouched. Portable WebView profiles now live under Data/WebView2; this regenerable live cache is excluded when cloning an already portable installation. Focused portable, WAL snapshot and catalog tests pass with both SQLite drivers; Go and Windows cgo GUI builds and inline JavaScript syntax pass. Manual dialog and relocated-startup checks remain pending. Existing large-media fixture changes were preserved; no commit or installation was made.

Portable path correction: startup converts only absolute dictionary roots within the current Data/Dictionaries to relative paths. External roots remain unchanged and pass portable validation; special copied-index relinking is limited to internal dictionaries. Create portable without dictionary copying preserves the copied DICT_DIR settings. Focused config/CLI/server tests cover mixed internal/external roots, prefix lookalikes and idempotence; relocated startup remains a manual check.

Previous checkout snapshot for the upstream resource-transfer session: branch `dev2` at
`6fefaa87`, with uncommitted SLOB and upstream P7 candidate changes. P1–P5 are
implemented; three low-risk P7 fixes are also implemented: MDict encoding
recognition, XDXF attribute escaping, and duplicate dictzip FIFO insertion
prevention. Full Go tests, `go build ./...`, and `git diff --check` pass. The
race detector could not run because GCC/cgo is unavailable. Manual verification
with real large ZIM files, a real multi-part ZIM, and a large SLOB remains
pending. See [`docs/UPSTREAM-TRANSFER-PLAN.md`](docs/UPSTREAM-TRANSFER-PLAN.md)
for remaining candidates and verification gates. P6 is the next planned stage.

Previous handoff snapshot (before the upstream resource-transfer plan): `dev2` at `78a18610`, with uncommitted Wildcards responsiveness changes, ribbon diagnostics, and a responsive result header. The header uses equal-height dictionary and page controls based on the edit-control UI tokens; its page controls keep their size and move below the dictionary selector only when its text field approaches 20 characters. At narrower selector widths, its match count and page range hide before the dictionary name is ellipsized. When the existing Browse/Wildcards timing option is enabled, System Log records each actual Wildcards ribbon rebuild (SQL adapter, geometry build and viewport paint timings) and one debounced summary per window-resize burst (event count, viewport dimensions, metrics changes and paint deferrals). The 2026-10-20 01:36 log shows six successful resize summaries, but repeated stream updates can spend 100–140 ms measuring/painting the visible strip and up to 254 ms in a synchronous UI render. Wildcards now caches estimated label widths across data updates, measures visible button widths in one DOM layout pass, and skips rewriting the current word list when another dictionary reports progress. Resizing its ribbon does not rebuild the full section model unless font metrics change; the viewport repaint is scheduled once per animation frame. The Wildcards stream yields to browser painting after about 12 ms of continuous event handling, and scan-only progress does not rerender the word list or ribbon. Inline JavaScript syntax, `go build ./...`, and `git diff --check` pass; Windows performance and visual behavior still need runtime verification.

Before a Wildcards query, the results pane is blank, the scrollbar is hidden, the ribbon arrows and dictionary picker are disabled. The picker becomes available after all dictionary searches settle and at least one match exists. Runtime verification pending.

Wildcards uses the Browse workspace layout: its fixed title/ribbon/scrollbar are above a separately scrolling headword pane, with the horizontal scrollbar inside the ribbon header. Browse and Wildcards arrow buttons use one CSS class and the existing shared ribbon scroller; Wildcards passes its arrows to that scroller. The Wildcards multi-column list has an inner column element so the constrained scrolling pane grows vertically, and saved word-list scroll positions use that pane.

Browse and Wildcards call the same `dict.SectionLabel` rule using the previous page's last title and the current page's first and last titles; Wildcards restarts label history for each dictionary. Wildcards keeps estimated chip widths for scrollbar position and measures visible chips using a real hidden button with the same CSS, then places the visible chips using those widths. The empty strip is guarded before local measurement. Visual verification remains.

On Windows, a second GUI launch now locates the existing `wuDict2` window, restores it, moves it into the current monitor's work area if needed, and focuses it without an already-running alert. Console launches retain the existing duplicate-instance error. WebView2's native hovered-link status overlay is disabled so destination URLs do not cover application controls. `go build ./...` and `git diff --check` pass; GUI relaunch/offscreen behavior still needs runtime verification.

Official Make and GitHub build entry points now use cgo/FTS5 only; the Pure-Go host, cross, parity-test, and release workflow targets are removed. Cgo release builds cover macOS/Linux amd64+arm64, Windows x64, and Android arm64; Linux ARMv6/v7 release artifacts are no longer produced. The underlying source fallback remains available to direct tag-less or `-tags purego` Go commands until a separate source-level change.

Settings → Appearance now has Fonts, Font size and Font weight rows in that order. Charis is the default; the selected family including System is stored in UIPrefs and applied to shadow articles and scripted frames. IPA/transcription elements always use Charis. Embedded fonts are available via same-origin assets for Windows and Android. Build passed; browser/device rendering remains unverified.

Applying a built-in appearance look (Clean, Warm/Sepia or Old Paper) switches Settings → Fonts to Charis, enables Hide examples without [*], and persists both choices. User-created looks keep the current Fonts and example-filter choices.

Appearance panel controls now stay beside their label column instead of being pushed to the far edge. Preset icons share a wrapping group with the dropdown, so they remain beside it when they fit and wrap underneath aligned to its left edge when they do not. Build passed; visual confirmation pending.

All 13 font assets in web/fonts are registered for the document and scripted article frames. Charis and Gentium Regular/Medium/Bold normal and italic WOFF2 faces map to CSS weights 400/500/700; Quivira OTF remains available as a fallback/selection. Build passed; rendering remains unverified.

Hide examples without [*] is indented by 4ch and disabled while the Examples layer is off. Settings dropdown controls use Language's shared control height/font tokens. Search as you type and Fonts selects now use a drawn triangle and zero vertical padding to align the text. Browse A-Z now ends the Dictionaries section with row spacing. Build passed; visual confirmation pending.

Export output-folder label, path input and folder button now share one non-wrapping row; the path field takes the remaining width. Runtime layout confirmation remains pending.

Export group checkbox now inherits the same 20px box and checkmark geometry as dictionary rows; only its mixed-state square has a dedicated centered mark. Removed the em-sized override that shrank the box without moving the shared checkmark. Runtime visual confirmation remains pending.

Export defaults to separate files. The group checkbox uses a centered square for mixed selection, a check for all selected, and an empty box for none. Runtime appearance remains unverified.

Export dialog now follows the supplied layout: group checkbox with mixed selection, selection count aligned right, one-line folder picker and Save/Cancel footer. Successful completion shows only the exported dictionary count (n of T exported); Cancel requests Stop before closing. Visual runtime check remains pending.

Settings → Tools → Export headwords now opens a shared-style dictionary list with groups, search/filter, checkboxes, Select all/none, separate-file choice and output folder. Export streams current prepared headword indexes to UTF-8 text (one displayed title per line, aliases/repeated titles retained; internal DSL subentries follow Browse exclusions). Existing filenames receive numeric suffixes. Stop removes the unfinished file and keeps completed separate files. Windows and Android use direct Go operations; browser mode has a protected POST adapter. A writable filesystem folder and an existing headword index are required; Android Play SAF destinations are not implemented. Current checkout is `dev2` at `f3c68bb6`; export changes are uncommitted. Go build, focused Unicode/cancellation/file-preservation tests, catalog checks and inline JavaScript syntax pass. Android FOSS Java compilation passes; APK/native JNI build and Windows/Android visual/runtime export checks remain pending.

Browse filtered results now have their own virtualized strip, one marker per available 300-word page labelled by its first headword. Chips and scrollbar navigate filtered pages. The full dictionary alphabet remains separate in its existing cache; its measured geometry is saved across filter toggles and restored without rebuilding when metrics match. Runtime verification is pending.

Browse typing and Wildcards result-filter debounce both use the shared 500 ms `indexconst.SearchDebounceMS`, restarted by every input edit. Browse's shared task panel sits inside the workspace immediately above the footer, shrinking the headword pane instead of shifting the input upward. Runtime layout/typing verification is pending.

Browse headword search finishes its own progress task on the done event and unconditionally in finally, including canceled/superseded requests. Task cleanup no longer depends on the current query sequence. Runtime confirmation of the reported lingering Searching headwords message is pending.

Results toggles keep their independently measured label width when the section computes its other label columns; the shared pass no longer replaces it with the wider section column. Their On/Off pairs begin just after the longest of those two labels. Groups: (Группы:) and Search as you type use natural-width labels and dropdowns filling the remaining row. Visual verification is pending.

Browse input search uses the shared `indexconst.SearchDebounceMS` (500 ms after the last edit). The input stays enabled during search streams and page/alphabet loading, and page-loading keyboard guards allow typing in it. Runtime focus/continued-typing verification is pending.

Settings places the Search as you type label before its dropdown. Language, Groups, Search as you type, Presets and Fonts dropdowns now use the same physical height as Edit dictionary settings: their height follows the base control size instead of shrinking with the smaller dropdown text. Runtime layout verification is pending.

Search as you type options now explicitly read Never, Below 10k/50k/100k/500k/1m, Always (localized in EN/RU); saved values and threshold behavior are unchanged. Runtime layout verification is pending.

Settings → Search as you type adds Always (Всегда), saved as -1 in the existing threshold preference. Shared Browse/Wildcards controls then search/filter on input regardless of headword count; existing numeric thresholds and default remain unchanged. Runtime verification is pending.

The built-in Warm appearance enables Window background → Color in both themes: #F4ECD8 in light and #1C1A17 in dark. Runtime checkbox/color verification is pending.


Current checkout: branch `dev2` at `53065eb8`; changes are uncommitted. Settings → Language is a permanent first row, outside the collapsible sections; System is the default and English/Русский are explicit overrides. System follows the requesting browser/app language where supported (English/Russian), with English fallback; Android resource contexts use the device locale. Browse A–Z now gets the same vertical row spacing as the other dictionary actions. Highlight matches and Read aloud already use the screenshot's measured shared label column and On/Off pairs. Browse and Wildcards use one `wudictHeadwordControl` API for the shared search/mask dock and virtual ribbon. Browse maps prepared-index anchors to offsets; Wildcards maps ordered SQL-cache results to dictionary/page targets while preserving separators and provisional slots. A shared scroller now owns thumb geometry, drag/track clicks, keyboard, wheel, touch and arrow movement; each page supplies its own position adapter and selection callback. Data loading remains separate. `go build ./...` and `git diff --check` pass; runtime interaction verification is pending.

The 2026-10-19 21:43 Windows log shows Browse completed all 30,294 Wiktionary strip labels and deep page reads, but the shared renderer double-subtracted native scroll position and omitted Browse's total track width. Uncommitted fix separates native Browse and logical Wildcards coordinates and computes track width from the final measured label. The renderer bounds item/geometry access, logs mismatches as `ribbon_guard` System Log errors, and cancels a drag if the list changes. `go build ./...` and JS syntax check pass; Windows visual verification is pending.

Wildcards `*` no longer forces every section chip to a fixed three-em width, which clipped distinguishing block labels to forms such as `H…`. It now uses the shared label-width measurement and virtual positions; the existing maximum label width and ellipsis remain.

The shared work progress panel now sits above the fixed search/filter dock in Wildcards, matching Browse's stacked layout. Wildcards content reserves the panel height while work is active; mode changes relocate an active panel immediately.

Browse and Wildcards bottom search controls no longer cap their input at 54 rem. The Wildcards dock and shared task panel also have no 54-rem maximum, so all three track the available window width.

Wildcards virtual ribbon now paints a continuous, theme-aware tint behind each dictionary's section chips, cycling through four soft hues. Bands follow the dictionaries and horizontal position; chips and the active marker remain above them. Visual checking in Clean, Warm and Old Paper remains pending.

Browse alphabet chips now cap visible labels at 12 em and render longer labels with an ellipsis; the full label remains available through the native tooltip and accessible name. The width cap is included in the measured chip geometry so virtual scrolling and navigation stay aligned. `go build ./...` and `git diff --check` pass; visual runtime confirmation is pending.

Browse labels follow the explicit boundary rule: shared character prefix plus the first differing character when the first word differs; shared whole words plus the full first differing word when later words differ. Existing letter/digit range suffixes and repeated-punctuation boundary handling remain. `docs/SPEC.md` records the rule. `go build ./...`, `build-windows.cmd debug`, and `git diff --check` pass; no tests were run.

Each app run now writes a separate local-time `system_YYYY_MM_DD__HH_MM_SS.log` in the library `.logs` directory; the five most recent sessions are retained and System Log export combines them oldest to newest. Listenerless Windows WebView and Android Go/JNI pair a timestamped Go crash file with the run. Windows also installs an unhandled SEH filter that writes a text report and a `.dmp` containing thread information and stack memory; the dump must be sent separately. Ordinary HTTP-server launches do not install these crash handlers. These handlers do not capture OS-forced termination. `go build ./...` passes; native crash capture is compiled but not runtime-triggered.

Browse crash report (2026-10-19): the log has no Go panic, but the ZIM Browse stream returned a successful empty page and alphabet (`0/0`) for Wiktionary, then the process restarted without a recorded clean exit. `zim.container.nsRange` previously swallowed read errors as an empty range. It now propagates those errors, and `zim.Open` rejects an empty content namespace when ZIM metadata claims articles. `go build ./...` passes; the user's source file and crash cause are not available for runtime confirmation.

Settings → Info now shows the library path and indexed dictionary count without the misleading in-use status derived from USE_CACHED. This flag does not indicate whether discovered dictionaries use their prepared indexes. UI runtime confirmation is pending.

Browse alphabet diagnostics: the existing Browse/Wildcards timings setting records Windows native Browse stream phases every five seconds, stream completion, browser alphabet progress/completion, and shared task start/finish. A 9.1M-word log showed the stream finished in 1.5 s and the old full-width pass took 31 s across 1,001 animation-frame chunks. Browse now measures the first `indexconst.BrowseStripExactWidthCount` labels exactly and assigns the remaining labels their mean width, fixing the estimated full range early; endpoints still map to the first and last blocks. The width cache uses a new session key. Alphabet task cleanup also removes matching orphan tasks and logs active task count and panel visibility after cleanup. `browseRangeEnd` catches a label-specific panic, logs the label pair and stack, falls back to that page's starting label and continues building the rest of the strip. Browse anchor-derived array indexes are range-checked and logged; Hangul initial indexes are checked before access. The Info section reports retained System Log errors/crashes and links to log export. Browse's current/total page count has a visible button outline. `go build ./...` and `git diff --check` pass; Windows visual/runtime verification remains pending.

Browse strip labels are calculated from the previous page tail, current page start and next page start. Indexing keeps the existing every-100-row anchors and adds the last row of every 300-row Browse page; `IngestVersion` is 5 so prepared indexes are marked outdated and rebuilt through the normal flow. Latin, Russian Cyrillic, Greek and digit transitions can add range suffixes; unknown scripts keep the shared title prefix. Build/runtime verification remains pending.

Browse now uses shared constants from `internal/indexconst`. Fresh prepared `text.db` indexes store a browse seek anchor (ordinal and source entry key) for every 100 Browse-visible rows plus the last row of every 300-row Browse page. Numeric page reads and key-based `Locate` start at the closest anchor; `IngestVersion` is 5 so existing prepared indexes are marked outdated and rebuilt through the normal flow. Databases without the new table keep the prior OFFSET fallback. `go build ./...` passes; index rebuild and deep-page performance are not runtime-verified.

Parallel index jobs now update the compact current-dictionary label, action/features and article counters atomically under the job lock; previously separate worker updates could pair one dictionary's name with another dictionary's counts. `go build ./...` passes; the concurrent Windows progress display is not runtime-verified.

Browse diagnostics now include per-page index path, requested offset/page size, sparse-anchor support/use and ordinal, anchor lookup time, page read time, rows returned, and total time. Browse pages explicitly constrain `w >= anchorWord COLLATE NOCASE` so SQLite seeks `idx_entry_w`; the tuple-only predicate produced a full covering-index scan on the 9.1M-word database. Successful page logs now emit an empty `error` field instead of Go's `%!q(<nil>)`. Browser strip measurement is chunked and separately timed as `browse_alphabet_measure`, starts only once non-empty labels arrive, and reports total labels, labels measured in the cycle, and chunks; page read/render timings remain separate. These diagnostics use the existing Browse/Wildcards progress timing option. `go build ./...` passes; measurements on large dictionaries need a rebuilt-app runtime run.

The shared task panel coalesces progress-driven renders to one animation frame and avoids rebuilding hidden task rows or measuring layout on every progress update. While a task is active, the Browse/Wildcards timing option records resize bursts with event-to-handler, next-frame and paint-frame delays; runtime confirmation is pending.
Checkout: branch `dev2` at `62e162ed` (`stripe megements`). One temporary SQLite file stores the submitted mask and group, ordered result blocks by dictionary, and optional bottom-filter blocks. The cache is shared by Windows and Android, and files expire 30 minutes after last access. `dict.MaskPageSize` sets the block size; each dictionary keeps its final short block. SQLite writes are serialized, and per-stream tokens reject stale writers after a restart. The alphabet strip refreshes when each base dictionary stream completes. Base and filtered searches use the effective `INDEX_WORKERS` count, capped by group size. `go build ./...`, Windows debug build, Android debug arm64/x86_64 build, inline JavaScript syntax checks and `git diff --check` pass. Wildcards parallel throughput, page navigation and cache reuse remain unverified at runtime.

An all-asterisk mask (`*`, `**`, etc.) now uses each prepared dictionary's Browse index for its total, alphabet and requested pages; visited pages are stored lazily in the same per-pattern SQLite cache. This avoids scanning and materializing every headword. Its stream now reports Browse alphabet sampling progress for formats that expose it (including ZIM), then completes when the requested page is ready. The fast path and its progress are not yet runtime-verified.

Folder rescans now queue missing Contains/full-text indexes when a matched dictionary already has its prepared headword index and the corresponding Edit Folders defaults are enabled. These optional indexes are added through `setFeaturesContext`, preserving existing features and paired media without forcing a full base-index ingest. New dictionaries still use the normal initial ingest queue.

Wildcards progress now reports each dictionary's headword scan position and total through the Browse stream. The shared task panel coalesces measurable progress updates until a 1-percentage-point change; unmeasurable progress is limited to one update per 200 ms. The Browse diagnostics option also logs per-dictionary Wildcards stream event counts and synchronous rendering costs for investigating UI stalls. The shared panel counts active dictionary scans and shows a percentage per dictionary when expanded. A single parent operation uses a `Stop` control in its expanded summary; concurrent top-level operations retain `Stop All`. Wildcards' combined dictionary block ribbon is virtualized like Browse: only visible buttons and 24 items on either side are mounted; its full measured width is pinned as both width and minimum width so horizontal scroll covers all blocks. The wildcard task disclosure stays available across child-row count transitions, including single-dictionary searches. Inline JavaScript syntax and `git diff --check` pass. A new screenshot showed the ribbon thumb still covering only about 40% of its track for a large all-words SQL dictionary. Root cause: prepared SQL dictionaries expose the intentionally sparse Browse alphabet instead of one jump label per 300-word Wildcards block. `Store.StreamBrowseMaskAlphabet` now scans the ordered headword index once and returns every block label, with progress updates; the all-words stream uses it instead of the sparse alphabet. Windows cgo debug build and `git diff --check` pass. The all-words ribbon reserves block slots from known dictionary entry counts and uses fixed-width chips. The user reported that its scrollbar still shrank as they dragged it. The stream marks a dictionary ready on every progress event while its alphabet is incomplete; the ribbon now keeps the reserved slots until that dictionary completes, including during resize-driven redraws. Missing entry counts fall back to the stream total. A restored all-words cache with a sparse legacy alphabet is discarded and rebuilt. The fixed-width track clips its virtual children so their changing absolute positions cannot enlarge the native scrollbar range. Windows cgo debug build and `git diff --check` pass; the current stream page is now derived from the first returned headword position, and page navigation explicitly scrolls the active ribbon block into view. The user then showed the native ribbon scrollbar almost filling its track. Wildcards now has a separate scrollbar whose thumb width is visible section count divided by total section count, with a 20 px minimum; dragging, track clicks and keyboard navigation map to section offsets. A normal-flow extent element fixes the browser scroll range independently of virtual buttons. JavaScript syntax and Windows cgo debug build pass; the scrollbar redraw and virtualized section rendering are coalesced to at most once every 50 ms during dragging. Drag tracking now listens at window level for pointer movement and release, so it continues when the pointer leaves the thumb. JavaScript syntax, `git diff --check` and Windows cgo debug build pass; runtime drag confirmation remains pending.

The System Settings diagnostic checkboxes are persisted with user settings on Windows and Android, so their opt-in choices survive restarts.

A33 dispatches the Browse, Edit Folders, Lemmatization, Dictionary Groups and double-tap probe HTML documents through the Android Go resource bridge. A36 dispatches the main `/` document through the same bridge; the user confirmed it works. A37 dispatches non-range `/files/` requests through Go too; the user confirmed background image loading. A38 added non-range `/res/` dictionary resources; the user confirmed article media and video seeking work. A39 attempted to dispatch ranged requests through JSON/JNI, but a large video would not start. A40 restored the working HTTP path for Range requests; the user confirmed video works. A41 streamed `/files/` and `/res/` through a JNI InputStream, but the emulator showed images loading while video stalled. DevTools showed WebView returning `Content-Length: 0, 198565544` on a `206` response and repeating tail-range requests; its own Range handling conflicts with a pre-ranged intercepted InputStream. A42 streams only requests without Range conditions and returns Range/If-Range requests to the working local listener. A42 `build-android.cmd debug intel` and `git diff --check` pass; the user confirmed video playback works on the emulator. The local listener is still required for ranged media. A43/A44 tried a private ContentProvider with a Go seek bridge for video and audio. Emulator logcat showed audio rejected as a local resource from the HTTP page. CDP on the large-media-test article showed `<video src="content://...">` with `MEDIA_ELEMENT_ERROR: Media load rejected by URL safety check`; the user independently confirmed that video did not play. A45 restored audio; A46 restores video and removes the unused provider/JNI seek bridge. Range requests still need the local listener while a different large-media transport is developed. A46 `build-android.cmd debug intel` passes; user runtime verification is pending.

Android runs the shared Go runtime in-process through `android/nativebridge`; Java starts only this JNI library. The bridge registers all formats, uses `server.NewRuntime`, `server.ConfigureLibrary` and `server.RecoverLibrary`, and Android exposes app operations only through Go/JNI, with no TCP listener or Android server mode. The legacy executable, adoption and fallback are gone. JNI busy callbacks keep `IndexService` informed. Settings restart and final-window/Exit shutdown end the app process because the prepared registry remains live after listener shutdown. The main page reads config, preferences, styles and presets through JNI and shared Go operations; the user confirmed their reads/writes and custom style file operations. The main dictionary list, search and ordinary GET Rescan folders stream through cancellable JNI polling and were confirmed by the user. `ignoreDiacritics` is passed to the shared search request, including on Windows. Browse's word-page and in-list find streams use the same JNI queue and were confirmed by the user. Browse's nonstream page, chooser dictionary/preference reads, and paged headword/wildcard results now use shared Go operations through JNI; the user confirmed Browse works. The user clarified that `Words` belongs in Android's bottom status bar, not the crowded top bar. A7 restores the bottom button for Android whenever a Browse/wildcards return URL exists, even with multiple result dictionaries, and keeps the top button Windows-only; runtime confirmation is pending. Browse and dictionary-list timing diagnostics now use JNI and the shared Go recorder; runtime confirmation is pending. Dictionary About now reads shared Go data over JNI and was confirmed by the user. The missing-dictionaries list and its delete/keep action now use JNI; runtime confirmation is pending. Library dictionary removal also uses JNI; the user confirmed ordinary startup and dictionary listing, while removal itself remains unverified. Saved appearances use JNI for listing, saving, applying and deletion; the user confirmed switching presets and persistence. On Android, choosing a window background image now turns Color on and disables its checkbox; choosing `None` makes the checkbox available again. Edit Filters uses JNI for groups.ini read/save/reset and its unmatched-dictionary check; the user confirmed it works. Lemmatization uses JNI for list, install, remove and progress polling; the user confirmed it works. Edit Folders uses JNI for migration status/action, current config, path validation, index defaults and folder save; the user confirmed it works. Android WebView archive intake and Java share/open imports use JNI; Java content-URI imports stream 1 MiB chunks and WebView file uploads stream 4 MiB chunks into the bounded Go spool (maximum archive size 16 GiB). A51 builds; runtime check is pending. The Android archive/dictionary picker overrides Android WebView's incorrect `application/zip` conversion of the extension list and opens with `*/*`; the user confirmed file selection works. Dictionary Groups data and all group edits now use the Android JNI bridge. Dictionary-list refresh now clears any stale loading notice after the stream ends; the emulator showed no active work with a stale "10 of 10 ready" notice. Android language preference saving calls the shared Go operation through JNI, and scripted-article inline sub-entry searches use the Android Go/JNI search stream; the user confirmed both. Embedded CSS, JavaScript, fonts and other `/assets/*` requests dispatch from Go through Android WebView interception; the user confirmed the A31 build works. User stylesheet `/style/*` requests now use the same direct resource path. A57 stores Android settings and search history in `settings.json` and `history.json` through JNI; the user confirmed migration and persistence. A58 uses the prior `http://127.0.0.1:<port>` origin only while `settings.json` is absent, to import old-profile localStorage; once that migration marker exists Android loads `https://wudict2.localhost` without a port. A shared Go/JNI resource adapter serves pages and ranged media in both MainActivity and LookupActivity. The page marker was A58. The user confirmed the app works, including the Go/JNI media path. User stylesheet loading still needs runtime confirmation. Paged Browse/Wildcards results still need runtime confirmation, and the user confirmed the stale progress notice is cleared. On Android, `Words` sits to the left of the bottom dictionary arrows. The main-screen Wildcards stream uses the Android Go bridge and was confirmed by the user. Reindex status/start/stop now use shared Go operations through JNI; runtime confirmation is pending. Explicit index-work status/start/stop also use those operations through JNI and were confirmed by the user. The Rescan Update dictionaries progress stream uses JNI and was confirmed by the user. Single-dictionary ingest progress uses the Android Go stream: the user confirmed deleting Contains completed but reported a lingering progress indicator. The legacy indicator now clears its hidden state even while shared progress is active; the user confirmed it disappears after Contains deletion. Removed an out-of-scope ingest callback from article rendering that could break anchor navigation. `build-android.cmd debug intel` passes for arm64 and x86_64; the APK contains only `libwudictbridge.so`. A55 moved Play SAF post-import rescan to a cancellable Go/JNI stream. A56 removes Android TCP listeners; browser and explicit server launches retain HTTP on other platforms. Index progress/Stop and Exit still need explicit Android checks.

A47 experiment: Android no longer returns `/files/` or `/res/` Range requests to the loopback listener. `DesktopMediaStream` answers through a Go pipe and JNI InputStream. A47 failed because WebView saw duplicate MIME and Content-Length headers. A48 removed those duplicates and reports remaining stream length through `InputStream.available()`. Runtime still failed: logcat showed WebView repeatedly requesting the final 1.6 MB range of a 198 MB video. The cause is an Android WebView bug that applies Range offsets a second time to custom `206` responses. A49 adds a prefix-aware `InputStream.skip()` workaround for single byte ranges; reads remain streamed from Go/JNI. `build-android.cmd debug intel` passes for arm64 and x86_64; the user confirmed playback works. A50 routes Android System Log export through JNI instead of `/api/system-log`; the user confirmed it works. A51 routes Java archive intake status, URL/path import, confirm/cancel, and chunked content-URI upload through JNI instead of loopback HTTP; unknown-length content providers stream with a bounded Go spool. The user confirmed archive import works. A52 sends Android power state directly to Go through JNI and replays it after runtime startup; `build-android.cmd debug intel` passes for arm64 and x86_64, and the user confirmed A52 works on the emulator. The local listener remains active while remaining Android API fallbacks are audited; Range media is already delivered through Go/JNI.

## Remaining manual check
Windows System Log export saves directly into the user's Downloads folder and opens a localized, compact in-app confirmation showing the full saved-file path, with a Show in folder action. Browser and Android export flows retain their existing behavior. Runtime verification is pending.

On Windows, verify direct GET resource delivery: Settings/editor assets, preset switching and GD fonts, user styles, and background images in the picker. The user has confirmed dictionary images, multi-gigabyte video, seeking and background-image thumbnails through the latest direct resource path. The user confirmed successful settings migration and that the next launch skips it; only the stalled-export timeout branch remains unverified. The user confirmed the three dialogs and startup splash on Windows. Also confirm browser and console mode still bind and serve HTTP. Remaining checks: dialog appearance with Background Image and Sepia on/off; browser and Android paths for all three dialogs; Edit Filters load/save/reset, close prompt, and translations; Edit Folders' layout, current paths, validation, index defaults, picker, archive intake, shared progress, save and list refresh; Lemmatization list/install/removal, progress polling, directory note and attribution; chunked browser archive upload; URL and native-path import; System/English/Русский selection and persistence on Windows and Android; System locale behavior on EN/RU devices; orphan actions/removal; About; Browse streams/search/filter/Stop/chooser; Wildcards filtering/page turns/prefetch; article CSS; explicit index rescan and reindex progress/Stop. Browser file input/drop uploads retain HTTP; verify browser upload remains functional and test Android WebView and Java archive imports. Then verify portable startup with prepared dictionaries and confirm indexes are reused after copying `Data`. Check the shared dictionary picker on Android.

## Server-removal estimate
The workspace-dialog refactor in [`docs/DIALOG-REFACTOR.md`](docs/DIALOG-REFACTOR.md) is confirmed on Windows; Android/browser checks and unused iframe-route removal remain. The earlier estimate was 89–91% complete, with regression checks the largest remaining stage. Windows startup without a listener, direct page navigation and `/res/` media delivery are confirmed; remaining Windows screens, assets/styles, browser and Android HTTP paths still need verification.
