# Agent handoff — current state

2026-10-06 blank Android reader after index rebuild: the supplied System Log shows all rebuilds finishing successfully and the server becoming ready after restart. Live emulator logcat reports `Cannot access 'PRESETS' before initialization` followed by `scopeOnce` at page startup; screenshot is a blank reader with the retained query. Moved both declarations before early UI calls in `index.html`. A startup audit found the same error for `PRESET_ROWS` under `?style=off`; moved it too. Isolated Chrome boots now show no page or console errors for normal, restored-query, dark-theme, and style-off modes; setup, lemmas, browse and groups pages also load without errors. Targeted server tests and diff check pass. No APK rebuild or installed-device verification; existing unrelated Settings/localization edits preserved.

2026-10-05 launcher Settings: Save System Log button precedes Restore defaults in SettingsActivity, reusing SystemLogExport and its activity-result handler (native diagnostics available if server is down). Buttons disable Android's automatic all-caps transformation and retain resource casing. Screenshot follow-up: Restore defaults now shares full-width layout with Save/Close; action gaps are 13dp and all buttons have a 55dp minimum height, growing for wrapped labels. EN/RU label added; FOSS/Play Java compilation passes. No APK/device check; existing unrelated edits preserved.

2026-10-05 optional [ex] folding: current dev had unrelated UI edits, preserved. Appearance checkbox below Examples saves hideUnmarkedExamples in server UI prefs; default off, effective only with Hide. Whole-example paragraphs get independent wu-exonly marks including outside media; translations/definition links keep paragraphs visible. Prepared marker v3, DSL reader v11/legacy GD v10; v1/v2 browser fallback works without reindex. Article find reveals either folding role. DSL/server suites, preference restart/localization checks, Node i18n and Chromium presentation/folding checks pass; no APK/device check or commit.

2026-10-05 upstream DSL merge: dev started clean at f72e6f3; merged master bcb6828 at the user's request. Shared parser sources already match master; preserved ReaderVersion 10, legacy GD version 9, enhanced application reader and golden hashes. Took upstream secondary-line-break/fuzz/linear tests and parser documentation; preserved all application UI, article colours, preferences, frame bridge, filters and server jobs. No upstream per-section full/brief button, Full preference or Ctrl+* binding: Examples on the status bar/Settings remains the global [*] toggle and [ex] stays presentation-only. Ported only summary .mnav[hidden] CSS fix. Upstream website/howto documentation remains upstream-facing; docs/DSL.md records the fork exception, and built-in howto stays unwired. Full uncached Go suite, build/vet, Node i18n/ingest/groups/defaults/own-parser checks and Chromium example styling/folding/progress-modal checks pass. No APK/device check, push or publication.

2026-10-05 dev2 DSL folding: [ex] keeps example styling but no longer marks content for hiding; [*]/wu-sec is the folding role. Restored master's absorbBreaks range handling. Prepared marker v2 separates presentation and folding; browser fallback repairs v1/legacy marks. Article find includes secondary zones. DSL reader v10, legacy GD reader v9. DSL/server suites, JS syntax, diff check and Chromium light/dark/paper styling/folding checks pass. Android verification outstanding.

2026-10-05 Rescan empty result: System Log's successful index jobs and the UI error identify nil `jobStatus.Failed` becoming JSON null after status copying; the dialog then read `report.failed.length`. Preserve nonnil empty failure slices across job copies and make the client treat missing failures as empty. No new checks run per instruction.

2026-10-05 Sepia article images: added local article-colour backdrops and multiply blending, including Oxford enlargeable images, to the day-only Sepia article layer. Chromium pixel checks confirm white pixels become the Sepia article colour in shadow and iframe articles and hidden full-size images stay hidden; targeted preset/assets/appearance tests pass. No APK/device verification. Existing Android cache edits preserved; no commit.

2026-10-05 Android WebView cache: MainActivity and floating LookupActivity now clear the app-wide WebView resource cache once when the APK version code differs from the saved code, before creating their page WebView. First launch with no saved code also clears once; normal reader lookups do not. FOSS debug Java compilation and `git diff --check` pass; no APK/device test.

2026-10-05 progress panel visual repair: the Android screenshot showed the new spinner/text but no border or stripes. The full-width opaque `::before` backing was painting over its parent panel; it is now a separate sibling below the panel. All CSS is assembled before appending its `<style>`. Targeted Playwright light/dark palette and mobile/desktop modal/Stop checks pass; Android rebuild/device screenshot still needed.

2026-10-05 Rescan window: removed its legacy duplicate `workNotice`/Stop path; the persistent shared footer is the only progress and Stop control. Closing/backing out of the Rescan dialog no longer blocks or cancels the server-owned rescan. The supplied 15:49 log confirms Stop requests were accepted and the active dictionary finished cleanly before the queue stopped; no Android retest.

2026-10-05 shared progress visibility: footer now has a theme-token colored spinner and more visible accent stripes; text and Stop button use active theme/paper colors, and reduced-motion disables spinner animation. Android theme/device visual check outstanding.

2026-10-05 index progress counts: shared footer formats known totals as “processed of total” using EN/RU number formatting and translation keys; unknown totals retain the processed-only count.

2026-10-05 shared progress appearance: footer now uses a visibly framed rounded panel with theme-aware paper, accent and shadow. While server work is active, in-window bulk/reindex/rescan progress and Stop widgets are hidden to avoid a second copy; the shared footer is the sole live progress/Stop control.

2026-10-05 progress backing: the shared bottom notice has a full-viewport-width opaque backing using the current paper/background colour and wallpaper, covering article text beside its rounded card and in the reserved bottom gap. Browser progress/modal/layout regression passes; Android visual check outstanding.

2026-10-05 duplicate progress: removed index/rescan/reindex live-progress forwarding to the main article status row; retain window-local progress and the shared footer. Bulk/single-delete queue submission now displays the footer synchronously before awaiting POST, retaining it through an initial no-job poll. Browser regression checks delayed-POST visibility. The 15:22 export confirms historical Android logs survive restarts. No APK/device retest.

2026-10-05 log retention: Go and Android keep five bounded log files (current plus four size rotations), previously two. Startup still appends rather than truncates. System Log export includes all five parts of each log in chronological order. Go logging tests cover rollover beyond five files and reinitialization without losing history. Limits: 10 MiB Go + 5 MiB Android before export; uninstall/clear app storage is not protected. Device export/restart retest outstanding.

2026-10-05 emulator progress follow-up: long stopping text squeezed the footer's progress column because the flex row did not wrap; footer now wraps its button and bounds text/button widths. New pages restore the last running snapshot from sessionStorage while awaiting a fresh server status; status/Stop requests have a 10s response timeout so a pending request cannot halt polling forever. Safe-stop requests now log targeted job IDs. Targeted server and mobile/desktop modal/layout tests pass. The reported post-swipe/Stop server hang is not diagnosed: need the user's System Log to distinguish finishing the large Webster index from a stalled server. No APK/device reproduction.

