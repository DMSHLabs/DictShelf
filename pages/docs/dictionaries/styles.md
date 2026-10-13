---
title: Custom styles
description: Your own CSS for wudict and for dictionary articles - the editor, the examples, dark mode and a compact phone layout.
---

# Custom styles

Two optional CSS files:

| File | Applies to |
| --- | --- |
| `app.css` | wudict's own page: colours, layout |
| `article.css` | every dictionary article |

They are in the `style/` folder beside the `wudict.toml` in effect, usually
`~/.wudict/style/`, created on the first save, up to 256 KiB each.

## Editor

<kbd>☰</kbd> → folder summary → <kbd>Custom styles…</kbd> opens the editor,
docked at the bottom; the page above previews the CSS as you type.

-   **App** and **Article** edit the two files. A dot on a tab marks unsaved
    changes.
-   <kbd>Examples…</kbd> inserts a preset at the cursor as editable text. A
    preset that needs both files inserts a part into each. Inserting the same
    preset twice adds it once.
-   <kbd>Save</kbd> writes the files.
-   **Files** holds files the CSS can reference, such as fonts and images.
    <kbd>Add…</kbd> uploads one: up to 8 MiB each, 64 files, 64 MiB in total,
    stored in `style/assets/` and served at `/files/<name>`.
    <kbd>Insert</kbd> writes a rule that uses the file: for a font, the
    `@font-face` and font rules in both stylesheets; for an image, a background
    for every article; otherwise `url("/files/<name>")` at the cursor.

## Undo and disable

| To | Do |
| --- | --- |
| undo typing or an inserted example | <kbd>⌘ Z</kbd> / <kbd>Ctrl Z</kbd> in the box |
| discard changes since the last save | close the editor; the page returns to the saved files, the unsaved text stays in the box |
| delete a file | <kbd>Clear</kbd>, then <kbd>Save</kbd> |
| load the page once without either file | open `/?style=off` |

The files can be edited in any text editor; they are served with `no-cache`,
so a reload applies the change.

## App or Article

`article.css` styles dictionary markup and cannot reach wudict's page.

| Rule | File |
| --- | --- |
| a custom property (`--token`) | `app.css`: properties set on `:root` are inherited by articles |
| a selector for wudict's elements (`.col`, `details.dict`) | `app.css` |
| a selector for dictionary markup (`body`, `p`, `table`) | `article.css` |

``` css title="app.css — sepia"
html:not([data-dark]){
  --bg:#f4ecd8; --bg-card:#faf3e3; --bg-bar:rgba(244,236,216,.92);
  --fg:#3b3229; --fg-soft:#6b5f4e; --line:#e0d5bd; --line-soft:#ebe2cf;
  --wd-article-bg:#faf3e3; --wd-article-fg:#3b3229;
}
```

`--wd-article-bg` and `--wd-article-fg` are the article surface; the others are
wudict's own tokens.

A dictionary that sets its own background on its markup is not reached by the
tokens. The colour examples therefore also insert into `article.css`:

``` css title="article.css — let the app's colours through"
:host, :root > body,
:host > *, :root > body > * { background:none !important }
```

Two levels deep only: a coloured box inside an entry keeps its colour.

## Examples

| Example | Box | For |
|---|---|---|
| Sepia | App + Article | warm paper in light mode, page and definitions together |
| High contrast | App + Article | near-black on a warm off-white, not a blinding `#fff` |
| True black | App | black chrome for OLED, white text in articles |
| Warm dark | App | the default dark, warmed |
| Wider column | App + Article | a wider reading column, and dictionaries that cap their own width |
| Compact | App + Article | reclaim desktop side padding on a phone, and make what is left fit |
| Quieter results | App | fewer counts, badges and borders around the definitions |
| Roomier lines | Article | prose leading and air between paragraphs |
| Serif font | App + Article | one family everywhere, over the dictionary's own |
| Bolder text | App + Article | synthetic weight where the type looks washed out |
| Justified text | Article | justified, with hyphenation |
| Wide tables | Article | conjugation tables scroll inside themselves |
| Dictionary roles | Article | colours and fonts for the labelled parts of Lingvo DSL and XDXF articles: part of speech, example, transcription, comment, stress |
| Quiet examples | Article | examples in the text colour, one size smaller |
| Tighter indents | Article | half the indentation of DSL articles; `0` flattens them |

## Dark mode

`[data-dark]` is set on the page, on each article and inside each article frame
whenever the theme is dark, chosen or from the system:

``` css
html[data-dark]      { /* dark */ }
html:not([data-dark]){ /* light */ }
```

In `article.css`: `:host([data-dark]), html[data-dark] { … }`.

In dark mode wudict inverts each article's colours: dictionary stylesheets have
no dark variant. Write article colours for light mode, and use `[data-dark]` in
`article.css` only to exempt an element from the inversion. The article tokens
under `html[data-dark]` in `app.css` are also values before inversion:
`--wd-article-bg:#fff` gives a near-black article, `--wd-article-fg:#000` white
text. Hue is preserved.

## Compact layout on a phone

Dictionaries designed for a desktop window reserve side padding and fix their
widths. The *Compact* example removes both below a width of 700px:

``` css title="article.css — compact"
@media (max-width: 700px){
  :host, :root > body { margin-inline:0 !important; padding-inline:0 !important }
  div, section, article, main, header, footer, aside, nav,
  p, h1, h2, h3, h4, h5, h6, figure, blockquote, pre, dl, dt, table
    { margin-inline:0 !important; padding-inline:0 !important }
  ul, ol, dd, menu
    { margin-inline:0 !important; padding-inline-start:1.1em !important }
  td, th { padding-inline:.25em !important }

  :host, :root > body { width:auto !important; max-width:100% !important;
    overflow-x:hidden }
  :host *, :root > body *
    { max-width:100% !important; min-width:0 !important;
      box-sizing:border-box !important; overflow-wrap:break-word }
  div, section, article, main, header, footer, aside, nav,
  p, h1, h2, h3, h4, h5, h6, figure, blockquote, dl, dt, dd, ul, ol
    { width:auto !important; float:none !important }
  table { display:block !important; overflow-x:auto !important;
    border-collapse:collapse }
  table * { max-width:none !important }
  pre { overflow-x:auto !important; white-space:pre-wrap }
  img, video, svg { height:auto !important }
}
```

`:host` is the root of an article without scripts; `:root > body` the root of
one rendered in a frame. Only the root needs these prefixes; other selectors in
`article.css` reach only article markup. The `app.css` part of *Compact*
removes wudict's own padding around the article.

??? info "What each part does"

    -   Margins and padding are removed from block elements. Lists keep
        `1.1em`, so bullets stay visible (`0` with `list-style-position:inside`
        for flush lists); table cells keep `.25em`.
    -   No element keeps its own width or exceeds its container, and long words
        break. `box-sizing:border-box` keeps padded boxes within 100%. Widths in
        `em` grow with the font size, so without this an article scrolls
        sideways more at larger text sizes.
    -   Tables and code blocks scroll inside themselves; table cells are exempt
        from the width limit.

## Custom styles and resource overrides

A [resource override](override.md) replaces one file of one dictionary. Custom
styles are added after each dictionary's own CSS, for all dictionaries;
`!important` resolves a conflict.

## If a rule hides the page

Open `http://127.0.0.1:6888/?style=off`: the page loads without either file.
The editor saves there without previewing; fix the rule and reload without
`?style=off`.
