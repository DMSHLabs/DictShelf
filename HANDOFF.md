# Agent handoff — current state

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
window to the top of the normal Z order. Live tray/focus verification remains
pending.

Release workflows now handle concurrent tag creation: after a rejected push they continue only when the remote tag resolves to the same commit; Android, Windows, and native-release workflows share this guard. `dictshelf-v1.02.17` already exists remotely.

Group selectors show each group's dictionary count. The shared status-bar formatter emits the selected group's found-dictionary count followed by its total in parentheses (for example, `Essential 4 (12)`) on Windows and Android, independent of translation catalog wording.

Android's Browse chooser, Dictionary Groups page, and Filters dialog now pass `false` to the native `startDictStream(boolean)` bridge when they do not request progress; omitting the argument made Browse's chooser fail before it could list dictionaries.

Wildcards dictionary-search concurrency no longer reuses the much smaller indexing concurrency limit. It runs up to four dictionary workers, bounded by available hardware threads while leaving one available for the rest of the app.

Canonical checkout: `D:\Projects\Android\DictShelf\`, branch `dev` at `8d970980` (working tree has uncommitted changes). Edit Folders now has **Dictionary folders** and **Download Dictionary** tabs in the same window; import is available when an install folder exists. Main search suggestions now follow the selected headword mode, including wildcards, cancel on edits or scope changes, rank matches before applying the 40-per-dictionary and 500-overall defaults, and identify these limits in Results. The dropdown expands to available viewport height and scrolls internally at the window edge. For every headword mode, suggestions with equal spelling and match quality prefer shorter headwords before match position. Search history supports up to 1000 entries, and Find in articles has its own filtered recent-query list under the shared history limit. Dictionary groups and Browse A–Z show headword counts; the status-bar dictionary picker shows found-versus-total article counts, with a plus while provisional, and marks the dictionary at the current reading position. Long-pressing dictionary names opens an information popup with source files (including attached `_abrv` companions), localized headword/article counts, and About text; prepared database paths are omitted. The Browse A–Z chooser also opens this popup on long press or right-click; in Browse's word-list view, tapping the dictionary name opens it, with long press and right-click supported too. New indexes persist the source headword count separately from article count. The group toolbar starts multi-selection with a Multi button; A–Z and pin are outlined buttons separated by one button height. Membership editing uses minus buttons to remove and plus buttons to add dictionaries. Long press and right-click on names continue to open dictionary information. Article headers support left-button hold and right-click on Windows. Full-text remains excluded. Wildcard suggestions stream the first headword immediately and flush more candidates every 250 ms without loading article bodies. Selecting a suggestion keeps *? as the input mode and shows exact+lemma results; the checkmark opens the Wildcards view. Runtime UI verification is pending.

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

## Search parallelism and the fan-out timeout (open)

Settings → System settings → Search indexes gained a **Максимум потоков поиска** field. Empty means
automatic: 5, then clamped to `min(cores-1, GOMAXPROCS, platform baseline)` — a desktop gets 5, a phone
keeps the `MaxProcs` policy (≤4), and battery-saver/thermal demotions follow GOMAXPROCS through
`applyProcs`. The same value bounds the client's Wildcards build (`maskWorkerCount`, ceiling `cores-1`).
The neighbouring Log file switch **Записывать длительность поиска по словарям** gates `search_fanout`
(per dictionary, `internal/search`) and `wildcards_append` (cache lock wait vs work, `wildcard_cache.go`).
Measurements on 42-, 110- and 388-dictionary lists show the setting changes nothing measurable past ~5:
the run is set by the slowest dictionary and by how the work distributes, not by the cap. The two search
clocks now live in `internal/indexconst/constants.go` (`SearchFanoutBudget`, `SingleSearchBudget`); the
dead `demandSearchBudget` was removed.

Open: a user's `contains` search over 388 dictionaries returned 0 results with every row saying
"timed out — index this dictionary to search it" (388 `search failure … mode="contains"`, all 30 s after
the search began). The message is wrong for this case: his verbose log opens the same library as
`entries contains=true (2ms)`, so the dictionaries are prepared with the trigram index. Most likely the
shared `SearchFanoutBudget` against 388 dictionaries at `dictionary_worker_limit=7` on an ordinary HDD —
~540 ms per dictionary does not fit an open plus `entry_trigram MATCH` from a cold disk. Which stage
dominates is unconfirmed: needs `system.searchArticleDiagnostics` ("Record search results and article
opening" — the only switch that sends `searchDiagnostics=1`), plus the same query on ONE dictionary as a
control (no fan-out, `SingleSearchBudget` = 5 min).

Separate difference found on the way, and it is a BEHAVIOUR DECISION, not a pending fix (re-read and
left alone, 2026-11-01): `internal/server/dmsh_search_service.go` (v2 - the desktop bridge and the native
path) opens a dictionary that has no prepared database, while the legacy streaming `handleSearch`
declines it first (`validPrepared` -> "dictionary index is not ready"). Both sides look deliberate rather
than accidental: v2 opens through `openWithin(fan)` under the fan-out budget and spells out why a
single named dictionary is deliberately uncapped, and its render path even words the timeout as
"index this dictionary to search it"; the legacy side has a test pinning its rule ("Search does not open
or index a source when its prepared index is absent", `TestSearchDoesNotIndexOnFirstUse` - note it passes
in PREFIX mode too, so legacy is not mode-aware and an earlier note here claiming "port it, mode-aware"
had it backwards). Neither path indexes on first use, so the shared rule is safe; what differs is whether
a keystroke may open an unprepared source at all. Making them agree means choosing, which is the user's
call. The self-indexed formats (ZIM, `entryHasSelfIndex`) are exempt on both sides.

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

## A ZIM with its own index answers full text itself (2026-11-01)

The product decision, made after the reader, the routing and the ranking were verified: for an archive
that carries its own Kiwix full-text index, full text is answered from the archive, no index of ours is
built for it, and its prepared database is the title index - bodies are read through from the archive.
The archive's index is 2.4-2.9x smaller than ours on the same text (518 MB against 1.29 GB on
wiktionary_ru, 3.18 GB against 9.21 GB on wikipedia_de) and already built, and the stored bodies bought
no read speed (tools/zimtime: medians 0.52-0.66 ms either way).

What is in place, and what each piece rests on:

- **The fact is recorded, not probed.** `zim.Dict.HasFullTextIndex` asks the container (a directory
  lookup and one byte of the cluster info field, no decompression - and an index inside a COMPRESSED
  cluster answers no, because reading it would mean decompressing 3 GB before the first term), and the
  ZIM reader writes `own_ft_index=1` beside `index_kind`. So a dictionary row and a search both know
  where full text comes from without opening a 48 GB container. A store test fails if the two packages
  spell that key differently.
- **Routing.** `upgraded.Caps()` advertises FTS when the archive answers it, and
  `upgraded.archiveFullText` answers from the source; the search skips its FTS5 rungs for such a handle
  and sets `Rung="words"`, which is also what turns the existing highlight marking back on (marker()
  returns nothing without a rung). Worth naming: "full text" on a title-only ZIM used to mean full text
  over TITLES, with the bodies fetched afterwards - this reaches the article text.
- **Ranking.** Xapian's own BM25 (k1=1, k2=0, k3=1, b=0.5, min_normlen=0.5), which needed the document
  lengths: a postlist named by an empty term, key `00 e0`, whose per-document "frequency" is the length.
  It is the one list the term walk cannot read as-is - its chunk keys carry no zero separator, and its
  first chunk is a placeholder whose entry counts are left at zero even when it holds 713 entries - so
  the chunk logic became `walkPostings`, which takes the prefix. Verified against the version file: the
  lengths sum to exactly the total document length for 1 528 188 and for 3 275 258 documents. Summed wdf
  had put the 410 KB "Liste von Jazzmusikern in Deutschland" above the Jazzmusiker article; BM25 puts
  the article first, and on the Russian Wiktionary the entry "духовой инструмент" above "инструмент".
- **The ingest rule**, in the three places the form is chosen (the server's reconcile, the CLI's ingest,
  the CLI's reindex - the mechanism is single-sourced as `HasOwnFullTextIndex`). An explicit `-fulltext`
  does not override it, and the CLI says so instead of appearing to obey.
- **The interface** says which it is: the chip reads "индекс уже внутри архива" and is locked (its "off"
  would clear an index that is not ours), and the full-text window gives such a row no checkbox, the
  state "индекс внутри архива", and counts it apart from what we built.

**What is verified, and how.** The reader and the ranking on all four archives through
`tools/xapian-glass-check` (also through `dict.Open`, which is the one assumption the routing makes);
the seam tests in the tree; `go build/vet/test` on every package touched. NOT verified: the interface
changes have never been rendered (no Playwright here, as before), and no database has been rebuilt
through the new rule yet.

**What the first real rebuild found** (wiktionary_ru, run twice with the CLI, 2026-11-01). 9 644 MB
became 426 MB in 34-40 s: `entry_fts` 20 MB, `entry_trigram` 22 MB, the headword list and its indexes
the rest, `article_fts` absent, `info.txt` reading "index = headwords only". Three defects only a real
run could show, all fixed here:

- **The reader and the plan are two halves of one decision, and the first run pinned only the reader.**
  The headword reader under the kept plan ("index article text") produced a database with an EMPTY
  `article_fts`, `ingest_level = text` and a receipt promising full text - 9.6 GB down to 426 MB, but
  inconsistent. The plan is now pinned where the form is decided: the server's reconcile (only when the
  caller left it nil), the CLI's reindex (inside the ZIM branch - pinning it for every format dropped
  full text on a non-ZIM reindex, which `TestReindexOneKeepsPlan` caught), and the CLI's ingest (only
  where this fork overrides the form).
- **A stale `stats.cache` stayed valid across the rebuild**, because everything it validates against -
  the source's uuid, size and mtime - is unchanged by a rebuild: the window would have priced the
  archive against article bodies that no longer exist. The form is now part of the validity check
  (`ReadStats`, with a test), so the warm pass re-measures.
- **`info.txt` said "text.db (articles + search index)"** for a database holding no articles; it now
  distinguishes the two forms.

The archive's own reader was unaffected throughout, and the re-run left the folder consistent: 426 MB,
`index_kind = headwords`, `ingest_level = headwords`, `own_ft_index = 1`, no `article_fts`.

**The conversion is in the window, and it was tested by using it** (2026-11-01). The full-text window
now carries, for a ZIM whose archive answers full text while the database still holds an index of ours, a
per-row "Convert" button instead of a checkbox: the choice column shows a cycle symbol, the state column
the button, and its tooltip says what the conversion frees - the article index when this build can
measure it, and the folder's own size always. The action is the base index re-created
(`feature:"base", action:"recreate"`), which is exactly the conversion: for such an archive the ingest
builds the title index, so a reconcile of the old form converts it. Tested by pressing it on the English
Wiktionary: **25.26 GB to 1.20 GB in 100 seconds**, and the result is consistent -
`index_kind=headwords`, `ingest_level=headwords`, `own_ft_index=1`, no `article_fts` - with full-text
search still answering from the archive (Thesaurus:musical instrument, instrument musical, musical
instrument). The Russian Wiktionary was converted earlier the same way by the CLI.

Two things the live run taught, both now recorded in the code:

- **The archive's single reading was reaching only one of the two search handlers.** `rung` was set, but
  the page is served by the LEGACY `/api/search` (server.go, `streamMsg`) while the desktop bridge uses
  `Server.Search` (dmsh_search_service.go, `SearchEvent`) - so the new `oneReading` flag has to be in
  both, and the header still said "phrase not found · words anywhere" for a dictionary that never tried
  a phrase. It is honest now: verified in a browser, the header reads just "5+ results", while a
  dictionary with a real ladder still announces a relaxation when it makes one (the condition is
  `!h.oneReading`, so nothing else changed).
- **`sqlite_dbstat` is not the way to make the measured sizes real.** mattn's driver gates dbstat behind
  that tag and the app's build does not set it, so `searchIndexSizes` has always returned zeros - but
  enabling it HANGS startup: dbstat has no index of its own, so each query walks every page of the file
  and the 42-dictionary discovery pass stalls for minutes on the four archives. Checked both ways
  (measured size 9 213 132 800 bytes for wikipedia_de with the tag, zero without; a tagged build prints
  no banner at all and is killed at 40 s). The offer therefore quotes the folder size, and the index
  figure appears only where a build can measure it (the pure-Go modernc build can; this one cannot).

## Our index on request, for an archive that has its own (2026-11-02)

The rule "an archive with its own index never gets one of ours" is now "not by DEFAULT": a caller that
names full text explicitly gets it, in the index-without-bodies form. Two reasons, and the user gave
both: it is what makes the strict readings (phrase, near) native on such an archive - positions in our
index instead of a text scan over the candidates theirs returns, with the fast SQL paging that comes
with it - and it is the only way to build and test the index-only form today, since no archive in the
library lacks an index. The override in the server's reconcile and in the CLI's ingest now fires only
when the request left FullText nil; the rebuild paths keep the form a database already has.

Built this way on the converted Russian Wiktionary, with `wudict ingest -fulltext <archive>`: 1 528 188
entries in 458 seconds, and the result is the form, exactly - `index_kind=headwords` with
`ingest_level=text`, `stored_body_bytes=0`, `article_fts` present, 1.76 GB (426 MB of headwords plus the
1.29 GB index, which is what the article index of this archive measured). 26.8 GB of article text was
read and indexed and none of it was kept: the whole point of the form, on a real archive, for the first
time.

The receipt now names all three forms (`store.writeInfo`): "headwords + search index", "articles +
search index", and for this one "headwords + article index (article text stays in the archive)" - the
rule was two-way while there were two forms, and the third made both of the old answers wrong in opposite
directions. Pinned by a test on the real fixture, not on the wording alone.

### First index built for an archive that has its own: Викитека (2026-11-02)

The user's decision was "строим индекс", so the smallest archive without one was rebuilt with
`wudict ingest -fulltext`: 589 871 entries, `text.db` 267 MB -> 3.51 GB, `stored_body_bytes = 0`,
`index_kind=headwords` + `ingest_level=text` (the third form), and the receipt now says so in the words of
the form: "text.db (headwords + article index (article text stays in the archive))". The row reads
`our FTS 2.95 GB` + `in the archive: words`, exactly as the chips decision intends.

The build is a correction to the index-only estimate band: 3.2 GB over 589 871 articles is **5.4 kB per
article**, above the encyclopedia band (2500-4000 B/article) that was measured on the two Wikipedias and
the two Wiktionaries. Wiki*source* articles are long texts with few articles, so "encyclopedia" is the
right family and the band is simply too narrow for it - the next four builds should be measured before the
band is quoted again.

### ...and it is fixed: a log line was asking the startup path a question (2026-11-02)

The 6.3 s was not the index, not the archive and not the reading of bodies. The probe chain, in order:
`prefix` on Викитека costs the same as `exact` and as a phrase, and a plain DSL dictionary in the same
server answers in 0.28 s; the store's own calls on the very same database are `open 2 ms`, `prefix 3.6 ms`,
`exact 0.6 ms`, `fulltext 3.8 ms` - so the index answers a phrase in 3.8 MILLISECONDS while the request
took seconds; and instrumenting the request showed the gap sitting between the opener's return and the
dictionary's own query, with `backend="prepared"` on the right database.

The culprit: `searchLocationForLog`, a helper that exists only to fill the "search database ..." line of
the diagnostics log, was called UNCONDITIONALLY on every result batch. It calls `validPrepared`, which for
anything whose entry path is a SOURCE rather than a text.db (every ZIM, every DSL source) resolves the
prepared database through `store.PreparedFor` - a library-wide lookup - and that is where the seconds went.
A native dictionary entry is already a text.db, so `validPrepared` returns at the first line: which is why
only ZIMs and sources paid, and why the cost scaled with the library rather than with the query.

Fixed by asking the helper only when the line will actually be written. Measured after, same server, warm:
Викитека `prefix` 6.7 s -> **0.15 s**, `phrase` 6.5 s -> **0.04 s**, `exact` 6.3 s -> **0.04 s**; the
Russian Wiktionary's phrase 5.2 s -> **0.10 s** (its first call is cold disk, the index is 1.29 GB);
Ushakov unchanged at 0.48 s. The first request after startup still pays the library's warm-up - all 44
dictionaries opened in series, ~46 s on this machine - and that is the next thing to look at, because it
is the same class of cost: opening every dictionary to answer a question about one.

Also in this change, because it fell out of the same probe: `internal/store/strip.go` moved to
`internal/htmlref/plain.go` as `htmlref.PlainText`. The archive reader has to fold the same text our
article index is built from, and importing `store` from `internal/format/zim` closed a cycle that broke
`go test ./internal/store` - the move keeps one implementation (the whole point, after the two-stripper
bug) in a leaf package both sides can import. `internal/format/dsl` no longer imports store for it either.
Two golden tests in `internal/format/dsl` fail on this checkout at the session's base commit as well; they
are recorded in `docs/WINDOWS-VERIFY.md` as known failures to look at, not caused by this work.

### And the same shape again: opening a dictionary walked every page of it (2026-11-02)

The first search after startup took **117.9 s**, and the log showed it opening all 44 dictionaries while
the user waited - "don't wait for all of them" was the request. Instrumented, the chain was exact:
`store.Open` of the 3.5 GB Wikipedia-source database takes 1.2 ms, while `store.PreparedFor` of the same
dictionary took 9.3 s - and inside it, `Inspect` **6.5-9.3 s**. `Inspect` ran `searchIndexSizes`, which
measures the FTS5 index sizes with `SELECT SUM(pgsize) FROM dbstat`: a page-by-page walk of the whole
database, seconds on a large one, ~1 s on a medium one, and it ran for EVERY dictionary in the path that
opens a dictionary - to fill the chips that show "contains 22 MB" and "our FTS 1.29 GB".

The fix moves the measurement out of the open path entirely: `Inspect` no longer measures anything,
`store.MeasureIndexSizes` is exported for the one caller that may pay it (the registry's background
measurement pass, once per process), the numbers are written into `stats.cache` (`StatsVersion` 2 added
`ContainsIndexBytes`/`FullTextIndexBytes`), and the row reads them from that cache in `attachEstimate` -
before its guards, because the dictionary that already HAS our index is exactly the one whose size a
reader wants, and that is the case the guards return early for. Measured after, same server: first search
**117.9 s -> 0.16 s**; every size is back in the listing (Викисловарь 1 291 067 392 = the old dbstat
number, Викитека 3 167 371 264 = its rebuilt index). `TestIndexSizesAreMeasuredOutsideTheOpenPath` pins
the new contract: Inspect reports none, MeasureIndexSizes answers, and the cache keeps it.

### A ZIM search costs ~6.3 s whatever it is asked (found 2026-11-02, not fixed)

Timed against a running server on the live library:

| what | time |
|---|---|
| `exact` one article, Викитека (our index, index-only form) | 6.30 s / 6.35 s |
| `phrase`, 20 results, Викитека | 6.52 s / 6.52 s |
| `phrase`, 1 result, Викитека | 6.45 s |
| `phrase`, ru-wiktionary (our index) | 6.85 s, then 3.38 s |
| `phrase`, a plain DSL dictionary (Ushakov, our index in text.db) | 0.28 s / 0.27 s |
| opening the archive directly (`zim.Open` + `Meta`) | **5-7 ms** |

So it is not the index, not the number of results and not the reading of article bodies: it is a FIXED
cost per request that only ZIM dictionaries pay, and opening the container costs nothing like it. The
plain dictionary in the same server answers in 0.28 s, which is what the index itself costs. This is the
thing to measure next, before anything is claimed about the index-only form on a phone: candidates are
the per-request source plumbing (prepared-database reopen, resource resolution, the article-result cache
write for a page) and the reading of the archive's own directory. Nothing here is caused by the chips or
by the strict-reading fixes of the same day - the index-only Russian Wiktionary showed it before them too
(3.4-6.8 s).

### What the row says now (same day, user's decision)

The archive's index is no longer presented as full text. The chip appears ONLY beside an index of ours,
and it says what it is: «в архиве: слова», because the archive's index stores words without positions -
it answers "words" and nothing else, and a phrase or proximity search over it means reading article
texts, which on a phone is a battery question as much as a time one. A ZIM whose archive has an index and
which has none of ours therefore shows one chip, "our FTS" (off, with a build estimate), and no claim of
full text at all; the converted Russian Wiktionary shows both, side by side. The window's state line for
such a row says the same thing instead of "ours not needed".

**The user's decisions, to implement next** (recorded here because they change the shape, not just the
wording): (1) strict modes over an archive-only dictionary must not scan article texts at all - they open
the build offer window, with the estimate, and the row says why it has no answer; the scan stays only as
an explicit "search the texts anyway" choice in that same window. (2) Every cap that hides results goes:
`archiveStrictCheck` above all, and the query side's `ftsq.maxTokens` (128) must refuse an absurd query
out loud instead of silently searching its first 128 words. (3) Results are delivered progressively: five
articles per dictionary first, then the search continues in the background to the end, or until the reader
changes the query or mode.

## Progressive results: the user's three decisions, to implement (2026-11-02)

Recorded before the work, because each one changes which part of the flow moves.

1. **RETRACTED 2026-11-02: the first portion IS five already.** This file claimed otherwise because the
   server's default (`n <= 0 -> 20`) was read as the answer; the page does not use that default at all. It
   sends its own size: `perDict=5` for both the group and the single-dictionary case (index.html:9740-9741)
   and `n=String(perDict)` in the request (9820). Verified on a live page: the request that goes out is
   `n=5`, the first paint shows exactly five headwords, the total (27) is already on the header, and the
   remainder arrives as a portion when the section is opened. Nothing to change here. The original claim
   follows, struck through by this note:

1. **The first portion is five, for every dictionary and every mode**, and the remainder arrives in the
   SECOND portion - uniform on purpose, "чтобы не было вопросов почему разное поведение". Today the first
   paint is however much the first answer carried: the state is created with
   `initial: msg.results.length` (index.html:1436) and the main search's size comes from the request's `n`
   (the page's own call was NOT located with confidence - the three `"&n="` sites are the suggestions
   (1691), the deferred search (7606) and `fetchSearchPortion` (10052), so the initial size for the
   cache-driven path is set server-side: `Search`'s `n := request.Limit; if n <= 0 { n = 20 }` and the
   legacy handler's equivalent). Setting this is the first edit, and it is where the next session starts.
2. **The background continuation is always on, and the setting is hidden.** DONE (2026-11-02):
   `search()` sets `activeSearchCountsInBackground=true` unconditionally (the one-line gate it replaced is
   quoted in the comment there), the checkbox is hidden both in the markup and by script while its saved
   value is still read and written so an older library's preference survives, and the interrupt - the part
   the user stressed - is the existing `stopArticleResultCaches` plus the stream's AbortController, which a
   new search runs before anything else. Verified on a live page: a phrase search creates an article-result
   cache (`details.dict[data-search-cache-id]`) with no preference set, and typing a different query
   replaces it with a new one. NOT verified: the status-bar task chip for the counting pass (my automation
   could not observe it; the chip is driven by the same queue that the flag now always fills). The gate today is
   `state.background` (`if(state.background){articleCacheQueue.push(state)}`, index.html:1440) and the
   control is `countAllResultsInBackground` with its checkbox (index.html:1532). "Главное, чтобы фоновый
   поток мгновенно прерывался и начинал искать при поиске другого слова": that part already holds -
   a new search calls `cancelArticleResultCaches()` and aborts the `AbortController`, and the cache task
   in the status bar has a stop button.
3. **Where the total is known exactly, no background pass runs - and that already holds.**
   Verified: a prefix search paints five headwords with `total=27` on the header and creates NO
   article-result cache at all (the counting pass is driven by that cache). The cache is created for the
   modes whose total is unknown until the articles have been counted, which is the full-text family and
   the mask view. The original note follows:

3. **Where the total is known exactly (headword modes), no background pass runs at all**: the first block
   is five and the rest is plain arithmetic - the block count and the number of words - which the paging
   controls already speak. The article-result cache (a SQLite store of the blocks the reader switches
   between, `articlePageCache`/`wildcardPages`) is for the modes whose total is NOT known until the
   articles have been counted, which is exactly the full-text family.

**Two things the user's screenshots caught the same day**, both of them consequences of that change:

- The archive's chip was still hidden when an index of ours was absent (a decision from earlier the same
  day, when the chip was the only claim of full text). With the new line on the plate the search DOES use the
  archive's index, so hiding it made a plate that answers from the archive look like a dictionary with no
  index at all. The chip is shown whenever the archive has one, and its label already says what that index
  can do ("in the archive: words") - there is no claim to withdraw any more.
- The words were NOT highlighted. The marks are built from the rung NAME (`marker`, server.go), and a phrase
  request carried `rung="phrase"` into the marking even when the answer was the words reading, so the marks
  looked for a phrase that is not in the article. Both marking sites now use the reading that ANSWERED
  (`words` when `NeedsIndex`), and the cache path carries that name to the page as well. Verified in the
  server's own answer: `mode=phrase&hl=1` on the Wikipedia gives `rung=words needsIndex=true` and 8 marks -
  «кларнетист», «джазовый», «кларнетисты»… The live page could not confirm it: articles render in a Shadow
  DOM or an iframe, and a plain `querySelectorAll('mark')` in the page context cannot see inside them.

### ...and the plate is not left empty: it answers with the words and says what is missing (2026-11-02)

The user's refinement, and better than what I had built: a strict reading on a dictionary whose archive has
its own index still is NOT verified by scanning, but the plate is not left empty either - it answers with
what that index can do (the words reading, its own ranking) and carries a line, on the plate's own row in
the accent colour, naming what is missing: "the archive's index found all the words. The exact phrase needs
our index: [Build]". The line is the substitution said out loud, next to the count, which is what makes it
not the silent relaxation this fork removed.

Three places had to agree, and the third is the one that took the test to find:

- `search.query`'s archive branch answers `FullTextContext` (words), sets `Rung="words"`, `OneReading` and
  keeps `NeedsIndex` (search.go).
- the plate renders the line whenever `h.needsIndex`, regardless of whether results came (index.html), with
  `.search-needs-index` in app.css: its own row, `var(--accent)`, wrapping instead of fighting the count.
- **the article-result cache path had to carry it too**: once the cache is running the page is drawn from
  it, not from the search that started it, so `needsIndex` was lost and the line never appeared (found by
  driving the page, not by reading the code). `dmsh_article_result_cache.go` recomputes it from
  `HasArticleIndex()` exactly as `search.archiveStrictDeclined` does.

Verified on the live page, scoped to the Wikipedia: phrase «джазовый кларнетист» -> 5 words results, one
`.search-needs-index` line in `rgb(224,134,0)` (the accent), text "found all the words. The exact phrase
needs our index: Build", and the API answer carries `rung=words, needsIndex, oneReading` in 0.15 s.

## Strict readings are declined, not scanned; the caps are gone (2026-11-02)

The user's decisions, implemented: "все ограничения убираем - это зло и неправда" and "искать по статьям
долго и энергопотребление растёт на телефонах". A dictionary that answers full text from its archive's
index and has none of ours **no longer scans article texts** for a phrase or a proximity reading: the
reading is declined and the hit carries `NeedsIndex`, the row says why in one line, and it offers the thing
that answers the same question from positions - our index - through the existing build window
(`openFullTextPicker`, preselected on that dictionary). The words reading is untouched: it is what the
archive's index was built for, and its ranking is the archive's own.

Measured on the live library: Википедия (no index of ours) `phrase` → **0.04 s**, no results,
`needsIndex`, where it used to spend seconds scanning; the same dictionary `words` → 0.24 s, 20 results;
Викитека (our index) `phrase` → 0.05 s, 1 result.

`archiveStrictCheck` is 0 (unbounded) and `search`'s scanner helpers stay for the one case that can still
want them - an explicit "search the texts anyway" in that window, which is NOT built yet: today the scan
has no caller, and that is deliberate, because the default must not do silently what the reader is being
offered an alternative to. `ftsq.Parse` also stopped truncating a 128-word query in silence: `Query.TooMany` says the query below is
a prefix of what was typed, `rungFor` refuses instead of searching the first 128 words, and BOTH search
entry points refuse the request out loud with "the query has more than 128 words to search"
(`ftsq.MaxTokens` is exported for the sentence). The page shows it in the reader's language through the
legacy text-match list (`errors.tooManyWords`, ru/en) - deliberately reusing that mechanism rather than
inventing a code field for one message, since the wire already carries `SearchError.Kind` for the family
that grows next.



## The strict readings were mostly blind, and one of them lied (2026-11-02)

Reported by the user, with a live example: «ищу джазовый кларнетист, статья „Ниммонс, Фил" содержит эту
фразу, а поиск не находит». Three defects, all of them in the archive's verifier
(`internal/format/zim`), all of them silent - and only the first is why the user saw nothing.

1. **`articleTokens` carried its own HTML stripper, and it counted every `</script>` as an OPENING tag.**
   The skip it kept therefore never ended, and everything in the article after the first `<script>` or
   `<style>` block was dropped: from a 65 KB article it produced two tokens, "ниммонс" and "фил" - the
   title. So the strict readings were matching only what the head of the document holds, which is exactly
   what the evidence showed once it was read properly: the four "hits" for `кларнетист` were all article
   titles (Кэмпбелл, Джеймс (кларнетист)…), the fourteen for `джазовый` all started with «Джазовый…»,
   and no phrase of two words could ever verify. The stripper is now `store.StripHTML`, the same function
   the article index of our own databases is built with (`internal/format/dsl` already imports `store`,
   so no cycle) - two implementations of "what does this article say" was the whole bug.
2. **The text was folded with the QUERY folding.** `glass.FoldTerms` drops repeated words, because a query
   is a set; the article was folded with it too, so a word's *second* occurrence vanished - and the second
   occurrence is precisely what a phrase needs. «Ниммонс, Фил» names the profession «кларнетист» in its
   infobox and again in the first sentence, so the one the phrase needed was the one dropped.
   `glass.FoldText` is the sequence folding now, `FoldTerms` is defined in terms of it, and the strict
   readings ask with the sequence.
3. **The bound was not reported.** `StreamHeadwords` returned nil when it stopped at `maxCheck` with
   candidates unread - indistinguishable from a finished walk - so a bounded list went out as the whole
   answer. It returns `ErrCheckBound` now: the streaming path finishes without the "done" marker, which
   the article cache turns into `Complete=false` / `HasNext=true`, the same affordance the reader already
   uses to fetch more. The collecting form has nowhere to carry it, so `archiveReading` drops the error
   there (the app streams; that path is the API's).

Verified live on the ru-wikipedia through `/api/search`: phrase «джазовый кларнетист» went from **0
results to 20**, «Ниммонс, Фил» among them; near likewise; words unchanged. And the cache build event
carries `"complete":true` for a phrase whose scan ends (71 articles) while one that hits the bound (300
articles, 12 s of reading) has no `complete` field at all, i.e. false. Both text defects are pinned by
`dmsh_articles_test.go` on a body with the shape of the real article.

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

## The article index in a file of its own: reader done, writer still off (2026-11-02)

The reader is in and tested; the writer stays OFF until the sibling-awareness audit below is done, and
that is a deliberate stop, not an oversight.

**How the reader does it, and why not ATTACH.** ATTACH is a per-connection statement and this process
pools connections without a limit (`pool.go` deliberately sets no `SetMaxOpenConns`), so ATTACH would mean
either a driver connector or an ATTACH before every query. Instead `Store.articleHandle()` opens the
sibling as its own read-only handle - lazily, on the first full-text query, validated by `dict_uuid`
exactly as `media.db` is - and the two read sites ask it for matching rowids (`ORDER BY rank`) and then
fetch their rows from `entry` by primary key in batches of 400, preserving the rank order in Go. One query
when the index is inside the database (the JOIN is still there, unchanged, for every dictionary built
before the split), two when it is in the sibling: `articleMatches(ctx, match, limit, withBodies)`.
`Close` closes the sibling handle too, which a test caught on the first run: on Windows a handle left open
makes removing the index - the one operation the split exists for - fail.

Two honest behaviours fell out of writing the tests:

- A split index whose FILE is gone is no index. `Open` and `Inspect` both check, because the meta still
  says `ingest_level=text`: without the check a search would answer "no such table" and the row would offer
  a full-text chip for a file the reader deleted. With it, removing the index is exactly what
  `ClearSearchIndexes` does - delete one file, no VACUUM, no rewrite of tens of gigabytes - and the
  dictionary afterwards reads as a dictionary without full text, offering to build one.
- A dictionary that is never searched by text opens nothing extra: that is why the sibling is lazy, same
  argument as D64 for `media.db`.

**What has to happen before the writer is switched on.** `Plan.SplitArticles` is set NOWHERE today, so
every existing database keeps its shape and nothing in the library changes. Turning it on means auditing
every place that already knows about `media.db` and teaching it about `articles.db` - the list, from
`grep MediaDBName|MediaSibling`: `cli --list` (cli.go:791), the `-full` media check (cli.go:970), remove.go,
clear_database.go (two sites), dictrows.go, server.go's file listing, adopt.go, link.go, clean.go,
reconcile.go, store.go's Open, stale.go, library.go's `MediaSibling`/`WriteInfo` (the receipt now names the
article index file too) - plus the portable/export bundle, which must carry the pair or the folder is not
one dictionary any more. `clear_indexes.go` already removes the sibling and skips the VACUUM. Until that
audit is done the reader is dormant but tested: `TestSplitArticlesAnswersLikeAnUnsplittedOne` ingests the
same content split and unsplit and insists the answers are identical, and
`TestRemovingTheSplitSiblingEndsTheFullText` pins the removal behaviour.

**The older half, kept for the record:** `Plan.SplitArticles` (recorded as `articles_split=1` ONLY when
true, so every database built before this keeps exactly the meta it had - the reader golden test caught
that on the first try);

What is in place: `Plan.SplitArticles` (recorded as `articles_split=1` ONLY when true, so every database
built before this keeps exactly the meta it had - the reader golden test caught that on the first try);
the ingest writes the article index into a `<text.db>.tmp-articles` file at the same time as the rest,
inside the same transaction (SQLite commits across attached databases atomically), detaches it while the
connection is still open - Windows will not rename an attached file - and renames the pair into place
together; `ClearSearchIndexes` removes the FILE and skips the VACUUM, which is the point of the split
(no rewriting 35 GB to return 9); the sibling's size is its file size, so the estimate needs no page
accounting for it; and a test builds one and checks that the file holds a working index of the same
articles, that removing it leaves the headword side whole and full text unadvertised.

**What is missing, and why it is not a detail:** reading the article index from the sibling needs the
file ATTACHed on EVERY connection of the pool. `ATTACH` is per-connection and `sql.DB` is a pool, so the
first attempt - one `ATTACH` in `store.Open` - left a database whose queries say
`no such column: articles.article_fts`: the statement ran on a connection that had never attached it.
The fix is a `driver.Connector` wrapper that attaches as each connection opens
(`sql.OpenDB(attachingConnector{...})` in `openRO` and in the ingest), which is a change to how the store
opens databases and deserves its own pass with its own tests.

**Until then nothing may turn the split on.** No caller sets `SplitArticles` - not the server's ZIM path,
not the CLI - so the option is inert: a database that asks for it would build correctly and then be
unsearchable in full text, which is worse than not having the split at all. The order is: the connector,
then the callers (the index-only ZIM form is the one that wants it), then the receipt and the window's
wording.

## A ZIM without its own index gets ours, without the bodies (2026-11-02)

The third form a prepared ZIM database can take, and the one the user asked for: the article text is
read and INDEXED, and the bodies are not kept. Storing them buys nothing - reading an article out of the
archive and out of the database are the same (tools/zimtime: medians 0.52-0.66 ms either way) - while
they are four to five times the size of the index copied for: on the English Wiktionary 19.9 GB of
bodies and headwords against 4.6 GB of index. So an archive that has no index of its own now gets one of
ours at about the index's price, and the articles keep coming from the archive.

How it is expressed, with no new meta key: a reader that says so. `zimformat.NewIndexReader` wraps the
read-through reader (the body it yields is exactly what the index needs to see) and answers
`DropBodies() bool`; the ingest asks any reader that implements it, indexes the text anyway and writes an
EMPTY `entry.m` (the column is NOT NULL, and an empty body is what "this database does not carry it"
looks like). The form is then derivable from what is already recorded: `index_kind=headwords` (no
bodies) with `ingest_level=text` (a text index) - a combination that did not exist before, which is why
nothing else had to change to describe it.

The three places that choose the reader ask for the TEXT rather than for the old form: a rebuild must
keep the index of an index-only database, so the test is `index_kind == "articles" || ingest_level ==
"text"` (registry.go, cli.go's ingestArticles, reindex.go). An existing read-through database - which no
longer exists in the user's library - is rebuilt into the new form by the same rule, since the bodies
are what the rule says not to keep.

Verified by a store test: a reader that drops bodies produces a database that answers a full-text query,
holds no body at all, and reads back as the title form with a text index, so a later rebuild keeps the
index. NOT verified live: no archive in the library lacks its own index, so this form has never been
built from a real ZIM - the first one that arrives (or a copy with its index removed) is the test.

**Still to do, and it is the other half of the plan:** the split of the article index into its own
`articles.db`. It is worth doing for the reason the user named - instant removal instead of a VACUUM
over the whole file, and a header file that stays small - and it is additive now that old formats keep
working: new builds would write the pair, old ones would read as they do. It is filing rather than
capability, which is why the form above came first.

## Three readings, chosen by the reader, not by the program (2026-11-02)

The search menu now offers the full-text readings as three modes - **Фраза** (the words adjacent, in
order), **Рядом** (within ten other words, any order) and **Слова** (anywhere) - and NO mode walks a
ladder. The mode is the promise: a mode that quietly answered a weaker question than the one on screen
was upstream's design (D102, "the ladder resolves it without asking the user to choose"), and the user
asked for it to go. The rungs did not disappear; they became the menu items, and the code got smaller:
`runPlan`, the plan plumbing and the "phrase not found · words anywhere" announcement are gone.

What each mode is answered by:

- **our index** (FTS5, which has positions): the mode's own rung, one expression - the phrase lowered as
  one quoted phrase, the distance as NEAR(...,10), the words as each word's prefix. Pinned by a test
  that asserts the three expressions differ and are what their label says.
- **an archive** (no positions anywhere in its index): "Слова" from the index; "Фраза" and "Рядом" by
  verifying the reading in the ARTICLE TEXT of the candidates the index returns
  (internal/format/zim/dmsh_readings.go). The check is literal (folded text, so inflections do not
  match - which is what "exact" means, and what our own unstemmed index does too) and bounded: the
  candidate count is the cost of the reading, reported rather than hidden.

Verified live, on both kinds of dictionary, with "музыкальный инструмент": on Ushakov's (our index) the
readings nest as they must - phrase 51, near 62, words 63 - and on the converted Викисловарь (an archive)
the strict modes return exactly the one article that contains the phrase, «музыкальный инструмент»,
while the words mode returns thousands.

Also: marking follows the family, not one member. Both search handlers enabled highlight marks for
`mode == FullText` only, so the day a reading was added the strict modes answered with rungs set and
nothing marked; `search.Mode.IsFullText` now says which modes are one family. And the test that pinned
the old descent (hilite_wire_test.go, "reversed words fall to NEAR") was rewritten to ask each mode for
the reading it promises - the behaviour changed by decision, so the test changed with it.

**The complete list works too, from the archive** (2026-11-01, asked for after the first test). A
search on an archive-backed dictionary now finds ALL its occurrences, exactly like a dictionary with an
index of ours: the header reads "Викисловарь 1–5 (166) Browse" where it said "5+ results · Complete list
unavailable", page two continues page one, and Browse pages the same list. Three pieces, each where the
knowledge lives:

- `zim.Dict.StreamFullTextHeadwords` - the archive's answer as headwords, best first, streamed and
  without reading a body. The naming rule (the title when the file records one, else the path) stays in
  the package that knows the container, and a hit whose article the archive no longer holds is skipped.
- `store.EntryIDsForHeadwords` - headwords to prepared entry ids, batched (400 a statement: SQLite's
  host-parameter limit is 999), keeping the order it is given, first-wins so a duplicate shows once, and
  dropping what the database does not hold. A test pins all three properties.
- `upgraded.archiveArticleIDs` - drives the two, emitting pages of indexconst.BrowsePageSize with the
  running total and the final done flag, exactly as the SQLite path emits them, so the result cache does
  not know which of the two it is talking to.

Two numbers worth keeping: the archive's rank order is what both the first page and the cache use, so
paging cannot reshuffle; and the total is the archive's, minus the handwords this database does not hold
(166 against the reader's 169 for "духовой инструмент" - redirects resolved to targets the prepared
database already has, and duplicates, both dropped on purpose). A query that matches a million articles
reads all of them once (about 8 bytes a hit inside glass.Search) - a background pass, not the keystroke
path.

Also: the paged answer needed the one-reading flag as well, because a paged result is drawn from the
result cache rather than from the search that started it (dmsh_article_result_cache.go now asks the
handle), or the header would claim a phrase had been tried and not found.

**The migration of the four existing databases is a click, not a job.** They are in the read-through
form; nothing rebuilds them by itself - no reconcile runs at startup, deliberately, because a silent
25 GB rebuild is not something to start behind the user's back. Rescan → "пересоздать" for the base
index rebuilds one in the title form (the plan difference is what makes the rebuild take the new form),
which frees the article index and the bodies: 1.3-9.2 GB of index plus 8-25 GB of bodies per archive.
The window does not yet offer that conversion with a size next to it; the chip and the window only stop
offering the wrong action for a database that is already in the new form.

**A query is answered as ordinary results** (`internal/format/zim/dmsh_xapian.go`, ours, beside
upstream's `zim.go` for the usual reason). `Dict.FullText(query, limit)` searches the archive's own index
and returns `[]dict.Result` - headword and article, converted by this package's own article pipeline - so
a caller cannot tell a Kiwix hit from a headword hit. Verified on the Russian Wiktionary:
`"джаз"` comes back as джаз, jazz, джаз-фестиваль, джаз-группа, кул-джаз, which is a sensible order for
nothing but summed wdf as a ranking; `"кларнетист"` as кларнетист, кларнетистка, Klarinettist, кларнет,
слёзный. `dict.ErrNotFound` means the archive has no index at all, `dict.ErrUnsupported` (wrapping
`ErrNoStemmer`) that it has one in a language this reader cannot stem - the two answers a caller needs to
decide whether to build an index of its own.

**The product wiring is a decision, not a remaining task** (2026-11-01): a Kiwix archive's index is
2.4-2.9x smaller than one of ours and already built, so a ZIM with one needs no `article_fts` of ours at
all. What it does NOT give: headwords (the index knows paths, and `FullText` resolves them back through
the archive's directory), positions (no phrases), Xapian's real ranking, or anything for an archive built
without `--indexing` - so our own index stays the fallback either way, as the user put it. The parts that
would change if we take it up: `Caps.FTS` would have to be advertised by something that is not
`article_fts` (the searcher holds the prepared database, not the archive), the ZIM ingest would stop
building `article_fts` (and the "remove full text" button would mean something different for these), and
the estimate window's ZIM rows - "this is what a build would cost" - would become "nothing is needed".
Each of those is a user-visible behaviour and disk-usage change, which is why it is written down here
rather than done.

Smaller, still open: `skip_to`, so a rare term's postings drive the intersection instead of first decoding
a common term's 1.4 M entries (measured: reading such a list whole is tens of milliseconds, so this is a
comfort, not a blocker); ranking, since what exists is AND-intersection ordered by summed wdf; and
scale - the intersection is materialised in memory, which 1.4 M hits makes a ~20 MB slice. The oracle for
correctness used to be "build Xapian 1.4.32 locally and diff"; the archive itself turned out to be a
stronger one, since the terms, their frequencies and the article text are all independently checkable.

- With the local MinGW GCC 13.1.0, `go test -tags sqlite_fts5 -trimpath ./... -count=1`, `go vet -tags sqlite_fts5 -trimpath ./...`, and `go build -tags sqlite_fts5 -trimpath ./...` pass after removing the duplicate LSD implementation. Real-dictionary integration and Windows/Android UI checks remain unverified.
- Android runs Go through JNI without an HTTP listener; Windows listenerless desktop uses WebView2 and a native bridge. Build recipes are in `docs/WINDOWS-VERIFY.md`; Android uses `build-android.cmd debug intel`. Do not install APKs or commit unless requested.
- Windows/Android search, index maintenance and visual checks remain pending. Other open items include portable Windows write-path gaps (`docs/PORTABLE-WRITES-AUDIT.md`), Edit Folders verification, icon visibility and MDX ad-slot reindex/display.
- No upstream is configured. DSL uses one GoldenDict-compatible reader and article preparation path. The LSD decoder and reader are in this branch; `docs/DSL.md` and `tools/dslcompare/README.md` describe the DSL architecture.
Earlier Browse/Wildcards controls, diagnostics, icon handling and settings changes are recorded in commit history; their Windows/Android visual checks remain pending.

The Oxford Advanced Learner's Dictionary 10th source MDX now has `Title="Oxford Advanced Learner's
Dictionary 10th"` instead of `OALDWU`; its UTF-16 header length and Adler-32 were updated, and the reader
reopened it and looked up `dog`. The original is preserved beside it as `.mdx.title-backup.bak`; prepared
data will be stale from the source size/mtime change and needs Rebuild.

