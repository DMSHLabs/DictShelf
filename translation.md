# Translating the wuDict2 interface

This is a working guide for an agent continuing localization or fixing related bugs. Read it **before changing translations**. Current status and unfinished tasks are in [translation_todo.md](translation_todo.md).

Keep both documents in English. Russian UI examples are intentional; reusable rules belong here, while pending work and verification evidence belong in the task list.

Project rules remain in [AGENTS.md](AGENTS.md), [CLAUDE.md](CLAUDE.md), [HANDOFF.md](HANDOFF.md), and the relevant documentation sections. This file covers localization specifically; it does not replace those documents. Historical verification of earlier stages is preserved in [docs/I18N.md](docs/I18N.md).

## 1. User decisions — do not reconsider by default

- Translate **only the interface**. The language of a dictionary, article, search, and pronunciation is independent of the interface language.
- The language is selected manually: **Settings → Language → English / Русский → apply and reload**. English is the default.
- Do not infer the interface language from the system, Android Locale, `navigator.language`, `Accept-Language`, region, or dictionary.
- Live switching is not required. Reloading the page is the intended way to apply the choice. Restarting Go is not required.
- The `Language` button stays in Settings. Its English label and the self-names `English` / `Русский` are intentional: readers must be able to find the selector even in an unfamiliar language. Temporary buttons on Browse and the Folders page were removed at the user's request; do not restore them for testing convenience.
- The setting is shared by the server installation, not separate for each browser. Already open pages change language after reloading.
- The user currently builds the APK and checks it on a device themselves. Do not routinely build an APK, install it, or operate the emulator without a new request. Lightweight local checks are allowed. Earlier installation permission from a previous stage is not a request to install every new version.
- Work on the currently selected branch and preserve uncommitted changes. When this document was created, that branch was `translation`; this name is not an instruction to switch branches. `dev` is for integration; `master` is for upstream. Commit, merge, and publish only at the user's request.

## 2. Implementation map

| Area | Files and purpose |
| --- | --- |
| Web interface catalogs | `internal/server/web/i18n/en.json`, `ru.json`: flat semantic keys, each holding a string or an object of plural forms |
| Server infrastructure | `internal/server/i18n.go`: embedding, catalog loading, fallback, `renderUI`, GET/PUT `/api/language` |
| Persisting the choice | `internal/server/prefs.go`: top-level `language` field in `state.json`, separate from `ui` |
| Client infrastructure | `internal/server/web/i18n.js`: `window.wudictI18n`, selection dialog, saving, and reload |
| Shared presentation rules | `internal/server/web/i18n.css`: language dialog and CSS editor control wrapping (`#styler:lang(ru) #stylerDock`). This file no longer adjusts translated label layout (see “Label widths and layout”) |
| Main screen | `internal/server/web/index.html`: search, results, Settings, system settings, and appearance |
| Standalone pages | `web/setup.html`, `lemmas.html`, `browse.html`; handlers in `internal/server` |
| Groups and looks | `web/group-editor.js`, `looks.js`; display names separate from stored names |
| Read aloud | `web/speak.js`: labels and displayed language names; the pronunciation selection algorithm is separate |
| Articles in iframes | `frameDoc` in `index.html`, `web/frame.js`: three interface labels passed through bridge-script attributes |
| Android | `android/app/src/main/java/com/legbehindneck/wudict/UiLanguage.java`, `values-ru/strings.xml` resources in main and flavor sets |
| Checks | `internal/server/i18n_test.go`, `tools/i18n-test.cjs`, Makefile: `i18n-check`, `i18n-check-js` |

The `web/...` paths in the table are relative to `internal/server/web/`.

The project has no npm build step for translation. Do not add an i18n framework, gettext, generated `index.ru.html` copies, or a separate catalog loader for the next small stage. Use the existing infrastructure.

## 3. Adding translations

### Catalogs

