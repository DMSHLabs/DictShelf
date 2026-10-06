# Localization: status and next tasks

2026-10-06 group editor: EN/RU labels for inline New Group/Rename/Save controls and group-name placeholder added alongside the dictionary search and keyboard labels. Phone layout and keyboard check remain.

2026-10-06 single DSL parser cleanup: removed unused Original/GD/both selector labels and retired selection error translations from both catalogs and the error matcher. Go catalog checks pass. The full Node i18n harness still stops at the existing `Чистый` versus `Чистое` assertion at line 226, before its later checks.

2026-10-06 dictionary-list recovery: EN/RU messages added for an interrupted list and for a group with members that are all unavailable for search. Group editor reuses its existing unavailable label. Go catalog checks pass; phone check after rebuild remains.

2026-10-05 launcher Settings: added Save System Log… / Сохранить System Log… before Restore defaults. Screen buttons preserve resource casing instead of Android all-caps. FOSS/Play Java compilation passes; phone layout/save check outstanding.

2026-10-05 Appearance: added EN/RU checkbox below Examples for also hiding examples without [*]. Server-persisted, default off, effective only with Hide. Node i18n and Go catalog checks pass; Android wrapping/touch check outstanding.

2026-10-05 shared index progress now shows processed entries “of” total when the indexer supplies a total; EN/RU catalog keys added. No device check.

2026-10-05 operation admission: EN/RU active-operation count and conflicting-operation error added to the shared progress/locking UI; device verification outstanding.

2026-10-05 System Log: save action renamed to Save System Log… / Сохранить System Log… in the System section, directly below System settings. Native success/failure labels retain System Log as requested; dated filenames use device-local time. Catalog/server checks and FOSS/Play Java compilation pass. Phone save dialog/toasts/layout remain unverified; no APK built. Existing Node i18n harness still fails in its renderSlot sandbox (missing FEAT_NAME, also on HEAD).

2026-10-04 upstream vocabulary (STYLE.md): the labels round is ported into EN/RU — mode names exact/prefix/contains/full-text («точно», «префикс», «внутри слова», «по тексту»), index/indexed/indexing instead of prepare/prepared/preparing («индексация», «проиндексирован», «библиотека»), the switches named by their artifact («индекс вхождений», «полнотекстовый индекс», «медиа-пакет» — new keys `dictUI.containsIndex`, `dictUI.ftsIndex`, reused `dictUI.packedMedia`), tooltips «Добавить/Убрать/Устаревший {what}», «Не проиндексирован…», orphan wording («осиротевший индекс»), lemma data («данные лемм»), setup page headings (Dictionary folders / Library), browse/lemmas messages, and the filters page («wudict недоступен»). Android: 13 reworded strings taken from upstream into `values/`, translated into `values-ru/` (notification channel/title/text, import text, the advanced-settings hints); 4 previously untranslated hints (edge, edge-none, access key, advanced) now have Russian text of their own. Catalog JSON validated and `TestI18nCatalog|TestI18nPagesAndCache` green; the phone check for the new Russian labels is owed.

2026-10-04 Edit filters: upstream rules editor now translated and linked from Settings. Hints describe filters in the membership editor; save/reset/unsaved-change and diagnostics labels localized. EN/RU browser load/save and catalog checks pass; raw server parser diagnostic details retained. Device background check pending.

2026-10-04 membership filters: Filter/Uncategorized/empty-result labels added in EN/RU; categories reuse facet translations. Browser checks on both languages and localization checks pass; Android picker appearance needs a rebuilt APK/device check.

2026-10-04 bulk dropdowns: EN/RU Keep/Create missing/Update existing/Recreate all/Delete labels and main-deletion note added. Defaults and action plans checked in Chromium on both languages; catalog/JS checks pass. Device check pending.

2026-10-04 Dictionary settings: short Original/Ориг. label and Start/Начать added; GD and Both/Оба complete the single-row parser selector. Chrome 320px EN/RU and localization checks pass; phone verification pending.

2026-10-03 Rescan progress: EN/RU dictionary position/name, cleanup and interrupted-connection messages added. Reuses localized article counters. Server catalogs and Node localization/syntax checks pass; phone verification pending.

2026-10-03 Russian Rescan: explicit dialog language and consistently stacked equal-width actions fix mixed wrapping of longer Russian labels. Existing wording retained; EN layout unchanged. Chromium 320/360/390/1100px at 100/130% and targeted server checks pass; phone verification pending.

2026-10-03 Dictionary settings layout: For all dictionaries / Для всех словарей heading added; table reuses existing Create/Delete and index labels. Node i18n, targeted catalogs and EN/RU Chromium 320/390/1100px layout probes pass; phone/text-scaling check pending.

