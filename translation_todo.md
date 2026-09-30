# Localization: status and next tasks

Updated 2026-09-30. Implementation rules are in [translation.md](translation.md). This file is the current task list, not permission to carry out every listed change automatically.

## Checkout state

Rechecked 2026-09-30 during the documentation audit: branch `translation_layout_fix`, HEAD `ace52db` (preview-release handoff). Uncommitted changes are limited to `translation.md`, `translation_todo.md`, and `HANDOFF.md`: English documentation and this audit. The Settings label layout fix was committed as `156e802` (`Settings pane layout fix`). The label column is measured (`panelRowColumns()` in `index.html` → `--rlcol` on the section), the drawer's `#panel:lang(ru)` rule was removed from `i18n.css`, the Results strip no longer clips “Читать вслух,” and the three appearance buttons move as a single `iconrow` block. The separate `#styler:lang(ru)` toolbar rule remains. Rules and measurements are in `docs/ANDROID-UI-HANDOFF.md`, “The drawer's rows are read down ONE column per section”; verified in Chromium at 320/360/375/393/412px in both languages and with simulated 1.4× text zoom, **not verified on a phone**. Check the actual `git status` before working: this snapshot becomes outdated quickly.

**Preview `wudict2-v0.6.0-ru.1` published** (2026-09-30, at the user's request). Tag on `01a1438`, APK built with `build-android.cmd release`: `versionName='wudict2-v0.6.0-ru.1'`, versionCode 426, `locales: '--_--' 'ru'` and the `web/i18n/ru.json` catalog with Russian strings confirmed inside the APK, sha256 `d1af995b935fcefbcdf69cff0bdf38b02fef85dcb6dc46fe2f0ae84833b56935`, same signing certificate as previous releases. The release is marked **pre-release**, at publication, `latest` remained stable v0.5.0: https://github.com/DmShAl/wudict2/releases/tag/wudict2-v0.6.0-ru.1. The build comes from a branch that also contains all unreleased `dev` work, so the release notes describe that work too. Before publishing: `TestI18n` and `TestAppearanceContract` passed, `git diff --check` was clean, `make i18n-check-js` was **not run** — that release run reported Node unavailable; this does not supersede the earlier successful Node checks.

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

## 1. Still to translate: dictionary categories and filters

- [ ] Inspect `internal/facet/facet.go`: displayed `FL`/`VL`, language names, and content-category names.
- [ ] Check all consumers: grouping in the dictionary selector, `traits()` and category labels in dictionary information, `groupLabel()`/search scope. A category must have the same name in different places.
- [ ] Preferred option to evaluate: translate display text on the client using stable `F`/`V`, with fallback to server labels. This is a proposal, not an already accepted decision.
- [ ] Do not change IDs, name-recognition rules, group membership, English `internal/lang` tables, or publisher brands. Verify that `g:...` and dictionary lists remain unchanged when the UI language changes.
- [ ] For language names, consider the existing `Intl.DisplayNames` approach with a safe fallback. Do not translate data fields used for recognition/search.

Completion: category/language labels are translated in the Russian version, selecting the same categories returns the same dictionaries, and unknown upstream values remain visible with their original names.

## 2. Still to translate: Go/API messages

- [ ] Collect messages actually visible to readers during import, index preparation, removal, downloads, lemma handling, path/configuration handling, and file saving. Do not begin by replacing every English string in Go.
- [ ] Separate the user-understandable cause from technical diagnostics (`HTTP`, OS/library text, addresses, paths).
- [ ] Choose a contract for each family: stable error code + parameters translated on the client, or localization in a specific server HTML path. A general per-request locale is not implemented and is not automatically required.
- [ ] Preserve client compatibility when extending APIs. Do not replace semantic values with Russian strings or break English API tests merely for localization.
- [ ] Check both languages and an unknown error: it must remain visible rather than disappearing behind an unknown key.

Local solutions already exist: `groupErrorText()` maps known group messages and leaves unfamiliar ones unchanged; the Folders page translates the known `folder not found` message. This is not a universal error system. Do not spread long-text matching without restraint; evaluate stable codes when extending it.

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

At the last stage before these documents were created, the following passed:

```text
go test ./internal/server -run 'Test(I18n|AppearanceContract)' -count=1
node tools/i18n-test.cjs
git diff --check
```

Checks covered catalogs, served pages, plural forms, preservation of data/identifiers during rendering, Advanced validation and payloads, pronunciation-language independence, iframe label escaping, and article preservation. These are checks of individual paths, not end-to-end coverage of every scenario.

An APK was not built or installed during those translation stages. The later preview release did include an APK build, as recorded above; that build is not evidence of phone/emulator verification. The layout stage separately recorded Chromium checks, including simulated text zoom. The full test suite was not rerun during the final translation stages; known baseline problems are documented in `docs/WINDOWS-VERIFY.md`.

This documentation audit checked the guide against the current implementation and corrected the checkout snapshot and verification boundaries. It did not change application code or rerun runtime tests, build an APK, or perform device checks.

## 7. Maintaining this file

After each stage, mark completed work, remove outdated descriptions of remaining work, and record new specific risks/unverified items. For verification, state the type of evidence: code/local test/browser/emulator/user report. Do not turn a local test into a claim about a phone.

Move rules useful to the next agent regardless of the current task into `translation.md`. Keep a brief status pointer in HANDOFF. Do not add the full diff, a list of every key, or repeated histories of every test run here.