2026-10-05 operation admission: shared server guard reserves pending requests before job registration, allowing concurrent individual index changes only for different dictionaries. Bulk/rescan/folder saves/reindex block all mutations; any active index work blocks dictionary removal, imports and folder/default saves. Existing ingest watchers remain read-only; conflicting change requests return 409. Shared UI polling greys/inerts matching controls, including newly rendered cards, without altering their preexisting disabled state. Single prepared-index removal has a per-dictionary job rather than the bulk job key. Shared Stop requests safe stopping of every active indexing job; the footer counts concurrent individual jobs. Server suite/build, admission/race-window tests and desktop/mobile control/modal/i18n/ingest browser/Node checks pass. No APK/device retest or commit.

2026-10-05 all indexing entry points: Edit Folders saving now runs as `folder-setup`, independent of its request; the setup page starts it asynchronously and follows its retained result. Read-only path validation remains page-local and cancelable. Both legacy GET Rescan and non-stream POST Rescan now share the same server-owned `rescan-indexes` job as the streaming UI. Single prepared-index deletion in Dictionary settings also uses the server queue. All report through the shared footer; safe Stop finishes folder application atomically. Regression tests simulate request cancellation for folder saves and closed Rescan streams. Server suite/build and Node ingest/i18n/desktop-mobile modal checks pass; Android retest outstanding, no APK built/installed.

2026-10-05 progress/navigation repair: the 11:35 emulator log stopped after four dictionaries because navigating to Lemmatization/Edit Folders destroyed the client-owned bulk queue. Bulk operations now run as a server-owned job (`/api/index-work`), surviving page departures; Stop completes the current operation and skips the rest. Every UI page loads the shared bottom progress/Stop script, polling bulk/rescan/reindex/ingest jobs. Notices move into the latest modal dialog's top layer so dialogs cannot obscure them or disable Stop. Search stays at the top. Dictionary settings follows live progress and clears Stop on completion. Go server suite/build, Node ingest/i18n checks, queue-disconnect/safe-stop tests and Chromium desktop/mobile nested-modal checks pass. No APK built/installed or emulator retest.

2026-10-05 bulk Recreate follow-up: emulator log at 11:03 shows ten successful forced base rebuilds followed by Collins optional Contains removal. `runIngest` retained the chip's busy flag after completion, so the immediately following optional creation was suppressed and the bulk queue reported generic failed. Terminal done/stopped/error now release the flag. Node regression uses the same chip for remove/create and retries, covering transient reconnect and stop; Go build and diff-check pass. No APK built/installed; emulator retest needed.

2026-10-05 lemma page wrapping: `dev` HEAD `ff09907`; uncommitted CSS fix lets long catalogue URLs, installation paths, language names and download errors wrap within the card. Targeted lemma/assets/appearance/page-preset tests and Chromium layout checks at 320/390/502/1100px pass. No APK build/install or phone check. Existing untracked Charis-Regular.ttf preserved.

2026-10-05 long-work progress/Stop: index creation/removal, folder index rescan and outdated-index rebuild expose current action, dictionary, feature, article counts and per-dictionary percent; Stop is cooperative and completes the active dictionary atomically before skipping queued work and cleanup. A sticky in-page progress banner under the search toolbar keeps status/Stop visible after Dictionary settings closes. Server-owned rescan status can reattach after page reload; existing import cancellation remains intact. Android task removal now defers stopping its Go server child until the active foreground work hold ends; explicit Exit already waits for work. Go build, Java compile and diff-check pass; browser/device interaction not verified, no APK/install.

2026-10-05 progress banner consistency: the same sticky banner now covers startup dictionary listing, Edit Folders validation/save, and the existing Rescan Folders/index flow. Startup Stop aborts the stream and server-side list fan-out (finishing an active dictionary, skipping queued ones) and leaves a retry screen; the partial list is not presented as complete. Edit Folders Stop aborts read-only folder checks, while applying changed folders is shown without Stop because the registry update is atomic from the UI's perspective. Folder validation reports completed folders / total. Go build, targeted setup/assets tests, i18n checks and inline-script syntax pass; browser/device interaction not verified.

2026-10-05 emulator follow-up: bulk action counts were mislabeled as dictionaries even though the counter counts per-index operations; translated messages now say operations. A transient EventSource disconnect no longer immediately fails indexing; the stream reconnects to the server-owned job, and Stop now asks the active dictionary job to stop cooperatively before skipping later operations. Ingest completion diagnostics record error/canceled/stop state. Native System Log export announces saving, records picker/export diagnostics, and verifies written byte count before success. Go build, targeted server/index-log tests, Node i18n/inline-JS checks and FOSS Java compile pass. The later 0-byte report was traced to missing Bearer auth; its fix and required emulator retest follow below.

2026-10-05 System Log export diagnosis: the supplied emulator log reported HTTP 401 for `/api/system-log`; native `SystemLogExport.serverSnapshot` omitted the shell's Bearer token. It now primes and attaches `ShellPrefs` authorization. Added auth coverage for the protected system-log endpoint. Targeted server auth/index-log tests, Go build and FOSS Java compilation pass; rebuilt APK/emulator retest still needed.

2026-10-05 follow-up review: dev HEAD df13f65 (single-parser changes committed by user). Full Go suite, vet, Node i18n/defaults/groups/own-parser/ingest checks and isolated Chromium full-page EN/RU boot pass; startup requests /api/dicts, never /api/rescan. Targeted server migration/rescan/removal/preferences race tests pass with Qt GCC, CGO_ENABLED=1 and sqlite_fts5. Fixed migration trimming a genuine source name ending in GD; only legacy comparison names lose their generated suffix, covered by a regression test. That small fix is uncommitted. The v0.8.0 startup callback defect remains fixed; no new loading hang reproduced. Older-APK installation failure still requires exact versions and Android error. No APK build/install or device validation.

2026-10-05 project-wide single DSL runtime: native .dsl/.dsl.dz registrations and Open now use NewArticleReader (reader v9); upstream NewReader and GD oracle remain reference/merge code. Comparison generation moved to test fixtures; registry no longer generates or selects variants. Search, groups, Browse, bulk actions, rescan and defaults share one entry/index; old selection routes and OpenAPI fields removed. Legacy receipts remain read-compatible, including cached-only dictionaries. Migration merges memberships/order, preserves effective Off/removal decisions and optional plans, rebuilds native copies from existing enhanced receipts, and records completed migrations per source so rescans cannot resurrect deleted indexes. Explicit database sources are untouched. Old upstream prepared HTML remains readable but reports outdated until explicit recreation. Fixed synchronous closing of removed registry entries on Windows and released removal locks before rescan; failed preference saves preserve indexes. Full Go suite/build/vet and Node i18n/defaults/groups/single-parser tests pass. Race check unavailable with CGO disabled. No APK/device check, corpus-wide reindex, commit or push.

