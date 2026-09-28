# Changelog

Notable changes in **wuDict2**, the Android fork of
[WuWeiDict](https://github.com/wuweidict/wudict), relative to the upstream
version it was forked from. Fork release tags are prefixed `wudict2-`; one
tagged line per release, newest first.

## wudict2-v0.5.0 — 2026-09-28

Still no upstream sync — upstream `master` is unchanged at `5f0ad02`.

### A layer's paper reaches the rest of the app

- **Sepia, High contrast and the night halves of True black and Warm dark now
  paint the Folders, Lemmatization and Browse pages too**, and the window
  behind them. Those four layers repaint the app's own paper, but until now the
  colour stopped at the app page and the group editor — the other pages stayed
  white while the window beside them was warm. The colour now travels from the
  preset, through the appearance bridge, to the shell and down to each page,
  resolved for the theme you are in, and it is there before the page is painted.
- **The checkboxes on Edit Folders and Lemmatization are drawn**, in the app's
  own square: ink border, drawn tick, and the same ground as the fields beside
  them. So they wear a paper instead of staying platform-white, and a wallpaper
  shows through them. The group window's boxes already turned with the paper;
  these two were the last that did not.
- **The notes the app draws while you work are darker.** The scope line, the
  "widened" notice and the morph note used the faintest tone in the system,
  which measured 2.9:1 against the light paper.

### The status bar

- **The walk no longer wraps.** ▲ is spent at the first dictionary and ▼ at the
  last, and a spent arrow is dimmed rather than removed — the same call the font
  steppers make at their bounds.
- **Every jump re-aligns its landing** for a moment afterwards, so a section
  that is still growing — it expands, a frame measures itself on load, images
  arrive later — cannot leave its header halfway down the screen. Measured after
  the fix: every jump, up or down, into a collapsed section or an open one,
  lands the header on the same line.
- **The dictionary chip moved into the bar**, between the arrows and the
  Examples fold. It is out of the search field on a phone, and it travels back
  into the field whenever the bar is hidden. In the panel its place is a
  drop-down of your groups, under the name "Dictionaries".
- The bar's spacing came from a phone: 10px gutters at the ends, a floor of one
  square of air on **each** side of the chip so it cannot press against an
  arrow, and the fold's handle shortens to "Ex" on a narrow screen rather than
  pushing the group's name out of the row.

### The panel and the page's top edge

- **"Open first: My order | Fastest" is gone** from the panel. The stored value
  is no longer read, so a reader who once chose "Fastest" is not stranded on it.
- **The empty band above the first dictionary is gone.** The page's top space is
  the bar's measured height at every width, and the first section now sits flush
  against the bar — 34px removed at 1100px and 20px on a phone. Between 601 and
  800px, where the old fixed value was 11px too small, the first card no longer
  starts above the bar's lower edge.
- The stuck section header no longer animates its offset: an offset that arrives
  after the edge it aims at is an offset that is under it, which is the header
  that was seen tucked under the bar while scrolling up.

### Android

- **"Show info messages"** in the shell's settings, on by default, turns off the
  notes that pass in a moment and cannot be read: "Starting wuDict2…", the
  waiting art and its "N of M ready" counter, and the lookup window's
  "Looking up …". A failure is not one of these notes — the error sentences stay
  in either window — and the morph note **stays** too: it says the answers are
  for a different form of the word, which is read rather than skimmed.

## wudict2-v0.4.0 — 2026-09-27

Still no upstream sync — upstream `master` is unchanged at `5f0ad02`. One
change this cycle, and it is fork-only.

### A status bar at the bottom of the screen

- **A fixed bar holding three controls and nothing else**: ▲ and ▼ walk the
  dictionaries of the current answer — opening each one as it lands, and
  wrapping at the ends — and an **Examples** chip folds and unfolds the
  example-only lines without leaving the article.
- It exists because the drawer covers most of a phone's width, and a control
  you judge by watching the text move cannot live there. Everything on the bar
  also exists in the drawer — the fold has its row, the jumps have the picker's
  list — which is what makes hiding the bar safe. Its own switch is a row
  there too, **Status bar: Show | Hide**, beside the controls it duplicates.
- **The arrows use the picker's jump.** A dictionary reached by an arrow is as
  much a chosen destination as one picked off the list, so it opens and lands
  instantly rather than gliding; the arrows walk the dictionaries in your own
  order and skip the "could not be searched" row, which is a reference and not
  a place to read.
- **It appears with the first results, not before** — an empty page has nothing
  to walk. It is on by default and hides itself where it has nothing to do:
  taken away entirely where there is nothing to walk, and down to the chip
  alone when only one dictionary answered.
- It wears the bottom inset itself, exactly as the top bar wears the top one,
  so it stands clear of the gesture bar with **no Android-side change**. The
  wallpaper layer paints it as the same piece of paper as the top bar, and the
  page's bottom space follows the bar's measured height — so showing or hiding
  it re-flows the text instead of leaving the last line behind it.
- The chip is one button with two states, and it is lit while the examples are
  **shown** — the same answer the panel's Show half gives, said in colour. The
  choice of whether the bar is there is per window, like wide mode; it is not a
  reading habit that follows you to another device.

## wudict2-v0.3.0 — 2026-09-27

No upstream sync in this cycle — upstream `master` still stands at `5f0ad02`,
so the upstream work described under v0.2.0 is unchanged. Everything below is
fork-only.

### The panel's controls state themselves

- **"Sort dictionaries" is gone**, and with it the choice it offered: the
  picker lists the library in your own arrangement and no other — the order
  you drag the cards into, which is also the order the search already
  followed. A sort preference left in an older `state.json` is not read.
- **"Read aloud" is a label and an On/Off pair** like the rows beside it; the
  label keeps its own colour, because the pair is what states the state.
- The three Results rows line up now: their label column had been shrinking to
  each row's own text, so the names and both button columns started at three
  different x positions.
- The drawer's **section rules are drawn from the ink** instead of borrowing
  the faintest tone in the system, which every card border also uses. On the
  light themes they measured ΔL* 7.6 and 8.7 against what is behind them —
  invisible on a phone — and are now 16.2 and 12.9, in the same band as the
  dark ones. The shared tone itself is untouched.

### Compact and Examples move onto the panel

- **Compact is a row on the panel**, under Font weight, instead of a checkbox
  hundreds of pixels down in Style layers. It stays a layer: the panel's pair
  and the sheet's checkbox are two handles on one switch, both painted from
  the same payload, and the row hides itself where there is nothing to switch.
- **A new Examples row, Show | Hide, sits under it** and folds away the lines
  that hold nothing but examples, leaving the entry's own text. A definition
  line that merely contains an example keeps its line.
- The two halves of that feature are deliberately separate: as an article is
  built, the lines that hold nothing else are **marked**, and the layer's own
  article rule is what hides them. So folding back is just taking the rule
  away — the examples return at once, in the articles already on screen, with
  no re-render and no second pass.
- Otherwise it is an ordinary style layer: state in `presets.json`, applies in
  both themes, article side only. **Nothing in the stored library changes**, so
  no dictionary rebuild is offered and existing libraries need nothing done to
  them.

## wudict2-v0.2.0 — 2026-09-26

Also carries everything merged from upstream since v0.1.0 — upstream
**v3.7.5** and the commits after it: the Browse A–Z word lists, full-text
query changes, index-version tracking with reindex prompts, configurable
dictionary groups, double-tap search, read-aloud fixes. Everything below
is fork-only work on top of that.

### Settings, reorganized

- **The ☰ panel is five sections** — Dictionaries, Results, Appearance,
  History, Info. The single "Folders & setup" fold is gone; the folder paths
  move to Info, next to About.
- **A Dictionary settings window** owns the per-dictionary cards, opened from
  "Edit dictionary settings" beside "Edit dictionary groups": name, format,
  entry count, capability chips, provenance, a Browse link and an "About this
  dictionary" fold-out. Collection-level actions (edit folders, rescan,
  lemmatization, full-text for every dictionary) stay in the panel, and the
  window remembers your scroll position.