1. Look for an existing key with the appropriate **meaning**, not merely the same English word.
2. If a new key is needed, add it to both `en.json` and `ru.json`.
3. Use a stable semantic name. Existing namespaces include `language.*`, `browse.*`, `panel.*`, `dictUI.*`, `layers.*`, `pages.*`, `search.*`, `system.*`, and `speech.*`.
4. Keep parameter names identical in both languages. Catalogs contain neither executable code nor HTML markup.
5. Supply complete sentences with parameters. Do not assemble Russian sentences from English grammatical fragments.

Example:

```json
"panel.replaceFile": "{name} is already there. Replace it?"
```

```json
"panel.replaceFile": "Файл «{name}» уже существует. Заменить его?"
```

```js
confirm(tx("panel.replaceFile", {name}));
```

Do not rename a key every time its wording is edited. Field, mode, and file names do not automatically become translation keys.

### Static markup

Use explicit slots:

```html
<h2>{{T:system.title}}</h2>
<button aria-label="{{T:panel.close}}">✕</button>
```

`renderUI` escapes text for HTML. Slots are valid in text nodes and **quoted** interface attributes. Do not insert them into JS, CSS, URLs, or dictionary content.

Static slots use `uiText()`, which accepts string values only: it neither selects plural forms nor interpolates parameters. Render counted or parameterized messages through the appropriate caller; do not place a plural-object key in a static slot.

The page must contain `{{I18N}}`, and its HTTP handler must call `renderUI` with the saved language. Slots in the HTML alone do not mean translation is connected.

### Dynamic JavaScript

```js
const tx = window.wudictI18n.t;
node.textContent = tx("search.noQuery", {query});
```

`t()` returns **plain text**. Prefer `textContent`. If existing code builds HTML, escape the result:

```js
box.innerHTML = `<p>${esc(tx("search.noQuery", {query}))}</p>`;
buttonHTML = `<button title="${escAttr(tx("panel.close"))}">✕</button>`;
```

Each page has its own helpers. Check that the required function exists and escapes for the intended context; do not assume `esc` and `escAttr` are interchangeable.

On the main page, `setStatus()` accepts HTML, whereas `stylerNote()` and `sysNote()` output text. `setStatus(esc(tx(...)))` is required even without user-supplied parameters: a translation must not become markup either.

**An actual previous bug:** an HTML template received `title=tx("...")` instead of interpolation. The JavaScript was syntactically valid, but the browser received an incorrect attribute. Do not replace strings globally throughout a file without distinguishing HTML, JS, templates, and comments. Review the diff after mechanical extraction.

Parameters are substituted in one pass. A dictionary named `My {name}` must remain exactly that, without another substitution. Do not add a second interpolation pass.

Preserve initialization order. The synchronous i18n bootstrap must run before code reads `window.wudictI18n.t`. Group state is initialized later by `group-editor.js`; early main-page rendering must not eagerly read `userGroups` before it is initialized. Keep the existing early-render fallback in `scopeLabel()` and check startup as well as reopening a menu when changing these helpers.

### Label widths and layout

English and Russian labels have different widths, and translations must not be squeezed into a fixed size. Pixel-based `flex-basis`/`width` values tailored to a particular string, a media query “for Russian,” or a `:lang(ru)` rule allowing wrapping inside a column produce the opposite of the intended result: a long label breaks at a space or in the middle of a word even when the drawer has enough room. This happened in Settings: a 60px column (the width of the longest ENGLISH label, “Font weight”) plus `#panel:lang(ru) .facts>.rowlabel{white-space:normal;overflow-wrap:anywhere}` in `i18n.css` caused “Толщина шрифта” to wrap at the space, broke “Оформление” and “Компактно” after “Оформлен” and “Компактн,” and the fixed 96px width in the Results strip clipped “Читать вслух” to “Читать в…”.

Since 2026-09-30, both are measured: `panelRowColumns()` in `index.html` uses a `Range` to measure the label content in each drawer section and publishes the longest width as `--rlcol` on that section. Label `flex-basis` uses that value, aligning labels in one column and making the controls of every row in the section start at the same x coordinate. A row wraps only when its own content no longer fits, and only that row wraps. Current rules and all measurements are in `docs/ANDROID-UI-HANDOFF.md`, “The drawer's rows are read down ONE column per section”.

