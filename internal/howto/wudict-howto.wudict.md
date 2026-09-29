# wudict howto
wudict: 1
from: en
to: en

Type `wudict help` in the search box for quick access, and [browse](/browse?dict=wudict-howto).

## wudict welcome
## wudict intro
## wudict 1st run
## wudict howto
## wudict help

<em>💡Tip</em>: you can return to this article at any time by searching for `wudict welcome` or `wudict intro`

> *Quick actions*

- [Browse](/browse?dict=wudict-howto) all entries in this guide
- [Add dictionaries](/setup)
- Add/remove [morphology files](/lemmas)
- Explore the [dictionary panel <kbd>☰</kbd>](<entry://wudict panel>)
- Discover [wudict markdown](<entry://wudict markdown>) (this howto is written in [↗ it](https://github.com/wuweidict/wudict/blob/master/internal/howto/wudict-howto.wudict.md))

> *Wudict features*

- Formats: MDict (`.mdx/.mdd`), StarDict (`.ifo/.idx/.dict/.dz`), Aard2 (`.slob`), Lingvo DSL (`.dsl/.dsl.dz`), Babylon (`.bgl`), ZIM (`.zim`), and Wudict Markdown (`.wudict.md`)
- Instant search across all dictionaries with four [search modes](<entry://wudict search>): prefix, exact, contains, and full-text search
- Dictionaries are automatically grouped by language, publisher, direction, type.
- Double-tap any word in a wudict dictionary article to instantly look up its definition
- Adjust the font size via the <kbd>☰</kbd> → <kbd>−</kbd> <kbd>+</kbd> widget
- Custom [lemmatization (word-form morphology)](<entry://wudict lemmatization>) for 24 languages, configurable via <kbd>☰</kbd> → <kbd>⚙️</kbd> → **Lemmatization**; English lemmatization comes pre-installed
- Android: Immersive mode (full screen), to draw content edge-to-edge, including the camera cutout area; enable by long-tapping the wudict icon in the launcher → **Settings**
- Browse **all headwords** in any dictionary via <kbd>☰</kbd> → <kbd>**Browse**</kbd>
- Select text in any app and pick → <kbd>**wuDict**</kbd> from the context menu
- Apply custom CSS styles and fonts via the embedded Styler: <kbd>☰</kbd> → <kbd>⚙️</kbd> → <kbd>**Custom Style**</kbd>
- Import dictionaries into wudict directly from your file manager: tap a `.mdx/.dsl/.bgl/.zim/.slob/.zip/.7z/.wudict.md` file → <kbd>Share</kbd> → <kbd>**wuDict**</kbd>
- Long-tap a dictionary download link on any web page (`.mdx/.slob/.dsl/.zip/.7z`) and pick <kbd>Share</kbd> → <kbd>**wuDict**</kbd> to install it locally

📱 Android only · 💻 desktop only · everything else works on both.

| Control | What it does |
| --- | --- |
| <kbd>starts with</kbd> | the [search mode](<entry://wudict search>): starts with, exact, contains, full-text |
| <kbd>All dictionaries</kbd> | search everything, a specific dictionary, a language or language pair, a publisher |
| <kbd>◐</kbd> | theme: light, dark, automatic |
| <kbd>⇔</kbd> | wide layout |
| <kbd>⊞</kbd> | expand every result |
| <kbd>☰</kbd> | the [panel](<entry://wudict panel>): your dictionaries and settings |

### Quick Start

1. [Add dictionaries](<entry://wudict add dictionaries>): MDict, StarDict, Slob, DSL, Babylon, ZIM, and [more](<entry://wudict formats>).
2. Search. Double-click a word in an article (📱 double-tap) to look it up.
3. In <kbd>☰</kbd>, drag <kbd>⠿</kbd> placeholder to set your search priority order.
4. Install word forms for your languages: ![](lemmas.svg) [Lemmatization](/lemmas). English is built in.

### Options

Open the dictionary panel <kbd>☰</kbd> to:

- **Open first**: <kbd>My order</kbd> opens the first dictionary in your list that has a result; <kbd>Fastest</kbd> opens the dictionary that was the fastest to return a result.
- **Sort dictionaries** controls the order used in the dictionary combobox, <kbd>Alphabetical</kbd> or <kbd>My order</kbd>.
- **Text size**: <kbd>−</kbd> <kbd>15px</kbd> <kbd>+</kbd> in the ![](cog.svg) row; click the number to reset.
- ![](rescan.svg) <kbd>Rescan folders</kbd> can be used to refresh the dictionary folders after adding or removing items.

More: [search](<entry://wudict search>) · [full-text](<entry://wudict full-text>) · [panel](<entry://wudict panel>) · [index](<entry://wudict index>) · [styles](<entry://wudict styles>) · [links](<entry://wudict links>) · [📱 Android](<entry://wudict android>) · [💻 desktop](<entry://wudict desktop>) · [FAQ](<entry://wudict FAQ>)

## wudict search
## wudict search modes
## wudict modes

**Pick a mode in the bar; accents and case never matter**: `corazon` finds *corazón*, `OXFORD` finds *Oxford*. Exact and starts with work at once; contains and full-text need an optional [index](<entry://wudict index>).

| Mode | Finds | Use it when |
| --- | --- | --- |
| **starts with** | headwords that start with your text | you know how the word begins (the default) |
| **exact** | the headword itself | you know the word |
| **contains** | your text anywhere in a headword | you know the middle |
| **full-text** | words inside article text, best first | you remember the meaning, not the word |

[base form lemmas](<entry://wudict lemmatization>) let you find **know** by typing *knew*.

***

- [index](/browse?dict=wudict-howto)

## wudict full-text
## wudict fts
## wudict full-text syntax

**Several words are a phrase; quotes make it exact**. `no pun intended` finds the phrase, then, only if nothing matches, the words in proximity, then the words anywhere, and the section header indicates which mode is active. Full-text searches only dictionaries whose full-text [index](<entry://wudict index>) is on. Dictionaries without a FTS are offered for indexing.

| You type | You get |
| --- | --- |
| `pun` | words starting with *pun* |
| `"pun"` | the word *pun* only |
| `"no pun intended"` | exactly that phrase, never widened |
| `pun OR joke` | either |
| `pun NOT punctuation` | *pun*, without *punctuation* |
| `NEAR("bank" "river", 4)` | both words within 4 words |
| `(pun OR joke) AND intended` | grouping |

Operators count only in capitals: `wage not minimum` is three words. A query never fails; what cannot be parsed is read as plain words. ![](highlight.svg) <kbd>Highlight matches</kbd> in <kbd>☰</kbd> marks the words found; step through them with the chevrons (💻 <kbd>←</kbd> <kbd>→</kbd>, <kbd>F3</kbd>).

- [index](/browse?dict=wudict-howto)

## wudict lemmatization
## wudict word forms
## wudict inflected words
## wudict lemmas

**Lemmatization is applied as a fallback when there are no exact matches**: *knew* finds **know**, *estuviera* finds **estar**. 24 languages: English is built in, the others install in one tap from ![](lemmas.svg) [Lemmatization](/lemmas) (also in <kbd>☰</kbd> → ![](cog.svg)).

Each language is a small download (under 2 MB) and is used only for dictionaries in that language: Spanish *sale* → **salir** is never asked of an English dictionary. Untick a language to delete it.

**IMPORTANT**

> ❗️ For lemmatization to work, wuDict needs to know the dictionary language! Some dictionary formats such as Babylon (`.bgl`) and Lingvo (`.dsl`) contain data about the headword language, which is sufficient. Dictionary formats like `.mdx` have no language metadata, wuDict infers the language using the following strategy in the given order of priority:

- if the dictionary file starts with e.g. `es-es` or `spa-eng` (for example, spa-eng-oxford.mdx) then the first language code will be used as the lemmatization language
- if the dictionary **title** contains a valid language code then that is used
- if the dictionary parent folder is a valid language code such as `es` or `spa` then this will be used as the lemmatization language.
- if none of the previous checks found a language code then English is used by default, which implies that for English dictionaries you do not need to rename your files or the parent subfolders to match `en` or `eng`.

***

- [index](/browse?dict=wudict-howto)

## wudict panel
## wudict ☰
## wudict settings
## wudict dictionary list

<kbd>☰</kbd> **holds your dictionaries and every setting**. The top row changes how you read; the ![](cog.svg) row changes what wudict does; the list below is your dictionaries, in the order results appear.

***

- [index](/browse?dict=wudict-howto)

### Display options

- ![](highlight.svg) <kbd>Highlight matches</kbd>: mark the words a [full-text](<entry://wudict full-text>) search found.
- ![](speak.svg) <kbd>Read aloud</kbd>: select text in an article to speak it using the OS's Text-to-speech engine. You can pick your preferred voice via the chevron next to the speaker icon (when the OS provides multiple voices for the detected language), see more details under [wudict Text-to-speech](<entry://wudict Text-to-speech>)
- **Open first**, **Sort dictionaries**: see [wudict welcome](<entry://wudict welcome>).
- **Group by**: which groups the dictionary picker offers (language, language pair, publisher, …).

***

- [index](/browse?dict=wudict-howto)

### Settings ![](cog.svg)

- <kbd>−</kbd> <kbd>15px</kbd> <kbd>+</kbd>: [text size](<entry://wudict text size>).
- ![](folder.svg) <kbd>Edit folders…</kbd>: [add dictionaries](<entry://wudict add dictionaries>).
- ![](rescan.svg) <kbd>Rescan folders</kbd>: find dictionaries added or removed since.
- ![](lemmas.svg) <kbd>Lemmatization…</kbd>: [word forms](<entry://wudict lemmatization>).
- ![](browse.svg) <kbd>Browse A–Z…</kbd>: [read a dictionary page by page](<entry://wudict browse>).
- ![](styles.svg) <kbd>Custom styles…</kbd>: [your own look](<entry://wudict styles>).
- <kbd>Full-text for every dictionary…</kbd>: build every full-text [index](<entry://wudict index>) at once.

***

- [index](/browse?dict=wudict-howto)

### Per dictionary

- <kbd>⠿</kbd> drag, or <kbd>⏫</kbd> <kbd>▲</kbd> <kbd>▼</kbd> <kbd>⏬</kbd>: its place in your list.
- the on/off knob: include/exclude from *All dictionaries* search, even when <kbd>OFF</kbd> a dictionary is still searchable when selected explicitly in the combobox.
- click or tap the dictionary name: search only this dictionary.
- <kbd>contains</kbd> <kbd>full-text</kbd> <kbd>media</kbd>: optional [indexes](<entry://wudict index>), with their size.
- ![](browse.svg) <kbd>Browse</kbd>: [A–Z](<entry://wudict browse>). **About this dictionary**: its details.
- the file row: its files, and 🗑 <kbd>Remove…</kbd> ([remove](<entry://wudict remove>)).

## wudict text size
## wudict font size
## wudict zoom

In <kbd>☰</kbd> → ![](cog.svg) row → <kbd>−</kbd> <kbd>15px</kbd> <kbd>+</kbd>; click the number to reset. It sets the size of article text; the app keeps it.

## wudict add dictionaries
## wudict add
## wudict folders
## wudict setup
## wudict import

**Put dictionary files in a folder wudict reads, or import them**. They are searchable at once, and indexed in the background. [Setup](/setup) lists the folders and imports files.

- **Import**: on [Setup](/setup), drop or choose a file, or paste a download link, then <kbd>Install</kbd>. An archive (`.zip`, `.7z`) is unpacked; the parts of a dictionary (an `.mdd` beside its `.mdx`) are found for you.
- 📱 Open a dictionary file with wuDict from your file manager, or share it to wuDict. Long-tap a download link on a web page → Share → wuDict.
- 📱 The wuDict build from GitHub also reads *Internal storage ▸ Dictionaries* once you allow *All files access*.
- 💻 **Folders**: on [Setup](/setup), paste the path of any folder; subfolders are read too. The default is `~/Dictionaries`.

After copying files into a folder, press ![](rescan.svg) <kbd>Rescan folders</kbd> in <kbd>☰</kbd>. Copy every part of a dictionary: see [wudict formats](<entry://wudict formats>).

***

- [index](/browse?dict=wudict-howto)

## wudict formats
## wudict dictionary formats
## wudict files

**wudict reads these formats**.

| Format | Files |
| --- | --- |
| MDict | `.mdx`, and its `.mdd` resources |
| StarDict | `.ifo` with `.idx` and `.dict` (or `.dict.dz`), `.syn` |
| Aard 2 | `.slob` |
| Lingvo DSL | `.dsl` or `.dsl.dz`, and its `.files.zip` |
| Babylon | `.bgl` |
| ZIM | `.zim` (Kiwix, Wikipedia, Wiktionary) |
| wudict markdown | `.wudict.md`, a [text file you can write](<entry://wudict markdown>) |

Speex `.spx` audio auto-converted for browser playback. A folder named after a language (`es/`) can be used to signal to wudict the language of a dictionary. The preferred way is for dictionary files to have a file prefix with a two character or three character language codes, e.g. `fr-es-oxford.mdx` or `fra-spa-oxford.mdx`.

***

- [index](/browse?dict=wudict-howto)

## wudict index
## wudict indexing
## wudict prepare

**wudict indexes each dictionary in the background**. You can search a dictionary even if it has no headword index, but the index makes it faster and more efficient for RAM/CPU consumption as well as battery life. In the dictionary panel <kbd>☰</kbd> you can also optionally enable <kbd>contains</kbd> and <kbd>full-text</kbd> indexes — these can be additional GB for large dictionaries, are optional and can be enabled on-demand.

- <kbd>media</kbd> packs a dictionary's pictures and sounds into its index, so it keeps working if you delete the original files, and also makes access faster to assets such as images and audio.
- <kbd>Full-text for every dictionary…</kbd> in the ![](cog.svg) row will generate full-text indexes for all dictionaries — IMPORTANT: depending on the size of you dictionary collection this can take extra GB of space and last a few minutes until complete.
- ⟳ on a switch, or a **Rebuild** line: an index made by an older wudict; one click rebuilds it.
- DSL, Babylon and wudict markdown are indexed as soon as they are opened; ZIM only when you ask, since this format already has its own index and wudict can use it.

An index usually takes less space than the dictionary file.

***

- [index](/browse?dict=wudict-howto)

## wudict browse
## wudict all headwords

**Read a dictionary word by word, A to Z**: ![](browse.svg) <kbd>Browse</kbd> on a dictionary in <kbd>☰</kbd>, or ![](browse.svg) <kbd>Browse A–Z…</kbd> in the ![](cog.svg) row. [Try it on this guide](/browse?dict=wudict-howto).

Tap a letter to jump; type the start of a word to go there; tap a headword to read it. 💻 Press the <kbd>←</kbd> <kbd>→</kbd> arrow keys turn pages, <kbd>Home</kbd> <kbd>End</kbd> jump to the first and last page.

***

- [index](/browse?dict=wudict-howto)

## wudict links
## wudict cross-references
## wudict double-click
## wudict double-tap

**Click a link in an article to follow it; double-click any word (📱 double-tap) to look it up**. A link is looked up in the dictionary you are reading first, then in all of them. Back returns to where you were.

Every search has its own address, so you can bookmark it: `/?q=word&mode=exact`. 📱 `wudict://lookup?q=word` opens a lookup from other apps and scripts.

***

- [index](/browse?dict=wudict-howto)

## wudict read aloud
## wudict speak
## wudict pronunciation
## wudict TTS
## wudict Text-to-speech

**Select text in an article and press the speaker icon that appear to hear it** in a system voice for the article's language. The TTS feature can be disabled via the ![](speak.svg) <kbd>Read aloud</kbd> in <kbd>☰</kbd>. A dictionary's own recordings play with a click on their speaker icon.

To see in action, select the text below, and then then click the speaker icon to hear it read aloud by the system Text-to-speech engine:

> For a moment, nothing happened. Then, after a second or so, nothing continued to happen.

***

- [index](/browse?dict=wudict-howto)

## wudict styles
## wudict custom styles
## wudict css
## wudict theme

**Use custom display styles**: ![](styles.svg) <kbd>Custom styles…</kbd> in <kbd>☰</kbd>. <kbd>App</kbd> styles the page, <kbd>Article</kbd> every dictionary article, use <kbd>Files</kbd> to add fonts and images and then <kbd>Insert</kbd> to add them into the CSS. Pick a preset, edit, <kbd>Save</kbd> — you can see a live preview of the changes as you type.

<kbd>◐</kbd> switches light, dark and automatic theme. <kbd>Clear</kbd> then <kbd>Save</kbd> removes a style. If a custom style caused the entire page to disappear, append `?style=off` to the URL (💻 desktop only).

***

- [index](/browse?dict=wudict-howto)

## wudict remove
## wudict delete

In <kbd>☰</kbd> → the dictionary → click 🗑 → <kbd>Remove…</kbd>, then choose what to delete: everything, only the index, or (once its media is packed) only the dictionary files. ❗️ There is no undo!

To exclude a dictionary from Search-All mode, just untick its checkmark in the dictionary panel <kbd>☰</kbd>.

***

- [index](/browse?dict=wudict-howto)

## wudict android
## wudict mobile

📱 **Look up a word from any app**: select it and pick <kbd>wuDict</kbd> in the selection menu, or share it to <kbd>wuDict</kbd>; the wuDict definition floats in a popup over the original app. To discard it, press **Back** or tap anywhere outside the popup.

> NOTE: If, instead of a popup, you'd rather have wudict open in a full window, then long press the wudict icon in the launcher and select <kbd>wuDict Settings</kbd> and check the corresponding checkbox under **Look up in the full app**.

- **Add dictionaries** by opening or sharing a file to wuDict: see [wudict add dictionaries](<entry://wudict add dictionaries>).
- **Full screen**: long-tap the wuDict icon on the home screen → <kbd>Settings</kbd> → immersive mode, optionally edge to edge, into the camera cutout.
- The keyboard hides when you scroll an article. Off screen, wuDict uses one core, to avoid draining the battery.
- **Reading apps**: in the reader's dictionary settings, choose wuDict, or a dictionary wuDict answers for: *ColorDict*/*GoldenDict*, *Aard 2*, *Lingvo*, *Fora* or *Dictan*. In Moon+ Reader, a *Customized* dictionary with the URL `wudict://lookup?q=%s` also works.
- `wudict://lookup?q=word` opens a lookup from automation apps and scripts.

***

- [index](/browse?dict=wudict-howto)

## wudict desktop
## wudict computer

💻 **wudict runs in your browser**, at `localhost:6888`, as a small program on your computer: a menu-bar icon on macOS, a tray icon on Windows. Nothing leaves the machine.

| Key | Action |
| --- | --- |
| <kbd>/</kbd> | go to the search box; typing anywhere does too |
| <kbd>Esc</kbd> | close <kbd>☰</kbd> |
| <kbd>←</kbd> <kbd>→</kbd> · <kbd>F3</kbd> · <kbd>Ctrl</kbd> <kbd>G</kbd> | step through [full-text](<entry://wudict full-text>) matches |

- **wuDict Hover** (Chrome, Firefox): hover a word on any web page (optionally holding <kbd>Alt</kbd>) to see its definition in a popup.
- **Command line**: `wudict searchall word` searches from a terminal; `wudict dump -format md` writes any dictionary as [wudict markdown](<entry://wudict markdown>); `wudict help` lists the rest.

***

- [index](/browse?dict=wudict-howto)

## wudict markdown
## wudict md
## wudict md format

**A wudict markdown file is a dictionary you can write in any text editor — this guide was written in wudict markdown**. Line 1 is `#` and the title, line 2 is `wudict: 1`; each entry is a `## headword` followed by its text. Save it as `name.wudict.md` in a dictionary folder.

<details>
<summary>An example</summary>

***

- [↗ wudict markdown specification](https://github.com/wuweidict/wudict/blob/master/docs/WUDICT-MARKDOWN.md)
- [index](/browse?dict=wudict-howto)

```markdown
# My Glossary
wudict: 1

## colour
## color

The property of an object that depends on the light it reflects. See [hue](entry://hue).
```

</details>

More `##` lines right under the first are other spellings. Links like `[hue](entry://hue)` trigger a lookup. Tables, lists, images and the rest of standard markdown work. 💻 Running `wudict dump -format md` from a console lets you export any existing dictionary to wudict markdown. To edit this guide, put a copy in your folder: [Setup](/setup) → **Put the wudict howto in this folder**.

***

- [index](/browse?dict=wudict-howto)

## wudict FAQ

<details>
<summary>wudict is using too much disk space — how do I claim it back?</summary>

For every dictionary in the panel <kbd>☰</kbd> the size of headword, contains, and full-text search indexes are displayed. Also for dictionaries with assets (e.g. `.mdd`) resources also might be imported into a wudict optimized sqlite-based storage. You can reclaim space by deleting the *contains*, and *full-text search* indexes for dictionaries for which you don't need them - just tap the corresponding badge in the panel. Clicking the dictionary source file in the card, located below the <kbd>Browse</kbd> button, will display a delete option for deleting both the indexes and/or the source dictionaries (use with caution!)

</details>

<details>
<summary>Why does searching estuviera find nothing?</summary>

Because word-form data for Spanish is not installed. wudict retries a failed search with the word's dictionary form — *knew* → **know** — but only English is built into the program; every other language is a small file you install.

Click <kbd>☰</kbd> → <kbd>⚙</kbd> → <kbd>🔤 Lemmatization</kbd> (or on the settings page), tick Spanish, and search again — it works immediately, with no restart. On a desktop, [`wudict lemmas download es`](/lemmas) does the same.

💡 **IMPORTANT**: Dictionary formats like `.mdx`, `.slob` and others contain *no metadata* about the headwords language. Lemmatization will only work for these dictionaries if their filename starts with a two or three character language code prefix e.g. with `es-es`, `fr-en` (will be detected as French). Or, as an alternative, you can place all the dictionaries for a specific source language under a subfolder that must match exactly the language code, e.g. `de` or `it`.

For English language dictionaries the `en-en` prefix is optional, since wudict will by default use English as the fallback lemmatization language if it cannot be detected from the dictionary metadata or from the filename or subfolder name. If the dictionary title contains an actual language name, such as Spanish, German, etc. that will be used as a fallback if other methods returned no results.

</details>

<details>
<summary>What is the format used for this dictionary and where can I see its source code?</summary>

The wudict howto dictionary uses **wudict markdown** format, and you can build your own markdown dictionary using the example source at [↗ wudict-howto.wudict.md](https://github.com/wuweidict/wudict/blob/master/internal/howto/wudict-howto.wudict.md)

also see [<strong>`wudict markdown`</strong>](<entry://wudict markdown>)

</details>

<details>
<summary>Can I convert any dictionary in my collection to wudict markdown?</summary>

The `wudict` binary on the desktop (windows/linux/mac) provides a `dump` command that can be used to export any dictionary to the **wudict markdown** format:

```sh
# with -mode html you get maximum fidelity with complex HTML rendered as markdown HTML blocks
wudict dump -format md -mode html -o my-output-folder ldoce6.mdx

# with -mode clean complex HTML is reduced to the subset that is allowd in markdown
wudict dump -format md -mode clean -o my-output-folder ldoce6.mdx
```

By default all resources (media, .js, .css) are included in the export. You can control what gets included with the `-resources` flag. See `wudict dump --help` for details:

```
dump --help
Usage of dump:
  -compress string
    	md only: gz
  -format string
    	csv, or md (wudict markdown) (default "csv")
  -mode string
    	md only: html (each article's HTML preserved) or clean (markdown only, lossy) (default "html")
  -o string
    	output folder for the dump and its resources (created if missing)
  -output string
    	long form of -o
  -resources string
    	all, text (only .css, .js and other text files), or none (default "all")
```

also see [<strong>`wudict markdown`</strong>](<entry://wudict markdown>)

</details>

***

- [index](/browse?dict=wudict-howto)
