# Agent handoff — current state

The build has one flavour since 2026-11-03: cgo, `-tags sqlite_fts5,sqlite_dbstat` (MinGW on
Windows, the NDK on Android, a native runner per platform in CI). The pure-Go SQLite driver
(`modernc.org/sqlite`, `-tags purego`) was removed with its dependency, its notices and its
build-tag prose; a build that selects no driver fails by name (`internal/store/driver_missing.go`).
`internal/speex/backend_nocgo.go` stays: it is the `!cgo` stub of the speex package, not a SQLite
flavour. `pages/docs` (the published site) still offers the upstream `-purego` downloads and the
`make cross` / `make build-purego` / `make android-go-purego` targets this fork has never had -
cleaning that page set is open.

The lemma catalogue is ours since 2026-11-03: `config.DefaultLemmaURL` and the built-in default
in `internal/lemmas` point at `raw.githubusercontent.com/DMSHLabs/DictShelf/lemmas/manifest.json`,
the fallback mirror at `dmshlabs.github.io/DictShelf/lemmas/`, and the scheme/host/path pin in
`fallbackURLs` follows them. **The `lemmas` branch and the site copy do not exist yet**, so
`dictshelf lemmas download` reports an unreachable catalogue until the files are built (the
Makefile lemma target) and pushed there. `pages/docs/extension.md` is now the contract only - we
publish no browser extension, and the upstream store links and the upstream social link in
`pages/zensical.toml` are gone.

Dictionary management defaults to Contains for new profiles, and its setup and
Edit Folders views explain the search-speed benefit and one-time indexing cost.

Android System Settings now uses its mobile inset bounds to determine window
height instead of a fixed `100dvh`. During background work, `#bottomDock`
raises the dialog's bottom edge by `--workh`; the fixed viewport height did not
account for that reduced space and could leave System Settings behind the dock.
The 2026-10-31 phone log shows System Settings rendered twice with
no read/render errors; visual verification of the CSS adjustment is pending.
Indexing refreshes Android's allowed CPU set at each job: active work uses
N−1 Go processors. Battery saver caps work to half of currently allowed CPUs;
moderate thermal status halves the active limit and severe status or critical
memory pressure caps it at one. Runtime changes also resize both dictionary
limiters. A new app version selects Parallel dictionaries once; later user
changes persist until the next version. Android and Windows builds pass; phone runtime verification is pending.

System Log error detection ignores empty `error=""` fields, which occur in
successful Browse diagnostics. Windows GUI builds now verify startup by exit
code because `windowsgui` executables have no capturable stdout; the cgo build
and `--version` startup check pass. Tray hides clear hot-key capture state;
global restore dispatches through the WebView window thread and moves the
window to the top of the normal Z order.

Windows tray behavior is read from the saved settings when the window is
created (`desktopApplySavedTraySettings`), not only when System Settings
renders: until that page was opened the native side kept its built-in defaults,
so a saved Minimize-to-system-tray did nothing after a restart and a disabled
close-to-tray kept hiding. The show hot key decides from the window's real
visibility and `IsIconic` rather than `desktopWindowHidden`, which records only
what this process did and went stale whenever the tray or a second launch
restored the window. A second launch finds the reader window by class and by
the executable of the process owning it (`desktopOwnReaderWindow`), not by
title: Windows 11 mirrors the title onto a shell-owned
`Windows.Internal.Shell.TabProxyWindow` that `FindWindowW` returns first, so
double-clicking the icon while the window was hidden appeared to do nothing.
Verified live on this machine: minimize-to-tray hides, a second launch brings
the window back, close-to-tray keeps the process alive and hidden, and the show
hot key restores and activates it (`visible=foreground=hwnd`). The hot key's
visibility-based branch is reasoned from those measurements, not measured
directly.

Release workflows now handle concurrent tag creation: after a rejected push they continue only when the remote tag resolves to the same commit; Android, Windows, and native-release workflows share this guard. `dictshelf-v1.02.17` already exists remotely.

Group selectors show each group's dictionary count. The shared status-bar formatter emits the selected group's found-dictionary count followed by its total in parentheses (for example, `Essential 4 (12)`) on Windows and Android, independent of translation catalog wording.

Android's Browse chooser, Dictionary Groups page, and Filters dialog now pass `false` to the native `startDictStream(boolean)` bridge when they do not request progress; omitting the argument made Browse's chooser fail before it could list dictionaries.

Wildcards dictionary-search concurrency no longer reuses the much smaller indexing concurrency limit. It runs up to four dictionary workers, bounded by available hardware threads while leaving one available for the rest of the app.