Consequences when adding a translation:

- Do not add language-specific drawer label widths or wrapping rules. Let the measured column accommodate accurate wording; do not shorten a translation solely to fit the old fixed column. The existing `#styler:lang(ru) #stylerDock` toolbar-wrapping rule is a separate exception, not a pattern for new labels; any generalization needs its own layout checks.
- If a new label uses the drawer's row markup (`.rowlabel` or `.seg>span` inside `#panel section.sect`), it is included in the measurement automatically. Labels outside that markup are a separate case: check whether they have a fixed width.
- “Fits in a 96px/60px column” is not a verification: the length of a label in a particular language is only visible when rendered. Inspect it in a browser at 320/360/393px in both languages, checking that it is neither clipped (`scrollWidth` versus `clientWidth`) nor wrapped when enough room is available.
- Long button and segment labels (`Показать`/`Скрыть`) are acceptable: the container moves the whole control onto a second line instead of clipping the text or separating the pair.
- Account for Android system font scaling (text zoom): labels grow with it, and the measured column grows with the labels. The layout must remain on one line where space permits, rather than breaking words internally.

### Numbers

For counted nouns, use a plural-form object and `Intl.PluralRules`, already used by `i18n.js`:

```json
"search.results": {
  "one": "{number} результат",
  "few": "{number} результата",
  "many": "{number} результатов",
  "other": "{number} результата"
}
```

```js
tx("search.results", {
  count: n,
  number: window.wudictI18n.number(n)
});
```

`count` is the original number used to choose a form; `number` is its human-readable representation. For example, a count with a `+` suffix in limited results must not be passed as `count`. Do not use English `n === 1 ? "" : "s"` logic or write custom Russian plural rules.

Neutral messages such as “Installed: 2 of 10” do not need separate plural forms.

Fallback: a missing key is taken from the English catalog; an unknown key is displayed as the key itself. This is a runtime safeguard; shipped catalogs must remain complete and pass the checks.

## 4. Persistence, bootstrap, and caching

- The source of truth is `state.json`, the **top-level `language` field**, not `ui.lang` or localStorage.
- GET/PUT `/api/language` updates only the language. Older tabs saving `/api/prefs` must not overwrite the new choice.
- Saving uses the existing preference locks and rolls the value back on a write error. Do not duplicate writes in Android.
- The catalog is embedded synchronously through `{{I18N}}`: JSON in `script#wudict-i18n`, followed by shared JS and CSS with hashes. Do not replace this with asynchronous fetch without a separate task: page code uses `tx` immediately, and late loading causes mixed languages and a flash of English.
- `basePage()` is memoized per process. Main-page language is applied in `pageFor()`, after shared preparation; the cache key and ETag include the language.
- For new pages, check their own handler, bootstrap, and absence of unresolved slots in the HTTP response. Pages and hashed assets have different cache policies; do not bypass them with a manual timestamp.
- The server-generated Folders introduction currently retains its original English branch and uses `pages.*` translations for the selected non-English language. This is a local implementation, not a general localization system for all Go/API errors.

## 5. What must not be translated

Do not change the following for localization:

- headwords, article HTML and text, dictionary About content, file/dictionary names, paths, and URLs;
- user-defined group and saved-look names;
- JSON, `state.json`, Android bridge, configuration, and HTTP parameter keys;
- values such as `exact`, `prefix`, `contains`, `fts`, `all`, `g:...`, loading states, and action identifiers;
- language codes, recognition tables in `internal/lang`, voice identifiers, and speech-engine values;
- CSS filenames and preset IDs, manifest fields, theme markers, and group mappings;
- publisher brands and proper names supplied by a voice provider.