2026-10-05 Dictionary settings single parser: removed DSL selector and variant labels; enhanced variant alone appears, one index/contains/FTS row and Browse, red missing-index warning regardless of legacy Original index. Generated GD name suffix hidden. Main-page load synchronizes persisted global selection to gd once (retry on failure); group filtering and bulk operations exclude Original, while hidden memberships/index files remain intact. Deletion confirmation no longer mentions another variant. Full Go suite/build, Node i18n/group/own-parser tests pass. Source descriptors/API still carry legacy GD identity for subsequent migration; no APK/device check or commit.

2026-10-05 Folders defaults: replaced two DSL rows with three format-independent checkboxes; base index permanently checked/disabled. New indexDefaults preference and PUT /api/index-defaults, exposed in config/OpenAPI; legacy choices seed optional flags. Saving affects future dictionaries only; new DSL uses enhanced variant, non-DSL joins persisted auto-preparation plans. Old DSL API/settings retained for subsequent migration. Full Go suite/build and tools/index-defaults-test.cjs pass. General tools/i18n-test.cjs repaired: loads real FEAT_NAME into both render sandboxes, updates enableMode parameter and stale RU expectations; EN/RU checks pass. No APK/device check or commit.

2026-10-05 own DSL parser, step 1: article.go is the production enhanced-variant pipeline: upstream range repair + NFC, retained ~ and nested heading variants, optional Enhancer cleanup/presentation and prepared examples. GD reference tree remains only for comparison tests; UI/source identities unchanged until step 2. Example detection no longer ignores ordinary definition links (only media controls), preventing whole-definition folding. GDReaderVersion 8 invalidates prior enhanced indexes. Full Go tests/build, DSL vet, 10-second article fuzz (84,731 executions) and 500-article samples each from Longman/Oxford/Zimmerman pass. Full dictionary corpus and phone visuals not checked; no APK/commit.

2026-10-05 DSL parser upgrade: switched clean master checkout to dev (c920c4b); ported dca92e8's Original lexer/zone balancer, cautious legacy IPA conversion, shared title rendering and tests. GD keeps reference tree semantics, enhancer and generated examples; gdLexer isolates its old attribute/media tokenization, renderer uses shared tag/media definitions. Original ReaderVersion 4; GDReaderVersion 7 because shared title display changed. Alternate ~ headings and nested GD title variants retained. Full Go suite/build, DSL vet and 5-second two-worker fuzz (22,611 executions) pass. No dictionary-wide comparison, APK/device check, commit or merge.

2026-10-05 startup regression: v0.8.0 passes Promise.all's resolved array directly to loadDicts(rescan), accidentally selecting /api/rescan and waiting for index maintenance before listing dictionaries. Reproduced in full Chromium startup with an isolated library (stayed in boot); changed callback to call loadDicts() without arguments and added rejection handling through bootFailed. Full-page EN/RU regression tools/dslcompare/verify_boot.cjs verifies ready and /api/dicts, rejects any /api/rescan request; Go build and targeted server/group/i18n tests pass. User's older-APK installation failure remains unexplained pending exact Android message/version. No APK built/installed or commit made.

2026-10-05 System Log: `dev` at `a320b66`, uncommitted. Supersedes the indexing-only log. Drawer System → Save System Log… is immediately below System settings; Android save picker requests Download and proposes `wuDict2_SystemLog_yyyy_MM_dd-HH-mm.log` in device-local time. `/api/system-log` snapshots Go diagnostics; Android export merges that snapshot with its own persistent warnings/crashes/process lifecycle log. If the server is unavailable it still saves Android diagnostics and marks the missing section. Logs stay bounded (Go 2×2 MiB beside DB_DIR; Android 2×1 MiB in internal app files). UTC records; URL credentials/query fragments redacted; no deliberate article/query-body logging.

Concurrency: Go owns one mutex around configuration/write/rotation/snapshot plus a stable OS lock file (also covers CLI processes sharing a library); Java owns a separate mutex and separate files in its single application process. Export writes independent snapshots after releasing logger locks; repeated native exports are serialized separately. Go warnings, HTTP 5xx, search failures, job/import failures and server lifecycle feed the log; native warning call sites in both flavors feed SystemLog. Crashes retain the platform handler; an OS kill cannot report its own cause. Reader-panic verification exposed a held SQLite transaction: rollback is now unconditional on return/panic, preserving propagation. Go concurrency/rotation/exact-record-count and multi-process tests pass with `-race`; native JVM concurrency/rotation/redaction harness and FOSS/Play Java compilation pass. Full Go suite, build and vet pass; Android/arm64 and Darwin/arm64 logger packages cross-build. Inline JS syntax/System section placement/native-route checks pass. Phone picker/save/restart/crash checks remain unverified; no APK built/installed. Existing unrelated sepia_article.css edits preserved. Existing Node i18n harness still lacks FEAT_NAME in its renderSlot sandbox (also on HEAD).
**Rechecked 2026-10-04 (release `wudict2-v0.8.0`):** published `dev` at the user's request — annotated tag on `1044edf`, APK from `build-android.cmd release`: `versionName='wudict2-v0.8.0'`, versionCode 515, `locales: '--_--' 'ru'`, 8,895,697 bytes, sha256 `475c9f84f40857f2d15463cc78b4a1f4b5b1a1a81c3a344ffae99a67bae3f08b`, the same signing certificate as every release. A normal release, so `latest` moved to it, and the asset now carries the ABI-spelled name (`wudict2-android-arm64-v8a-foss.apk`). Checked before publishing: `go build`/`go vet` clean, the server's `TestGroup|TestUserGroup|TestPrefs|TestBulk|TestIndex|TestI18n|TestOpenAPI|TestRoutes|TestGDAssets` green, and the packaged `.so` unpacked and confirmed to carry `groups.ini`, the filters page, the bulk-action table, the locked/only-copy wording and the curated-groups API, with **0** hits for `Monotype`/`ArialPlus`/`QuiviraPhonetic`; the published asset was downloaded back and hash-matched, and the body is byte-identical to the changelog section. No device or emulator run; `make i18n-check-js` was not run — still no Node here. Notes cover the filters editor, the group editor's Filter control, the one-table index maintenance and the one-word-per-concept overhaul; narrative in `docs/handoff-archive.md`. Also seen while verifying, and not this session's doing: the `wudict2-v0.7.1` release is a **draft** now, carrying eight desktop assets (darwin/linux/windows builds and a macOS app zip) — the desktop workflow writing into that tag.