Canonical checkout: `D:\Projects\Android\DictShelf\`, branch `xapian-glass` (created from `dev`; `dev` is the integration branch, `master` is upstream only). Edit Folders now has **Dictionary folders** and **Download Dictionary** tabs in the same window; import is available when an install folder exists. Main search suggestions now follow the selected headword mode, including wildcards, cancel on edits or scope changes, rank matches before applying the 40-per-dictionary and 500-overall defaults, and identify these limits in Results. The dropdown expands to available viewport height and scrolls internally at the window edge. For every headword mode, suggestions with equal spelling and match quality prefer shorter headwords before match position. Search history supports up to 1000 entries, and Find in articles has its own filtered recent-query list under the shared history limit. Dictionary groups and Browse A–Z show headword counts — the group editor and Browse's chooser print both counts when they differ (articles / headwords, `dictUI.articleAndHeadwordCounts`, drawn by `dictCounts` in `index.html` with a copy in `browse.html`), one when they agree or when an older library has only the article count; the A–Z list itself holds article headwords, which is why both numbers are shown for a dictionary with many aliases; the settings card and the information popup print the same two counts as labelled lines (`dictUI.numberOfHeadwords` / `dictUI.numberOfEntries`, the card's under the chip row and the meta line no longer repeating the article count), and every count is formatted with the reader's own separators through `window.wudictI18n.number`, not `en-US`; the full-text chips read `full-text` (our FTS5 index, which stores positions, with its size beside the name once built) and `built-in: words only` (the archive's own index) on every dictionary — the built-in one carries the qualifier, so the same index is named the same way on a ZIM and on any other format; the status-bar dictionary picker shows found-versus-total article counts, with a plus while provisional, and marks the dictionary at the current reading position. Long-pressing dictionary names opens an information popup with source files (including attached `_abrv` companions), localized headword/article counts, and About text; prepared database paths are omitted. That popup now prints the dictionary's source size and its index size above the headword count — each a human-readable figure with the exact byte count, from `/api/about`'s `fileSize` and `indexSize`. The source size is summed over the files the endpoint already stats (a folder contributes nothing: About stays stats, never a walk); the index size is `text.db` plus `articles.db` when the build split the article index into its own file, so it is one file or two. The Browse A–Z chooser also opens this popup on long press or right-click; in Browse's word-list view, tapping the dictionary name opens it, with long press and right-click supported too. New indexes persist the source headword count separately from article count: since `IngestVersion` 8 both are COUNTED at ingest — `entry_count` is the articles, `headword_count` is those plus every alias pointing at one (`SELECT count(*) FROM alias`) — where 7 and earlier recorded what the source had declared, which Lingvo DSL declares as nothing at all. A library built by an older build therefore reads as outdated and shows the old (or missing) headword number until it is rebuilt. The group toolbar starts multi-selection with a Multi button; A–Z and pin are outlined buttons separated by one button height. Membership editing uses minus buttons to remove and plus buttons to add dictionaries. Long press and right-click on names continue to open dictionary information. Article headers support left-button hold and right-click on Windows. Full-text remains excluded. Wildcard suggestions stream the first headword immediately and flush more candidates every 250 ms without loading article bodies. Selecting a suggestion keeps *? as the input mode and shows exact+lemma results; the checkmark opens the Wildcards view. Runtime UI verification is pending.

Windows and Android now open one shared, collapsible System Settings dialog. Android reveals its System section through the early WebView Java bridge, so a late shell flag or missing canExit value cannot hide the entry. Its Windows-only tray/icon controls and Android-only lookup/launcher-icon controls are rendered from platform state; the old `sysSettings` markup remains hidden as the source of bridge-bound controls. Android System Settings offers two Icon 2 launcher choices: dark (default) and light; the selected variant is now always manual and no longer follows the Android theme. The former standard icon is hidden from the picker; its launcher alias remains for migration and is disabled when an old saved choice is loaded. A connected phone screenshot showed the active light PNG foreground cropped the star and book. All three launcher foregrounds now use Android VectorDrawables generated from the SVG sources by `tools/make-android-launcher-vectors.py`, with artwork inside the adaptive icon safe zone. Resource processing passes; rebuilt APK appearance still needs device verification. Three launcher aliases target a transient LauncherEntryActivity, which opens MainActivity in its own task; alias switching no longer targets the running window. The active alias retains the Settings long-press shortcut. The light icon now uses a dark blue tile (#172A40). In the System Settings picker, the dark-icon preview now has a full cream adaptive tile and safe-zone artwork scaled like the light option; launcher resources and desktop icons are unchanged. Alias changes now use DONT_KILL_APP in the Android 13+ atomic update as well as the older path, so selecting an icon does not terminate the running app; Java compilation passes, phone verification awaits a rebuilt APK. Android System Settings has an opt-in diagnostic checkbox that stays available if config loading fails and records open, native-state, config-read and render errors in System Log. Phone diagnostics found repeated openings failed because previously moved controls were not restored to hidden `sysSettings`; the renderer now returns cached controls before clearing/rebuilding the dialog. Android Java compile and embedded JavaScript syntax checks pass; APK rebuild and device verification of this fix are pending.

Android Settings → Update shows byte progress and a determinate/indeterminate
bar on Android and Windows, including when notifications are off. Missing
install permission opens DictShelf's localized settings dialog and starts the
installer on return; DictShelf dialogs use the selected paper/image, while OS
installer/settings screens retain Android's appearance. On `Power_Armor_18T`,
the installed APK is versionCode 1116 and downloaded `1.02.15` is 1113; its ZIP
and signature are valid and the signer matches, so Android rejects the
downgrade. `DICTSHELF_VERSION_CODE` lets `build-android.cmd` make an older test
APK; see `docs/ANDROID-FORK.md` for 1112 → 1113. The updater now checks that a
saved DownloadManager APK still opens, clears stale records, and re-downloads
if the file disappears during install confirmation. Runtime verification is
pending.

The Windows taskbar icon refresh removes the visible window's taskbar button
after a theme-driven icon change and adds it again 150 ms later on the UI thread.
Rapid changes cancel earlier additions. A Windows Go build passes; live Explorer
behavior still needs user verification.
Taskbar icons now use only `taskbar-dark.ico` and `taskbar-light.ico` in the active user data root (`Data/icons/` for portable mode); changing the tray icon rewrites this pair and cleans legacy generated files. Startup reads saved app/tray icon choices and system tray theme before creating the window, so its initial big icon matches the taskbar theme and the small titlebar icon matches the app choice. Window relaunch-icon properties are committed explicitly. Close, minimize-to-tray and hide-to-tray recalculate the variant from current tray choice and Windows system theme; a hidden helper repeats the pinned-shortcut refresh 900 ms later with shell notifications. Windows Go build passes; live Explorer startup flicker verification remains pending.

Windows executable builds generate a version resource from `DICTSHELF_VERSION`
or the local Git version, including DictShelf/DMSH Labs identity and the UTC
build date. The Windows GitHub Action now passes its selected release tag through
`DICTSHELF_VERSION`. A sample beta resource was compiled and verified through
Windows `FileVersionInfo`; full build-script runtime verification remains.

Windows close-to-tray diagnostics record WM_CLOSE, the effective setting and
visibility after ShowWindow. The log showed taskbar restoration can leave the
internal hidden flag stale, so WM_CLOSE now checks actual Windows visibility
before deciding whether to hide or consume a repeated close. The Windows CLI
package cross-compiles with vet disabled; GUI runtime verification remains
pending. System Settings now has an independent Minimize to system tray option;
when enabled, the native minimize command hides the window, while the existing
Close to system tray setting continues to govern WM_CLOSE. Its default is off.
The Windows executable cross-build succeeds; GUI runtime verification remains
pending.

Style layers now have visual sections Appearance, Layout, Typography and Dictionary content. Section grouping is separate from the preset asset/conflict groups. Wikipedia full-width is available for MediaWiki Vector; Hide examples remains Settings-only, and the redundant Quiet labels and Quiet examples layers were removed. Changes are not runtime-verified. Portable Windows update logs showed the replacement process starting, with WebView startup ready after about 26 seconds; a manual relaunch was ready in about 1 second. `startWindowsExecutable` no longer sets `HideWindow` for the GUI app, while the helper still starts hidden. Windows now verifies the expected installer or portable EXE and its SHA-256 for the current launch path; a WebView localStorage marker alone no longer selects Install Update. The action stays Install Update and downloads when needed. After a newly completed download, a themed EN/RU dialog asks whether to install; Cancel preserves the verified file. Confirmation records the target version; the exact version shows a one-time success dialog after the next ready startup, while an old version clears the marker. The Update section heading now shares the orange alert mark with ☰. Android reuses a saved DownloadManager item only when its version and APK SHA-256 match the current catalogue; stale entries are removed and retry filenames are unique. Windows icons now use six SVG variants under `packaging/brand/source/icon*`; `tools/make-icons.sh` exports them with Inkscape. System Settings → Window presents the six application and tray icon choices in two separate grids, each showing 32px and 48px previews; both default to `icon2_transparent`, and the matching desktop shortcut follows the application choice when writable. Runtime verification remains pending.


Dictionary Settings now show measured Contains and Full-text index sizes from
SQLite dbstat when supported; the measurement is included in prepared-row
metadata and cached with dictionary rows. Indexing Stop is available only in
the shared bottom progress panel, not beside the Dictionary Settings status.
Dictionary icons can load from adjacent files and embedded LSD/ZIM resources
before preparation finishes, so newly discovered dictionaries do not show a
fallback icon just because their text.db is not ready. Focused store-size and
dictionary-icon tests and `go build ./internal/server` pass; desktop runtime
verification remains pending.

System Log includes Save and Clear actions. Clear requires confirmation, uses
the selected paper colour and image, deletes retained history, and keeps the
active session logging. Browser, Windows and Android routes share the same
clear operation. Runtime verification is pending.

The two formerly failing Windows server tests now match current behavior:
the search-mode assertion checks the compact `=` mark and localized menu label,
and dictionary-folder removal waits for asynchronous indexing to finish.
Both targeted tests passed three repetitions; `go test ./internal/server
-count=1` passes.

The active main search now has a bounded, unnamed temporary SQLite cache for
article page responses shared by manual paging and Find in articles. It
evicts least recently used page rows at 128 MiB and closes the prior database
when a new main search starts. Focused LRU, reset, growing-result and shared
search tests pass; `go build ./internal/server` passes. The full server suite
still fails in `TestI18nPagesAndCache` and `TestRemoveTakesTheEmptiedFolderWithIt`.
Desktop and Android UI checks remain pending. Find in articles now retains
per-dictionary, per-block match counts in browser memory and recomputes the
global position from examined blocks. It can revisit an evicted article page
for Go to match. JavaScript syntax passes; runtime UI verification is pending.

The shared bottom dock and background task progress are on `dev` through
`fda62b4`. Find in articles now places the query and mode labels beside their
controls and offers a scroll-to-current-match button. Crossing article result
blocks keeps the displayed match count stable, checks ready articles without a
fixed delay per block, and restores the original result page before wrapping
when later blocks contain no match. JavaScript syntax and diff checks pass;
Windows and Android runtime verification of this navigation remains pending.

Indexing modes in `dev` are now Parallel dictionaries and Adaptive (the former
`auto2` schedule). Saved `auto`, `auto2`, and `pipeline` selections load as
Adaptive; existing Parallel selections remain Parallel. A job involving one
dictionary selects sequential preparation for small sources or the shared
reader/writer/worker pipeline for large sources, regardless of the saved mode.
The shared pool now permits this per-job pipeline override while Parallel is
selected. Focused indexing tests and `go build ./...` pass. The full server
package test has an unrelated reproducible Windows failure in
`TestRemoveTakesTheEmptiedFolderWithIt`; device timing remains unverified.

Historical snapshot: branch `codex/self-hosted-github-links` from `dev` at `cc65d7e`. This branch updates the Go module path and product links to `github.com/DMSHLabs/DictShelf`, and points the documentation site at DMSHLabs GitHub Pages. `build-windows.cmd` now reuses the checked-in `dictshelf_windows_amd64.syso` by default; `--update-icon` regenerates it from `packaging/windows/dictshelf.ico`. Before replacing an executable, the script force-stops it and waits for it to disappear, up to 60 seconds. Release workflow dispatch offers `release`, `pre-release`, and `beta` for Android, Windows, and Linux/macOS native builds. New tags use `dictshelf-vMAJOR.MINOR.PATCH`; previews add `-pre-release.N` or `-beta.N` and are GitHub pre-releases. The Update panel stores the selected channel and an enabled-by-default notification preference in `state.json`; it checks the static `update-catalog.json` on the dedicated `update-catalog` branch after settings load on every page start without blocking the UI, then daily while the page remains open; requests time out after 10 seconds (Beta includes Pre-release and Release; Pre-release includes Release) and marks ☰ plus the Update section heading when it finds an update. The action stays Install update, downloads when needed, and asks for confirmation after download; Cancel preserves the file. The confirmed target version is acknowledged once after the new version next starts. The version links to the release page. During downloads, Windows streams byte progress from the release asset and Android polls DownloadManager; the Update section shows a localized percentage and the ☰ control shows a matching accent ring, or an indeterminate ring if size is unknown. Update actions download first and offer installation afterward: Windows installers go to Downloads; portable builds stage `_DictShelf.exe` beside the running executable and launch a separate temporary copy of the GUI EXE in helper mode. The helper waits for the main process using Windows APIs, verifies the staged digest again, keeps the old executable as `.bak`, replaces only the executable, and relaunches without PowerShell or a console window; the new app removes the helper copy after it exits. Android saves and verifies an ABI-matched APK under a version/hash-specific unique filename; it reuses a DownloadManager item only when its version and SHA-256 match the current catalogue, then asks before installation. Publishing workflows regenerate the catalog from non-draft GitHub Releases; client checks do not call the GitHub API or change version history. Windows and native workflows load the tag resolver from the workflow revision while using the selected source checkout for tag history. The DownloadManager completion notification remains available for manual APK installation if DictShelf lacks permission. Play omits install permission and the APK update action. Windows and Android updater behavior remains unverified at runtime or in Actions. Both build scripts show command-line help for `-?`, `--?`, `-h`, `--h`, `-help`, and `--help`. Older notes below are historical snapshots.

Historical snapshot at `da540c7`. The two previously reported server
test failures are fixed: `TestDictsRunningJobIsNotKept` now waits for the
first stable dictionary-list validator instead of assuming a fixed number of
requests is enough, and the EN/RU full-text mode hint catalogs no longer have
empty values. Both targeted tests passed five repetitions, and
`go test ./internal/server -count=1` passes on Windows; search keeps five articles in the first populated section, later sections use Settings → Results, and later pages in that first section start after its five-result first page.

Historical snapshot at `da540c7`. Settings checkboxes use one shared customizable control, and the bottom status bar is always visible; preserve untracked `help/changelog.txt`. pipeline from `8b3ee97e` merged. Per active dictionary, it runs a source-reader goroutine and a coordinator/writer; CPU preparation uses a shared pool sized from logical processors and concurrent indexing sessions, with 16 outstanding batches per dictionary. `INDEX_WORKERS` is now obsolete and ignored; per-dictionary and global worker limits derive from logical processors. `Contains` creates and inserts both `entry_fts` (headword token FTS) and `entry_trigram`; `article_fts` is independent FullText and remains absent when disabled. Verified in generated DB schemas. Full `go test -tags sqlite_fts5 ./... -count=1` passes. Old `dev` (`c79732e4`) does one sequential read/prepare/write pass per dictionary, with only cross-dictionary parallelism. With `-headwords -contains`, single-run dev/DictShelf2 times: Oxford DSL 9.4/7.3 s (71,293 rows); Urban DSL 107.3/78.3 s (1,418,551); Longman MDX 10.2/8.0 s (69,172); Russian Wiktionary ZIM first run 39.4/47.7 s. Three ZIM runs gave medians 38.8/47.4 s (dev/DictShelf2); new pipeline is slower on this title-only ZIM case. Entry row counts match; DB byte sizes differ slightly due to layout. LSD Longman DOCE 5 on DictShelf2: 54,819 entries in 3.9 s; dev has no LSD ingest Reader. Output DBs are under `D:\Dictionaries\wuDict\Test\index-contains-{dev,dictshelf2}-2026-10-25`.

The boot screen reports dictionary-file checks and metadata reads. The native
WebView2 splash says `DictShelf`, then updates with localized dictionary-check
and metadata-read counts until the list is ready. After the first dictionary
arrives, the separate status line does not duplicate the indexing task shown in
the work panel. The indexing panel shows active dictionaries in start order,
per-dictionary and overall elapsed time, completed dictionaries of the total,
and progress units. These UI changes are not runtime-verified yet.

Transferred indexer work was verified in its source worktree: full Windows cgo/FTS5 tests, focused race tests, Android Java/arm64 bridge builds, and Android/macOS pure-Go cross-builds passed. The opt-in Wildcards cache benchmark used Longman MDX + Oxford 10: upper `*a*` took 7.82 s with FTS/trigram indexes and 2.98 s without; lower `*e*` was 2.94/2.78 s; Combined base was 2.70/1.10 s. This is one workload; these checks have not been rerun on the current DictShelf2 checkout. A later three-dictionary Wildcards run (Longman MDX, Oxford 10 MDX, Wiktionary ZIM; `*d*g*`/`*home*`) took 28.68 s for the upper stage with result-cache FTS/trigram and 23.99 s without; lower group filtering took 27/114 ms. Batching 128 index rows per SQL statement cut index insert calls from 675,492 to 6,309 but the upper stage still took 28.48 s on this small result set. On the 9.13M-word mask `*`, batching cut both-index creation from 4m23s to 2m52s and index insert calls from 27.39M to 219,168, so it is now retained. The real UI now defaults both Wildcards cache indexes off; explicit on choices persist as `1`. The opt-in benchmark now selects FTS and trigram separately: on the same three dictionaries, FTS only took 27.11 s upper / 115 ms lower, trigram only 28.61 s upper / 27 ms lower, and neither 24.91 s upper / 121 ms lower. The corresponding Combined-base times were 1.83, 2.13 and 1.19 s. The app defaults both Wildcards cache indexes off. The full-match mask `*` on Wiktionary ZIM returned 9,129,950 words: upper-cache creation took 4m23s with both indexes and 36.7s without (27,389,850 per-row index INSERTs; peak observed temporary cache was 1.55 GiB). This run stopped after upper-cache creation; lower filtering and Combined mode on all 9.1M rows were not measured. The performance harness and BAT now support `UPPER_ONLY=1` for this case. With batching, trigram-only creation took 2m21.5s, about 31s less than both indexes; parallel FTS construction within the one SQLite database cannot bypass its single writer. On the same ZIM, the lower `**home**` filter took 230ms with indexes (2,317 matches from 2,474 trigram candidates) versus 5.2s without; FTS prefix `h` took 4.8s versus 9.6s without (243,637 matches).

## The index and the readings, as rules (2026-11-03)

Summarised here because the narrative of how each arrived is now in
`docs/handoff-archive.md`; what follows is what the code does today.

- **The article index is a file of its own.** `text.db` holds the headword tables, `articles.db` beside
  it holds `article_fts`; the PRESENCE of the file is the answer to "where is the index", not a version
  or a key, which is what lets every library built before the split keep working. The four rules: full
  text is created only as `articles.db`; when that file exists the database is not asked about a
  full-text index; when it does not, the database is; and a full-text rebuild takes the index out of the
  database and creates it as the file (that is the migration). A stale sibling is deleted by the build
  that wrote none, because `keptUUID` would otherwise let the previous edition's index pass the uuid
  check.
- **The index is rebuilt from the database when the database has the text**, and from the source
  otherwise: `store.RebuildArticleIndexContext` builds `articles.db` out of `entry.id`/`entry.w`/`body`,
  refusing (before writing anything) for a reader that drops bodies and for an index still inside
  `text.db`. The source path stays for those two.
- **A dictionary's index cannot be deleted at all.** `DELETE /api/library?prepared=1&source=0`, the bulk
  panel's base delete, the card's "index only" and `wudict rm -keep-source` are gone; `Registry.Remove`
  refuses the combination when a source exists. A dictionary can be removed whole, or its files only.
  The remembered state (`dslRemoved`) now means "no index right now": the queue prepares any dictionary
  whose index is missing instead of respecting it, and a stale marker beside a complete index is cleared.
- **A loose database's article index mirrors its owner**: `<name>.text.db` carries
  `<name>.articles.db`, exactly as `MediaSibling` does, so two databases copied into one directory do not
  come to share one index.
- **Strict readings on an archive-answered source are declined, not scanned.** When full text comes from
  the archive's own index (no positions) and no index of ours exists, a phrase or proximity search is
  answered with the WORDS reading; the plate names the reading that answered, and the "needs our index"
  line offers to build one. The complete-list pass refuses for the same reason (it can only page our
  `articles.db`), which is what the card's "Complete list unavailable" means - now traced in the system
  log (`search_complete_list stage=declined|failed`) and reported by the client through
  `search_article`/`search_error`.
- **The build is journal-less on both files and asks for a real page cache.** `createArticlesDB` sets
  `PRAGMA articles.{journal_mode,synchronous,cache_size}` right after the ATTACH, because `dsnIngest`
  sets those per database at connection open - before the attached file existed (`ingestCacheKiB`, 64 MiB,
  one value for every platform).

## Removal, as rules (2026-11-03)

- **A removal moves the folder out of the way before deleting anything** (`store.removalInfix`,
  `.removing.<hex>` beside the folder). Windows blocks that move when a file inside is held, exactly as
  it blocks an unlink - so a removal that cannot move the folder fails and leaves it WHOLE, where
  deleting file by file would have taken `text.db` away and left the caches. A removal that moved the
  folder but could not delete it leaves the contents under the debris name: nothing lists it
  (`store.Folders` skips it), `clean` reports it as an interrupted removal, and the startup sweep
  removes it - the sweep now covers the library root as well as each prepared folder, because that is
  where prepared folders and their debris live.
- **Both retries are bounded and share one rule** (`retryWhileHeld`): a sharing violation, a lock
  violation or ACCESS_DENIED - Windows reports 5 as well as 32 for "in use", and a held file inside a
  folder being MOVED reports 5 - are retried for about a second, then the real error is reported
  unchanged. `TestARemovalThatCannotMoveTheFolderLeavesItWhole` and the sweep test pin it.

## Session narratives moved to the archive (2026-11-03)

The dated sections below were session narratives of finished work; they now live in
`docs/handoff-archive.md` (grep there, or read the commit messages - they carry the same
stories). Titles, for grep:

- Progressive results: the user's three decisions and what became of each (2026-11-02; all resolved - one retracted, two verified)
- A ZIM with its own index answers full text itself (2026-11-01)
- Our index on request, for an archive that has its own (2026-11-02)
- Strict readings are declined, not scanned; the caps are gone (2026-11-02)
- The strict readings were mostly blind, and one of them lied (2026-11-02)
- The attached article temp is journaled no more, and the build's cache is real (2026-11-03)
- An index cannot be deleted, and every dictionary has one (2026-11-03)
- The index chip is a measurement, not a door (2026-11-03)
- The article index is a file of its own (2026-11-03; reader 2026-11-02)
- A ZIM without its own index gets ours, without the bodies (2026-11-02)
- Three readings, chosen by the reader, not by the program (2026-11-02)

## Windows close-to-tray change

Windows System Settings now has `Close to system tray`, enabled by default. The
main window hides on close; clicking the tray icon or choosing `Открыть окно`
restores it. The tray menu omits Rescan dictionaries and Open dictionary folder,
and its remaining commands are in Russian. The setting and tray behavior are
Windows-only. The first build failed at startup because GetUserDefaultUILanguage
was looked up in user32.dll; it belongs to kernel32.dll. The lookup is fixed,
and `build-windows.cmd` now builds and verifies DictShelf.exe. Native UI behavior
remains to be checked on the desktop.

Windows System Settings now has a Hot keys section before System. It configures
global show/hide and selected-text lookup; the settings and main-window size live in
`state.json`. The default lookup shortcut is Ctrl+Shift+B, Ctrl+Shift+B: the first press sends ordinary Ctrl+C
to the previously active app so it copies the selection, and the second brings DictShelf to the foreground
and searches the first copied word in Exact + lemmatization (`=+`) mode. A clipboard-sequence check prevents searching stale text when copying fails. The default shortcuts are
`Ctrl+Shift+F11` and `Ctrl+Shift+B, Ctrl+Shift+B`. The window restores its last saved size.
Both hot-key actions accept one chord or a two-chord sequence; the second chord may differ.
Windows captures and replays physical scan codes so letter shortcuts ignore the active layout.
Windows UI behavior still needs desktop verification.

Windows profile defaults now match the current profile where configurable: light theme, its UI font scales and article colours, 20 results per portion, Below 50k Search as you type threshold, ignored diacritics, hidden unmarked examples, beta updates, Parallel dictionaries indexing, and the current global hotkeys. Existing saved preferences take precedence. 16 GB result-cache limit, system language and combined dictionary limit already matched their defaults. Windows chooses the initial window size and position; saved dimensions are restored on later starts. The Windows cgo/FTS5 GUI build succeeds; runtime UI verification remains pending.

All app checkboxes use the shared customizable `checkbox.css` control; pages tune its CSS
variables for local sizing, spacing, and disabled colours. Boolean Settings use the same
square style, including generated System rows. System checkbox positions align within each
headed block and precede their labels; the desktop dialog normalizes rows before caching
their containers, so reopening retains the controls. The result-cache checkbox stays before
its value field. Runtime visual verification remains pending.

## Product identity

- Product: DictShelf; publisher: DMSH Labs. Android ID, Java package and Gradle namespace: `com.dmshlabs.dictshelf`; Windows artifacts: `DictShelf.exe`, `DictShelf-cli.exe`, `DictShelf-server.exe`; macOS bundle ID and launchd label: `com.dmshlabs.dictshelf`; Linux user unit: `dictshelf.service`.
- Desktop data root: `~/.dmshlabs/dictshelf`; config: `dictshelf.toml`. The prior wuDict2 installation remains separate. Settings import/export is the agreed transfer path and is not implemented yet. Go module/package names and other internal identifiers remain `wudict`.

## Search and menu state

- Choosing a saved word from search history preserves full-text mode; dictionary cross-reference lookups still switch to prefix mode.
- Search pickers retain symbolic marks (`ab→`, `→ab`, etc.) with compact inline examples in hint-size text. The main search uses the custom picker on Android as well as desktop; native option labels no longer expose internal names such as `prefix`. The article-text row retains its page icon. Runtime verification remains pending.
- Find in articles uses the shared symbolic match menu for prefix, suffix, contains, exact-word and exact-word + lemma article searches. Lemma lookup uses each dictionary's language for single-word queries; phrases are matched literally. Its `full text` item stays below a divider and appears only while server-marked FTS results are active; it navigates those marks and disables literal-search options. `Search across all result blocks` is enabled by default and loads adjacent result blocks at dictionary boundaries before continuing to another dictionary. Navigation watches result-layout changes and waits for the selected match to remain visible before settling. Switching modes does not resubmit main search. Navigation is available when article highlighting is enabled. Runtime visual verification remains pending.
- Main full-text searches keep result sections collapsed while background all-match counting runs. The saved Open all preference and a restored Browse target still open their requested sections.
- Every result header offers Browse: transferable headword modes carry their query; full-text and other modes open the dictionary's unfiltered A–Z list.
- Long-pressing or right-clicking article text offers search above or below that point. The position remains relative to all currently loaded matches, with the article range shown on later result pages; Go to match supports literal and FTS results. Runtime interaction remains unverified.
- The shared progress notice now publishes its height while any task is active, including article counting, so the main status bar can sit above it. Runtime visual verification remains pending.
- Contains builds headword token FTS and trigram indexes together; FullText builds separate article FTS. `IngestVersion=6` offers existing indexes for rebuild, while old databases remain readable. Main headword search and Browse fall back to the base index when optional indexes are absent. Search modes include suffix, whole-word and explicit lemma variants; article full-text stays separate.
- The grouped no-index list under multi-dictionary full-text search has an Add All action in its header, which queues all listed dictionaries for full-text indexing. Per-dictionary Add links sit in a shared left column beside names; both use `--ui-font-addition`. Visual verification remains pending.
- Main search now uses lemmas only in `wordLemma` and `exactLemma`: the typed form and a distinct lemma go into one per-dictionary query in both HTTP and native bridges. The old empty-result second wave was removed. Focused morphology checks and the full `internal/server` test package pass with Windows cgo/FTS5; Windows/Android UI checks remain.
- Main search starts with five results per dictionary; its `5+` action reprioritizes that dictionary within the active new-index queue without adding a worker. Once the full count is ready, the initial results switch to paged article controls. The queue and demand endpoint support native and HTTP transports; runtime UI checks remain.
- Main search mode menu follows Browse/Wildcards grouping and styling. Choosing Recent list preserves the current search mode. Main search progress and Stop use the shared bottom task panel. Search diagnostics record query, mode, dictionary scope, chosen backend/database/index and per-dictionary results in HTTP and desktop bridge paths; runtime UI verification remains pending.
- Android lookups launched through recognized reader integrations default to `wordLemma` (`·ab·+`); an explicit `mode` parameter in a `dictshelf://` URI takes precedence. `build-android.cmd debug intel` passes; device handoff is unverified.
- Example folding now recognizes the requested diamond, square, triangle, circle and star bullets (including `⊙`, `⊚`, `⦿`, and `⁕`). It keeps an authored leading marker and suppresses the shared marker on that line. Visual verification remains pending.
- The Examples settings now include Dim background (on by default) and Fade text (off by default) after Also hide examples without [*]. Both preferences are stored in `state.json` and update existing article styles live; visual verification remains pending.
- The Dim background and Fade text switches share a measured label column so their On/Off controls align; the measurement follows the visible text size and updates when the panel is laid out.
- Search-return snapshots and Browse find/filter results are bounded SQLite caches shared by the native and HTTP paths. Browse links retain the originating query and dictionary through article navigation.