At commit `f44e7127`, linked-group synchronization cached its last completed
input snapshot and skips dictionary metadata reads while state.json, groups.ini, dictionary
entries/generations and root folders are unchanged. The cache version advances only after a successful
refresh; the diagnostic reports its version and hit/miss. Focused linked-group rescan/edit checks, `go
build ./internal/server`, and `git diff --check` pass. Android runtime timing is pending. No upstream is
configured. The application uses one DSL reader with GoldenDict- compatible repair and article
preparation; there is no Original/GD selection or duplicate DSL index. `docs/DSL.md` and
`tools/dslcompare/README.md` describe that current architecture. The LSD decoder and reader work is
present in this branch; upstream review and pending ZIM checks remain as recorded below. The user-owned
untracked `DictShelf.exe` was preserved. This session adds a saved Combined list dictionary limit
(default 500, maximum 5,000), a specific over-limit message, and moves Clear browser cache beneath its
Browser cache description. Desktop System settings no longer show internal config key names. `go build
./...`, `git diff --check`, and translation JSON parsing pass; tests were not run, and Windows/Android
settings UI verification remains pending.

Dictionary names now have a shared icon route: a supported adjacent image file wins, then a format icon
(including OpenZIM Illustration metadata). When no icon is found the UI leaves the name unadorned. Search
result headings and Dictionary settings show available icons; browser, Windows WebView and Android
WebView serve them through their local Go resource path. `go build ./...` and `git diff --check` pass; UI
runtime checks remain pending.