**The 2026-10-04 upstream syncs (third, fourth and fifth — code cleanup/minify, editable groups, pages/docs/labels):** their merge narratives and resolutions moved to `docs/handoff-archive.md` → "The 2026-10-04 upstream syncs", because HANDOFF had grown past its cap. What stands here: the fork's parallel files were adapted to upstream's renames (`preparedFor` → `validPrepared`, `store.Ingest` → `IngestPlan`, `reconcile`/`reconcileLocked`, the fork's `stripComments(css)` renamed `stripCSSComments`), upstream's lazy section rendering (`fillSection`/`det.wuFill`) is deliberately NOT adopted, and `master` must stay untouched for upstream syncs.

**Rechecked 2026-10-04 (sixth upstream sync — README only):** `dev` is at the merge of upstream `master` (`8be3257`, carrying `0a34f2b`, `b9e556f` and `0930cf8`, all `chore(docs)`). The whole round is eight lines of upstream's `README.md`: the User Guide link re-pointed at `wuweidict.github.io/wudict`, a tip pointing at the author's page of ready FOSS dictionaries (`https://legbehindneck.com/wudict/`), three guide deep-links added to the feature list (custom groups, lemmatization, custom styles) and a stray trailing space. The fork's README is its own document — no "Features" list, no "User Guide" line — so **nothing was ported** and the one conflict was resolved by keeping ours. No code, labels, assets or tests changed; `go build` and the server tests re-ran green anyway. Two things left for the fork to decide, not taken: whether to carry the downloads tip into its own README (its link is the author's dictionary page), and which host the README's manual links should use — the fork's copy mixes `wuweidict.github.io` (android/mac/windows) with `wudict.legbehindneck.com` (linux), which is upstream's own drift, and upstream's newest README says the guide is at github.io while its `SiteURL` (taken in the previous round) is legbehindneck.com. Not pushed — `dev` fast-forwarded in the user's checkout only.

2026-10-04 filter editor closer: shared .top/.x header adds a top-right close button; history.back restores the previous page, direct-entry fallback goes to /. Unsaved changes require the existing localized confirmation before leaving. Targeted catalog/asset checks pass.

2026-10-04 filter editor background follow-up: missing host CSS now makes page transparent over native wallpaper; card, textarea/code and action buttons use translucent surfaces with light/dark ink from the chosen colour. Browser checks cover both locales, light/dark colours with and without image, including control backgrounds; targeted server checks pass. No APK/device check.

2026-10-04 Edit filters: Settings links to existing upstream /groups rules editor, renamed/localized as Edit filters (EN/RU). Page renders locale and enabled page presets per request, reuses lemma/setup native background hook, preserves groups.ini/API/save/reset semantics. Match diagnostics read fork filters rather than curated picker groups. Browser EN/RU load/save/match checks, localization and targeted server tests pass. No APK/device check.

2026-10-04 filter empty-category follow-up: membership filter choices derive only from available nonmember dictionaries. Empty categories, including Uncategorized, are omitted; when the last matching dictionary is added, selection returns to All. EN/RU browser regression verifies both ordinary and uncategorized category removal.

2026-10-04 group editor filters: /api/dicts exposes upstream-derived classifications as separate filters for the fork, preserving curated search groups. Show All membership editing places an All dictionaries/category/Uncategorized dropdown between current members and available rows; only available rows are filtered. Categories grouped/localized by facet, selected filter survives additions, reopening defaults to All. Android shared themed picker supports the dynamic filter. Fixed stale member/order URLs to /api/user-groups. Chrome EN/RU outside-only filtering/membership/empty/uncategorized checks, localization checks and full server suite pass; FOSS Java compilation passes. No APK/device check.

2026-10-04 bulk dropdown menu appearance: Android Shell's shared picker now also installs on dynamically rendered #bulkIndexes selects, observing only that container for replacements. Uses existing DictionaryPicker page colour/image background and text colours, preserving options, disabled rows and change events. Chromium bridge replacement/selection regression and FOSS debug Java compilation pass. No APK install/device visual check; rebuilt APK needed.

2026-10-04 bulk action dropdowns supersede checkboxes: each feature defaults to Keep; Create missing / Update existing / Recreate all / Delete target absent / present / all / present indexes. Start confirms the action names and job counts before sequential execution. Main-index refresh uses explicit ingest rebuild=1 (store.Always), preserving optional features; Contains/FTS refresh removes and recreates the selected optional index. Main deletion resets/disables optional dropdowns. Chrome EN/RU defaults, mixed plan, update/recreate selection and force flag checks, localization checks and full server suite pass; force rebuild regression checks current indexes actually rebuild and retain optional features. No APK/device check.

2026-10-04 Dictionary settings balance: parser selector is one row (Original/GD/Both; Ориг./GD/Оба), full Original/GD names retained as titles. Bulk index actions are mutually exclusive Create/Delete checkboxes per feature; Start opens a combined confirmation then runs the selected jobs sequentially with existing Stop/progress. Base deletion clears/disables optional actions because it removes those indexes too. Chrome EN/RU 320px selector and mixed delete/create plan checks, localization JS and targeted server asset/appearance tests pass. Localization harness now supplies articleSelectionCSS in its frame stub. No APK/device check.

2026-10-04 Compact examples: shared example CSS preserves 1.1em start padding with important, overriding Compact's paragraph padding reset so text clears the diamond. Chrome geometry regression at 15/20/30px and existing example styling/folding checks pass; targeted server asset/appearance checks pass. Existing unrelated work preserved. No APK/device check; reload only, no reindex.

**Merge review fixes 2026-10-04:** checkout `dev`, HEAD `707a70b`; uncommitted fixes requested by the user. Declared the missing ingest-following set and restored the panel's rebuild offer. Detached mutable preference maps, nested lists and pointers before mutation/identity healing; fault-injection tests verify that failed saves keep active DSL settings and group order. Fork activation is one `srv.UseUserGroups()` call in the CLI: upstream facets/rules and diagnostics no longer enter app dictionary rows; article-language detection and the curated membership editor remain. Upstream groups.go/groups_test.go/groups.html/facet/rules.go remain verbatim; their API/page still answer by direct URL. Shared server construction keeps upstream behavior for its original tests. `go test -count=1 ./...`, build, vet, Node ingest UI and parser/group tests pass. No commit, APK build/install or device check.

**Rechecked 2026-10-04 (release `wudict2-v0.7.1`):** published `dev` at the user's request — annotated tag on `ad5c748`, APK from `build-android.cmd release`: `versionName='wudict2-v0.7.1'`, versionCode 494, `locales: '--_--' 'ru'`, 8,848,889 bytes, sha256 `5c7c36cc06da34b2646233774fe022d4bf09e5127d66d6a80cce5fb67778a99a`, the same signing certificate as every release. A normal release, so `latest` moved to it. Checked before publishing: `go build ./...` and `go vet ./...` clean, `go test -count=1 ./internal/server ./internal/store ./internal/format/dsl` green, and the packaged `.so` unpacked and confirmed to carry the parser-aware pickers (`matchesDSLParser`), the shared example look (`wu-example-block`), the rescan progress strings, `articleSelectionCSS` and the serve-time `stripComments`, with **0** hits for `Monotype`, `ArialPlus` and `QuiviraPhonetic`; the published asset was downloaded back and hash-matched, and the release body is byte-identical to the changelog section. No device or emulator run; `make i18n-check-js` was not run — still no Node here. The notes cover the two DSL readers reaching the group editor, the Browse picker and the dictionary count, the shared example look, the Rescan dialog's progress, always-selectable article text, and the upstream comment stripping; narrative in `docs/handoff-archive.md`.