## Search parallelism and the two caps that went away (2026-11-03)

Settings → System settings → Search indexes has a **Максимум потоков поиска** field. Empty means
automatic: 5, then clamped to `min(cores-1, GOMAXPROCS, platform baseline)` — a desktop gets 5, a phone
keeps the `MaxProcs` policy (≤4), and battery-saver/thermal demotions follow GOMAXPROCS through
`applyProcs`. The same value bounds the client's Wildcards build (`maskWorkerCount`, ceiling `cores-1`).
This one STAYS: it is a lane count for the work, not a clock on it, and it never changes what a search
returns. The neighbouring Log file switch **Записывать длительность поиска по словарям** still gates
`search_fanout` (a per-dictionary duration measurement, `internal/search`) and `wildcards_append`
(cache lock wait vs work, `wildcard_cache.go`). Measurements on 42-, 110- and 388-dictionary lists show
the setting changes nothing measurable past ~5: the run is set by the slowest dictionary and by how the
work distributes, not by the cap.

**Both search clocks are gone.** `SearchFanoutBudget` and `SingleSearchBudget` are deleted from
`internal/indexconst/constants.go`, and the file now carries the reasoning in place of them: a clock
cannot tell a slow disk from a wedged backend, and the fan-out one measured how long a dictionary
*waited for its turn*, so it failed dictionaries that were working — the user's 388-dictionary `contains`
search, where every row said "timed out" while the same verbose log opened that library as
`entries contains=true (2ms)`. A search now runs until it finishes or the reader stops it. What the
user's ideology asks for in place of a clock is a watchdog on PROGRESS: where the loop is ours
(headwords, article ids, archive records) a thread that stops moving is a bug or a corrupt file and is
reported with the dictionary's name and size and the record it stopped on; where the call is opaque (one
SQLite `MATCH`) there is no progress to watch, so there is no limit there either — the reader has Stop.
**That watchdog is not written yet**; it is the one remaining piece of this section.