Dictionary icons also appear in dictionary search, export and group lists. A prepared legacy abbreviation
glossary is excluded from the library list when its discovered parent DSL is compressed (`.dsl.dz`) as
well as when it is plain. The `TestAbbrevCompanion` check and `go build ./...` pass; UI runtime
verification remains pending. Dictionary Groups renders each name in its own text span beside the icon;
the list-search highlighter targets only that span. Its marks therefore remain inline and cannot become
separate flex items with gaps between letters. The shared list search now removes old marks before
applying each current query and repaints on both input and search events (including search-field clear
actions); runtime confirmation remains pending.

The icon request is now dispatched by the Windows WebView resource adapter as well as the HTTP and
Android paths. DSL abbreviation companions accept both `_abrv` and `_abrev` spellings (plain or dictzip):
they are absorbed by the parent glossary and excluded from discovery when that parent exists.

Android log 2026-10-23 05:35–05:37 shows the picker repeatedly spending 1.9–3.3 s in its native
user-groups call, while Browse article paint takes 4.4–5.4 s: startup dependencies wait about 1.6–2.5 s,
then the 383-dictionary list takes about 2.5 s, while scoped search itself takes under 50 ms. Android now
passes the existing diagnostic flag into the Go group snapshot and logs native-bridge stages. The picker
reuses the loaded group snapshot instead of fetching it on every open. Browse headword links carry `from`
as well as `dict`; startup skips the group read for scoped article links and defers it until after the
list/search has had two animation frames to paint. Log 2026-10-23 11:21 confirms first Browse article
paint in ~0.4 s, but also shows a second search after list completion; that retry lost the one-dictionary
scope and could show all-dictionary results. Scoped searches no longer mark themselves stale just because
the full dictionary list is still loading, and any necessary retry preserves its scope. Latest log still
shows group linking at 3.4–3.6 s for 383 dictionaries; optimizing that native snapshot is separate
follow-up work. Linked-group synchronization reads each dictionary's metadata at most once per refresh,
even with several linked groups. With Browse/article diagnostics enabled, `dictionary_picker_groups`
splits metadata, matching, membership updates and total time. The 2026-10-23 21:01 Android log showed
0.41–0.86 s per refresh, almost entirely metadata reads, before the completed-snapshot cache was added.

