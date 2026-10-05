# Changelog

Notable changes in **wuDict2**, the Android fork of
[WuWeiDict](https://github.com/wuweidict/wudict), relative to the upstream
version it was forked from. Fork release tags are prefixed `wudict2-`; one
tagged line per release, newest first.

## wudict2-v0.9.1 — 2026-10-06

From `dev`. Mostly the tail of v0.9.0's parser change — and one thing about it
has to come first.

> ### ❗ Rebuild your DSL indexes after updating
>
> **The DSL reader changed in v0.9.0, so an index prepared by an older version no
> longer matches what the reader produces.** Rebuild it: **Settings → Edit
> Dictionary settings**, and in the **Dictionary settings** window set **index**
> to *Update existing* — or to *Recreate all* if you would rather rebuild
> everything. **Do the same for contains and full-text if you have them.**
>
> The app points at it too: dictionaries whose index is out of date are marked
> *Outdated …: click to rebuild*, and a notice above the results says how many
> dictionaries are affected with a **Rebuild** button. The bulk table in
> Dictionary settings is the way to do them all at once.
>
> Nothing else is touched by this: your dictionary files, media, styles and
> settings stay exactly as they are.

### The second reader: the last traces

- **Nothing in the interface mentions the second DSL reader any more.** The
  per-family scopes are gone from the bulk index table, the setup page no longer
  has a "DSL Dictionaries Parser" section, and no row in the settings speaks of
  parser variants — one reader, one set of indexes.
- **Reindexing a DSL dictionary that has an `_abrv.dsl` companion no longer gets
  stuck** (upstream's fix): the rebuild finishes and the dictionary opens again.
- The page's start-up watchdog now aborts its request and reports what happened
  instead of waiting on a silent server.
- Upstream also taught `wudict dump` to show progress while exporting — that is
  the desktop CLI, not the app.

## wudict2-v0.9.0 — 2026-10-06

From `dev`. The two DSL readers became one, index work became visible and
stoppable, and the app can hand you its own log.

### One DSL reader again

- **The GD-compatible reader is gone**, and with it the Original / GD
  compatible / Both choice. It existed because the original reader could not
  cope with malformed DSL; upstream's reader now can, so the second one has no
  reason to be here.
- **A dictionary that was listed twice is listed once.** With both readers on, a
  DSL dictionary appeared as `NAME` and `NAME GD`; the migration collapses that
  pair back to the dictionary itself, rebuilt with the lenient reader. It
  changes preferences only — your source files and the prepared data are never
  touched, and an old comparison receipt stays readable while its source is
  gone.
- Upstream's reader brings its own work: crossed and unclosed tags,
  transcription, and Lingvo's **secondary zone** (`[*]`), which is now
  **collapsed when a section opens**. Reveal it by tapping the icon — or Ctrl+*
  on a desktop — and a section opens in whichever state you chose last, except
  when a full-text match sits inside the zone, because that match is the reason
  the article is there at all.

### Index work you can see and stop

- **Running index operations are shown in one place**: a notice at the bottom of
  the screen with a spinner, "Active operations: 2", "3 of 12" and what it is
  doing — Creating, Recreating, Updating, Removing — plus a **Stop** that stops
  after the current dictionary rather than interrupting a write. When it stops,
  it tells you how far it got.
- **A dropped connection reconnects** and says so, instead of leaving a spinner
  you cannot trust.
- **While work runs, the server refuses work that would collide with it**
  ("dictionary operations are running; wait for completion or stop them") and
  the page locks the controls that would fight it, so the two halves cannot
  disagree about what is happening.

### A log you can hand over

- **Save System Log** — from Settings → System in the page, and from the shell's
  own settings screen as well, where it still works when the page cannot load
  and keeps the Android diagnostics. The log rotates (2 MB, five files), and
  index operations are recorded in it.

### Smaller things

- **Examples gained a second switch**: "Also hide examples without [*]", beside
  the Examples row, for dictionaries whose examples are not marked as such.
- **The folder and lemma dialogs report progress** — "Checking folders — 3 of
  12…", "Applying folder changes…" — instead of appearing to hang, and the lemma
  dialog wraps its text rather than clipping it.
- Fixed: the page could throw while starting up, because some of its code read
  variables before they were defined.

## wudict2-v0.8.0 — 2026-10-04

From `dev`. The dictionary list can be sorted by rules now, index maintenance
became one table with one button, and the words the app uses were overhauled
across both languages.

### Filters: the rules behind a dictionary list

- **Settings → Edit filters** opens the rules editor. Each line matches text in
  dictionary titles or file names, ignoring case, and puts what it matches into
  a named set the rest of the app can use. The page is in English and Russian,
  saves to the same `groups.ini` the app reads, offers **Reset to default**,
  names the file it writes, and reports the lines it cannot use instead of
  failing quietly. The built-in list stays in force until you replace or reset
  the file, and a `groups.ini` that cannot be read is offered for replacement
  rather than ignored.
- **The word is deliberate: these are *filters*, not "groups".** In this app a
  group already means the set you curate by hand (Settings → Edit dictionary
  groups); a filter is a rule that computes one. The interface follows that
  distinction everywhere.
- **The group editor gained a Filter control** while Show All is on: it narrows
  the dictionaries you can add — by language, by language pair, or by the rules
  above — and Untagged for dictionaries no rule classifies. Current members stay
  visible whatever the filter says, and adding one moves it above the control.

### Index maintenance

- **One table, one Start.** The dictionary settings window lists the three kinds
  of index — the headword index, the contains index and the full-text index —
  each with a single choice: Do not change, Create missing, Update existing,
  Recreate all, Delete. **Start** first shows what will happen and to how many
  dictionaries; deleting the headword index disables the other two rows, because
  it takes them with it.
- **Index state reads the way a reader thinks**: "Not indexed" with a click to
  index it, "Indexed: 41 MB on disk", "Outdated contains index: click to
  rebuild", an estimate for a dictionary you have not indexed yet — and
  "Locked: the dictionary files are gone, and this is the only copy" when a
  prepared database is all that is left.

### One word per concept, in both languages (merged from upstream)

- Upstream's wording overhaul is in: *index / indexed / indexing*, *contains
  index*, *full-text index*, *media pack*, *lemma data*, *orphan*, and *exact /
  prefix / contains / full-text* as the four match kinds. The setup, Browse and
  lemma pages use the same vocabulary and gained the sentences that explain what
  they do — what a dictionary folder is, what the library holds, what indexing
  buys, and when lemma data helps.

## wudict2-v0.7.1 — 2026-10-04

From `dev`. Mostly a settling release: the two DSL readers from v0.7.0 reach the
windows that list and group dictionaries, examples in those articles get one
look for both readers, and the second upstream sync landed.

### The two DSL readers reach the rest of the app

- **The group editor and the Browse picker follow the parser in force.** With
  one parser selected they list that variant alone; with both, both. A hidden
  variant keeps its membership and its position, so reordering the visible
  dictionaries no longer disturbs it, and a variant you selected whose index is
  missing stays visible instead of vanishing.
- **The dictionary count no longer doubles** when both readers are on — it
  counts DSL sources, not the views of them.

### Examples in DSL articles

- **Both readers show examples the same way**: one tint, one diamond, no
  duplicates, and inline examples hidden as intended. The GD reader now emits
  ready-made example markup instead of leaving it to the page, the marker
  scales with the text rather than sitting at a fixed size, and the Original
  reader's own background is fixed alongside.

### Rescan folders

- **The dialog reports progress** — "Dictionary 3 of 12: Oxford…" while it
  works, a line while it cleans up and refreshes the list, and a message if the
  connection drops before the result arrives, instead of appearing to hang.

### Reading

- **Article text is always selectable.** A dictionary's own stylesheet could
  switch selection off, which on Android also removed the long-press Copy menu;
  prose is selectable on both surfaces now, whatever the dictionary says.

### Also in this build: merged from upstream

- **Comments no longer reach the browser.** The server strips them from every
  page, stylesheet and script as it serves them, so the pages are smaller;
  preset files are served as they are and keep theirs.
- The rest of that upstream commit is internal — the store's ingest and
  reconciliation paths were renamed and restructured, and this fork's own code
  was adapted to the new names.

## wudict2-v0.7.0 — 2026-10-03

From `dev`. A release about keeping the data in step with the dictionaries —
plus a second DSL reader, and a way to search what is already on the screen.

### Rescan folders now clears what it no longer finds

- **Prepared data for dictionaries that are no longer in the folders is deleted
  when you rescan.** This is deliberate: change the dictionary set and the
  app's data footprint follows it down instead of growing forever, and a
  rebuild is quick and happens once. Your own files are never touched — the
  dictionary, its audio and its images all survive; what goes is the prepared
  and packed data derived from them.
- **The same dialog maintains the indexes you keep.** Per dictionary you choose
  Recreate (create again, including anything missing), Update (rebuild the
  indexes it already has) or Delete; Keep preserves a removal across a restart,
  and new dictionaries get the indexes the Edit Folders defaults ask for.
  Unused indexes, duplicates and old packed media are removed either way.

### Find in articles

- **A magnifier on the status bar searches the articles already on screen.** It
  walks what is loaded in reading order — collapsed dictionary sections
  included — and the bottom strip's ↑ and ↓ step through the matches and
  **loop**, saying "from the beginning" or "from the end" when they wrap.
- Word-prefix, whole-word and contains matching, match case, highlight-all, and
  a switch that lets the search look inside folded examples — only the current
  match is unfolded, and it is folded back when find ends. The query and the
  options persist, and dictionary markup, links and selections are left intact.

### DSL: a second reader, and its fonts

- **A DSL reader compatible with GoldenDict's**, alongside the original one.
  It is a Go tree parser inspired by GoldenDict's `ArticleDom` — not a literal
  port of every feature — carrying the formatting repairs GoldenDict Enhancer
  2.3 is known for: crossed and unclosed tags, inline formatting inside margin
  blocks, nested link labels, `~` in alternate headings, nested optional parts
  (32-key ceiling) and the legacy Lingvo transcription table in `[t]`. Part of
  the Enhancer's visual design came with it. How close its output is to
  GoldenDict's own is checked against GoldenDict's source by a differential
  harness, but visual parity is not device-verified.
- **The two readers index separately**, because they parse DSL — headings
  included — a little differently. With both enabled a dictionary is listed
  twice: `NAME` from the original parser and `NAME GD` from the GD-compatible
  one. Each owns its prepared database and search indexes; the DSL file and its
  media are shared, and removing one leaves the other alone.
- **Which reader, per installation**: Original, GD compatible, or Both, and the
  index defaults for newly added DSL dictionaries are set on the setup page.
- **The fonts the Enhancer's stylesheet assumes.** Its phonetic face is the
  official **Quivira.otf**, bundled and used unmodified — the licence of the
  cut-down "Quivira Phonetic" does not allow changing it. Its four Arial-based
  faces **cannot ship**: they are built on proprietary fonts whose licence
  permits neither modification nor redistribution, so the style falls back to
  the system font. You can supply your own instead: add `wugd_Regular`,
  `wugd_Bold`, `wugd_Italic` and `wugd_BoldItalic` as `.ttf` or `.otf` under
  Settings → Custom CSS → Files, and they are used exactly where the Arial
  family was.

### Exit

- **An Exit row in Settings.** It closes every app window and stops the server,
  while a reader window hosting an external lookup is left alone. If a transfer
  or demanded work is still running it asks rather than cutting it off, and
  offers to exit when the work finishes. Exit ends this run only — a reading
  app's dictionary button still starts wuDict2 again.

### The Settings drawer

- **Its sections fold independently** and remember which ones you left open.
- **Language is a section of its own**, its row naming the language in force
  ("Language — Русский"), and its dialog no longer inherits the mobile sheet's
  stretching.

### Also in this build: merged from upstream

- **wudict markdown**: a setext heading — a line of text with `---` under it —
  is content again rather than the start of an entry, only `## ` starts one,
  and a `## ` swallowed by an HTML block is now reported with the line that ate
  it instead of silently not becoming an entry.
- **The share page's "Open in wuDict"** is an `intent://` link, so a messenger's
  in-app browser hands the link to the app.

## wudict2-v0.6.0 — 2026-10-02

The full build, from `dev`. Two things in it are new and worth testing: the
**Russian interface**, now covering what the v0.6.0-ru.1 preview left in
English, and a **GoldenDict-compatible DSL parser**. Everything else is
ordinary released work, and the preview's contents are all here too.

### The Russian interface

- **The two gaps the preview listed are closed.** Dictionary categories,
  language pairs and filter labels are translated, and so are the server's own
  messages — import, index preparation, deletion, downloads, orphan review,
  paths and configuration. An unknown message still stays visible in English
  rather than disappearing behind a missing key.
- The Android resources are complete except the application name, which is
  inherited on purpose. The new DSL parser's controls, errors and bulk actions
  arrived in both languages with it, and the Russian counts are grammatical for
  1, 2 and 5.
- English is still the default; the choice lives in Settings → Language and
  belongs to the installation, not to one browser. The device pass is still
  owed, as for the preview.

### GoldenDict-compatible DSL parsing

- **A DSL dictionary can now be read GoldenDict's way.** DSL files in the wild
  are written for GoldenDict's parser, so a dictionary that renders correctly
  there could render differently here. The reader chooses once for the
  installation — **DSL parser: Original / GD compatible / Both** — from the
  dictionary panel.
- **The setup page gained a "DSL Dictionaries Parser" section**: which of the
  two variants get an index, contains or full-text on newly added DSL
  dictionaries. Existing dictionaries are not touched, and on the very first
  setup the parser choice is synchronised from these defaults once.
- **Index work follows the choice.** Bulk create and delete for index, contains
  and full-text are scoped to the selected parser — all dictionaries, Original
  only, or GD compatible only — behind a compact confirmation that names the
  count and the feature, with a live "N/total · name" progress and a **Stop**
  that halts between dictionaries. A search no longer silently rebuilds an
  index you deleted, and deleting a base index also removes the contains and
  full-text indexes that depended on it.
- **GD articles get their own style** and respect your font size and weight
  instead of hard-coding 13px; DSL underlining is a plain underline again
  instead of the old highlighted box. Packing controls are hidden for DSL
  dictionaries, and existing packed media is left alone.
- The parser is checked against GoldenDict's own implementation: a comparison
  harness — Python plus an oracle built from GoldenDict's source, vendored with
  its provenance recorded — diffs the two parsers over a corpus of DSL
  constructs. That harness is a development tool and does not ship in the app.

### Also in this build: merged from upstream

- **A dictionary collection can be installed from a link.** The setup page's URL
  field takes a web folder page, a `.txt` list, several links at once, or a
  share link; wuDict reads the list once and shows every dictionary it finds
  with its date and size, installing only the ones you tick. Each dictionary's
  media is offered beside it, and a failure names the dictionary while the rest
  still install.
- **Google Drive and Nextcloud are supported sources** for such an install:
  Drive file links are rewritten to a direct download (a folder is not
  supported, and a refused file explains itself), and a Nextcloud public share
  is listed through the share's own WebDAV.
- **On Android a share link opens the import directly** — the link form is a
  verified App Link — and the "an import is already running" dead end is fixed:
  the app releases its own job on every exit path, and when the slot is held by
  something else it offers to stop it or leave it running.
- **Tick now means overwrite and untick means skip.** wuDict never installs a
  second numbered copy, and both screens gained Select all / none. Un-ticking a
  dictionary while replacing it keeps the media you already have.
- **Rescan folders offers to delete orphan indexes**: prepared dictionaries
  whose source files are gone are listed with how much space they hold, each
  row ticked, and one button that names the whole action. Un-ticking one
  remembers it as kept and never asks again.
- **Windows:** editing a dictionary source no longer makes the next open fail
  with "Access is denied" — a retired backend held the old database open while
  the rebuilt one was renamed over it.
- Documentation and build hygiene: the wudict howto documents the shareable
  URLs, CI now runs on Windows and macOS as well, the Gradle distribution is
  pinned by checksum, and the Android build no longer carries Google's tracker
  packages.

## wudict2-v0.6.0-ru.1 — 2026-09-30

A **preview**, and the first release cut from a branch other than `dev`:
`translation_layout_fix` is `dev` plus the Russian interface. It is the FULL
app — everything in "Also in this build" is ordinary released work — and the
translation is what this build asks you to test. English stays the default.

### The Russian interface

- **Settings → Language** offers English / Русский and applies on reload. The
  choice belongs to the installation and not to one browser, so every window
  that reaches the server follows it. Nothing is decided from the system
  language, the Android locale or the dictionary.
- **Translated**: the main screen (search, results, Settings, appearance, the
  CSS editor and its dialogs), the dictionary settings and group windows, the
  Folders, Lemmatization and Browse pages, the speech menu, the system settings
  window, the in-article dialogs, the Android window titles and dialogs, and
  the counters, with Russian plural forms.
- **Not translated yet**, so English will show there: dictionary categories and
  filters (the language and content labels the dictionary list groups by), and
  the server's own messages — import, index preparation, deletion, download,
  lemmas, paths and configuration, saving files. Unknown messages stay visible
  in English rather than disappearing behind a missing key.
- The interface language is separate from the dictionary's: articles, search,
  pronunciation and dictionary names are untouched by it.
- The layout of the longer Russian labels is part of this work: the Settings
  rows' label column is measured rather than fixed, and the appearance buttons
  travel as one block.

### Also in this build: the unreleased work since v0.5.0

- **The upstream catch-up** — the first since v0.5.0; upstream master moved from
  `5f0ad02` to `6cc84cc`, and the wudict markdown format, the cross-reference
  link fix and the reader-app dictionary button below are all of it.
- **wudict markdown: a dictionary is one Markdown file.** A plain CommonMark
  file anyone can read in a Markdown viewer counts as a dictionary when its
  first two lines are `# Title` and `wudict: 1`: entries are `## headword`
  headings, links use `entry://`, and resources live in a folder beside it.
  Accepted as `.md`, `.wudict.md`, and compressed as `.wudict.md.gz` / `.dz` —
  add one to a dictionary folder or share it to the app and it is indexed on
  first open, like DSL or BGL. Any dictionary wuDict reads can also be written
  out in the format.
- **A dictionary button in a reading app.** Ten reader actions are answered, so
  the dictionary button of Moon+ Reader, ReadEra, Librera, FBReader, CoolReader
  and others opens wuDict2's floating lookup. The shipped docs list which
  reader to pick for each app; Kindle, Play Books and Kobo offer no outside
  dictionary and cannot be served. A fourth lookup preference controls it, off
  by default, and a caller asking for full screen opens the full app.
- **Cross-references are ordinary lookups now.** A Babylon or repacked
  article's `bword:` links are written out as `entry://`, and
  `entry://@subentry` becomes the slash-less `entry:@subentry`, so a sub-entry
  link survives the dictionary scripts that round-trip their own anchors.
- **System Settings moved into the page**, from the shell's own settings screen
  into the ☰ drawer's System section: the lookup switches, Show info messages,
  Access, Clear browser cache, and the Advanced server rows with Restore
  defaults. The native screen keeps the server port, a signpost and the
  restart. With it the port field shows the port in force instead of an empty
  box, size fields state the unit they mean, a refused write snaps the control
  back with a sentence, and a failed start is retried when the window regains
  focus instead of needing the app swiped away.
- **Labels are drawn in ink.** Every label, heading, hint and count wears the
  full text colour, and the two greys are reserved for disabled states; before
  this those lines read as switched off in every look. A new **Quiet labels**
  layer restores the grey hierarchy, on all four pages, for anyone who wants
  it. Three danger reds the dark theme never reached, and the article link
  colour, are fixed alongside.
- **Status bar and picker**: the bar stands from the first paint whenever it is
  switched on, not only after a search has produced sections, and its arrows
  stand down on their own when there is nothing to walk; its end gutters
  widened to one button height so the arrows are not pressed against the edge
  of the screen; the scope chip names its scope in full in the bar.
- **A Windows build script** for the desktop product (`build-windows.cmd`) —
  it builds `wudict.exe` and an installer. A developer tool; the Android app is
  unaffected.

A preview in what it asks for, not in what it contains: the translation's
device pass is unfinished. The project's Go checks for it are green
(`TestI18n`, `TestAppearanceContract`); the JS-side i18n check needs Node,
which this machine does not have, and so was not run; and the label layout was
measured in a desktop browser at 320–412px in both languages but is **not yet
checked on a phone**.

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

### True black and Warm dark take effect at night

- **The night halves of the True black and Warm dark layers work now.** They
  attached and changed only the article tokens while the chrome stayed the
  app's grey — the page's own dark palette is written at a higher CSS weight
  than the layers' bare `:root`, so they lost. They now declare themselves at
  the page's own "the page is dark" marker, which is a tie the later layer
  wins, so True black reaches `#000` and Warm dark `#1c1a17`, as their names
  promise. Neither touches the light theme.
- A test now asserts the weight of **every** app half against the app's own
  palette, property by property, so this cannot quietly come back.

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