2026-10-03 Exit: last Android Settings row and native busy/wait dialog localized in EN/RU using the selected interface language. Node i18n checks and FOSS/Play Java compilation pass; device layout/language verification pending.

2026-10-03 article find: dialog/strip labels, modes, options, counts, wrap notices and hidden-example hint are catalog-backed in EN/RU. Node/catalog/asset and Chromium 320/390/1100px checks pass; no APK/device verification. Check Russian labels with the soft keyboard and increased text size, plus the strip count and example checkbox on the phone.

Updated 2026-10-02. Implementation rules are in [translation.md](translation.md). This file is the current task list, not permission to carry out every listed change automatically. Bulk Stop/finishing-current-dictionary/stopped-result labels added in EN/RU; Node and Chromium checks pass. Stop is a safe queue boundary, not cancellation of the active server ingest.

## Checkout state

2026-10-02 Language selector: own collapsible Settings section, current language self-name on the door; compact dialog at phone widths. Node/targeted I18n/AppearanceContract checks and isolated Chromium EN/RU 320/390/1100 layout checks pass; APK/device verification pending.

2026-10-02 compact Rescan dialog: shortened Create/index labels and explanations in EN/RU, title matches the command, header close and menu chevron added. Chromium confirms one-screen layout at 320/390/1100 × 700; Node checks pass. Device verification pending.

2026-10-02 unified Rescan / Update dictionaries: new/existing sections, mandatory base creation, optional Create checkboxes, Recreate/Update/Delete/Keep controls and exact-action explanations are in EN/RU. Separate Clear database door removed. Browser EN/RU 320/390/1100px and Node checks pass; APK/device verification pending.