**The fan-out memory cap went with it** (`SEARCH_MEMORY`, `Registry.searchBudget`, the `fanout` /
`admit` / `settle` / `openWithin` machinery, `tooHeavy`, and the wire's `deferred` / `bytes` /
`indexing` fields). Its whole job was to bound the burst of *preview* backends one wide query
materialised, and that burst no longer has a producer — see the rule below. The client's deferred
section went too (`loadDeferred`, `deferLabel`, `data-deferred`, the `search.tap` / `search.click`
catalog keys, and the dead `summary .count.hint` rule in `app.css`), and `Expand all` no longer has to
exclude anything because opening a section fetches nothing. Read the deletion as the rule, not as
tuning: nothing about a search changes what it *returns* any more.

**A dictionary without an index is not searched at all, on either path.** `entry.searchOpen()`
(`dmsh_search_open.go`) is the one guard, shared by the native service and the legacy streaming
`handleSearch`: a dictionary that owns no database and is not self-indexed is refused with
`errIndexNotReady` — *"the dictionary index is not ready yet; it is being prepared"* — and its row shows
exactly that through `errorText`; the source is never read and never indexed on first use. The legacy
path already had the narrower rule; v2 (the desktop bridge and the native path) used to open the source
under the fan-out budget, and the difference is closed by taking the stricter side, which is the user's
ideology stated flatly: a dictionary is added and indexed, and one with no index takes part nowhere.
Self-indexed formats (ZIM, `entryHasIndex`) are exempt on both sides, as before.