2026-10-04 article text selection: dev started at c3acbb9 with dictionary-count/server edits; HEAD advanced externally to 36d36f4 during this session and includes the selection code. Shared selection CSS appended after dictionary content on shadow and iframe surfaces explicitly allows prose selection/native copying, overriding dictionary user-select:none; controls retain their own behavior. Chrome real mouse drag + clipboard copy verified on both surfaces with blocking dictionary styles, inline JS syntax, targeted appearance/asset tests and go build pass. No APK built/installed; Android long-press, handle dragging and Copy menu in main/lookup windows need a rebuilt APK/device check.

2026-10-04 dictionary counts: dev HEAD c3acbb9 at start. UserCount now counts unique DSL sources rather than Original/GD backend rows; Settings /api/config, first-run setup and rescan found totals share it. Cached-only GD families count once, separate source paths remain distinct, builtins excluded. Per-root discovery counts already use real files; operational/stream totals retain backend units. Server suite passes, including paired and cached-only DSL regression. No APK/device check.

2026-10-03 Original example tint follow-up: background-image/sepia presets clear backgrounds on direct shadow children, so Original paragraphs lost tint while nested GD paragraphs kept it. Shared example background-color now beats that reset with repeated marker-class specificity and important. Real preset/direct-root Chrome regression passes for block and inline examples; targeted server tests/build pass. No APK/device check.

2026-10-03 GD examples prepared during ingest: GD reader v6 generates whole-paragraph/inline example classes, author-marker wrappers and folding marks; styled wrapper carries data-wu-examples=1. JS immediately skips prepared GD articles (browser test proves zero paragraph/example scans); Original and old GD retain shared fallback. GD golden updated; zero-option reference/oracle path unchanged. Parser tests cover mixed/nested examples and 35 marker variants with cleanup on/off; browser styling/folding/skip checks pass. Existing GD indexes need rebuilding for parser preparation, not for shared CSS. No APK/device check for this slice.

2026-10-03 examples shared with Original: browser marker now uses the same rules for Original and GD, including whole-example paragraph diamonds/tint and inline fragment tint/folding. Shared web/examples.css is embedded into ARTCSS for both article surfaces, independent of GD font styles; GD stylesheet v12 removes the former duplicate rules. JS remains necessary for GD styling/inline folding (ingest only stamps whole-paragraph wu-xonly); kept one shared pass and avoids repeated paragraph classification. No index rebuild for this change. Chrome Original/GD light/dark/paper folding/restore, marker/text/idempotence and size checks pass; server/DSL tests and build checked. No APK/device run for this slice.

2026-10-03 GD diamond sizing: CSS v11 uses a .5em-wide/.7em-high diamond, centred on the first line via .5lh plus paragraph padding and translateY(-50%). Fallback retains default 1.618 line height on older WebViews. Chrome geometry checks at 15/20/30px with 1.3/1.618/2 line heights and GD assets/style tests pass. No APK/device check for this change.

2026-10-03 GD mixed examples follow-up: only example-only paragraphs receive the whole-block tint/diamond; inline wu-ex/dsl_ex fragments in mixed paragraphs get the same 8%-ink tint and wu-xonly folding without a marker. Non-example text remains visible. Article find reveals hidden inline fragments as inline and restores them. GD empty-content test ignores Unicode punctuation/symbols and links; leading author markers recognise dots, squares, diamonds, stars, arrows, checkboxes and dashes (35 browser cases). Original retains prior behavior. CSS v10 serving upgrade; reload, no additional index rebuild. Chrome light/dark/paper, show/hide/restore, text/idempotence checks, targeted GD/server tests and Go build pass. No APK/device test for this slice.

2026-10-03 GD example styling: dev HEAD 3ff0a46; uncommitted on top of the margin fix. Browser marker decorates only GD paragraphs beginning with an example (including plain following translation): larger single marker, rounded 8%-ink translucent paragraph tint, inherited readable ink. Follow-up uses a single diamond marker; recognises author ◆/♦/◇/◊ as well as dots/squares and preserves their source text for copying without displaying a second marker; inline examples and Original untouched. Existing folding still uses wu-xonly, so mixed translations stay visible. CSS v9 is upgraded at serving time; no extra index rebuild for this styling. Chrome light/dark/paper and repeat-marker/text-preservation checks pass; Go server/DSL tests and build pass. Preview: tools/dslcompare/results/gd-examples-preview.png; verify_example_style.cjs. No new APK install/device styling check.

2026-10-03 GD margin spacing: dev HEAD 3ff0a46, clean at start; uncommitted fix. Crossed inline wrappers (notably [trn] spanning closed [/m] blocks) no longer retain structural newlines as extra breaks. Follow-up on the actual user Test.dsl/pot also suppresses empty reopened formatting spans, which otherwise create anonymous browser lines; regression now requires exact closed/open HTML equality. Chrome 390px: six paragraphs, zero inter-paragraph gaps/empty spans. Explicit [br] and escaped-space blank lines survive. GD reader version 5 requires rebuilding GD indexes; Original unchanged. DSL regression/golden and server/store tests plus Go build pass. Emulator Small/emulator-5554 checked with installed debug versionCode 487: Test GD was outdated and still held the old breaks. Rebuilt only that index, preserving FTS; real WebView pot now has six paragraphs, five zero gaps and no breaks/empty trn spans. No APK install needed. Evidence: tools/dslcompare/results/pot-spacing/emulator-report.json and emulator-pot.png; phone unverified.

2026-10-03 bulk actions layout follow-up: removed duplicate Create/Delete column headings; added a top separator before For all dictionaries. Action buttons and semantics unchanged. Node UI syntax/localization and targeted server appearance tests pass; no phone check.

2026-10-03 Rescan progress: clean dev at start; maintenance POST optionally streams NDJSON while retaining its JSON API. Rescan dialog and main status show dictionary position/name and rebuild article counters, then cleanup and result. A disconnected browser does not interrupt maintenance. Targeted Rescan/ClearDatabase/I18n/Appearance tests, streamed-progress regression and Node localization/syntax checks pass. No APK/device verification.

2026-10-03 DSL visibility follow-up: group editor and standalone Browse chooser filter by global parser, not index availability. Group hints/counts use visible dictionaries; drag/keyboard reorder skip hidden variants and replace visible slots only, preserving hidden membership/positions. Saved preference order still retains all dictionaries. Node group-parser regression and server Group/DSL/I18n tests pass; no APK/device check.

2026-10-03 Russian Rescan layout follow-up: RU existing-index rows now all stack label above full-width equal action buttons; EN retains its current row layout. Dialog has explicit selected-language marker. Chromium EN/RU 320/360/390/1100px at 100/130% scale and targeted I18n/Appearance/Rescan tests pass. Phone wallpaper/large-text check pending; no APK build/install.

