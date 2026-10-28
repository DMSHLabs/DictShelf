# Agent handoff — current state

Indexing modes in `dev` are now Parallel dictionaries and Adaptive (the former
`auto2` schedule). Saved `auto`, `auto2`, and `pipeline` selections load as
Adaptive; existing Parallel selections remain Parallel. A job involving one
dictionary selects sequential preparation for small sources or the shared
reader/writer/worker pipeline for large sources, regardless of the saved mode.
The shared pool now permits this per-job pipeline override while Parallel is
selected. Focused indexing tests and `go build ./...` pass. The full server
package test has an unrelated reproducible Windows failure in
`TestRemoveTakesTheEmptiedFolderWithIt`; device timing remains unverified.

Canonical checkout: `D:\Projects\Android\DictShelf\`, branch `dev` at `da540c7`. Release workflow dispatch offers `release`, `pre-release`, and `beta` for Android, Windows, and Linux/macOS native builds. New tags use `dictshelf-vMAJOR.MINOR.PATCH`; previews add `-pre-release.N` or `-beta.N` and are GitHub pre-releases. The Update panel stores the selected channel and an enabled-by-default notification preference in `state.json`; it checks the static `update-catalog.json` on the dedicated `update-catalog` branch daily while the page runs and marks ☰ when it finds an update. The version links to the release page. Windows desktop offers an Update button: installed copies launch the matching Inno Setup update; portable copies replace only the running EXE through a waiting helper, then restart. Publishing workflows regenerate the catalog from non-draft GitHub Releases; client checks do not call the GitHub API or change version history. Windows and native workflows load the tag resolver from the workflow revision while using the selected source checkout for tag history. The FOSS Android updater downloads an ABI-matched APK into Downloads, checks its catalogue SHA-256, and opens Android's installer when allowed; the DownloadManager notification provides the manual route. Play omits install permission and the update button. Windows and Android updater behavior remains unverified at runtime or in Actions. Older notes below are historical snapshots.

Current checkout is `dev` at `da540c7`. The two previously reported server
test failures are fixed: `TestDictsRunningJobIsNotKept` now waits for the
first stable dictionary-list validator instead of assuming a fixed number of
requests is enough, and the EN/RU full-text mode hint catalogs no longer have
empty values. Both targeted tests passed five repetitions, and
`go test ./internal/server -count=1` passes on Windows; search keeps five articles in the first populated section, later sections use Settings → Results, and later pages in that first section start after its five-result first page.

Current checkout: branch `dev` at `da540c7`. Settings checkboxes use one shared customizable control, and the bottom status bar is always visible; preserve untracked `help/changelog.txt`. pipeline from `8b3ee97e` merged. Per active dictionary, it runs a source-reader goroutine and a coordinator/writer; CPU preparation uses a shared pool sized from logical processors and concurrent indexing sessions, with 16 outstanding batches per dictionary. `INDEX_WORKERS` is now obsolete and ignored; per-dictionary and global worker limits derive from logical processors. `Contains` creates and inserts both `entry_fts` (headword token FTS) and `entry_trigram`; `article_fts` is independent FullText and remains absent when disabled. Verified in generated DB schemas. Full `go test -tags sqlite_fts5 ./... -count=1` passes. Old `dev` (`c79732e4`) does one sequential read/prepare/write pass per dictionary, with only cross-dictionary parallelism. With `-headwords -contains`, single-run dev/DictShelf2 times: Oxford DSL 9.4/7.3 s (71,293 rows); Urban DSL 107.3/78.3 s (1,418,551); Longman MDX 10.2/8.0 s (69,172); Russian Wiktionary ZIM first run 39.4/47.7 s. Three ZIM runs gave medians 38.8/47.4 s (dev/DictShelf2); new pipeline is slower on this title-only ZIM case. Entry row counts match; DB byte sizes differ slightly due to layout. LSD Longman DOCE 5 on DictShelf2: 54,819 entries in 3.9 s; dev has no LSD ingest Reader. Output DBs are under `D:\Dictionaries\wuDict\Test\index-contains-{dev,dictshelf2}-2026-10-25`.

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
- Android lookups launched through recognized reader integrations default to `wordLemma` (`·ab·+`); an explicit `mode` parameter in a `wudict://` URI takes precedence. `build-android.cmd debug intel` passes; device handoff is unverified.
- Example folding now recognizes the requested diamond, square, triangle, circle and star bullets (including `⊙`, `⊚`, `⦿`, and `⁕`). It keeps an authored leading marker and suppresses the shared marker on that line. Visual verification remains pending.
- The Examples settings now include Dim background (on by default) and Fade text (off by default) after Also hide examples without [*]. Both preferences are stored in `state.json` and update existing article styles live; visual verification remains pending.
- The Dim background and Fade text switches share a measured label column so their On/Off controls align; the measurement follows the visible text size and updates when the panel is laid out.
- Search-return snapshots and Browse find/filter results are bounded SQLite caches shared by the native and HTTP paths. Browse links retain the originating query and dictionary through article navigation.