What is left for the 388-dictionary report is not code: the user's own machine should build our article
index for the three archives and re-run that `contains` search with `searchArticleDiagnostics` on ("Record
search results and article opening" — the only switch that sends `searchDiagnostics=1`), so the next
session starts from a measurement rather than a guess.

## Lemma files on this machine (Russian installed 2026-11-01)

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

## ZIM prepared databases now have two forms (read-through full text)

A ZIM's prepared database was always title-only (SPEC §24), so an `article_fts` built over it could
only ever hold titles: a full-text search on a ZIM returned title matches and nothing from article
bodies, while the receipt still said `index = full text` and meta recorded `ingest_level = text`.
Verified on disk against the user's own libraries: the DSL Urban Dictionary's `article_fts` answers
`"for example"` with 8 089 articles against 1 headword, while the Wikipedia ZIM's `article_fts` gives
exactly the same count as `entry_fts` for every query (`"в том числе"` → 0, `"кларнетист"` → 8 titles).

Now the form is explicit and self-described: `index_kind = headwords` (title-only, unchanged) or
`articles` (read-through - article clusters decompressed, bodies stored and indexed, articles served
from the database). `store.ZIMForm` is the one reader of the marker; `dict.Meta` gained `Articles`
beside `HeadwordsOnly`; the nine places that compared the raw string now ask which form they actually
need. Reading is chosen by the request in `registry.go` (nil FullText keeps the form that is there),
so an ordinary rebuild cannot drop a read-through index, and only a database with neither marker is
converted. The bulk "add full text" action skips ZIM dictionaries - a read-through build decompresses
the whole corpus (tens of GB, hours on a mechanical disk) and stays a per-dictionary click, made on the
chip in Dictionary Settings.

Both ZIMs on this machine were then built read-through, and the cost is now measured rather than
guessed: `wiktionary_ru_all_nopic` (2.15 GB source, 1 528 188 entries) → 9.0 GiB in about nine minutes,
and `wikipedia_ru_all_maxi` (37.1 GB source, 2 199 370 entries) → **31.1 GiB** (33 439 162 368 bytes).
The `7.9 M entries` seen in the build log is a stage counter, not the database: the finished Wikipedia
holds exactly the 2.2 M entries the title-only build held. The honest-index test passes on both -
`"см. также"` answers 136 053 Wiktionary articles against 0 headwords, `"таким образом"` 91 356
Wikipedia articles, and the reader's original complaint is closed: `"джазовый кларнетист"` now answers
73 articles in 2 ms, `Беше` and `Беше, Сидней` among them. A phrase in 192 797 articles (`"в том
числе"`) takes 207 ms, a selective one ~0 ms - the user's verdict is that the size is significant but
not catastrophic, and article-text search is instant.

A gate that had to be added after the first real build: the automatic first-run queue began read-through
builds for every ZIM, because the old title-only databases record `Plan.FullText=true` (the level they
were ASKED for, not what they hold) and the queue inherits it. `preparedHasIndexes` now treats a ZIM's
headwords as the whole question, so no unattended pass ever upgrades a ZIM; only the chip does.

## The work panel's rows are a function of the job state, not of an event

An indexing task's per-dictionary rows and its "Show tasks" control appear only while the task carries
children, and children used to be attached by a `wudict-work` listener that required the event to carry
a `busyProgress` array. Whenever that attachment was missed - the array absent, or a page that restored
its banner before the dictionary list had loaded - the panel fell back to its one-line form: message,
progress bar and Stop, with no rows and no disclosure. That is the old progress bar's shape, and it is
what the user reported while two ZIMs were building (`notice_text` in the live system log recorded it
with and without the toggle minutes apart). `window.wudictTaskRows` in `index.html` now rebuilds the
rows on every render from the state the panel was handed, so the list cannot depend on which event
arrived; only this job's dictionaries are rows, because `busyDicts` is a library-wide lock list and the
header's "active operations" count is deliberately wider than the task (it can read 2 while the job
indexes 1). `tools/work-progress-ui-test.cjs` covers layout and locking but needs Playwright, which is
not installed on this machine.

## What a read-through ZIM costs, measured on the user's four archives

Measured 2026-10-31 by walking only the ZIM directory (header, MIME list, cluster pointers, dirents -
never a cluster payload) and, for the prepared databases, `sum(length(entry.m))` plus `dbstat` per
table. The directory walk agrees with the prepared database's entry count to a handful of rows on all
four (the readers drop articles whose body is empty, which is the difference), so the parse is checked
against the library, not assumed. All figures bytes.

| archive | source | text clusters | articles | redirects | media entries | text db | stored markup | article FTS |
|---|---|---|---|---|---|---|---|---|
| wiktionary_ru nopic | 2.15 G | 1.079 G | 1 528 188 | 999 439 | 182 | 9.64 G | 6.90 G | 1.29 G |
| wiktionary_en nopic | 9.16 G | 4.363 G | 9 087 944 | 42 006 | 56 | 25.26 G | 15.64 G | 4.56 G |
| wikipedia_ru maxi | 37.1 G | 13.775 G | 2 199 375 | 2 825 093 | 2.88 M | 33.44 G | 22.78 G | 7.41 G |
| wikipedia_de maxi | 48.6 G | 13.624 G | 3 275 258 | 1 878 526 | 4.84 M | 35.04 G | 22.29 G | 9.21 G |

Three things the numbers settle. **Bodies dominate, the FTS does not**: the article index is 19, 29,
33 and 41 % of the stored markup, so the body copy is 69-97 % of everything a read-through build adds
and the searchable text is the smaller part of the bill. **The container is mostly not text**: Kiwix's
own Xapian index sits in namespace X as three blobs and is 8-48 % of the file (0.91 G, 4.13 G, 3.48 G,
3.76 G), and in the maxi Wikipedias the images are another half - so estimating from the source file
size is wrong twice over. **Expansion is a family constant, not a number**: compressed text clusters
become 1.65x stored markup on both Wikipedias (1.653 and 1.636 - the two agree to 1 %) but 3.58x on
the English and 6.39x on the Russian Wiktionary, so one coefficient cannot serve both families. The
free classifier is compressed bytes per article: 480 B and 706 B for the wiktionaries, 4.2 KB and
6.3 KB for the Wikipedias, which matches the observed split.