- **Dictionaries can no longer be switched off.** The "Enable all" master
  switch and the per-card toggles are gone from the UI, and the page no longer
  reads the stored flag, so a dictionary switched off in an older version
  cannot be left invisible with nothing left to switch it back on. Every
  installed dictionary is searched.
- **Setting rows read as a label and a choice** — "Highlight matches" On/Off,
  "Open first" My order/Fastest, "Sort dictionaries" Alphabetical/My order;
  history length and Clear get their own section.

### Day and night

- **The theme control is two buttons** — Auto, and a state button whose label
  names the action ("Switch to night"). The accent marks which of the two is
  in force, so "following the phone" and "pinned" are no longer the same
  press. Storage is unchanged, so nothing needs migrating.
- **Appearances are per theme.** Your app and article sheets are two pairs
  now (day and night) with only the resolved one linked, so a day sheet no
  longer paints over a dark page; the window colour and wallpaper are per
  theme in the shell too, applied before the page exists. A third built-in
  wallpaper, `paper_03.jpg`, is the dark paper.
- **Style layers are per theme.** The presets manifest gained night slots:
  True black and Warm dark are night-only, Sepia and High contrast day-only,
  Background image has a file for each, and a layer with no file for the theme
  you are in contributes nothing. The conflict rule is an overlap test now, so
  **Sepia by day and True black by night can be on together**.
- A layer that cannot take effect where you are shows as a dimmed row with the
  reason on it ("Light theme only — the page is in the dark one"), and enabled
  article layers are loaded at start-up instead of waiting for the Appearance
  sheet to be opened once.

### Saved appearances

