---
title: Home
description: wudict searches MDict, StarDict, Aard 2, Lingvo DSL, Babylon, ZIM and wudict markdown dictionaries in the browser, on macOS, Linux, Windows and Android.
---

# All your dictionaries, in your browser

**WuWeiDict** (`wudict`) searches all your local dictionaries at once and shows
the results in the browser, at [localhost:6888](http://localhost:6888). It is a
single executable with no dependencies.

[Install](start/install.md){ .md-button .md-button--primary }
[First run](start/first-run.md){ .md-button }
[Browser extension](extension.md){ .md-button }

---

## Formats

<div class="grid cards" markdown>

-   :fontawesome-solid-file-zipper:{ .lg .middle } **MDict**

    ---

    `.mdx` and `.mdd`. HTML articles; resources (audio, CSS, scripts) in `.mdd` archives.

-   :fontawesome-solid-book-bookmark:{ .lg .middle } **StarDict**

    ---

    `.ifo`, `.idx`, `.dict`. Synonym files, resources and dictzip compression
    are supported.

-   :fontawesome-solid-layer-group:{ .lg .middle } **Aard 2**

    ---

    `.slob`. One file with articles, images, audio, scripts and stylesheets.

-   :fontawesome-solid-language:{ .lg .middle } **Lingvo DSL**

    ---

    `.dsl` and `.dsl.dz`, with `.dsl.files.zip` resources.

-   :fontawesome-solid-earth-americas:{ .lg .middle } **Babylon**

    ---

    `.bgl`. Character sets detected automatically; resources read from the file.

-   :fontawesome-solid-globe:{ .lg .middle } **ZIM**

    ---

    `.zim`. Kiwix and Wikimedia archives, searched in place.

-   :fontawesome-solid-file-lines:{ .lg .middle } **wudict markdown**

    ---

    `.wudict.md`. A dictionary as one CommonMark file, readable and editable
    in any text editor.

-   :fontawesome-solid-box-archive:{ .lg .middle } **wudict library**

    ---

    `text.db`. wudict's SQLite format: one folder per dictionary, with an optional `media.db`; portable between machines.

</div>

[Formats](dictionaries/formats.md){ .md-button }

---

## Search

All dictionaries are searched at once, and each dictionary's results are shown
as soon as it answers.

| Mode | Matches | Needs |
| --- | --- | --- |
| **exact** | the headword, ignoring case and accents: `corazon` finds *corazón* | |
| **prefix** | headwords that start with the query | |
| **contains** | headwords that contain the query anywhere | contains index |
| **full-text** | words in the article text, ranked by relevance | full-text index |

The contains and full-text indexes are added per dictionary in the dictionary
panel (<kbd>☰</kbd>), which shows each index's size.

With lemma data, a search for an inflected form finds the lemma: *understood*
finds **understand**, *estuviera* finds **estar**. English is built in; other
languages are installed from <kbd>☰</kbd> → folder summary →
<kbd>Lemmatization…</kbd>.

[Search](start/search.md){ .md-button }

---

## Platforms

| Platform | |
| --- | --- |
| **macOS** | command line, and [wuDict.app](apps/macos.md) with a menu-bar icon |
| **Windows** | command line, and an [installer](apps/windows.md); tray icon |
| **Linux** | command line, with an optional systemd user unit ([Run at startup](running.md)) |
| **Android** | [app](apps/android.md), with lookup from the text selection and share menus and from reading apps |

---

## Privacy

No account, no telemetry, no analytics, no crash reporting. wudict connects to
the internet only to download what you ask for. See the
[Privacy Policy](privacy_policy.md).