**Release `wudict2-v0.6.0` published** (2026-10-02, at the user's request, from `dev`): the first normal release to carry the finished UI localization, so the translation ships in an APK for the first time outside the ru.1 preview. Tag on `bc04112`; `versionName='wudict2-v0.6.0'`, versionCode 468, `locales: '--_--' 'ru'`, `web/i18n/ru.json` confirmed inside the packaged APK, sha256 `934f8113c040179b9edb0cd4b169b0ba06473a3a3031ac1287c027a0cfc4f44f`, pre-release flag off (`latest` moves to it). The release body names the Russian interface as one of the two things to test. `go test ./internal/format/dsl` and the server's `TestGDAssets|TestGDStyle|TestI18n|TestAppearanceContract|DSL` pass; `make i18n-check-js` was still not run (no Node on this machine), and the phone pass below is still owed.

2026-10-02 follow-up after `10eaec5` on `dev` (clean at start): completed the DSL minimum-index prompt in Russian, added three DSL-defaults API error translations, and recognized known errors inside JSON responses read as text. Updated the collection import hint and made bulk confirmation counts grammatical for 1/2/5 dictionaries without changing parameters. Existing labels and Android resources were already translated. Targeted Go I18n/AppearanceContract, Node checks (including JSON/unknown fallback), and diff checks pass; no APK/device check.

2026-10-02 Edit Folders: new DSL parser section, explanation, minimum-index validation and save-error messages translated in EN/RU. Existing Original/GD labels reused. Browser 320/390/1100px and Node checks pass; device verification pending.

2026-10-02 bulk indexes: new Create/Delete sections, All/Original/GD scopes, confirmation, no-op and completed/failed counts translated in EN/RU. Node candidate/locale tests and Chromium 320/390/1100px bulk create/delete/layout checks pass; no APK/device verification.

2026-10-02 translation follow-up: `dev` at `af82000`, clean at start. Added catalog-backed orphan review, selection/action counts, warning and result messages; collection select-all/none links; newly introduced collection/Drive/removed-index errors; eight missing main Android resource translations. Existing DSL labels and user wording were preserved. Go I18n/AppearanceContract and Node checks passed; Android XML parsed and positional parameters matched the English resources. Main resource coverage is complete except the intentionally inherited app_name. New tests cover orphan selection labels and Russian counts; duplicate catalog keys are rejected. No APK or device verification.

- [ ] Check orphan review at phone widths: warning, previous path, mixed delete/keep selection, and partial failure. Deletion behavior is unchanged; use disposable test data if exercising deletion.
- [ ] Check native collection selection, the existing-import conflict dialog, and missing-browser message with the manually selected Russian language.

2026-10-02 DSL index management: `dev` at `19967aa`, uncommitted deletion shortcuts, independent original/GD Full text toggles, EN/RU confirmation and create-index warnings. Chromium verifies cancellation, cross-card targeting, search exclusion, explicit rebuild, independent full text and 320/390/1100px layouts in both languages; Go regression (including restart) and Node localization checks pass. No APK/device verification.

2026-10-02 DSL selection slice: `GD_DSL` at `074cf1e` before edits; new per-family checkboxes, bulk variant control, unavailable state and known DSL error messages have EN/RU translations. Go localization/appearance/preferences/DSL mode tests and Node catalog/card checks pass; Chromium verifies 320/390/1100px in both languages. No APK/device verification. Earlier snapshots below are historical.

Rechecked 2026-09-30 for server-message localization: branch `translation`, HEAD `45e99df`, clean at session start. Category localization and the README upstream merge are committed. The current uncommitted slice adds known server-error translations, tests, and documentation. The Settings label layout fix was committed as `156e802` (`Settings pane layout fix`). The label column is measured (`panelRowColumns()` in `index.html` → `--rlcol` on the section), the drawer's `#panel:lang(ru)` rule was removed from `i18n.css`, the Results strip no longer clips “Читать вслух,” and the three appearance buttons move as a single `iconrow` block. The separate `#styler:lang(ru)` toolbar rule remains. Rules and measurements are in `docs/ANDROID-UI-HANDOFF.md`, “The drawer's rows are read down ONE column per section”; verified in Chromium at 320/360/375/393/412px in both languages and with simulated 1.4× text zoom, **not verified on a phone**. Check the actual `git status` before working: this snapshot becomes outdated quickly.

**Preview `wudict2-v0.6.0-ru.1` published** (2026-09-30, at the user's request). Tag on `01a1438`, APK built with `build-android.cmd release`: `versionName='wudict2-v0.6.0-ru.1'`, versionCode 426, `locales: '--_--' 'ru'` and the `web/i18n/ru.json` catalog with Russian strings confirmed inside the APK, sha256 `d1af995b935fcefbcdf69cff0bdf38b02fef85dcb6dc46fe2f0ae84833b56935`, same signing certificate as previous releases. The release is marked **pre-release**, at publication, `latest` remained stable v0.5.0: https://github.com/DmShAl/wudict2/releases/tag/wudict2-v0.6.0-ru.1. The build comes from a branch that also contains all unreleased `dev` work, so the release notes describe that work too. Before publishing: `TestI18n` and `TestAppearanceContract` passed, `git diff --check` was clean, `make i18n-check-js` was **not run** — that release run reported Node unavailable; this does not supersede the earlier successful Node checks.

**Historical upstream gaps, resolved by the translation follow-ups above** (`dev` at `23670f3`, 2026-10-02): the dictionary-collection merge added eight Android strings with no `values-ru` entries yet — `intake_same_elsewhere`, `intake_other_elsewhere`, `intake_all`, `intake_other_running`, `intake_other_unnamed`, `intake_other_stop`, `intake_other_keep`, `intake_no_browser` — so they read English in the Russian app until translated. On the web side the five new keys (`pages.alreadyIn`, `pages.replacesIn`, `pages.downloadedFiles`, `pages.collectionExtras`, `pages.alsoFailed`) are already in `ru.json` and `pages.url` was extended in both languages; `pages.addHint`'s Russian still describes the old wording (a link to one file, not to a folder of them or a list). The orphan review's sentences arrived as literals in the page's JS, so they read English too until keyed.

Continue to commit, merge into `dev`, and publish only when requested; do not install in the emulator automatically.

## Already implemented

- [x] Manual English / Русский selection in Settings, English default, `language` persisted in `state.json`, applied through reload.
- [x] Catalogs, fallback, interpolation, plural forms, synchronous bootstrap, and language-aware page caching.
- [x] Browse, including empty states, counts, and navigation.
- [x] Main/FOSS/Play Android resources and resource contexts following the manual language choice.
- [x] Settings, appearance, built-in layer/look names, CSS editor, and related dialogs.
- [x] Dictionary settings and groups: cards, indexes, removal, confirmations, and known group-validation errors.
- [x] Folders: hints, server-generated introduction, import, download, and counts.
- [x] Lemmas: labels, installation/download states, confirmations, and displayed language names.
- [x] Search and results: modes, progress/empty/retry states, result plurals, index-creation offers, link-scope and morphology messages, and match navigation.
- [x] System Settings: web text, Advanced explanations, ranges, validation, reset/cache, and settings-application notices. Advanced rows themselves use already translated Android resources.
- [x] Read-aloud menu: read/stop, voice, unavailable voice, language names, and navigation to system speech settings.
- [x] Identified remaining UI strings: preference-saving and audio errors, stylesheet-file hints, file-replacement confirmation, and layer-switch errors.
- [x] In-article iframe windows: close label and two error states through a small host → frame contract.
- [x] Temporary Language buttons on Browse and Folders removed; the regular Settings button retained.
- [x] Settings label columns measured from rendered content; Results labels no longer use the old fixed width. Browser verification is recorded above; phone verification remains below.

A checkmark means implemented in code, **not** exhaustively tested on a phone.

## 1. Implemented: dictionary categories and filters

- [x] Translate Language, Language pair, Content, Publisher, and all nine current content categories using stable facet/value IDs on the client.
- [x] Share display labels between the dictionary selector, dictionary-information traits/tooltips, and scope labels read from selector options.
- [x] Display language names, bilingual pairs, and monolingual labels using `Intl.DisplayNames`; retain original labels when unavailable or unknown. English labels and publisher brands remain literal.
- [x] Preserve Go recognition rules, API metadata, group IDs, membership, and dictionary order. Node checks exercise actual grouping, trait rendering, scope naming, and selected dictionary IDs in both languages, including unknown values and missing `Intl.DisplayNames`.
- [ ] Verify the translated categories and their selection on a phone after rebuilding. Check long language-pair labels and confirm that the same category selects the same dictionaries in English and Russian.

Implementation is complete; device verification is still pending. Future upstream categories fall back to their original server labels until a translation is added.

## 2. Go/API messages: common cases implemented, deeper diagnostics remain

- [x] Add a shared display-only adapter for known legacy server messages: import/archive/download restrictions, missing paths/files, lemma configuration and download validation, preparation/removal, saved looks, styles, presets, and file saving.
- [x] Connect immediate errors and asynchronous import/lemma/ingest/search failures. Preserve filenames, paths, checksum values, diagnostic suffixes, unknown messages, and English responses.
- [x] Keep existing API response fields and operation behavior unchanged. The adapter uses bounded exact/prefix matches and explicit complete-message templates; see `translation.md` for its maintenance contract.
- [x] Verify catalogs, registered mappings, parameter preservation, unknown-error fallback, and actual import/lemma/search rendering with Go/Node checks. Escape saved-look application failures before passing them to the HTML status renderer.
- [ ] Audit deeper failures as they are encountered: archive/parser/OS/network diagnostics, malformed lemma catalogues, less common removal refusals, and authentication/host errors. Do not mechanically translate all Go strings or arbitrary diagnostic substrings.
- [ ] Verify representative failures on a phone: inaccessible folder, unsupported/password-protected archive, missing companion files, failed download, and failed saving. Local fixtures are not a device test.

`groupErrorText()` still handles group-specific validation. The Folders page now uses the shared `errorText()` adapter, including `folder not found`. This is not a universal error system; new API families should prefer stable error codes.

Completion: common expected errors have a translated, understandable explanation, unknown technical details remain available, and API values and operation behavior are preserved.

## 3. Audit for missed strings

- [ ] Check rare branches after the current stage: lost network, server refusal, read-only directory, unavailable path, damaged data, partial loading.
- [ ] Check lazy-loaded JS and Android bridge messages, not only `index.html`.
- [ ] Check dynamic `title`/`aria-label`: a successful JS syntax check does not catch every error in generated attributes.
- [ ] Review conditional/computed translation keys and indirect display helpers: the literal-reference scanner does not enumerate them. Check important branches through actual rendering tests.
- [ ] When the user supplies screenshots, record the exact screen, steps, English/Русский selection, and actual text. Find the string's source first, then fix the appropriate layer.

Do not treat intentional `Language`, `English`, `Русский`, English iframe fallbacks, dictionary/voice/publisher proper names, codes, configuration parameters, or console diagnostics as defects.

## 4. Device and appearance — verification still required

The user confirmed that the Language dialog worked and Browse changed language at an early stage. This does not confirm every subsequent screen.

- [ ] Reopen the search-mode picker and “All dictionaries” in an APK with the fixed `UiLanguage.context()`. Earlier logs confirmed `BadTokenException`; the fix compiled, but do not assume final user verification of a new build without their report.
- [ ] Switch English → Русский → English; check new pages, reload, application restart, and persistence of the choice.
- [ ] Confirm that Language remains only in Settings; the first-run Folders page no longer has a separate selector, at the user's request.
- [ ] Check phone widths: 320/360px, long labels, search modes, Examples buttons, System Settings, and the CSS editor. Specifically check Settings labels: `Оформления`, `Размер шрифта`, `Толщина шрифта`, `Компактно`, `Примеры`, and `Нижняя панель` must be fully readable on one line; appearance-row controls must begin at the same x coordinate; `Читать вслух` in Results must be fully visible with its pair beside it. A control should move to a second line only when that row's content really lacks room, and the three appearance buttons must wrap together.
- [ ] Check day/night, custom background color, wallpaper, and Quiet labels. Translation must not restore grey text to active controls or break window appearance.
- [ ] Check Russian counts: 1/2/5/11/21/22; limited results with `+`; long dictionary/group names and paths.
- [ ] Check import, notifications, and native dialogs in the required flavors; Android's system picker may remain in the system language.
- [ ] Check Advanced: inherited value, ranges, invalid input, restore, and the notice about applying settings on next opening. Do not change real settings solely for testing without necessity and an authorized context.
- [ ] Check read aloud with a real engine: voice selection, stopping, multiple voices for one region, unavailable voice, speech settings. English text must remain English with the Russian interface.
- [ ] Check expandable subentry links in an ordinary article and a JS/iframe article: closing, no result, and loading failure.
- [ ] Check double-tap/selection in English, Russian, and other available dictionaries: UI language must not change article segmentation.

Do not run these actions automatically in the emulator: the user has currently taken responsibility for verification.

## 5. Possible improvements — evaluate separately, do not mix with straightforward translation

- [ ] **Server-generated Folders introduction:** `setupPage` retains the English branch, while the other branch uses the catalog. When adding languages, evaluate a unified template path that preserves English compatibility. Do not rewrite it merely for symmetry.
- [ ] **Sentence fragments:** the Folders page and saved-path hint still have separate text fragments around DOM nodes. If the next language needs a different word order, move to a safe template with DOM parameters; do not insert unchecked HTML from the catalog.
- [ ] **Simplifying knowledge storage:** `docs/I18N.md` contains early stage history; rules now live in `translation.md`, and remaining work lives here. Do not maintain three independent TODO lists. Preserve verification facts during future cleanup without presenting them as new checks.
- [ ] **Key names:** do not rename working `panel.*`/`pages.*` and other namespaces in bulk without a reason. If a key is misleading, change it together with every reference and check.
- [ ] **A new language:** this is a separate task. The en/ru allowlist is currently repeated in the Go loader and normalization/validation, JS dialog, Android `UiLanguage`, checks, and `/api/language` schemas in `internal/server/web/openapi.yaml`. One new JSON file is not enough; Android resources, plural forms, fallback, and tests are needed. Keep static-slot keys as strings and check dynamic key sets too. Do not enable automatic system-language selection.
- [ ] **RTL:** not implemented. Direction, CSS, and menus require separate verification; do not promise Arabic/Hebrew support after adding only a catalog.
- [ ] **Main-page `html lang`:** do not switch it before separating the article-segmentation fallback from the UI language. Retaining `en` is currently intentional. This is an accessibility/language-architecture task, not a mechanical attribute replacement.

## 6. What was verified locally

Server-message slice: `node tools/i18n-test.cjs`, targeted `Test(I18n|AppearanceContract)` server tests, and `git diff --check` passed. New checks cover registered exact/prefix mappings, literal parameters, unknown-error fallback, and import/lemma/search failure rendering. No APK build, full-suite run, or device check.

At the last stage before these documents were created, the following passed:

```text
go test ./internal/server -run 'Test(I18n|AppearanceContract)' -count=1
node tools/i18n-test.cjs
git diff --check
```

Checks covered catalogs, served pages, plural forms, preservation of data/identifiers during rendering, Advanced validation and payloads, pronunciation-language independence, iframe label escaping, and article preservation. These are checks of individual paths, not end-to-end coverage of every scenario.

An APK was not built or installed during those translation stages. The later preview release did include an APK build, as recorded above; that build is not evidence of phone/emulator verification. The layout stage separately recorded Chromium checks, including simulated text zoom. The full test suite was not rerun during the final translation stages; known baseline problems are documented in `docs/WINDOWS-VERIFY.md`.

Category localization: targeted `Test(I18n|AppearanceContract)` server tests, `go test ./internal/facet ./internal/lang`, `node tools/i18n-test.cjs`, and `git diff --check` passed. The new Node cases cover labels, language pairs, unknown values, absent language-name support, HTML escaping, unchanged metadata, and identical selected dictionary IDs. No APK build or device check was performed.

## 7. Maintaining this file

After each stage, mark completed work, remove outdated descriptions of remaining work, and record new specific risks/unverified items. For verification, state the type of evidence: code/local test/browser/emulator/user report. Do not turn a local test into a claim about a phone.

Move rules useful to the next agent regardless of the current task into `translation.md`. Keep a brief status pointer in HANDOFF. Do not add the full diff, a list of every key, or repeated histories of every test run here.