## Verification and remaining checks

- With the local MinGW GCC 13.1.0, `go test -tags sqlite_fts5 -trimpath ./... -count=1`, `go vet -tags sqlite_fts5 -trimpath ./...`, and `go build -tags sqlite_fts5 -trimpath ./...` pass after removing the duplicate LSD implementation. Real-dictionary integration and Windows/Android UI checks remain unverified.
- Android runs Go through JNI without an HTTP listener; Windows listenerless desktop uses WebView2 and a native bridge. Build recipes are in `docs/WINDOWS-VERIFY.md`; Android uses `build-android.cmd debug intel`. Do not install APKs or commit unless requested.
- Windows/Android search, index maintenance and visual checks remain pending. Other open items include portable Windows write-path gaps (`docs/PORTABLE-WRITES-AUDIT.md`), Edit Folders verification, icon visibility and MDX ad-slot reindex/display.
- No upstream is configured. DSL uses one GoldenDict-compatible reader and article preparation path. The LSD decoder and reader are in this branch; `docs/DSL.md` and `tools/dslcompare/README.md` describe the DSL architecture.
Filter mode now explicitly requests the visible dictionary's first result page when its SQL metadata is
ready but no page is loaded; this avoids leaving the headword pane on its loading message after a ribbon
selection was skipped.

The shared Browse/Wildcards bottom search now shows recent queries on focus. Wildcards mode has a
separate list; all other match modes share the regular list. The lists use the main search's persisted
history and length setting, and Clear history clears the active list. `node --check` on both history
scripts and `git diff --check` pass; UI verification remains pending.

The shared Browse/Wildcards match-mode menu now expands to all available vertical space, scrolling only
when the complete menu cannot fit. Prefix, suffix and contains descriptions now state that the single
query is compared with each word in a headword. Whole word and Exact now also explain what part of the
headword they match. Section headings use `--ui-font-section`, and options use `--ui-font-addition`. JSON
parsing, JavaScript syntax and `git diff --check` pass; visual verification remains pending.

Wildcards dictionary searches now use the folded exact-key index for masks without `*` or `?`. Masks
containing wildcards use the optional trigram index when the longest safe literal run is at least three
characters; for example, `asd*rtyu?rgfhfb*` uses `rgfhfb`. The full mask still validates every candidate.
Both the plan and final diagnostic record the selected path and trigram literal. `go build ./...` and
`git diff --check` pass; runtime timing is pending.

The shared headword-mode menu now moves to the document layer while open, so scrolling containers around
the Browse and Wildcards controls cannot cover or clip it. Its fixed position follows the button on
resize and scroll. The menu is width-limited and aligns each mode icon and its wrapping description in
two columns. JS syntax and `git diff --check` pass; Windows/Android visual verification is pending.

Browse now marks and scrolls to the current `Whole word + lemmatization` match even when the matched
headword is the lemma and does not contain the typed form. Inline script syntax and runtime verification
remain pending.

Exact and Exact + lemmatization highlighting now checks the full folded headword against the typed query,
instead of highlighting every headword with letters. A selected lemma match remains marked as the current
match.

Wildcards now shows the number of dictionaries with matches beside the result total in separate mode.
Switching a completed separate result to Combined list builds its sorted, deduplicated SQLite list from
the saved result blocks instead of scanning all dictionary indexes. The existing combined list path is
retained for a search started directly in that mode. `go build ./internal/server` passes; that direct
path has no per-dictionary match count until a separate result is available. Windows/Android runtime
checks remain pending.

Browse and the Wildcards lower search share an eight-mode headword picker: prefix, suffix, contains,
Whole word, Whole word + lemmatization, Exact, Exact + lemmatization, and wildcards. Whole word +
lemmatization includes the typed form and its lemma; Exact + lemmatization matches both the typed
headword and its lemma as exact headwords. Prefix, suffix, contains and both Whole word modes accept one
word; paste keeps the first whitespace-delimited word without changing modes. The menu explains lemma
search. Cache identities were advanced to avoid reusing results from the previous lemma fallback
behavior. `go build ./...`, JS syntax, translation JSON parsing and `git diff --check` pass; UI
verification on Windows and Android is pending.

Wildcards lower-search diagnostics now include the typed term, expanded lemma alternatives, match mode,
selected index query, candidate count and whether the SQL result used indexed candidates or scanned
cached blocks. Combined-list filter diagnostics log the same query and candidate plan. Build verification
is passing with `go build ./...` and `git diff --check`.

Fresh Browse diagnostics showed Exact and Exact + lemmatization taking about 1.1 s, while prefix, suffix,
contains and whole-word modes took 1–5 ms. Exact queries compared `wf` without the `NOCASE` collation
required by `idx_entry_wf`; the ordinary and streaming queries now specify it. Build verification is
passing with `go build ./...` and `git diff --check`; rerunning the timing on Windows is pending.

