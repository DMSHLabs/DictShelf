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

Separate bug found on the way: `internal/server/dmsh_search_service.go` (v2 — bridge and desktop) opens
unprepared dictionaries, while the legacy `handleSearch` declines them first (`validPrepared` →
"dictionary index is not ready"). Port it, mode-aware: Contains/FTS decline, Exact/Prefix keep the
preview path.

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
layout by eye in the source. Also unverified on a real archive: that removing full text really drops a
ZIM's bodies (by the code the reader is chosen by the request, so it rebuilds title-only and frees
them - the page's promise of freed space rests on that) and that the CSS block scoped inside the
dialog is the right place for those rules rather than app.css.

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

**Our reader works** (`internal/format/zim/glass`, pure Go, no cgo): it parses the version file of a real
archive (format 1134, revision 1, block 8192, 1 528 188 documents, postlist root 571 with 4 617 462 terms,
docdata root 59 228 with exactly one record per document, and the termlist/position/spelling/synonym
tables empty - which matches libzim indexing without positions) and **finds terms in the postlist table**:
`"джаз"` comes back with a 517-byte first chunk and `"кларнетист"` with 47 bytes, where gozim failed with
`root block 571 not found`. Two things that finding settled: terms are the plain lowercase words - the
`Z`-prefixed stemmed forms are *not* in this index, so a query needs unaccenting and case folding but no
snowball stemmer (per archive: a build with a stemmer configured may differ, so try the plain term first
and only then a stemmed one); and a branch item keeps its key length two bytes further along than a leaf
item, which is a descent bug that fails silently rather than loudly.

**What is still to implement** (in `internal/format/zim/glass`, our own package): the version parser and
this block/table layer first (that is the fix gozim never had), then the postlist chunk decoding
(`glass_postlist.cc`: termfreq, collfreq and the docid deltas; no positions exist, libzim indexed with
`index_text_without_positions`), then the docdata table for docid -> article path, then the query side:
unaccent (ICU-equivalent - libzim's own step, see `src/tools.cpp`), tokenize, drop stopwords, look the
terms up, and intersect the posting lists (`OP_AND`). The corpus to check
against is the user's four archives; the oracle to check *correctness* against is Xapian 1.4.32 built
locally, never shipped.

**Where the reader stands and where it stops.** Working and verified against the Russian Wiktionary: the
version file, the single-file block space, the B-tree descent, item keys and the docdata table -
`internal/format/zim/glass`, exercised by `tools/xapian-glass-check`, which also fetches the article the
index names and checks the term is in it. Not working yet: the *body* of a posting chunk. A term's tag
decodes its first three values plausibly for several terms (documents, occurrences, first document id),
and `"кларнетист"` is 47 bytes for 11 documents and 32 occurrences, but no arrangement of the remaining
bytes that I tried consumes the tag exactly while yielding 11 documents summing to 32 occurrences: the
layout derived from `make_start_of_first_chunk` + `make_start_of_chunk` (entries, collection frequency,
first document, a flag byte, the span to the chunk's last document, then a frequency and gap/frequency
pairs) lands one document and one occurrence short, and a scan over header lengths 3..16 with both value
orders found no exact fit. The item framing itself is sound: 70 bytes = 2 size + 1 key length + 20 key +
47 tag, flags `0x60` (first and last component, uncompressed), key = the raw UTF-8 term, which also says
the postlist key carries no prefix and no length.

The next step is not more reading: build Xapian 1.4.32 locally, write a short C++ program that opens the
extracted blob and dumps one term's posting list (`xapian-check`, or through its API), and compare byte
for byte with what our decoder sees. That is the oracle we planned to build anyway, and it turns the
remaining question into a diff. Two smaller things for the same pass: the docdata keys use the
sort-preserving encoding (implemented, wants a test against a known document id and path), and the
container needs the byte-range API so a reader works from a mapped archive instead of the copied blob the
tool uses today.

**The glass reader works, in pure Go, on a real archive.** A term now resolves to its complete posting
list, the document ids map to article paths, and the articles really contain the term:
`"кларнетист"` gives 11 documents and 32 occurrences (matching the index's own header), the first three
being `C/кларнетистка`, `C/кларнетист` and `C/Klarinettist`, each verified by opening the article and
finding the word in it; `"джаз"` gives 162 documents and 715 occurrences, all 162 decoded. Two bugs stood
in the way, and both were invisible in the code and only visible in the bytes:

- **An item's size field covers the key AND the tag**, not the whole item (Xapian's diagram is
  `I K key X tag` with I the length of the key-tag pair), so the tag ends three bytes past the size:
  the 2-byte size field and the key length byte are in front of it. Reading the size as the whole item
  cut every tag three bytes short, which looked like article paths truncated inside a character and a
  posting list ending one document before its header said.
- **Xapian 1.4.32's own decoder for the sort-preserving key encoding adds one to the count of bytes in
  the key**, which would make the keys of a real archive one byte longer than the bytes in the file. The
  archive is the authority here: our encoder reproduced `83 d1 0a` for document 250 122 by itself, which
  the file holds, so the decoder follows the data. Both facts are pinned by tests that carry the real
  bytes (`internal/format/zim/glass/item_test.go`).

What that buys, and what is still to do. An archive that ships `X/fulltext/xapian` can now be searched
from our own code with no C++ toolchain at all and no index build of our own - and that index is 2.4-2.9x
smaller than the one we would build. Still to do, in order: the query side (unaccent and case folding as
libzim does, intersect the terms with AND, map the paths back to our entries), the container's byte-range
API so a reader works from a mapped archive instead of a copied blob, and then the product decision -
using the archive's index where it exists and building ours only where it does not.

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