- **A whole appearance can be saved under a name and applied again.** The
  panel's Presets row is the drop-down plus save, update and delete; it reads
  "Custom" when the screen no longer matches anything saved, and switching
  away from unsaved changes asks before discarding them. Three ship built in —
  **Clean**, **Warm** and **Old paper** — and only your own can be updated or
  deleted.
- A preset covers text size and weight, which style layers are on, the
  per-theme window colour and wallpaper, your sheets, and the screen edges. It
  deliberately excludes the theme, so applying one at night does not turn the
  lights on.
- The style-layer door is renamed **"Style layers…"**, because "Presets" now
  means the saved appearance.

### Appearance sheet

- **One subject at a time**, named in the sheet's own head and opened from its
  door in the panel: Edges of the screen, Window background, Style layers,
  Custom CSS. The sheet is non-modal, so the page stays visible while you
  adjust it.
- The screen rows are **radio groups with every option visible** instead of
  drop-downs, and the margin-colour field sits on its option's row, dimmed
  when that option is not the chosen one.
- **Font weight** (Normal, Medium, Bold) joins font size and reaches article
  text as well as the app's own.
- **A built-in colour picker** (RGB sliders, swatch, hex preview) replaces the
  platform chooser, which could not open at the field's existing colour.

### Dictionary picker and search bar

- **The picker is one list** — the dictionaries that answered the current
  search, in the order their sections appear. The All/Found toggle is gone
  from the page and from the shell, and no stored mode is read any more.
- Rows are exactly what is on screen, and the group drop-down names the
  dictionary being searched when the scope is a single dictionary.
- **Picking a dictionary jumps to it instantly**, whatever the system's
  reduced-motion setting says; the other two scrolls still follow it.
- The search bar's controls are separated by hairlines, the scope chip reads
  "All" instead of a bare glyph, and the caret is handed back to the field only
  when the URL carries no `?q=` — a word opened from a word list no longer
  comes up under the history drop-down.

### Android app

- **New launcher icon** — the lens with a "2", in both its ordinary and
  monochrome forms, and in the Play listing images.
- **The Browse page wears the host background**, like the setup and lemmas
  pages.
- A closed dialog (the colour window, dictionary settings) no longer paints
  for a frame during launch.
- **Debug builds can carry an x86_64 server** (`build-android.cmd debug
  intel`) so the app runs on an x86_64 emulator; release APKs stay arm64-only.
- The native Settings screen lost its Screen section — those rows live in the
  web Appearance sheet now — and gained a clear-cache button. The Play listing
  text's "uDict" typo is fixed.

## wudict2-v0.1.0 — 2026-09-20

Baseline: upstream wuDict master at commit `223b990` (2026-09-17),
shortly after **v3.7.4**, before `v3.7.5-alpha.1`. Everything below is
fork-only work on top of it.

### Android app

- **Distinct wuDict2 identity** — own application ID, launcher name and a
  dedicated lookup activity for external readers, so it installs and runs
  **beside** the upstream wuDict app; default server port 6889
  (upstream: 6888). The shell verifies the responding server's library
  directory before adopting it.
- **Transparent system splash** — cold start no longer flashes white.
- **Window background**: color and image, with built-in paper wallpapers;
  applied to both app windows and to system dialogs, including the
  dictionary-selection dialog.
- **Native dialogs**: dictionary picker, and folder picker ("Browse…") on
  the setup page.
- `build-android.cmd` — one-command local APK build from Windows.

### Search and results UI

- **Search history** with suggestions.
- **Tap a word in an article** to look it up — no selection needed.
- As-you-type updates only the suggestion list; articles refresh on word
  selection, Enter or leaving the field — far less re-render churn on
  phones.
- **Dictionary groups** with sorting.
- Pinnable search bar; fixed auto-hide on scroll and inertial scrolling of
  the results panel; dictionary selector moved left of the input field;
  status messages no longer shift the title/article.

### Appearance

- One **Appearance sheet**: window background (color/image, thumbnail
  strip, dominant-color auto-suggest from the chosen image, recent colors),
  App/Article/Files CSS, keyboard-aware sizing.
- **CSS presets as toggleable layers**: background group (Sepia /
  True black / Warm dark / Background image) and fonts group (Serif /
  Condensed / Light) are pick-one, the rest are free toggles; presets layer
  under your own CSS, state survives reload, `?style=off` bypasses
  everything.

### Server reliability and performance

- Corrupt or hostile dictionary files can no longer trigger gigabyte
  allocations — every parser path is bounded (bgl, mdx, slob, stardict).
- Windows: rebuilding or re-packing a dictionary no longer fails at the
  final rename with "Access is denied".
- Large packed media (video, big PDFs) **streams with Range/seek** instead
  of loading whole blobs per probe.
- `/res/` resources and the library listing answer without opening SQLite
  per item — articles with many images serve noticeably faster.
- Wedged queries are abandoned on cancel instead of stalling the stream;
  concurrent settings saves are serialized and atomic (overlapping saves
  no longer lose keys); assorted race fixes, per-request HTTP client reuse,
  intake dispose ordering on Windows.
