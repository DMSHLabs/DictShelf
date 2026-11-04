# Agent handoff — current state

Contents - read the section you need, not the whole file:

- [The build](#the-build)
- [Domain, site and App Links](#domain-site-and-app-links)
- [The Android bridge and the app on a device](#the-android-bridge-and-the-app-on-a-device)
- [The lemma catalogue](#the-lemma-catalogue)
- [State by area, newest first](#state-by-area-newest-first)
- [The index and the readings, as rules](#the-index-and-the-readings-as-rules)
- [Removal, as rules](#removal-as-rules)
- [Windows close-to-tray change](#windows-close-to-tray-change)
- [Product identity](#product-identity)
- [Search and menu state](#search-and-menu-state)
- [Search parallelism and the two caps that went away](#search-parallelism-and-the-two-caps-that-went-away)
- [ZIM prepared databases now have two forms](#zim-prepared-databases-now-have-two-forms)
- [The work panel's rows are a function of the job state](#the-work-panels-rows-are-a-function-of-the-job-state)
- [What remains](#what-remains)
- [What left this file on 2026-11-03](#what-left-this-file-on-2026-11-03)

Numbers (index sizes, build costs, timings) are in `docs/MEASUREMENTS.md`; how to
build, test and poke the app on this machine is `docs/WINDOWS-VERIFY.md`; the
per-session narrative is `docs/handoff-archive.md`. `docs/INDEX.md` is the map.

## The build

The build has one flavour since 2026-11-03: cgo, `-tags sqlite_fts5,sqlite_dbstat` (MinGW on
Windows, the NDK on Android, a native runner per platform in CI). The pure-Go SQLite driver
(`modernc.org/sqlite`, `-tags purego`) was removed with its dependency, its notices and its
build-tag prose; a build that selects no driver fails by name (`internal/store/driver_missing.go`).
`internal/speex/backend_nocgo.go` stays: it is the `!cgo` stub of the speex package, not a SQLite
flavour. The published site carries no `-purego` and no `make cross` / `make build-purego` /
`make android-go-purego` (targets this fork has never had): rechecked 2026-11-03 with a grep over
`pages/`, whose only remaining mention is the FAQ line saying the `-purego` artifacts are gone. One
consequence is open: the manual's own pages are deployed but not in the site's navigation, which is
Home plus the share page (an earlier decision - the menu was cut to those two entries).

## Domain, site and App Links

The project has a domain: **dictshelf.com**, served by GitHub Pages with that custom domain on the
docs site, which makes the ORIGIN OURS and is the only way Android App Links can be verified at
all. `pages/.well-known/assetlinks.json` is copied to the site root by `docs.yml` (with
`include-hidden-files: true` - the artifact action drops dot-directories by default) and names our
release certificate
`B7:DD:C9:5D:EA:6C:16:63:ED:C7:93:70:30:1B:C1:0E:C2:25:50:85:DA:07:AE:B3:C3:E4:E4:91:AB:EB:25:20`
(read out of the user's signed APK with `apksigner verify --print-certs`; no keystore password is
needed for that, and none was ever entered in a session); the debug entry was REMOVED from that
file on purpose, so a debug build no longer opens the domain, and the keystore itself was never
committed (`*.jks` is ignored and no blob exists in any revision). The manifest has an
`autoVerify` filter for `dictshelf.com` and `www.dictshelf.com` with exact paths `/share` +
`/share/`, and `pages/docs/share.md` is our own share page (`/share/`).
`build-android.yml` fails a signed release whose keystore SHA-256 is not named in that file, so a
rotated key cannot silently stop opening links; the Play App Signing certificate is added by hand
from the Play Console (there is no Play build yet, so there is nothing to add).

**The domain is wired up as of 2026-11-03** (Porkbun, done through the in-app browser with the
user's own login - no credentials ever entered the session): the parking `ALIAS @` and wildcard
`CNAME *` to `uixie.porkbun.com` are DELETED, and 4 A + 4 AAAA (GitHub's) at the apex plus
`CNAME www -> dmshlabs.github.io` are added; the two MX records and the SPF record were left
alone on purpose (Porkbun email forwarding), as were two stale `_acme-challenge` TXT records.
The authoritative nameserver answers with the GitHub addresses and the parking 302 is gone.
On GitHub: **Pages was DISABLED in this repository** (`Source: Deploy from a branch / None`) -
it is now enabled with Source = **GitHub Actions** (what `docs.yml` deploys through), the custom
domain `dictshelf.com` is saved, its **DNS check is successful**, the certificate has been issued
and **`Enforce HTTPS` is on**.

**The docs site had never been published at all** (`dmshlabs.github.io/DictShelf/` answered 404,
so every link to it in README and in the manual was dead), because EVERY docs workflow run died at
`make api-lint`: `internal/server/web/openapi.yaml` did not parse. Three pre-existing faults, all
invisible to `go test` (api-test scans routes, it does not parse YAML): a folded block in the
`groupwithin` description ended early; the `application/x-ndjson` media type of `/api/browse/find`
had been spliced into `IntakeExtra.properties.size` (its events - a block of words, progress, the
final count, an error after the stream has started - are browse-find's); and six lint errors hid
behind the parse failure (`nullable: true`, which OpenAPI 3.1 has no such keyword for, and three
unquoted descriptions whose commas split their flow mapping, leaving a bogus property named after
the rest of the sentence). Repaired in `9fc05ed7`, with the six missing `operationId`s added so the
lint is clean rather than merely passable.

**The site IS LIVE since 2026-11-03**: <https://dictshelf.com/> (www 301s to it), deployed from
`dev` - the manual says DictShelf (the install and building pages carry zero `wudict` and zero
`-purego`), `/share/` is the share page, and `/.well-known/assetlinks.json` answers
`200 application/json` at the origin root; Google's Digital Asset Links API lists
`com.dmshlabs.dictshelf` with its certificate fingerprint, so the App Link data is in place.
App Links were verified END TO END on the emulator (2026-11-03): a real share link
(`#https://download.freedict.org/…/freedict-eng-deu-1.9-fd1.slob`) handed off to the app, which
downloaded the 38 MB dictionary and indexed it ("Added freedict-eng-deu-1.9-fd1.", then indexing
345 000 of 464 791), and a repeat import answered "already installed"; the user then confirmed an
import on their own phone. What CANNOT be verified on a `google_apis` emulator image is the
network half - `cmd package query-services -a android.content.pm.action.VERIFY_DOMAIN` answers
"No services found" there, so the domain state has to be forced (`pm set-app-links --package
com.dmshlabs.dictshelf 1 dictshelf.com www.dictshelf.com`; 0 undoes it). The recipe is in
`docs/ANDROID-FORK.md`. Released APKs predate the manifest filter and still do not open the
domain. Verified with the same commands CI runs:
`redocly lint` (valid), `redocly build-docs`, and `zensical build --strict` over `pages/`
("No issues found") - the first time that site build has completed anywhere.

## The Android bridge and the app on a device

The Android intake path had three defects that made a share link die in the app; each is fixed and
was reproduced on the emulator before and after. The bridge sent the bare link and the bare path
where the Go dispatcher wants a JSON request, so every import answered "invalid import request"
(`32cd21a9`; `internal/server/dmsh_intake_request_test.go` pins the shape on the Go side). The
start checked the Go runtime once instead of waiting for it, so a cold start killed its own runtime
and showed "Go runtime is unavailable" over a working app (`58a59653`). An omitted bridge argument
arrived as `null` and a call that answered nothing claimed the runtime was missing rather than
naming itself (`0dd8469f`). Emulator build rule: `build-android.cmd debug intel`; `intel release` is
allowed and produces `…-foss-emu.apk` (arm64-v8a + x86_64) - an emulator-only artifact, which is
what the `-emu` suffix exists to say, and it must never be published.

The switches are DONE (they had been held until the domain answered): `site_url` in
`pages/zensical.toml` and `cli.SiteURL` are `https://dictshelf.com/`, the CLI help's MANUAL
address with them, and every `dmshlabs.github.io/DictShelf/...` link in README (10 of them) and
in the intake examples now names the domain. Note also that
`ext/server/.well-known/assetlinks.json` is NOT verbatim upstream (master names
`com.legbehindneck.wudict`, the fork edited it to our package in `7bcac814`), while the
dead-upstream registry claims that directory is taken verbatim.

## The lemma catalogue

The lemma catalogue is ours since 2026-11-03: `config.DefaultLemmaURL` and the built-in default
in `internal/lemmas` point at `raw.githubusercontent.com/DMSHLabs/DictShelf/lemmas/manifest.json`,
the fallback catalogue at `raw.githubusercontent.com/DmShAl/lemmas` (raw, not a Pages site: it is
what a downloader reads without the site's own limits), and the scheme/host/path pin in
`fallbackURLs` follows them. **The `lemmas` branch and the site copy do not exist yet**, so
`dictshelf lemmas download` reports an unreachable catalogue until the files are built (the
Makefile lemma target) and pushed there. `pages/docs/extension.md` is now the contract only - we
publish no browser extension, and the upstream store links and the upstream social link in
`pages/zensical.toml` are gone. Share links are a FORMAT since 2026-11-03, not an address:
`intake.Unwrap` reads the link after `#` on ANY page (only the wudict project's page stays in
`SharePages`, because its bare address is a page of dictionary links and its links open the app
on Android 12+ through its assetlinks.json). Hosting OUR OWN page of dictionaries is still open;
the asset for one - upstream's, unwired - is `ext/server/wudict/index.html`, and the dead-upstream
registry says it stays unwired until that decision is made.

## State by area, newest first

- A run of state paragraphs follows, newest work first, with the areas interleaved:
  **Android** System Settings geometry and the indexing CPU limits; **System Log** and
  the Windows start-up check; **Windows** tray, close-to-tray and hot keys; **release
  workflows**; group and status-bar counts; the **Android bridge** call shapes;
  **Wildcards** concurrency; then the longer UI paragraphs - the shared System Settings
  dialog, the updater, taskbar icons, style layers, index-size display, the bottom dock
  and Find in articles, indexing modes, the boot screen. Each one says at its end what
  is verified and what is not. History and measurements are not here: see the last
  section of this file.

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

Canonical checkout: `D:\Projects\Android\DictShelf\`, branch `dev` (the integration branch; `master` is upstream only, and the glass work lives on `xapian-glass`). Edit Folders now has **Dictionary folders** and **Download Dictionary** tabs in the same window; import is available when an install folder exists. Main search suggestions now follow the selected headword mode, including wildcards, cancel on edits or scope changes, rank matches before applying the 40-per-dictionary and 500-overall defaults, and identify these limits in Results. The dropdown expands to available viewport height and scrolls internally at the window edge. For every headword mode, suggestions with equal spelling and match quality prefer shorter headwords before match position. Search history supports up to 1000 entries, and Find in articles has its own filtered recent-query list under the shared history limit. Dictionary groups and Browse A–Z show headword counts — the group editor and Browse's chooser print both counts when they differ (articles / headwords, `dictUI.articleAndHeadwordCounts`, drawn by `dictCounts` in `index.html` with a copy in `browse.html`), one when they agree or when an older library has only the article count; the A–Z list itself holds article headwords, which is why both numbers are shown for a dictionary with many aliases; the settings card and the information popup print the same two counts as labelled lines (`dictUI.numberOfHeadwords` / `dictUI.numberOfEntries`, the card's under the chip row and the meta line no longer repeating the article count), and every count is formatted with the reader's own separators through `window.wudictI18n.number`, not `en-US`; the full-text chips read `full-text` (our FTS5 index, which stores positions, with its size beside the name once built) and `built-in: words only` (the archive's own index) on every dictionary — the built-in one carries the qualifier, so the same index is named the same way on a ZIM and on any other format; the status-bar dictionary picker shows found-versus-total article counts, with a plus while provisional, and marks the dictionary at the current reading position. Long-pressing dictionary names opens an information popup with source files (including attached `_abrv` companions), localized headword/article counts, and About text; prepared database paths are omitted. That popup now prints the dictionary's source size and its index size above the headword count — each a human-readable figure with the exact byte count, from `/api/about`'s `fileSize` and `indexSize`. The source size is summed over the files the endpoint already stats (a folder contributes nothing: About stays stats, never a walk); the index size is `text.db` plus `articles.db` when the build split the article index into its own file, so it is one file or two. The Browse A–Z chooser also opens this popup on long press or right-click; in Browse's word-list view, tapping the dictionary name opens it, with long press and right-click supported too. New indexes persist the source headword count separately from article count: since `IngestVersion` 8 both are COUNTED at ingest — `entry_count` is the articles, `headword_count` is those plus every alias pointing at one (`SELECT count(*) FROM alias`) — where 7 and earlier recorded what the source had declared, which Lingvo DSL declares as nothing at all. A library built by an older build therefore reads as outdated and shows the old (or missing) headword number until it is rebuilt. The group toolbar starts multi-selection with a Multi button; A–Z and pin are outlined buttons separated by one button height. Membership editing uses minus buttons to remove and plus buttons to add dictionaries. Long press and right-click on names continue to open dictionary information. Article headers support left-button hold and right-click on Windows. Full-text remains excluded. Wildcard suggestions stream the first headword immediately and flush more candidates every 250 ms without loading article bodies. Selecting a suggestion keeps *? as the input mode and shows exact+lemma results; the checkmark opens the Wildcards view. Runtime UI verification is pending.

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

test failures are fixed: `TestDictsRunningJobIsNotKept` now waits for the
first stable dictionary-list validator instead of assuming a fixed number of
requests is enough, and the EN/RU full-text mode hint catalogs no longer have
empty values. Both targeted tests passed five repetitions, and
`go test ./internal/server -count=1` passes on Windows; search keeps five articles in the first populated section, later sections use Settings → Results, and later pages in that first section start after its five-result first page.

The boot screen reports dictionary-file checks and metadata reads. The native
WebView2 splash says `DictShelf`, then updates with localized dictionary-check
and metadata-read counts until the list is ready. After the first dictionary
arrives, the separate status line does not duplicate the indexing task shown in
the work panel. The indexing panel shows active dictionaries in start order,
per-dictionary and overall elapsed time, completed dictionaries of the total,
and progress units. These UI changes are not runtime-verified yet.

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
- **The Contains index is `detail=none`, so a candidate is a few TRIGRAMS of the term and never the whole
  word.** Every trigram read narrows with `trigramCandidates` (up to three single trigrams of the term -
  first, middle, last - ANDed) and lets the matcher decide the rest; a PHRASE is refused by that table
  (`fts5: phrase queries are not supported (detail!=full)`), which is what quoting a whole word produced.
  The complete-list pass did quote it, so a Contains search whose longest word was four runes or longer
  (`home`) lost its complete list on EVERY dictionary holding a Contains index, while three-rune queries
  (`dog`) were fine - found 2026-11-04 from a live system log and fixed in `StreamArticleIDs`, pinned by a
  test that fails on the old line with the error above. Several windows are not a precision requirement
  but a cost one: one trigram narrows by three letters whatever the query's length, and the measured win
  is in `docs/MEASUREMENTS.md` (candidates and time, one window against three, on the real library).
  `entry_fts` and `article_fts` stay `detail=full`, which is why phrases are legal there.
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

## What remains

Three things are open, in the order they were asked for. Everything else in this
file is state: what the code does today, and how far each area has been verified
on a device.

- the **progress watchdog** - the one piece missing where the search clocks were
  removed (see [Search parallelism and the two caps that went away](#search-parallelism-and-the-two-caps-that-went-away));
- the **`lemmas` branch is not published**, so `dictshelf lemmas download` reports an
  unreachable catalogue (see [The lemma catalogue](#the-lemma-catalogue));
- the two **UI items below**, both found by the user's eye and neither covered by a test; the
  third section under them is finished work, kept because those two touch the same code.

### Upstream reviewed on 2026-11-04, and what was left

The whole upstream delta since `bf4e83cd` — the 2026-10-11 sync, 42 commits, up
to `5b646bbe` — was read against this fork's code, and the review cursor
`upstream-reviewed-2026-11-04` now sits on `5b646bbe`. Eight changes were taken,
one commit each, all dated 2026-11-04: the CI runners and the release NDK, the
`IMPORT_INSECURE` hints in both catalogues, a child `<source>` as an audio link,
layout spaces at a line's ends in the wmd writer, read-aloud language and the
Other-languages menu, the headword's speaker button, the hidden bar coming back
on focus, and the browser address with the escaped key. The MDict port
(`65325a4d`) followed the same day and is DONE — see below — and so is the wmd
half of `96e9a334`, ported as internals on 2026-11-04. What was
left, and why:

- **ZIM — four pieces taken on 2026-11-04.** The bound on a redirect chain (8 →
  50, the writer's own watchdog from `libzim/src/entry.cpp`); `isArticle` now
  requires a redirect's target to be an HTML article and `Resource` requires
  stored content (`dirent.isContent()`, upstream's name); About gained the
  archive's own article count so a ZIM's alias-inclusive entry count is printed
  as names BESIDE articles (`1 528 188 / 2 527 809` for the Russian Wiktionary);
  and `Exact` retries a missed path without a leading "X/" segment, as
  `Archive::getEntryByPath` does. Each was gated on the ZIM suite, `TestRealFile`,
  `TestRealFileRedirects` and `tools/xapian-glass-check` against a real archive
  (the fixture and its command are in `docs/WINDOWS-VERIFY.md`), and every claim
  above is a number from that archive rather than a reading of upstream's diff.
  **NOT taken: upstream's browse-side de-duplication by target** — measured, the
  Wiktionary's 999 440 redirects resolve to 153 594 articles and 124 221 of those
  are named by more than one path (worst 23), so collapsing the row lists would
  fight the two counts this fork prints on purpose.

- **`65325a4d`, MDict parser correctness and speed — TAKEN 2026-11-04.** It is
  the one item a wholesale take-over worked for, and the reason is worth keeping:
  this fork's `gomdict` differed from upstream's parent by ~35 lines, and 30 of
  them were a single consequence of `internal/limits` being absent here
  (`maxLZOBlock` as a constant, plain errors instead of `limits.Exceeded`). So
  the port takes upstream's package whole and ADDS that package (103 lines,
  stdlib-only, `MDictBlock: 256 << 20` — the same number the constant carried),
  which deletes the adaptation rather than preserving it. `internal/format/mdx`
  keeps only its own `offline.go` and the `Contains` caps line. MDX
  `ReaderVersion` went 2 to 3 with the parser, so prepared MDX libraries read as
  outdated and rebuild once. `git diff 65325a4d -- internal/gomdict
  internal/limits` shows the module path and nothing else.
- **`2376d239` (ZIM), and this one is NOT a copy job.** Measured 2026-11-04: this
  fork's `internal/format/zim` differs from that commit's parent by 4678 added
  and 1540 removed lines over 38 files, i.e. it is a fork-owned implementation —
  its own `glass/` package (Xapian full text: bm25, postlist, query, stem,
  inflate, preserving), `dmsh_index_reader.go`, `dmsh_readings.go`,
  `dmsh_stats.go`, `dmsh_xapian.go`, `icon.go`, `cluster_layout_test.go` — while
  the upstream commit rewrites `container.go` (620 lines) and DELETES `title.go`,
  `title_test.go`, `split_test.go` and `suite_test.go`, which this fork still has
  and its own code may call. Never copy upstream's `container.go` over ours.
  Take the three behaviours one at a time — the metadata-redirect follow,
  robustness against damaged records, the namespace strip in `Exact` — each its
  own commit, each gated on the ZIM package suite AND the real archive
  (`docs/WINDOWS-VERIFY.md` → "ZIM archives on this machine"), and bump
  `internal/format/zim`'s `ReaderVersion` (4 today) whenever what an ingest
  writes changes.
- **`96e9a334` (wmd) — TAKEN 2026-11-04, as internals.** The reader is upstream's
  two-parse one now (phase 1 `index.go`, phase 2 `units.go`, `labels.go`'s
  `refLabels`), while this fork's DIRECT backend stays exactly as it was:
  `wmd.go` still holds `Dict{reader *Reader}`, empty `Caps()`, `Exact`/`Prefix` =
  `dict.ErrUnsupported`, and no `store.OpenSelfPrepared` call came back.
  Extracting upstream's directory over ours was tried earlier the same day and
  reverted: it silently resurrects the store-prepared Dict. Two spots had to
  differ, each decided on purpose and written into its commit: `index.go`'s
  `urgentNow` reads `runtime.GOMAXPROCS(0) > 1` instead of upstream's
  `dict.Urgent` (this fork has no `internal/dict/urgent.go`; its lane is
  `server/power.go`'s GOMAXPROCS), and this fork's own
  `defer dict.ClearActivity(path)` in `openSource` was DROPPED — it contradicted
  `bound_test.go`'s contract that the CONSUMER of the text clears the mark, which
  this fork's own `internal/dict/activity.go` states in words, and nothing in the
  fork reads `dict.ActivityOf` at all. `bound_test.go` itself came back here: it
  had been lost in the 2026-10-11 merge `88a9ffeb`, not dropped on purpose.
  **`ReaderVersion` stays 1** — `TestReaderGolden` did not move, so the rewrite
  changes how the text is read and not what it yields, and no library rebuilds.
  Measured in `internal/format/wmd/read_bench_test.go` on synthetic 20,000-entry
  dictionaries (no real `.wmd` exists on this machine): 51→22 ms plain, 207→56 ms
  with 20,000 unreferenced definitions, 319→73 ms with them referenced. NOT
  fixed, and the spec already documents it as a stock-parser limit (R9.3):
  goldmark v2.1.5's own quadratic in the definitions of ONE paragraph. The store
  half of that commit (`pipeline.go`/`ingest.go`/`compress.go`) stays this fork's
  own ground.
- **P6 of the transfer plan** (`c8b92dda`, `f1c5fb27`) — `internal/limits` itself
  now exists (it came in with the MDict port), so what remains is WIRING it: the
  `MAX_*` config keys, and the bounds at ZIM cluster decompression, slob bins,
  StarDict indexes and the intake's space checks. Mind the ZIM one — it is the
  same package as the bullet above.
- **Product decisions, not work**: MEGA links (`c9fcb3d3`), favorites
  (`81949eda`), CLI globbing (`4b00e48b`) — each is cheap once it is decided,
  and each wants this fork's own CLI or UI rather than a port.
- **`8a48536d`, live Browse progress — TAKEN 2026-11-04, without the launch.**
  The fork had this once and removed it on purpose (`c2196377`): Browse must not
  build indexes (the rule `internal/format/wmd/wmd.go` states for its own
  prepared index), so what came back is the SHOWING of work that is already
  running, and nothing else. `browse.html`'s `unprepared()` asks the server one
  question — `POST /api/demand` with the new `promote` flag, which reorders the
  running index queue and never starts anything (`demand.go`'s `promoteQueued`,
  `PromoteDictionary`) — and then polls `GET /api/index-work` (no `?id=`, so the
  auto-selected job is the initial queue or a reindex; `busyDicts`,
  `busyProgress` and `pendingDicts` are read off it). The card waits 400 ms,
  shows the dictionary's own words counted or "Finishing…" once the counted part
  is over (the bar drops its `value` there — a bar parked at 100 while work goes
  on is a lie), takes the words when the dictionary leaves the queue, and falls
  back to the `browse.indexRequired` card when nothing is running. Ten keys —
  `browse.indexing*`, `browse.indexFailed`, `browse.tryAgain` — went into both
  catalogues; the browse 409 now carries `name` so the card can say which
  dictionary it means; `promote` is in `openapi.yaml` and in the desktop/Android
  `demand` bridge. Verified live against a preview server: the card appears at
  ~0.4 s, the count grows, the page issues NO `/api/ingest` (nothing is
  started), and the words take over at the end; killing the server mid-build
  gives the failure card with "Try again". `internal/server` carries
  `TestPromoteDictionaryReordersWithoutStarting` for the reorder-only contract.
  **Left unverified in the wild**: a dictionary that is merely PENDING (queued,
  not yet in a worker) when Browse opens — this machine has 24 cores so the test
  library never queued one; that card state was checked only against a stubbed
  `/api/index-work`.
- **Already ours, so nothing to do**: `34541b56` (the undeclared-mime ZIM fix),
  the panel's rename/collapse/placeholder items, `groups.ini`, the macOS
  deployment target and the systray crash. Upstream's wildcard search and the
  panel's pin model stay dead per `AGENTS.md`.

### Two more from the user's screenshots, to implement (2026-11-02)

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

### The three readings and their chips (2026-11-02 - done; kept because the two items above touch the same code)

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

### The workspace-dialog refactor

The workspace-dialog refactor in [`docs/DIALOG-REFACTOR.md`](docs/DIALOG-REFACTOR.md) in [`docs/DIALOG-REFACTOR.md`](docs/DIALOG-REFACTOR.md) is confirmed on Windows; Android/browser checks and unused iframe-route removal remain. The earlier estimate was 89–91% complete, with regression checks the largest remaining stage. Windows startup without a listener, direct page navigation and `/res/` media delivery are confirmed; remaining Windows screens, assets/styles, browser and Android HTTP paths still need verification.

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

## What left this file on 2026-11-03

Four kinds of thing did not belong in the state of the fork, and each now has one
home. Nothing was lost; the commit message carries the narrative of the move.

- **Numbers** - what a read-through ZIM costs, what each index costs, what removing
  full text frees, the size-estimate bands, the Wildcards cache benchmark and the
  ingest write-path measurement - are in `docs/MEASUREMENTS.md`.
- **Xapian's own full-text index in a ZIM** ("glass": the spike, the format read off
  Xapian 1.4.32, the reader and its three byte-level corrections) is in
  `docs/GLASS.md`.
- **This machine's state** - the Russian lemma pack, what it costs and what is odd
  about its data - and the browser-automation recipe are in `docs/WINDOWS-VERIFY.md`.
- **Session narrative** - every "Historical snapshot" paragraph, and the dated
  sections listed below - is in `docs/handoff-archive.md`, which indexes them by
  date. Read them by grep, or read the commit messages.

The dated sections that had already moved there on 2026-11-03: progressive results
and the user's three decisions (2026-11-02); a ZIM with its own index answering full
text itself (2026-11-01); our index on request for an archive that has one
(2026-11-02); strict readings declined rather than scanned, and the caps that went
with them (2026-11-02); the strict readings being mostly blind, one of them lying
(2026-11-02); the attached article temp and the build's cache (2026-11-03); an index
that cannot be deleted, and every dictionary having one (2026-11-03); the index chip
as a measurement (2026-11-03); the article index as a file of its own (2026-11-03);
a ZIM without its own index getting ours, without the bodies (2026-11-02); and the
three readings chosen by the reader (2026-11-02).