The existing Browse/article navigation diagnostics checkbox failed to record direct Browse headword
clicks because those links carry `dict`, while the main page diagnostic gate required `from`. The gate
now accepts `dict` and uses it as the article dictionary ID; the Android log confirmed
`article_transition` stages from the click through first article paint.

Wildcards' combined mode keeps the dictionary field's geometry, displays “Combined list” / «Общий
список», and removes the dropdown affordance while disabling selection. The merge icon's tooltip
describes its next action in English and Russian. After a combined SQL-cache stream completes, a missing
visible page is read again after the stream closes, so the lookup can see the completed cache. Android
runtime verification remains pending.

Combined Wildcards top-level result rows now store their own result ordinal as the base position, and the
renderer uses that ordinal when highlighting and scrolling. This fixes match navigation when all
dictionaries are merged; the merged SQL signature changed to v4 so stale position rows rebuild. `go build
./...`, inline JavaScript syntax and `git diff --check` pass; Android runtime verification remains
pending.

Browse and Wildcards ribbon clicks reveal a partly clipped heading and, when it fits, one neighboring
heading. Clicking the first or last heading aligns to that strip edge; Browse no longer centers each
selected heading or scrolls when it is already fully visible. Shared ribbon geometry checks and inline
JavaScript syntax pass; Windows/Android interaction remains to be checked. Wildcards toolbar: the
merge-books control is fixed to 3.2em with a larger icon; page controls show “Next page” / “Previous
page” when the row has room, fall back to arrows, then hide before the page counter on narrow rows.