2026-10-03 Dictionary settings layout: dev HEAD 0ea3e18, clean at start; this slice uncommitted. User approved trying vertical full-width DSL parser choices, a three-row Create/Delete table for all dictionaries, separate stacked variant blocks and title above reorder controls. Existing action datasets and confirmations retained. EN/RU isolated Chromium layout probes at 320/390/1100px pass without horizontal overflow; Node i18n and targeted server asset/appearance/catalog tests pass. Preview: tools/dslcompare/results/layout-preview/ru-390.png (simplified solid background, isolated representative card). APK not built/installed; real wallpaper, font scale and touch checks remain for phone.

**Rechecked 2026-10-03 (release `wudict2-v0.7.0`):** published `dev` at the user's request — annotated tag on `fb73c66`, APK from `build-android.cmd release`: `versionName='wudict2-v0.7.0'`, versionCode 483, `locales: '--_--' 'ru'`, 8,835,605 bytes, sha256 `5ee146e040f33dff11dbb71301e6be8f3669e4a4a35d9370b0a693027ef78c7d`, the same signing certificate as every release. A normal release, so `latest` moved to it. Checked before publishing: `go build ./...` clean and the server's `TestClearDatabase|TestRescan|TestI18n|TestAppearanceContract|TestGDAssets|DSL` green; the packaged `.so` was unpacked and confirmed to carry `web/i18n/ru.json`, `Quivira.otf`, the `wugd_*` custom-font hooks, `/assets/article-find.js`, `/api/clear-database` and the `wudict:exit` bridge, with **0** hits for `Monotype`, `ArialPlus` and `QuiviraPhonetic`; the published asset was downloaded back and hash-matched, and the release body is byte-identical to the changelog section. No device or emulator run, and `make i18n-check-js` was not run — there is still no Node on this machine. The section describes the Rescan purge and index maintenance, Find in articles, the two DSL readers with their separate indexes and the font situation, Exit, the foldable Settings drawer and the second upstream sync; narrative in `docs/handoff-archive.md`.

2026-10-03 Exit phone follow-up: user reports first Exit restarting the app, second closing it. Exit now calls finish on all registered windows and keeps shutdown pending until the final onActivityDestroyed; only then stopAny runs. A new external lookup during teardown revokes the old shutdown. Removed finishAndRemoveTask during iteration. Exit button now lives in .facts and receives the same colour/image-aware .mrow styling as the other Settings buttons. Temporary Java lifecycle harness passes first-request/window-order/fresh-lookup/busy Cancel+wait cases; FOSS/Play Java compilation, Node i18n and Chromium shared-style comparisons at 320/390/1100px pass. Actual first-click close and reader restart still require a rebuilt APK/phone check.

2026-10-03 Exit: current checkout `dev` HEAD `b99ba0f`, clean at start; this slice uncommitted. Android-only Exit / Выход is the last separate Settings row. Application lifecycle registry closes all app windows, preserving a reader task hosting Lookup; stops owned/adopted server without disabling future external intents. Active native transfers or demanded server work offer wait-until-finished or Cancel. GET /api/power reports demanded work (also for an adopted child); cancelled startup cannot spawn later. Full Go tests/build, Node i18n and FOSS/Play Java compilation pass. No APK/device verification: check ordinary Exit, multiple popup windows, busy wait/Cancel, adopted server and immediate reader lookup after Exit. Earlier search/UI changes below are included in b99ba0f; their commit-state notes are historical.

2026-10-03 wording: user approved lowercase **exact / точно** for the article-find exact mode, retaining whole-word/whole-phrase matching. EN/RU labels updated; Node localization checks pass.

2026-10-03 wording: user approved **Search in hidden examples** for the article-find checkbox; English catalog updated, Node localization checks pass.

2026-10-03 article-find mode picker: Android now routes the article-find mode select through the existing compact native search-mode picker, including the configured colour/background image and current selection. Shared bridge selection/change/cancel probe and FOSS debug Java compilation pass; no APK/device check.

2026-10-03 quick-find touch targets: previous/next in the article-find strip are at least 44×44px, with an extra gap (about 15px total at default text size). Browser layout probe at 320/390/1100px passes; no APK/device check.

2026-10-03 status-bar spacing follow-up: below/equal 600px all four inter-control gaps are equal and may shrink; above 600px the dictionary arrows form a closer pair, with three equal larger gaps after Down, Find and the dictionary chip. Fixed end gutters retained. Chromium EN/RU 320/390/600/601/620/1100px geometry checks pass without overflow; no APK/device check.

2026-10-03 article find: `dev` HEAD `cd150f4`, clean at start; this slice uncommitted. Magnifier after dictionary arrows opens Find in articles; word-prefix/whole-word/contains, case, highlight-all and hidden-example search, no regex. Next/previous leave a bottom navigation strip; matches span loaded articles in reading order, including collapsed dictionary sections. Only the current hidden example is temporarily shown; leaving it/ending find restores its display. Query/options persist locally; main search/clearing finishes article find. Shadow and same-origin iframe articles preserve markup and selections (older browsers select the current match). Full Go suite/build/vet, targeted asset/i18n and Chromium EN/RU 320/390/1100px checks pass. Module tidy reports only pre-existing go.sum CRLF/LF differences. No APK/device check; soft keyboard, hidden examples and bottom insets remain for the phone.

2026-10-02 Settings language/Browse: dev HEAD 52febec, clean at start; uncommitted. Language is a persisted collapsible section; its door shows Language — English/Русский. Language dialog overrides mobile sheet stretching and empty error spacing. DSL Browse sits below its variant with a small left indent and navigation arrow. Node/targeted server checks and isolated Chromium EN/RU 320/390/1100 layout checks pass; no APK/device check.

2026-10-02 Settings disclosures: dev HEAD ea3b66b, clean at start; changes uncommitted. Six headed sections now collapse independently and persist locally across page reloads; first use/new sections default open. Dictionary settings source/About triangles have a .4em gap. Label columns are recalculated when a section opens. Targeted server/UI and Node syntax/localization checks plus section-state restoration probe pass; Chromium unavailable in current Node module environment, no APK/device check.

Rescan dialog compacted: command/title share panel.rescan, menu door has a chevron, header has a close button (disabled during maintenance), Create labels and existing index names shortened in EN/RU. Browser verifies no dialog scrolling at 320/390/1100 × 700; Node localization checks pass. APK/device check remains with the user.

Rescan dialog opening is synchronous and uses the already loaded configuration; an extra configuration request can no longer delay it. Chromium EN/RU 320/390/1100px checks now also block that request and verify immediate opening. Android APK has not been rebuilt or installed.

Background DSL preparation now reuses a fresh index with the requested Contains/FTS plan; successful preparation removes older folders belonging to that exact source/variant, preserving explicit database sources. Claims reuse existing ownership before vacant names, avoiding duplicates after a name collision disappears. Full Go suite, targeted race checks and vet passed; changes remain uncommitted, no APK/device check.