## What each index costs on the non-ZIM library (three passes)

38 dz/mdx dictionaries, measured with `text.db` size, `sum(length(entry.m))` and a full `dbstat`
breakdown after each step; the entry count never moved, and `entry` was byte-identical before and
after full text - the control that the bodies were not re-encoded.

| step | text.db | delta | what it was |
|---|---|---|---|
| base (index only) | 2 988 MiB | - | 4 010 535 entries, bodies 2 126 MiB deflate (69 % of the db) |
| + contains | 3 058 MiB | +2.3 MiB | `entry_fts` + `entry_trigram`: 88.6 MiB = 2.9 % = **23 bytes per entry** |
| + full text | 4 565 MiB | +1 507 MiB | `article_fts`: 1 353 MiB, the rest free pages (VACUUM would return ~150 MiB) |

So contains is priced per headword and full text per body byte: contains is flat at 19.8-42.4 B per
entry (23.2 average) whatever the dictionary weighs, while full text added 16.5-117.6 % of the file
before it (median 45 %), i.e. 0.49x the database that existed. The spread is deflate's: the index is
built over `StripHTML` of the *uncompressed* body while the file holds it compressed, so a small
dictionary whose markup compressed badly pays more for the index than the bodies take. ZIMs are not in
this series - their full text is the read-through form above.

## Size estimates: measured, predicted, not yet shown

Stages 1-4 are in the tree. The ingest records `body_bytes`, `stored_body_bytes`, `headword_bytes`
and `stats_version` in the meta of every dictionary; a ZIM reader adds what its directory held
(`zim_articles`, `zim_redirects`, `zim_media`, `zim_other`, `zim_clusters`, `zim_text_bytes`,
`zim_mixed_bytes`, `zim_other_bytes`, from `internal/format/zim/dmsh_stats.go`, also usable alone as
`zim.MeasureDirectory`). Every one of those keys is in `volatileMeta`, so recording them moved no
fingerprint and the golden tests still pass; `TestStatsMetaIsVolatile` keeps it that way.

`internal/estimate` holds the coefficients (`Version = 1`) and predicts a band: contains 18-45 bytes
per entry; full text a share of the current file for dz/mdx (0.16-1.18, median 0.45), a share of the
stored bodies for a ZIM that has them (0.18-0.42), or a multiple of the archive's compressed article
text for a read-through build (2.4-2.6 encyclopedia, 5.7-9.0 dictionary, split at 1500 compressed
bytes per article). `estimate_test.go` carries the measured corpus as a table - 38 dz/mdx rows with
their real growth, four ZIM rows with their real builds - and asserts the measurement falls inside the
predicted band. It earned its keep on the first run: three constants were set from rounded numbers and
were wrong (contains 20 against 19.8, full-text 0.17 against 16.5 %, wiktionary 5.8 against 5.79).

`stats.cache` beside `text.db` (shape: `version`, `index_uuid`, source fingerprint, numbers) caches it.
It is written in place rather than through a temporary - a cache whose reader validates its content
needs no atomicity, and a stray temp file is something every listing of that folder would have to know
about - and it is accepted only when `index_uuid` matches `dict_uuid`, the version is current and the
source fingerprint still matches, while a source that is GONE is accepted (folders outlive sources).
`Registry.warmStatsStart` fills it once per process from `Warm()`: two workers, one dictionary
committed before the next, no prompt and no panel row, so a mid-flight close leaves the finished ones
measured and the rest for the next start.

The numbers now ride on each dictionary list row (`dictInfo.Estimate`, `internal/server/dmsh_estimate.go`),
attached AFTER the row cache - the cache key knows nothing about the sidecar, and a cached row would
otherwise keep saying "unknown" until something unrelated invalidated it. That means the page needs no
new endpoint, no bridge method and no `openapi.yaml` entry: it reads what `/api/dicts` already sends.
Two facts the first test run settled: a dz/mdx row gets its band from the file's own size and needs no
measurement at all (`Basis: database`), so an old library is estimated out of the box; a ZIM row gets
none until its directory has been measured (`Stats.ZIMForm`, which the sidecar now records beside the
numbers), and that absence is exactly what the page shows as "Calculating".

The window is in as `#fullTextPicker` (index.html), opened from a `full-text search` row in the
Settings drawer - the owner's decision is that full text is its own door, not a chip beside three
cheap ones. It lists every dictionary alphabetically with its band (or "already built", or
"calculating" when nothing is measured yet), a checkbox only where there is something to build, a
sticky summary with the selection's total and what the volume will have left, a warning when the high
end does not fit, and the ZIM part of the total called out separately because that one is a rebuild
that copies article texts. The build goes through the existing work API (`feature: "fts"`,
`action: "create"`) so progress, Stop and the task panel are the ones already there. Free space rides
on the list response's `end` message (`freeBytes`, `libraryVolume`, `fsx.FreeSpace` on the library
volume) rather than on a route of its own.