Browse's “Loading headwords” page indicator now waits 500 ms for both ordinary page reads and in-flight
page prefetches, avoiding a flash on quick Next transitions. Runtime verification is pending.

Browse deep headword jumps spent about 450 ms in `Locate`'s SQL count even when fewer than 100 rows were
counted. The query now gives SQLite an explicit lower bound at the closest 100-row anchor, and the anchor
ordinal includes the anchor row; focused position/query-plan checks pass. Wildcards per-dictionary lower
search retains base offsets. Combined lower search carries both source and upper-mask base positions;
navigation, highlighting and Find from here now use the latter, with a new indexed cache signature.
Runtime checks remain pending. Browse diagnostics split anchor lookup, bounded count, page read, HTTP
response, and Android bridge timings.

Browse `*home*` with trigram enabled failed because the candidate SQL joined two tables that both expose
`w`, while the shared internal-entry filter used unqualified `w`. The two joined mask-candidate queries
now qualify the filter as `e.w`; runtime verification is pending.

Selecting Wildcards resets its view to separate dictionaries; explicit URL/history/article-return
restoration keeps its saved view. View switches restore completed metadata and visited pages from bounded
memory caches (128 entries, 30-minute TTL). The 17:58 log shows repeated cached switches in 63–77 ms
without spinner; initial SQL builds remain long.