2026-10-02 unified index maintenance: dev HEAD 7e9a6d4; this slice is uncommitted. Rescan folders opens one Update dictionaries dialog: new dictionaries have mandatory base plus optional Contains/FTS creation; existing ones choose recreate/keep base and update/delete/keep optional indexes. Update never creates absent optional indexes; Keep preserves removals even after restart. New optional choices share the Edit Folders defaults; configured DSL variants are respected. Maintenance drains/suspends the automatic worker, cancels stale pending plans, and always removes unused/disabled caches and old packed media. Originals and explicitly supplied database sources survive. Go/Node and Chromium EN/RU 320/390/1100px verified; no APK/device check. Browser recipe: verify_clear_database.cjs, isolated clear-database-preview at port 6912.

2026-10-02 GD fonts: only unmodified Quivira.otf embedded for phonetics; Arial variants and QuiviraPhonetic removed from distribution. WuGD Arial faces resolve user uploads /files/wugd_{Regular,Bold,Italic,BoldItalic}.{ttf,otf}, TTF first, system sans-serif fallback. CSS v7 refreshes prepared articles without rebuild; uploaded/replaced fonts require page reload.

2026-10-02 localization follow-up: `dev` at `10eaec5`, clean at start. Completed Russian DSL validation, three defaults API errors, collection hint and bulk count wording; known JSON-wrapped errors translate without dropping unknown response diagnostics. Targeted Go/Node/diff checks pass; no APK/device check. Changes uncommitted; details in translation_todo.md.

2026-10-02 first setup only: while the initial empty library is being configured in Edit Folders, selected Original/GD index defaults also set the global parser (one variant or Both). First dictionary discovery ends this persisted setup phase; subsequent defaults edits and launches never overwrite parser choice or clear checkboxes.

2026-10-02 bulk index sections simplified to three actions each (index/contains/full-text). Candidates follow the header's DSL parser: Original/GD restrict DSL variants, Both includes both; non-DSL sources still included. Existing confirmations, status and Stop remain.

2026-10-02: Dictionary settings source-name/About disclosure markers use full triangles at 12px, matching the one-position reorder arrows (open ▼, closed ▶).

2026-10-02 global DSL parser: Dictionary settings header Original/GD/Both controls persisted installation-wide search selection and card visibility. Single mode shows only that variant's card/index row; missing selected index is red even if hidden sibling is prepared. Both shows both cards/rows. Per-family checkboxes removed from cards; legacy API kept. Switching never deletes indexes or changes new-index creation defaults.

2026-10-02: media packing control hidden from Dictionary settings (all formats); existing media databases and source resources untouched, packing API retained.

2026-10-02 missing-index UI: missing variant Browse is hidden; if its sibling index exists, show a normal-colour note in that Browse row. Both indexes missing: no Browse buttons/individual notes, only shared red create-index warning.

2026-10-02 follow-up: per-variant index deletion keeps the red striped inline confirmation; lists only existing index/contains/FTS/packed media, uses Delete/Cancel. Shared source audio/images and other variant survive; packed media is variant-local. Contains/FTS remain enabled without base and explicitly restore it (user reaffirmed; do not disable them in Dictionary settings).

Read whole before planning; keep under ~400 lines. Historical narratives belong in `docs/handoff-archive.md` (grep only; indexed by date) or commit messages. `git show 51c0737:HANDOFF.md` holds the pre-split file. Windows recipes and known baseline failures: `docs/WINDOWS-VERIFY.md`. Latest settings follow-up: bulk confirmation is a compact modal popup (cancel is initial focus); index status/progress is mirrored into settings. Edit Folders now saves defaults for new DSL families (Original index initially; at least one index required), queues selected contains/FTS automatically on rescan/intake/restart, and never changes already registered/prepared families. Legacy installations retain existing selection until defaults are saved; opening Edit Folders activates the displayed defaults. Pending plans and known sources persist; failed builds remain retryable. Chromium EN/RU 320/390/1100 layouts and automatic GD preparation verified; full Go/Node checks pass; no APK/device/commit. Preview port 6910, tools/dslcompare/results/index-removal-preview; verify_defaults.cjs.
## Branch state (verify with git before trusting; GD CSS v6 restores plain underline for [u], no reindex; bulk confirmation overrides mobile sheet stretching: centered fit-content, max 420px, 32px side space, transparent backdrop; Stop finishes active dictionary then skips queue, never disconnects SSE mid-index; stopping/result EN/RU labels and Chromium safe-stop checks pass)
**Rechecked 2026-10-03 (second upstream sync, from `origin/master`):** `dev` is at `ca3321c`, the merge of `df35303` — two upstream commits, and **no conflicts at all**, because neither touches a file this fork has customised: nothing had to be ported into the fork's pages, strings or assets. wumark's parser stops treating a setext heading (`---` under a line of text, ordinary in hand-written markdown) as the start of an entry — only `## ` starts one now — and reports a `## ` line that an HTML block swallowed, naming the line where the entry went missing and the line that took it. The hosted share page offers "Open in wuDict" through an `intent://` link, with the share link in `INTAKE_URL` and a Play fallback; the Android half of that commit is a comment. `ext/server/` stays upstream's hosting asset — this fork's server does not serve it, which is why its `APP_ID` being upstream's is right there and would need changing only for a share page of our own. Verified: `go build`, `go vet` and the whole suite green in this worktree, which is a fresh (LF) checkout — no failures at all.

**Rechecked 2026-10-02 (fonts purged from history, release replaced):** the licence problem described in the paragraph below is CLOSED. `a195d5d` had already replaced the four Monotype faces and `QuiviraPhonetic.ttf` with the author's own `Quivira.otf` (free for any purpose, public domain from 2019); what remained was purging the old files from the commits that carried them. That was done as a **narrow** rewrite: 13 commits on `dev`/`GD_DSL` were re-created without those five paths, with a per-commit assertion that the only change is those deletions and `git diff a195d5d dev` coming out empty. Force-pushed: `dev` → `56a6057b`, `GD_DSL` → `c6809a0e`, tag `wudict2-v0.6.0` → `ca726dd` (now on `56a6057b`). **`master`, the other fourteen branches and the six older tags did not move** — verified against a recorded before-list, and `master` must stay untouched for upstream syncs. **`git filter-repo` is the wrong tool here**: it strips the `gpgsig` header, this repo's upstream-sync merges are GitHub-signed, and un-signing them re-hashes every commit descended from the first of them — the whole repository, all 16 branches and all 7 tags, which would have cost the `master`↔`dev` common history and forced re-pointing all 7 releases. That was run once and fully rolled back (the restore was checked ref by ref). The pre-rewrite repo is saved as a mirror clone in `D:\tmp\wudict-backup.git` (147 MB, every old SHA) — delete it once the rewrite is trusted. The local `.git` is 18 MB and the five blobs are pruned; one stale codex `turn-diffs` checkpoint ref held a copy of them and was deleted (local tooling scratch, present in the backup mirror). The release was then rebuilt and its asset REPLACED: 8,809,885 bytes, versionCode 471, sha256 `e0281deac8a40f77710ad428722ccd3ed9befdaefd5be262c8eff2f4a122e8f0`, same signer, and the packaged `.so` now reports **0** `Monotype` hits against 2 `Quivira.otf`. The release survived the tag update, staying published and not a draft.