The first separate-to-combined switch exposed SQLite locks and a 98-second wait before its first page.
The generated merged database uses WAL, but page reads, cache-group marking and cache pruning could
reopen it with the default DELETE journal mode. Merged-base readers now consistently use WAL, including
the cache-pruning metadata read; the merged-find cache remains DELETE. `go build ./internal/server` and
`git diff --check` pass; Windows/Android runtime recheck is pending.

Separate/Combined Wildcards switches now emit correlated client timings for cache reuse, restore, stream
headers/first event and first SQL page. Go diagnostics split merged cache lock wait, source validation,
block copy, sorting/indexing, base page read and merged-find page/cache preparation. The opt-in Browse
and article diagnostics setting enables these records; runtime capture is pending.

System Settings no longer duplicates the Save System Log action; saving remains available from the System
menu. The disabled cache-limit field expands to show the complete localized unlimited value while
respecting narrow dialogs.

The 2026-10-24 02:06 log traced a 109-second Combined-list 99% pause to the first-page request rebuilding
a full merged Browse index (14.6M source rows) after the 59-word Wildcards merge had already completed in
67 ms. That page request now validates and reuses the `mergedcache` produced by the Wildcards stream. `go
build ./internal/server` and `git diff --check` pass; runtime confirmation is pending.

At commit `f44e7127`, the 2026-10-23 Windows log confirmed Dictionary settings
blocked WebView2's UI thread while serving the English Wiktionary ZIM icon: its `/dict-icon/fb1c556f5740`
request took about 105 seconds, then the queued article resources resumed. Dictionary icon discovery now
starts in the background, returns HTTP 202 while pending, and the UI retries without blocking. Found and
absent results are atomically persisted in the prepared folder's `icon.cache`, keyed to its text database
UUID and the neighboring image's size/modification time. Decodable raster icons larger than 128 px are
reduced before saving. Rescan and successful reindex queue background refreshes; lookup is skipped unless
a usable prepared index exists. The Windows desktop System Settings window also includes the Search and
article diagnostics checkbox; its row existed in the main dialog but was omitted from the desktop
dialog's manually assembled list. `go build ./internal/server` and `git diff --check` pass;
Windows/Android runtime checks remain pending.

Browse A–Z now shows dictionary icons in its chooser and active dictionary header, and Wildcards shows
the selected dictionary icon in its dropdown. These surfaces use the async icon route and honor the saved
icon visibility setting; Windows/Android visual confirmation is pending.

System Log now has an independent Search and article diagnostics checkbox. When enabled, it records the
selected query/mode/scope, ordered dictionary slots, each dictionary's response and result count, the
opened result, first article paint, and stream completion/cancellation/errors. English and Russian labels
are added; build and Windows/Android runtime verification remain pending.

Edit Folders in the embedded main-window dialog now uses the main window cached folder configuration
immediately, reveals the import section as soon as the configured destination is known, and no longer
writes index defaults on open. Folder validation no longer appears as a global indexing task. `node
--check internal/server/web/setup.js` and `git diff --check` pass; Windows runtime confirmation is
pending. Settings → Appearance has a saved On/Off control for dictionary icons. It defaults on; the
shared icon style hides current and future icons without rebuilding result lists. Go build and Windows UI
verification remain pending. MDX prepared indexes now remove empty elements with known advertising-slot
IDs (including numbered `ad_contentslot_N` slots); nonempty and media-bearing slots remain. MDX Reader
version is 2, so old prepared indexes are offered for Rebuild. `go build ./internal/format/mdx` and `git
diff --check` pass; end-to-end Windows reindex/display verification is pending.

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

New-profile defaults: Warm (Sepia) appearance with day and night background colours enabled; Windows and Android use Parallel dictionaries indexing and a 16 GB result-cache limit. Search as you type defaults to Below 50k on Windows and Below 10k on Android; background match search is off by default and saved as an explicit opt-in in `state.json` (the legacy opt-out key remains readable). Windows show/hide hotkey `Ctrl+Shift+F11, Ctrl+Shift+F11`, and only the Dictionaries and Results drawer sections are expanded. Android's System Settings hides Advanced and the divider above it. `build-android.cmd debug intel` passes; emulator verification is pending. Existing explicit hotkeys, appearance layers, indexing modes and drawer section choices remain in effect.

When `looks.json` has no recorded current look yet, the looks dry-check
recognizes the default Warm state as the current built-in, including its native
day and night background colours on Android. This keeps the Appearance row from showing
Custom on a clean profile and avoids a false unsaved-changes prompt when
switching from Warm. The UI uses that dry-check result to render the selected
built-in without writing a selection to disk. All focused look tests pass.

Android System Settings hides the Advanced section and its Reset default
button. Windows uses a separate System Settings dialog and does not show this
section or button. Runtime Android UI verification is pending.