Browse/Wildcards diagnostics correlate switch and run IDs, requested query/group, FTS/trigram variant,
exact browser cache key, saved cache ID, alternative index variants, session-write result, SQL page
reads, build/resume, spinner and task-panel visibility. This distinguishes a new submit from a view
switch and a settings-key miss from a lost cache entry. Wildcards rows snapshot FTS/trigram settings for
cache identity; lower rows inherit them. Returning to completed caches reads only a missing visible page
and keeps the spinner off. The merged signature validator accepts both index flags and the selected
dictionary IDs; runtime lower `dog` searches succeed.

System Settings → Search now applies optional FTS/trigram indexes to both Wildcards SQL result databases.
In the separate-dictionaries database, upper-mask words are indexed with their dictionary and base
positions; the lower search selects FTS candidates for plain token queries, trigram candidates for masks
with a 3+ literal, and scans blocks otherwise. The index combination is part of server and browser cache
identity. Focused four-combination tests, index rebuild tests, Go build, JavaScript parse and diff checks
pass. Runtime timing on Windows/Android is pending.

At the 2026-10-21 stage, branch `dev` was at `5bdb31cf`. Browse's match-count button, “Find from here”
action and indexed candidate search in the merged Wildcards database are committed. Opt-in Browse
diagnostics now logs per-request headword search mode/source, first event/word and total timings,
found/scanned counts; merged search additionally logs cache hit/miss, selected index, candidate count,
and cache-build time. The existing Settings → System Browse/Wildcards diagnostics checkbox enables it in
both views. The 2026-10-21 System Log includes an earlier Wildcards run that scanned dictionary indexes
(up to 61 s) and a later run using the upper-mask SQLite cache. That cache run still took 16.6 s on 197k
rows because it emitted one event per match; batching now emits the first result immediately and
subsequent results in batches. Go build passes; runtime timing after this fix is pending. Plain Browse
`dog` in a 9.1M-entry dictionary took 11 ms; merged Wildcards narrowed `dog` to 6382 FTS candidates and
built 6249 results in 115 ms. Merged filter caches now resolve each match's base position through the
unfiltered mask's indexed result cache; their signature changed so old incorrect positions rebuild. This
addresses “Find from here” after jumping to the middle of the combined list. Ordinary Wildcards' lower
search for selective top masks now scans completed result blocks in the wildcard SQLite cache, not
dictionary indexes, and records the same base offsets. Streaming search and later page/position reads now
require a valid wildcard SQL cache; missing, malformed or unavailable cache data returns an error instead
of falling back to a dictionary index. Combined mode passes its prepared merged-database ID to the lower
search, validates the selected dictionary IDs from SQL metadata, and does not reopen dictionaries on that
path. The Wildcards UI cache-key version changed so an older session cannot restore a combined row
without that ID. The UI waits for upper mask completion and shows one aggregate word-count progress with
the currently scanned block's dictionary name. Masks made only of `*` and `?` normalize to a
minimum-length mask and build the same complete 300-word SQL cache; all-star queries no longer use lazy
page-only storage. Runtime UI confirmation remains. Earlier Browse/Wildcards, Android UI, and
task-progress work is included in the checkout. Browse and Wildcards SQL result caches now use indexed
300-headword blocks, retain files across idle periods/restarts, and share a five-file/age/disk-size
cleanup policy. Settings → System controls the combined cache disk cap (16 GiB on Windows and Android, 8 GiB
on other desktops, or No Limit). Old cache schemas are prepared on first open. `go build ./...`, inline
JavaScript syntax checks, catalog JSON parsing, and `git diff --check` pass; the FOSS debug APK also
builds successfully. No tests were run, and the APK was not installed. Windows/Android UI and
long-running cache behavior need runtime verification. Merged Wildcards progress now uses one scale
across building the combined SQL list and applying its mask, reserving 100% until both caches are
committed. Switching back to per-dictionary mode reads completed SQL-cache pages without showing a
full-search task; runtime verification remains pending. Combined Wildcards diagnostics record index
combination, cache hit/build, row counts and elapsed time. Each combination stays isolated. Android UI
changes keep the Browse A–Z heading and Find/Filter dock fixed while its dictionary list scrolls; the
Reading it word by word hint stays with the heading above its divider, and filter mode is selected by
default. The Wildcards merge button uses a two-state SVG: several books → one book, or one book → several
books. Its title and accessible name describe the next transition in English and Russian; it sits beside
Total and block paging in the Wildcards header. The result-count control is visibly button-shaped. The
normal per-dictionary and merged views use separate SQLite caches; after the initial merged cache is
built, toggling back and forth reuses each view. The merged list is deduplicated and globally sorted,
applies the active mask and filter, and retains source positions for paging and “Find from here”. The
selected view is kept in the search URL and article-return link. The previous/next-match buttons now
retain the merged flag and source dictionary IDs when creating filter rows; without those fields,
merged-match requests silently queried a nonexistent dictionary. Cache eviction links both views by
dictionary group and FTS/trigram setting, including merged find caches; an active member protects the
whole group. Match pages now also carry a separate ordinal within the masked merged list: the source
position is still used to highlight the exact headword, while the masked ordinal selects the correct base
page. Older merged-find SQL caches are rebuilt automatically if they lack this ordinal column. `go build
./...`, inline JavaScript parsing, catalog JSON parsing and `git diff --check` pass; the FOSS arm64 debug
APK builds successfully and was not installed. Windows/Android runtime behavior remains unverified. The
Android-only `A60` header marker is now shown only by debug builds. The wildcards results now use a
full-width unboxed surface with compact top controls. Browse and wildcard headword searches reveal their
first matching word as soon as the stream provides it, including searches started by Apply. Wildcard SQL
cache pages reassemble variable-sized stream events into fixed-size pages within each dictionary,
preserving the group's dictionary order during parallel search. The wildcard search match counter opens a
localized Go to match dialog and navigates to the selected result. In Wildcards search mode,
right-clicking or long-pressing a headword opens “Find from here”; the chosen headword receives a
rectangular outline and subsequent match navigation starts beside it. Opening an article stores the
clicked word and restores it to view on return via ← Word.