**Rechecked 2026-10-02 (release):** published `dev`, at the user's request, as a **normal** release — `wudict2-v0.6.0`, tag on `bc04112`, APK from `build-android.cmd release`: `versionName='wudict2-v0.6.0'`, versionCode 468, `locales: '--_--' 'ru'`, 10,071,197 bytes, sha256 `934f8113c040179b9edb0cd4b169b0ba06473a3a3031ac1287c027a0cfc4f44f`, the same signing certificate as every release so far. Not a pre-release this time: the ru.1 preview announced this line, so v0.6.0 is that line finished and `latest` moves to it. Checked before publishing: `go test ./internal/format/dsl` green (it carries the GD parser tests) and the server's `TestGDAssets|TestGDStyle|TestI18n|TestAppearanceContract|DSL` green; the packaged `.so` was unpacked and confirmed to contain `web/i18n/ru.json`, the GD font and preset paths and the GD parser sources. No device or emulator run, and `make i18n-check-js` was not run — there is still no Node here. **Open licence question, surfaced and NOT resolved:** the five TTFs under `internal/server/web/fonts/` ship in the APK and are served to the page, four of them carry `Monotype` / `The Monotype Corporation` in their own metadata, and `THIRD-PARTY-NOTICES.md` has no font entry at all (`grep -ic 'font|ttf'` = 0). Whoever owns the licence should add the notices or swap the faces before this APK is distributed widely.

**Current slice 2026-10-02:** `dev` HEAD `84864f4`, uncommitted settings/index and GD typography fixes. GD CSS inherits Settings size/weight, headword/IPA use em ratios; stored v4 style imports upgraded to v5 at serving time (no reindex needed), author bold/italic retained. Chromium live 15/24px, 400/500/700 weights/fonts/Examples pass. Bulk Create/Delete All/Original(non-GD)/GD scopes with index/contains/FTS; family checkboxes retained. Confirmation/sequential jobs/no-source skips/completed-error counts; queued cards show preparing/removing, active cards show article counts mirrored in both DSL cards. Persisted removal guards also cover non-DSL. Controls left-aligned, progress may wrap; rocket only absent, size confirms whole-index deletion (contains/FTS/cache), Browse icon below. Sources/other variant stay; explicit create restores removed indexes, both absent show red !. Prior full Go/race/Node and Chromium EN/RU 320/390/1100px bulk/deletion/FTS/layout pass; no APK/device/commit. Preview http://127.0.0.1:6910, tools/dslcompare/results/index-removal-preview; verify_index_removal.cjs, verify_modes.cjs and verify_enhancer.cjs. Localization: translation_todo.md.
**Rechecked 2026-10-02 (upstream sync: collections, orphans, the Windows fix):** `dev` is at `23670f3`, the merge of upstream `master` (`58b5dea`) into it — sixteen commits: URL-installable dictionary collections (a share link to a dictionary, a web folder of them or a list; Google Drive and Nextcloud; the `ext/server` share page; Android intent handling), the orphan-index review (Rescan folders offers to delete prepared data whose files are gone), CI on Windows/macOS, gradle-wrapper checksums and docs. **Upstream fixed what was reported from here**: `releaseSuperseded` closes the backends a rescan retired before an open re-prepares in place (the `rename … Access is denied` of TestRescanSeesEditedSource), `wmd`'s TestPlainMarkdown closes its reader, and their new `.gitattributes` (`* text=auto eol=lf`) makes a fresh checkout LF — which is what the remaining CRLF-comparison failures were waiting for. See `docs/WINDOWS-VERIFY.md`: the known-failure list is empty in a checkout made after this merge, verified at `23670f3` in a fresh worktree.

Four files conflicted, and two of the resolutions carry a decision worth knowing: the orphan offer's box sits in the **drawer, under the Rescan row** (upstream's comment says "where the question was asked", and in this fork that row is in the drawer, not in the dictionary window), and its "was <path>" line is `var(--label-quiet)` because the grey-text guard refuses `var(--fg-faint)`. In `setup.html` the port went behind new catalog keys (`pages.alreadyIn`, `pages.replacesIn`, `pages.downloadedFiles`, `pages.collectionExtras`, `pages.alsoFailed` — in `en.json` and `ru.json` both), upstream's dropped "add a second copy" option is dropped here too (its JS no longer reads `srcCopy`; the row's `have++` had lost its declaration, and the page is strict, so that line threw for an already-installed dictionary), and `internal/cli/cli.go` imports `internal/howto` again for the new `isHowtoSource` while the guide itself stays unwired. Three setup-page test probes were fixed on this side (they told the pages apart by text, which the i18n catalog now ships in every page). Verified: build and vet clean, the suite went from eleven failures to four and those four are gone in a fresh checkout; the orphan offer was also seen working in the browser.
**Rechecked 2026-10-02 (GD integration):** this merge combines `dev` at `1050f5b` with `GD_DSL` at `a9a66bc`, as requested; clean before merging, only HANDOFF/WINDOWS-VERIFY conflicted. New compatibility fixes: orphan detection follows the real DSL behind a GD descriptor; deleting an open GD orphan closes its serving backend; regression checks cover both indexes and prove disabling a variant does NOT orphan it. `/api/dsl-mode` and DSL metadata now documented in OpenAPI; the reference-parser JS test normalizes CRLF. `.gitignore` audited: parser/oracle/fixtures/fonts/wrapper remain visible, build/results/cache/scratch remain ignored; removed the broad fuzz-seed exclusion. Full Go suite/build/vet and server/store/DSL race tests, Node localization and 9 Python tests pass; original GD oracle 19/19, Chromium mode/Enhancer/font/Examples checks pass in EN/RU 320/390/1100px. Three existing Markdown files were converted locally to LF (no indexed content change). Isolated merged preview http://127.0.0.1:6909, fixture/library in tools/dslcompare/results/merge-preview; screenshots/reports in results/enhancer-preview. No push/APK/device or repeated full dictionary corpus run.

**DSL facts carried forward:** per-family Original/GD/Both selection persists separately from order/groups, defaults both; bulk affects registered families only. Both indexes remain, inactive variants excluded from picker/search even explicit scopes; inactive metadata cannot auto-prepare. Last surviving copy stays available; moving source paths resets selection. GD reader v4 production Enhancer/styles ON, GDOptions{} OFF for oracle; generated wu-xonly preserves mixed translations, native bold/italic/metadata retained, Examples commands unchanged/no GD article buttons. Five embedded fonts/scoped CSS v4 and stylesheet rewrite exemption; pre-v4 GD indexes need rebuild, ordinary parser unchanged. Prior full key/tree corpus: 28 files/3,259,925 cards; Stage 1 HTML sample: 27,002 cards (2,335 media excluded), only spacing differences. Native scanner/index bytes and full visual GD parity NOT proven. Details: docs/DSL.md section 11 and tools/dslcompare/README.md.

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

**Older upstream-sync history:** docs/handoff-archive.md, "The 09-26 upstream sync into dev2", carries those resolutions, verification and merge-work workflow.

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