Built-in looks and layers are translated **for display** by stable ID. The `looksDisplayName`, `layerText`, and `layerTitle` helpers leave user-defined names and unknown upstream additions unchanged. Do not write Russian names back into the manifest or user data.

`All Dictionaries` is a reserved group. Translating its label does not mean renaming the group in the API or changing the reserved-name rule. Likewise, “содержит” is a label, while `contains` is an unchanged mode/feature.

## 6. Interface language versus article language

This is a critical boundary:

- The main document currently retains `<html lang="en">`. `pick.js` inherits language for word segmentation; simply switching this attribute to `ru` changes how dictionary articles are processed.
- Apply the selected language to **interface containers**: dialogs, panels, and speech menus. Not to a shared container that includes articles.
- The standalone Browse, Folders, and Lemmas pages use the selected UI language because they do not have the same article-segmentation path.
- Do not run a general DOM traversal replacing English words. Do not pass articles through `renderUI` or `t()`.

### Articles with JavaScript in iframes

`frameDoc()` builds `srcdoc`, and `frame.js` runs inside a separate document. It must not receive the entire catalog or depend on access to the parent's `window.wudictI18n`.

Only the required labels are passed to the bridge script through escaped attributes:

- `data-ui-close` → `dataset.uiClose`;
- `data-ui-not-here` → `dataset.uiNotHere`;
- `data-ui-load-failed` → `dataset.uiLoadFailed`.

On the receiving side, messages are assigned through `textContent`, and the close tooltip through `.title`. English fallback strings support older host code; do not treat them as missing translations or remove them blindly.

If another label is needed, extend this small contract. Check quotation-mark and `<script>` escaping, preservation of original article HTML, and the unchanged iframe document language.

## 7. Android: resources and an already fixed crash

- Android strings stay in resources. Main and flavor translations must match their own base resources: FOSS and Play have different capabilities.
- `UiLanguage.selected()` reads `.wudict/state.json` relative to `AppDirs.home(context)`. Go owns writes to that file. Unsupported or absent values mean English.
- Existing activities/the service use `UiLanguage.resources()` for application resources. Check `%1$s`, `%1$d`, plurals, XML escaping, and intentional inheritance of the `app_name` brand.
- Do not use `Locale.setDefault`: it affects pronunciation and other mechanisms outside the interface.
- Native windows that are already open may need to be closed and reopened. Reloading the web page alone does not promise to recreate every Android window.
- The system file picker, permissions, and system speech-settings page belong to Android. Their language is not controlled by this translation.

**Fixed defect:** the search-mode and “All dictionaries” buttons crashed with `BadTokenException: token null`. The cause was using a bare `createConfigurationContext()` as the dialog context: it lost the Activity's window services.

The current `UiLanguage.context()` creates a `ContextThemeWrapper` over the original Activity context and calls `applyOverrideConfiguration`. Preserve this approach. `createConfigurationContext()` is suitable for retrieving resources, but must not simply replace the Activity passed to a dialog. Do not “simplify” the code back to the broken form.

A native dialog can contain text supplied by JavaScript as well as Android resources. For picker labels, trace `livePickerPayload()` and the display-name helpers in `index.html` through the bridge to `DictionaryPicker.java`. Changing `values-ru/strings.xml` alone cannot translate text supplied in a payload. Keep user names and IDs intact, and retain English bridge fallbacks for missing fields.

### System Settings

`Shell.systemState()` builds Advanced-row names and hints from Android resources. The web page translates its own text, ranges, confirmations, errors, and the stale-settings notice.

Do not copy the Advanced parameter table into JavaScript: its contents depend on the flavor. Do not change `row.key`, `kind`, `onValue`, numeric bounds, or `sysCommitNumber` rules for translation. Do not add an immediate-restart button: this window's design and reasons are documented in `docs/ANDROID-UI-HANDOFF.md`.

## 8. Read aloud

`speak.js` separates two different questions:

1. **How to label languages and actions:** `tx()`, `langName()` using `Intl.DisplayNames` with the UI language, and `voiceLabel()`.
2. **Which language to pronounce the text in:** `decide`, `userLang`, `pickVoice`, dictionary languages, the selected text's script, and the stored voice URI.

Change the first layer, not the second. The presence of `navigator.language` in pronunciation rules is not itself a localization bug: the user prohibited making the **interface** depend on the system, not requested a change to the speech algorithm.

The Android bridge supplies generated language/region voice labels; `voiceLabel()` localizes their display and the online marker while retaining the voice object and identifier. In an ordinary browser, a voice name may be a brand and remains literal. Missing `Intl.DisplayNames` must not break the menu: the fallback is the language code.

## 9. Finding remaining strings and working with upstream

Search not only for text between HTML tags, but also for:

- `title`, `aria-label`, `placeholder`, and `<option>` labels;
- `textContent`, `innerHTML`, `setStatus`, `stylerNote`, `sysNote`, `confirm`, and `alert`;
- HTML inside JS templates, error conditions, empty states, and rare loading branches;
- strings arriving from Go/Android, iframes, and lazy-loaded files.

Do not treat every English literal as an omission: protocol values, CSS, comments, console diagnostics, and dictionary data are nearby. Change a string only after verifying its path to the user.

To keep future merges manageable:

- preserve functions, DOM IDs, handlers, APIs, and execution order;
- do not mix string extraction with major refactoring, whole-page formatting, or a UI-framework change;
- keep the English catalog clearly traceable to upstream;
- port upstream behavior changes, then connect their new text to the catalogs;
- reconsider a key's meaning when a function changes, rather than attaching an old translation to new logic;
- review new English strings again: catalog-completeness tests do not detect every raw literal.

The technical upstream-sync and `merge-work` procedure is documented in HANDOFF and the relevant area documentation. This document does not authorize merges/commits or change Git policy.

## 10. Checks and completion criteria

Quick checks:

```text
go test ./internal/server -run TestI18n -count=1
node tools/i18n-test.cjs
go test ./internal/server -run TestAppearanceContract -count=1
git diff --check
```

The first two have equivalents: `make i18n-check` and `make i18n-check-js`. Node is needed only for checks; npm and package installation are not required. If Node is not in PATH on this Windows machine, its path can be found in `docs/WINDOWS-VERIFY.md` or the available runtime configuration; do not add a machine-specific absolute path to application code.

Go checks cover catalogs, parameters, plural forms, explicit key references, persistence, cache/ETag, defaults/fallback, and served pages. When adding a separate JS/HTML file, include it in the checked-source list in `i18n_test.go`.

Reference scanning is limited to static slots and literal calls such as `tx("panel.close")` or `t("panel.close")` in that list. It does not enumerate keys selected by a conditional, concatenation, or an indirect helper. Review those key sets explicitly and cover important dynamic branches with rendering tests. Catalog parity does not prove that every call resolves to a key, and neither check detects all untranslated raw literals.

Node checks cover syntax and selected real rendering paths: names, cards and results, plural forms, Advanced validation, pronunciation independence from UI language, and iframe creation with safe labels. When changing these paths, extend the relevant test rather than merely checking that a string exists in source.

Syntactic success **does not prove** that HTML attributes are correct, a menu fits a phone, or a native dialog does not crash. Record device results separately. Do not claim verification for branches that were only read in code.

The full test suite has known Windows baseline failures; see `docs/WINDOWS-VERIFY.md`. Do not fix them incidentally or hide them behind “everything passed.” Compare a new failure with the original state instead of automatically attributing it to the baseline.

After the work:

1. Review the diff: identifiers, user data, English fallback, and escaping.
2. Run proportionate local checks; state separately whether an APK was built and a device test performed.
3. Update [translation_todo.md](translation_todo.md): completed work, remaining work, and unverified items.
4. Record changed rules/architecture here. Do not copy the current TODO back into this file.
5. Briefly update HANDOFF within its limit, retaining a link to these documents. Do not inflate HANDOFF with a list of every translation key.