Browse's bottom match count is now a button that opens the existing customized Go to match dialog. In
Browse search mode, long-pressing on touch or right-clicking a headword opens “Find from here”; it
outlines the selected word and uses SQLite NOCASE-compatible binary bounds so Next starts after the word
and Previous before it. `go build ./...`, inline JavaScript parsing, and `git diff --check` pass;
Windows/Android interaction still needs runtime verification.

Merged Wildcards search previously scanned every row in the combined SQL table in Go and rebuilt the
query result cache. The merged database now builds FTS5 and (when supported by SQLite) trigram indexes;
plain headword queries and masks with a literal run of at least three characters use SQL index candidates
before the existing exact matcher. A full scan remains for masks without a 3+ character literal or any
indexed bottom-search term. The base cache signature version changed to rebuild older merged databases.
Go build and `git diff --check` pass; runtime speed and SQLite index behavior are not yet measured on
Windows or Android. Windows log 2026-10-21 19:35:28 exposed a slow Wildcards lower `dog` search inside
`*d*g*`: SQLite ordered the dictionary table before FTS, taking 31 s for 2,045 candidates and over a
minute for 2,771. The candidate query now forces FTS first with `CROSS JOIN`; the latter query takes ~10
ms against the saved cache, but app runtime remains unverified. The ordered SQL wildcard cache supports
seeking to matches before or after a headword position across dictionary boundaries. The live Wildcards
pane now assembles visible 300-word pages by cumulative result offset, since stream events can be shorter
than a page; treating one event as a page shifted the outlined match onto unrelated words. A completed
visible page is reread from the SQL cache. “Find from here” treats the outlined word as a cursor between
matches: if 100 matches precede it, the counter shows 100 and the first Next/Previous goes to match
101/100. Explicit Go to match clears that cursor. A single dictionary also wraps match navigation at its
ends. Position lookup now uses indexed first/last positions and the starting match ordinal of each SQL
cache block; SQLite selects one block via its B-tree and Go binary-searches that block's positions. The
unreleased cache has no schema migration; remove existing Wildcards cache files before using this change.
In-memory search positions use binary search too. Focused position, page-boundary, and SQLite query-plan
checks passed before migration removal. Android versionCode now uses 941 as a baseline at commit
`bb2a4b17` and increments for each later commit; all flavours and build types share it. The FOSS debug
APK for arm64/x86_64 built successfully with manifest versionCode 941. Preserve all other working-tree
changes. Android uses the in-process Go/JNI bridge and opens no TCP listener.