The build direction is gone from every other window: the dictionary chip keeps only "off" (removal is
this dictionary's own state and cheap - the bodies are not decompressed again), and both the bulk
button and the single "add full text" action open this window. CLI flags are untouched: a script is
not a window. The ZIM skip and its `dictUI.bulkZimSkipped` string are now dead on that path.

Not verified: the dialog has never been rendered - this machine has no browser automation (see the
Playwright note above), so the JS is only checked by `tools/i18n-test.cjs` (syntax + keys) and the
layout by eye in the source, and that the CSS block scoped inside the dialog is the right place for
those rules rather than app.css.

**What removing full text frees, measured without removing anything** (2026-11-01). SQLite's own page
accounting (`dbstat`, read-only) gives the sizes inside the four prepared ZIM databases, and the article
index is exactly the space a removal frees - `ClearSearchIndexesContext` drops `article_fts` and
vacuums:

| archive | text.db | our article_fts | headword fts | trigram | bodies (entry + its indexes) |
| --- | --- | --- | --- | --- | --- |
| wiktionary_ru 2026-09 | 9 645 MB | **1 291 MB** | 20 MB | 22 MB | 8 018 MB |
| wikipedia_ru 2026-10 | 33 439 MB | **7 414 MB** | 33 MB | 61 MB | 24 830 MB |
| wiktionary_en 2026-08 | 25 265 MB | **4 556 MB** | 101 MB | 128 MB | 19 946 MB |
| wikipedia_de 2026-10 | 35 044 MB | **9 213 MB** | 44 MB | 82 MB | 25 059 MB |

The first three match the figures the earlier measurement passes recorded (1.29, 7.41 and 9.21 - GB,
not GiB as that note said), so the estimate the window shows for a ZIM is anchored to measurements that
a second, independent method reproduces. Two things that paragraph above had wrong, both worth knowing
before trusting the button: the **bodies are NOT freed** by removing full text - 8-25 GB of stored
articles stay, and they go only when the request asks for a rebuild (`index: true`, which re-ingests the
archive in its headwords-only form, `internal/server/clear_database.go` takes the `ClearSearchIndexes`
branch for `fullText` alone and the `reconcile` branch only for `index`) - and the archive's own Kiwix
index (0.52-3.18 GB, already inside the ZIM) is 2.5-2.9x smaller than the one we build, which is what
makes it worth reading instead of building.

## Reading Kiwix's own full-text index (spike findings)

`X/fulltext/xapian` is stored **uncompressed**, one blob per cluster, starting with the Xapian magic
`0f 0d "Xapian Glass"`, so it can be read in place from a mapped archive - no copying. Sizes 518 MB,
2.52 GB, 3.18 GB for three of the four archives, against our own built article index of 1.29 GiB,
7.41 GiB, 9.21 GiB (**2.4-2.9x larger**); `X/title/xapian` and the title listing are uncompressed too.
libzim (cloned at `D:/Projects/Android/libzim`) documents the whole scheme: the query is unaccented
first - via **ICU**, in libzim, not Xapian - then parsed with `OP_AND`, the stemmer and stopwords taken
from the index metadata, `STEM_ALL`, `FLAG_CJK_NGRAM` (`src/search.cpp`); the index side wrote
`index_text_without_positions` with `STOP_ALL`, so **no positions exist and only stemmed forms do**.
libzim opens the index by seeking the archive and handing the fd to `Xapian::Database(fd)`.
The pure-Go reader (gozim, MIT) parses that real version header correctly - `DocCount` = 1 528 188,
exactly our entry count - but fails on every table access with `root block 571 not found`. Not the block
size (both sides use 8192); the likely cause is the single-file layout, where all six tables are opened
at `version_file.get_offset()` and block numbers are global to the area, while gozim assumes the
multi-file shape. If we take this path the code belongs in our own package beside the container, reading
a byte range rather than a `[]byte`, with Xapian built **locally as a test oracle** and never shipped,
because a reader of our own makes compatibility our burden and its failures silent.

## The glass format, as read off Xapian 1.4.32 (written down before the code)

Everything below is from `D:/Projects/Android/xapian-core` and was checked byte by byte against the
`fulltext/xapian` blob of the Russian Wiktionary, so a reader can be written from this section alone.

**Integers.** `pack_uint` is LEB128 (7 bits per byte, high bit continues). `pack_string` is a packed
length then the bytes. All values *inside blocks* are **big-endian** (`aligned_read4`, `unaligned_read2`).

**Version file, at the start of the blob** (`glass_version.cc`): magic `0f 0d "Xapian Glass"` (14 bytes),
then the format version (2 bytes, big-endian, `GLASS_FORMAT_VERSION`), then the database uuid (16 bytes),
then `pack_uint(revision)`, then **six** `RootInfo` records in table order (postlist, docdata, termlist,
position, spelling, synonym), then the statistics. A `RootInfo` is: `pack_uint(root)`, `pack_uint(level << 2
| sequential << 1 | root_is_fake)`, `pack_uint(num_entries)`, `pack_uint(blocksize >> 11)`,
`pack_uint(compress_min)`, `pack_string(free_list)`. The statistics are eight packed integers: doccount,
last_docid - doccount, doclen_lbound, wdf_ubound, doclen_ubound - wdf_ubound, oldest_changeset,
total_doclen, spelling_wordfreq_ubound. Each `RootInfo` carries the block size, so the version file needs
no separate field.

**Blocks** (`glass_table.cc`): `read_block(n)` reads at `offset + n * block_size`, where `offset` is the
descriptor position when the database was opened - for libzim that is the blob's offset inside the ZIM
(`src/tools.cpp:320` seeks and hands the fd to `Xapian::Database(fd)`), and for our own reader it is 0 at
the blob's first byte. **Block numbers are global to the file** and shared by all six tables: that is the
single-file layout, and it is exactly what a multi-file-shaped reader gets wrong (gozim's
"root block 571 not found"). Block 0 is unused (`blockNone`), so block 1 starts at `block_size`.
A block's header is 11 bytes: revision (4, BE), level (1 - 254 means freelist), MAX_FREE (2, BE),
TOTAL_FREE (2, BE), DIR_END (2, BE). The directory follows the header: 2-byte big-endian item offsets at
11, 13, 15, ... below `DIR_END`, ascending by key, each giving the item's offset in the block. Items are
packed downward from the end of the block.

**Items.** A *leaf* item (level 0) is `I2 K1 key [X2] tag`: `I2` is the item size in 2 bytes whose top
three bits are flags (`0x80` tag is compressed, `0x40` last component, `0x20` first component); a
component other than the first carries `X2`, its 2-byte counter. A *branch* item (level > 0) is
`block(4, BE) K1 key X2` where the leading four bytes are the child's block number and `size() = key_len +
1 + 2 + 4`. The compression flag means the tag is zlib-deflated above `compress_min`.

**Checked on the real file**: block 571 of the postlist table (at 571 * 8192 bytes into the blob) has
revision 1, level 2, directory from 11 to 323 (156 items) with item offsets 8185, 8172, 8159, ... spaced
by 13 bytes, and block 1 has level 0 with eight 13-byte leaf items - all consistent with the layout above.
So the addressing model is confirmed, not guessed.

**The reader works end to end** (`internal/format/zim/glass`, pure Go, no cgo), against the Russian
Wiktionary's 518 MB `X/fulltext/xapian` (format 1134, revision 1, block 8192, 1 528 188 documents,
postlist root 571, docdata root 59 228 with one record per document, the termlist/position/spelling/
synonym tables empty - libzim indexed without positions). What it reads: the version file, the
single-file block space, the B-tree descent, item keys and tags, **chunked** posting lists, the docdata
table and the query side (metadata, stopwords, stemming, AND). Nothing is copied: the tool extracts the
blob once, and the package takes a `[]byte` (the byte-range API is still to do).

`tools/xapian-glass-check <archive.zim> [query ...]` is the check: it prints the index's metadata, runs
each query, and opens every article a query returns to verify that a word of that article stems to each
term asked for - which is the index's own criterion, so index and reader agreeing is the whole result.
Numbers from it, on the Russian Wiktionary: `"кларнетист"` 11 documents / 32 occurrences, `"джаз"` 162 /
715, `"год"` 41 037 / 59 574, `"слов"` 1 449 345 / 5 081 953. The last two are the interesting ones:
their lists are split across **42** and **1 448** table entries, and every entry is read, equal to the
termfreq in the first chunk's header. Multi-word AND behaves: `"духовой инструмент"` gives бассетгорн,
бомбарда, альт-саксофон; `"музыкальный инструмент"` gives фортепиано, укулеле, ионика.

**It reads all four archives in place, in three languages** - `Dict.XapianIndex()` hands out a seekable
window onto the archive file (`io.NewSectionReader` over the container's own handle), and
`glass.OpenSource` reads blocks through it, so nothing is copied and the tool runs in well under a
second instead of copying 518 MB - 3.18 GB first:

| archive | index | documents | language | example |
| --- | --- | --- | --- | --- |
| wiktionary_ru 2026-09 | 518 MB | 1 528 188 | `rus` | `"духовой инструмент"` → 169 |
| wikipedia_ru 2026-10 | 2.52 GB | 2 199 375 | `rus` | `"духовой инструмент"` → 774 |
| wiktionary_en 2026-08 | 3.11 GB | 9 087 944 | `eng` | `"musical instrument"` → 7 197 (Porter2: *musical* → *music*) |
| wikipedia_de 2026-10 | 3.18 GB | 3 275 258 | `deu` | `"Jazzmusiker"` → 18 973 (*-iker* off, so the stem is `jazzmus`) |

Each row's query was checked the same way - every returned article opened and every term found in it.
The English row is the clearest sighting of the stemmer being the archive's own: no English index holds
"musical" as a term, and the query works anyway because both sides reduce it to "music".

**Terms are stemmed, and the query has to stem too** - an earlier note here said the opposite
("a query needs unaccenting and case folding but no snowball stemmer") and it was wrong, on evidence
that looked convincing and was a coincidence: the first term checked, `"кларнетист"`, stems to itself.
The index is built with Xapian's `STEM_ALL` and `STOP_ALL` (`src/writer/xapianWorker.cpp:49-58`), so it
holds stems and only stems: in this archive `"джазовый"` is **0** documents while `"джазов"`, what the
Russian stemmer makes of it, has 29, and `"джазов"` itself stems on to `"джаз"` with 162. The stemmer is
whichever the index's `language` metadata resolves to - libzim runs it through ICU to a two-letter code
and hands it to `Xapian::Stem` (`src/writer/xapianIndexer.cpp:44`, `src/tools.cpp`), which here is `rus`
-> `ru` -> Snowball Russian. `internal/format/zim/glass/stem.go` carries Snowball's own algorithms
compiled to Go (`github.com/blevesearch/snowballstem`, BSD-3-Clause, notices regenerated), keyed by the
codes and names Xapian accepts; a language Xapian stems but we do not (Armenian, Basque, Catalan,
Indonesian, Lithuanian, Nepali, and the legacy earlyenglish/german2/kraaij_pohlmann/lovins variants) is
reported as unstemmable - `ErrNoStemmer` - rather than searched with the wrong one, while a language
Xapian has no stemmer for (Japanese, Thai) was never stemmed and is searched plain.

Three more things only the bytes settled, each of which the code got wrong first:

- **A common term's list is several table entries, not more bytes under one key.** Xapian breaks it into
  ~2 KB chunks: the first under the term itself, each later one under the term, a zero byte and the
  chunk's first document id in the key's own encoding (`glass_postlist.h`, `make_key`). Reaching them
  needs a table cursor, not a lookup - `nextKey` keeps its descent path so it can climb out of a leaf -
  and the next chunk does **not** start at the previous chunk's last document plus one, because a term
  is not in every document: that gap is why a computed key finds nothing.
- **The document-id key encoding is glass's, not chert's.** Both are in `common/pack.h`; glass's is a
  unary length marker (leading set bits) and, below `0x8000`, **two plain big-endian bytes with no
  marker at all**. Ours was the chert reading, which agrees with glass above `0x8000` and differs below
  it - so every id an early check happened to use was above it and looked right, while ids below it
  resolved to a *different* document (`49 a8` is 18 856, not 2472) or to none. The symptom was a search
  whose hit count was short of the term's own list: 1 433 839 against 1 449 345, the difference being 16
  chunks re-read because the walk's document ids were decoded from the wrong form. Pinned against the
  archive's real bytes in `item_test.go` (`04 24` for 1060, `80 85 91` for 34 193).
- **An item's size field covers the key AND the tag** (Xapian's `I K key X tag` with I the length of the
  key-tag pair), so the tag ends three bytes past the size. Reading the size as the whole item cut every
  tag three bytes short, which showed up as paths truncated inside a character.

## Two more from the user's screenshots, to implement (2026-11-02)

1. **The article find dialog needs the other two readings.** For the `[A … B]` (words) mode it already
   works end to end: the dialog's dropdown carries that reading (`articleFindMode`, index.html:964, with the
   `hidden` option `fts` revealed when the dictionary has full text), opening it from that mode pre-fills the
   search string and selects the reading, and then the ↑/↓ buttons walk the highlighted words. What is
   missing is the same for `[A B]` (phrase) and `[A ↔ B]` (near): two more options with their own labels and
   hints (the keys `articleFind.fullText`/`articleFind.fullTextHint` are the shape to follow), the pre-fill
   for those modes, and a matcher whose semantics are the ones the labels promise - adjacent and in order,
   and within ten words in any order. The dialog's matching engine is the thing to read first: it is what
   decides whether those two readings can be computed client-side from the rendered article (as words
   presumably are today) or have to ask the server, which for an archive-backed dictionary has no positions
   at all (the plate line already says so).
2. **The find dialog's total does not follow the dictionary's.** The screenshots show "1 of 155" and later
   "83 of 155" in the find bar while the plate above says "(200)": the dialog counted when it searched, and
   the plate's number comes from the result cache, which the background counting pass later raised. The
   dialog's count has to read the same source - either subscribe to the per-dictionary cache total as it
   grows and when it closes, or recompute when the cache is done (`cached.done`, the same flag that makes
   `paging.exact` true). Found by the user's eye, not by a test: nothing in this area is covered by one.



## Two chips, and who answers which reading (2026-11-02)

Done, and verified on the live library (44 dictionaries, 6 ZIM archives, one of them the converted
Russian Wiktionary with an index of ours). The row carries two separate facts instead of one overloaded
one: `archiveIndex` (the ARCHIVE carries its own full-text index - from the meta, no container opened)
and `ourFullText` (THIS database holds an article index of ours - the plan). `Caps.FTS` keeps its
meaning, "full text works", and either fact can be why.

The chips say exactly that. On a ZIM whose archive answers but which holds no index of ours: `our FTS`
(off, tooltip "full text already works through the archive's index; ours adds exact phrases and
proximity") beside `in the archive` (filled, locked - a fact, and locked because the index is not ours
to remove). With our index built as well, the first chip becomes `our FTS 1.3 GB` and a click removes it,
which is the switch it has always been for every other dictionary. Non-ZIM dictionaries are untouched
(`full-text`), and the pair is only shown when there IS a pair.

The window follows: `built` now means our index, so an archive that answers full text by itself gets a
checkbox and a band instead of a "✓" that claimed credit for a file Kiwix shipped - "ours not needed: the
archive answers", with the estimate. And it needed a new estimate basis, because what is built on request
is NOT the read-through form the `zim_text` band prices (2.4-9x the compressed article text): it is our
index alone, bodies dropped. `BasisIndexOnly` = 2500-4000 bytes per article for the encyclopedias and
350-1200 for the dictionaries, split by the same compressed-bytes-per-article discriminator as the other
band, measured from the four archives converted here (501 and 845 B/article for the Wiktionaries, 2809
and 3369 for the Wikipedias - all four inside their band, pinned by a test). First version of this
divided by 1000 and quoted 8 MB for a Wikipedia; the live check on the real rows is what caught it.

**Which source answers which reading**, now explicit and mode-aware (`search.archiveAnswersMode`, and the
same rule in `upgraded.StreamArticleIDs`): "words" keeps the archive's own answer even when an index of
ours exists beside it - that reading is what the archive's ranking was built for - while "phrase" and
"near" go to ours when it exists, because the archive's index stores no positions and can only verify a
phrase by scanning article text, bounded (archiveStrictCheck). Without an index of ours, both strict
readings stay with the archive exactly as before. This needed `Store.HasArticleIndex()` (`ftsOK`, "an
article index of ours is here") to be threaded through the three places that used to ask `OwnFullText`
alone: `FullTextMatch` returned nil for such a dictionary, which would have silently emptied a phrase
search that the router had just sent there.

Verified live: the API returns the two facts and the band per row; the panel shows `our FTS` + `in the
archive` on five archives and `our FTS 1.3 GB` + `in the archive` on the converted one; the full-text
window lists six ZIM rows with a checkbox each and "ours not needed: the archive answers", the converted
one with "built over the archive's index", and the count line reads "Built: 39 of 44 (6 from the
archive itself)". What is NOT verified: an actual on-request build from the window on a second archive
(hours of work), and the Russian rendering of the new strings (the catalogs are key-matched and pass
`tools/i18n-test.cjs`; the test server came up in English).

**The three readings have their own marks** (the user's design, 2026-11-02): `[A B]`, `[A ↔ B]`,
`[A … B]` - two words and what stands between them, tight for a phrase, an arrow for "near", an unbounded
gap for "anywhere". They are typed glyphs beside the headword patterns (`ab→`, `=`, `*?`), not drawn: the
first version drew them in SVG with the bound hanging over the arrow, and the user's answer was that the
blocks were unclear and that letters in brackets are what reads - which also removes the width problem the
drawn marks were invented for. Two consequences worth keeping: the menu's icon column went from 3.1em to
4.4em to hold `[A ↔ B]` (every row shares it, so all of them start their text at the same place), and the
bound is no longer written in the mark at all - it is stated in the row's own hint, which is the one place
a reader can read it. `TestModeMarksQuoteTheNearRule` now pins that: the near hint in both catalogs has to
contain `ftsq.NearDefault` (which `search.archiveNearDistance` also reads), and each reading has to have a
mark at all. The labels and hints were shortened to one line each as part of the same pass
(`search.modePhrase`/`Near`/`Words` and their hints).

That test exists because the work found a real defect: `setSearchMode` gated on `MODE_SHORT`, a table of
the FALLBACK MARKS, and the three readings were absent from it - so choosing "Phrase" and reloading
dropped the reader back to the first option in silence, while the menu kept offering the mode. The gate
is now derived from the select's own options (`searchModeExists`), which cannot drift from the control it
describes. Verified by driving the page: all three readings survive a reload, each wearing its own mark.

## Server-removal estimate The workspace-dialog refactor in [`docs/DIALOG-REFACTOR.md`](docs/DIALOG-REFACTOR.md) is confirmed on Windows; Android/browser checks and unused iframe-route removal remain. The earlier estimate was 89–91% complete, with regression checks the largest remaining stage. Windows startup without a listener, direct page navigation and `/res/` media delivery are confirmed; remaining Windows screens, assets/styles, browser and Android HTTP paths still need verification.


Update checks: System Settings now has Release, Pre-release, and Beta channels. Beta means the word beta anywhere in the version string, case-insensitive. The new-profile default is Beta on Windows and Android; saved selections still take precedence. The selected channel is saved with UI preferences; the app checks GitHub Releases at startup at most once per day and has a manual check. Windows Go server build and Android debug APK build passed; APK was not installed. Runtime UI behavior is pending. Build scripts accept --channel release, pre-release or beta; beta adds a -beta.1 suffix and pre-release adds -pre. GitHub publication metadata remains a separate step.

New-profile defaults: Warm (Sepia) appearance with day and night background colours enabled; Windows and Android use Parallel dictionaries indexing and a 16 GB result-cache limit. Search as you type defaults to Below 50k on Windows and Below 10k on Android. On the first launch after each app-version change, background match search is enabled, Method is set to Parallel dictionaries, and Contains is enabled in Dictionary management and new-dictionary Rescan defaults; later edits persist for that version. The current version is recorded in `state.json`, `settings.json`, `history.json` and `dictshelf.toml`. Windows show/hide hotkey `Ctrl+Shift+F11, Ctrl+Shift+F11`, and only the Dictionaries and Results drawer sections are expanded. Android's System Settings hides Advanced and the divider above it. `build-android.cmd debug intel` passes; emulator verification is pending. Existing explicit hotkeys, appearance layers and drawer section choices remain in effect.

When `looks.json` has no recorded current look yet, the looks dry-check
recognizes the default Warm state as the current built-in, including its native
day and night background colours on Android. This keeps the Appearance row from showing
Custom on a clean profile and avoids a false unsaved-changes prompt when
switching from Warm. The UI uses that dry-check result to render the selected
built-in without writing a selection to disk. All focused look tests pass.

Android System Settings hides the Advanced section and its Reset default
button. Windows uses a separate System Settings dialog and does not show this
section or button. Runtime Android UI verification is pending.