Startup progress cleanup: successful initial dictionary-list loading now clears its status immediately
and removes only its transient loading task even when indexing is also active. The independent indexing
task remains visible with its own progress. The deferred progress asset also initializes its modal-dialog
tracking before publishing its task API; otherwise an already-running startup load could throw during the
ready event and prevent progress rendering. Android debug APK rebuilt and installed alongside release on
the connected phone. The WebView reaches phase `ready` with 21/21 dictionaries, an empty status row, no
visible server-work notice and zero active UI tasks; the dialog initialization exception no longer
appears. A follow-up removes the legacy startup status counter entirely, including its unused EN/RU
catalog entries: the task manager is the only loading indicator, with a plain task label. Release app
remains installed and untouched.

Windows CI run `39398437050` passed toolchain setup but failed in `runtime/cgo` with `cgo.exe: exit
status 2`. The checked-in fix for the old local GCC path is present. A local Go 1.26.5 cgo Windows build
succeeds with Qt GCC 13.1. Both Windows workflows now use the `windows-2022` image's
`C:\mingw64\bin\gcc.exe` instead of installing a rolling MSYS2 toolchain, and print its version/target.
The runner image documents GCC 14.2.0 and binutils 2.44. CI rerun is pending.

Windows builds use `build-windows.cmd` for the listenerless WebView app, `cli` for the console CLI
(no-argument help, no TCP listener), `server` for the console HTTP server, and `installer` to package the
already-built desktop exe. Both Windows workflows build and upload all four artifacts.

Portable mode includes the active user data under `Data`; dictionary roots inside `Data/Dictionaries`
become relative paths. The latest Windows write-path audit is in
[`docs/PORTABLE-WRITES-AUDIT.md`](docs/PORTABLE-WRITES-AUDIT.md); strict confinement still needs
follow-up. Windows builds provide the standalone GUI, CLI, HTTP server and installer targets. Android
runs Go in-process through JNI without a TCP listener; its WebView receives app pages and resources
through the same runtime bridge. ## Remaining manual check Windows System Log export saves directly into
the user's Downloads folder and opens a localized, compact in-app confirmation showing the full
saved-file path, with a Show in folder action. Browser and Android export flows retain their existing
behavior. Runtime verification is pending.

On Windows, verify direct GET resource delivery: Settings/editor assets, preset switching and GD fonts,
user styles, and background images in the picker. The user has confirmed dictionary images,
multi-gigabyte video, seeking and background-image thumbnails through the latest direct resource path.
The user confirmed successful settings migration and that the next launch skips it; only the
stalled-export timeout branch remains unverified. The user confirmed the three dialogs and startup splash
on Windows. Also confirm browser and console mode still bind and serve HTTP. Remaining checks: dialog
appearance with Background Image and Sepia on/off; browser and Android paths for all three dialogs; Edit
Filters load/save/reset, close prompt, and translations; Edit Folders' layout, current paths, validation,
index defaults, picker, archive intake, shared progress, save and list refresh; Lemmatization
list/install/removal, progress polling, directory note and attribution; chunked browser archive upload;
URL and native-path import; System/English/Русский selection and persistence on Windows and Android;
System locale behavior on EN/RU devices; orphan actions/removal; About; Browse
streams/search/filter/Stop/chooser; Wildcards filtering/page turns/prefetch; article CSS; explicit index
rescan and reindex progress/Stop. Browser file input/drop uploads retain HTTP; verify browser upload
remains functional and test Android WebView and Java archive imports. Then verify portable startup with
prepared dictionaries and confirm indexes are reused after copying `Data`. Check the shared dictionary
picker on Android.

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
