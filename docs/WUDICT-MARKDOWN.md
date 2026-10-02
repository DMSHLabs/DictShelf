# wudict markdown — format specification, version 1

Normative. The two examples of §10 are, byte for byte, `docs/wudict-markdown/examples/clean.wudict.md` and
`html.wudict.md`.

A wudict markdown file is **one dictionary in standard CommonMark**, plus GFM tables. Any stock
CommonMark renderer displays it correctly. Nothing in it needs a parser plugin: a dictionary reader only adds
the reading rules of §3 on top of the stock parse.

```
# Pocket English
wudict: 1
from: en
to: en

A small general-purpose English dictionary.

## run
## runs
## ran

*v.* /rʌn/

1. To move quickly on foot.
2. To operate or manage: *she runs a bakery*. Compare [walk](entry://walk).

## aesthetic

<div class="entry"><span class="pos">adj.</span> <span class="ipa">/iːsˈθɛtɪk/</span>
<div class="sense"><b>1</b> concerned with beauty</div></div>
```

## 1. Conventions

- MUST, SHOULD and MAY are used as in RFC 2119. A rule stated in the present tense is a MUST.
  - **Reader:** file → dictionary.
  - **Writer:** dictionary → file.
  - **Host:** the application that displays articles.
- **Markdown** means CommonMark 0.31.2 with raw HTML enabled, plus one GFM 0.29 extension: **tables** (§4.10). No
  other syntax has meaning in this format.
- An extension is used only where CommonMark has no readable equivalent. Tables qualify, because a raw `<table>`
  is impractical to edit by hand. Strikethrough does not, because inline `<del>` is standard CommonMark.
- Where goldmark v2.1.5, the reference reader, starts an HTML block differently from CommonMark, a reader starts
  it as goldmark does (Appendix A).
- **WS** is U+0020 or U+0009. **Trim** removes leading and trailing WS only. **Control characters** are
  U+0000–U+001F and U+007F. **HTML whitespace** is SP, TAB, LF, CR and FF. A **letter, digit or mark** is a code
  point of Unicode category L, Nd or M.
- **Ignoring case** compares code points after the Unicode simple lowercase mapping of each (`İ` → `i`; no
  final-sigma or other context rule).

## 2. File

- **R2.1** The file is UTF-8. Readers remove a leading BOM and turn CRLF and lone CR into LF. An invalid UTF-8
  sequence becomes one U+FFFD per byte, and U+0000 becomes U+FFFD.
- **R2.2** The file is named `<stem>.wudict.md` or a plain `<stem>.md` (compressed: R2.4). Whatever its name, a file
  is a dictionary only when its lines 1-2 are the title and the `wudict` field (R3.1): that field is the gate,
  and the name only nominates the file. A file that fails the gate is not a dictionary: a reader ignores it where
  it finds it (a folder, an archive), and reports why when it was asked to open that file. Writers write
  `.wudict.md`.
- **R2.3** Resources live beside the file, in a folder named like the file with `.gz` or `.dz` dropped and its
  final `.md` replaced by `.files` (`<stem>.wudict.files/`, `<stem>.files/`), or in a zip archive of that name +
  `.zip`; a reader looks in the zip first. Relative references in bodies resolve inside it, `.` and `..` resolved
  at its root, and MUST NOT leave it. A reader MAY also resolve a reference to a file beside the dictionary.
- **R2.4** Packaging:
  - `<stem>.wudict.md.gz` is the file, gzip-compressed. Readers SHOULD read it, including concatenated members, and
    `.wudict.md.dz` (dictzip) as gzip.
  - Writers MAY write `.gz`, with mtime 0 and no name or comment.
  - Decompression MUST be bounded. Readers accept at least 1 GiB of decompressed markdown; past their bound, or
    when the file is not gzip → **E-format**.
  - No other compression or archive belongs to version 1.
- **R2.5** Writers end the file with exactly one LF and write no trailing WS on any line, except inside HTML
  blocks and code blocks.

## 3. Structure

A reader parses the whole file as Markdown (§1) and reads the dictionary off the **top-level blocks** of the
resulting document. It does no line scanning of its own beyond R3.1.

- **R3.1 Header.**
  - Line 1 is an ATX level-1 heading: `#`, SP or TAB, and the title. The title is its text (R3.4) and is
    non-empty.
  - Line 2 is `wudict: <version>`, where the version is `MAJOR` or `MAJOR.MINOR` (ASCII digits, leading zeros
    allowed). It is the version of this specification, not a flag.
  - If either condition fails → **E-format**. If MAJOR is unsupported → **E-version**. A reader accepts every
    MINOR of a MAJOR it supports; a MINOR adds only header keys and file extensions (Appendix B).
  - A line 2 that plainly means to be the field (`wudict` in any case, optional WS, `:`) but is not exactly
    `wudict: <version>` (`Wudict:1`, `wudict :1`, `wudict: one`) is E-version with the message "line 2 must
    be `wudict: 1`".
- **R3.2 Header fields.**
  - Line 2 starts the header paragraph. Each of its lines, its indentation ignored, is `key: value`: the key
    `[a-z][a-z0-9-]*`, `:`, one or more WS, and a non-empty value, trimmed and taken literally.
  - `from` and `to` are the BCP 47 tags of the headwords and of the article text; the first of each counts. A
    repeated `wudict` is ignored.
  - Any other key is a header field, kept in order and shown as plain text.
  - The paragraph ends at the first blank line. A `---` or `===` line right under it would make it a setext
    heading → E-format. A line in it that is not `key: value` ends the header, and it and the rest of the
    paragraph are ignored → W-entry.
- **R3.3 Description.** The top-level blocks after the header paragraph and before the first entry form the
  description, an ordinary body (§4).
- **R3.4 Heading text.** The text of a heading is the text content of its parsed inline content: escapes
  resolved, entities decoded, code spans kept as their content, an autolink as its URL as written, an image as
  its alt text, emphasis, link and raw-HTML markup dropped with their text kept. The result is trimmed.
  Headwords are compared as exact code-point sequences; readers do not case-fold or normalize.
- **R3.5 Entries.** Every **top-level ATX level-2 heading** (`## `) starts an entry. A setext level-2 heading
  (text over `---`) is content: hand-written markdown often has one.
  - **Heading group.** Headings with no block between them form one entry; blank lines between them do not
    count. Link reference definitions do not count as blocks; an HTML comment does. The first heading is the
    **headword** and the others are its **aliases**, in order. Headings with empty text are skipped, and repeats
    within a group are dropped.
  - **Body.** The top-level blocks after the group, up to the next entry or the end of the file.
  - A heading inside a list, a blockquote, an HTML block or a code block is content, as Markdown defines it.
    A line that would start an entry (`##`, then WS or the line end, at column 0) inside an HTML block is almost
    always one the block swallowed by accident: a type-6/7 block runs to the next blank line, a `<pre>`,
    `<script>`, `<style>` or `<textarea>` block to its closing tag. It stays content, and → W-entry, naming that
    line and the block's first.
  - Repeated headwords across groups are separate entries (homographs).
- **R3.6 Redirect.** An entry whose body is exactly one paragraph of exactly one line `see: <target>`, its
  indentation ignored, is a redirect. The target is the literal text after `see:` and WS, trimmed.
  - Every name of the entry leads to the entries whose headword or alias equals the target.
  - Readers resolve nothing beyond that one step, and match the target exactly or, failing that, ignoring case
    (§1).
  - If no entry matches, the redirect is shown as a lookup link to the target (§5). A reader MAY yield it as an
    article whose body is that link, under every name of the entry.
- **R3.7 Empty entry.** A heading group followed by no body (end of file) is not an entry → W-entry.

## 4. Bodies

- **R4.1** A body is Markdown (§1). Readers render it with the stock renderer of their Markdown library:
  - raw HTML passed through;
  - no typographer, no linkify, no automatic heading ids;
  - no sanitizing (R9.1).

  Link reference definitions apply file-wide, as in CommonMark.
- **R4.2** Anything Markdown cannot express stays **raw HTML**. That is standard Markdown: inline tags
  (`bank<sup>1</sup>`) and HTML blocks, including a whole body as one HTML block (R6.4).
- **R4.3** `<details>` holding Markdown is written as an HTML block, a blank line, the Markdown, a blank line and
  `</details>`:

  ```
  <details>
  <summary>More examples</summary>

  - *run a risk*
  - *run late*

  </details>
  ```
- **R4.4** Admonitions (`!!! note "Title"`, zensical/Material style) have **no meaning in version 1**. They render
  as stock Markdown renders them: `!!! note` as text, and an indented body as a code block. A later MINOR may
  give them meaning.
- **R4.5** Strikethrough is inline HTML: `<del>x</del>`. `~~x~~` has no meaning in version 1. It is a GFM extension
  that some viewers enable, so writers escape `~` in text so that such viewers show the same text.

## 5. Links and resources

- **R5.1** A **lookup link** is `entry://` + target, optionally followed by `#` + fragment.
  - In the target, `%`, `#`, control characters and a leading `@` are percent-encoded (`%25`, `%23`, `%XX`,
    `%40`), and nothing else.
  - As a Markdown destination it is written in angle brackets when it contains a space:
    `[long run](<entry://long run>)`.
- **R5.2** Reading a lookup link:
  1. Take the text after the scheme (`entry:` or `bword:`, with or without `//`, case-insensitive; also `d:`,
     `x:`).
  2. Split at the first literal `#`.
  3. Percent-decode each half. The decode is all or nothing: if a half does not decode to valid UTF-8, it is
     kept as written.
  4. Trim the target.

  An empty target is an anchor in the current article. A target whose **undecoded** form starts with `@` and is
  longer than one character is an MDict sub-entry, which a host may inline. Writers emit only `entry://`.
- **R5.3** Stock renderers percent-encode destinations. That is allowed, because R5.2 decodes. A renderer MUST NOT
  decode or double-encode an existing `%HH`.
- **R5.4** A **resource reference** is a relative `src`, or a relative `href` whose last segment, `#…` and `?…`
  cut, ends in a file extension of Appendix B, ignoring case. It resolves in the container (R2.3): `#…` and `?…`
  are cut, a leading `./` (and, in a `src`, `/`) is dropped, and the rest is percent-decoded.
  - A relative `href` that starts with `/`, `#` or `?`, or lies under `res/` or `assets/`, is the host's.
  - Any other relative `href` names a headword, and is read as a lookup link (R5.2).

## 6. Writer

- **R6.1 Layout.**

  ```
  # <name>
  wudict: 1
  from: <tag>                     (when known)
  to: <tag>                       (when known)
  <key>: <value>                  (each header field, source order)

  <description>                   (when non-empty)

  ## <headword>
  ## <alias>                      (each, source order, then folded redirects)

  <body>

  ## <name>                       (a redirect that reaches no article, R6.4)
  see: <target>
  ```

  Sections are separated by one blank line. The headings of a group are on consecutive lines, and a `see:` line
  follows its group directly.
- **R6.2 Names** (title, headwords, aliases, targets, header values):
  - invalid UTF-8 → U+FFFD, as R2.1 repairs it;
  - CR, LF and TAB → SP; other control characters (NUL included) removed; trimmed;
  - an empty headword is replaced by the first non-empty alias; an entry with no name is dropped;
  - repeats in a group are dropped.

  In headings, the writer backslash-escapes `` \ ` * _ [ ] < > & ~ ``, and escapes the first `#` of a trailing run
  of `#` that follows WS (`## C \#`).
- **R6.3 Header keys.**
  - A source key is lowercased, each run of characters outside `[a-z0-9]` becomes `-`, and leading and trailing
    `-` are trimmed.
  - A key starting with a digit, or equal to `wudict`, `from`, `to` or `meta`, gets the prefix `x-`.
  - A key that ends up empty is written `meta: <original key>: <value>`.
  - A multi-line value is joined with SP.
- **R6.4 Redirects are folded.**
  - A source redirect to a target that is a headword or alias of an article becomes an alias of **every** such
    article, after the article's own names. Chains are followed to an article, stopping at a repeat.
  - A redirect whose target matches no article is written as its own entry with the body `see: <target>`.
  - An article with an empty body is dropped, and the writer counts it.
- **R6.5 Modes.** The writer has two modes, chosen for the whole file, never per article. In both, a body is
  first repaired as R2.1 repairs a file (invalid UTF-8 and NUL → U+FFFD): a writer never emits what a reader
  would have to repair.
  - **`clean`:** every body goes through the allowlist of R6.6, then the conversion of R6.7. Cleanup is
    destructive and best effort: anything outside the allowlist is dropped or unwrapped, never kept.
  - **`html`** (default, since it loses nothing): every body is written **verbatim** as one HTML block:
    - The body is wrapped in `<div>` + LF … LF + `</div>` **unless** it is already a single element with nothing
      around it but HTML whitespace, whose start tag opens a type-6 HTML block (`div`, `section`, `table`, …):
      the tag name followed by SP, `>` or `/>`.
    - The block holds **no blank line**: inside `pre`, `textarea` and `listing`, a blank line is written as
      `&#10;` at the start of the following line; elsewhere, blank lines are removed.
    - Lookup links take their canonical spelling (`bword:` → `entry://`, `entry://@x` → `entry:@x`), because
      writers emit only `entry://` (R5.2).
    - Nothing else changes.
- **R6.6 The `clean` allowlist.**

  | Source | Written as |
  |---|---|
  | `p`; `div`, `section`, `article` and other block containers (unwrapped) | paragraph breaks |
  | `br` | `\` + LF inside a paragraph (`<br>` + LF right after a text `\`); dropped at its end; doubled ones merge |
  | `em`, `i` · `strong`, `b` | `*x*` · `**x**` when the content starts and ends with a letter, digit or mark, holds no unescaped `*` and does not follow a `*`; otherwise inline `<em>` or `<strong>` |
  | `sup`, `sub`, `u`, `small`, `ins`, `del` (`s` and `strike` become `del`), `kbd` | the same tag as inline HTML, without attributes |
  | `code`, `samp`, `tt` · `pre` > `code` | code span (inline `<code>` right after a backtick) · fenced code block (info from `language-X`) |
  | `a[href]` | `[text](dest "title")`, `dest` in the canonical form of R6.9; an `a` without `href` is unwrapped |
  | `img[src]` (`image` is `img`, as HTML parses it) | `![alt](src "title")` |
  | `audio`, `video`, `source`, pronunciation `object` | `[▶](file)` |
  | `ul`, `ol[start]`, `li` | `-` / `1.` lists, R6.10 |
  | `blockquote` | `> ` lines |
  | `h1`–`h6` | `###`–`######`; `h1` and `h2` are **demoted** to `###` |
  | `hr` | `***` |
  | `table` | GFM table: the first row is the header; cell alignment (`align`, or `text-align` in `style`) is kept; block content in a cell is flattened with `<br>` |
  | `details`, `summary` | R4.3 |
  | `script`, `style`, `template`, `iframe`, `noscript`, `noembed`, `noframes`, `canvas`, `map`, `embed`, form controls (`input`, `select`, `textarea`, `button`), document parts (`head`, `title`, `meta`, `link`, `base`, `frame`, `frameset`), other `object` | removed with their content |
  | anything else | unwrapped: its content is kept; one that holds a block (by tag or by the stylesheet) is laid out as CSS lays out a block inside an inline box: the text before, the block and the text after are separate blocks |
  | every attribute not named above | removed |

  **The dictionary's stylesheet.** A dictionary's layout is often in its CSS, not in its tags: nested
  `<span class="…">` elements the stylesheet makes blocks, hides, or spaces apart. So `clean` MAY read the
  stylesheets its articles link (`<link rel="stylesheet">` to a resource of the dictionary) and reduce them to one
  table: for each class, whether it is a block, inline or hidden (`display`), and whether it is set apart (a
  positive `margin` or `padding` on its left or right). Only class selectors are read, by the rightmost compound;
  pseudo-classes, attribute and id selectors are ignored; a compound selector (`.a.b`) never hides. An element
  takes the strongest display of its classes (block, then inline, then hidden). Then:
  - an element the stylesheet hides is removed with its content;
  - an element that is inline by tag and a block by its classes is a paragraph break, keeping its own markup when
    it has some (`b`, `a[href]`, …); inside a heading, a cell or inline markup it is a hard break (a SP in a
    heading);
  - an inline element the stylesheet sets apart has a SP on either side.

  A dictionary without a stylesheet, or a writer that reads none, converts by tags alone.
- **R6.7 Conversion.**
  - Text whitespace runs become one SP; lines break only at hard breaks and block boundaries.
  - WS at the start or end of an element's content that is written as markup (emphasis, the inline tags, a link)
    is written outside it: `a<b> x </b>y` → `a **x** y`, `cart<a href="…"> at upset</a>` → `cart [at upset](…)`.
  - Text backslash-escapes `` \ ` * _ [ ] < > & ~ | ``, and a `!` right before a link's `[`.
  - At the start of a line - of a paragraph, or after a hard break - text also escapes `#`, `+`, `-`, `=` and `:`
    (`:` and `-` could start a table delimiter row), and the delimiter of a 1-9 digit run followed by `.` or `)`
    and then a space, a tab or the end of the line (an ordered list marker). This is decided on the finished line,
    whatever node its text came from.
  - A body whose only block would be the one line `see: …` has its `:` escaped (`see\: …`).
  - Code spans use the shortest backtick run not in the content. Fences are max(3, longest run + 1) backticks.
  - Inside a table cell, a `|` in a destination is written `%7C`, and in a title or alt text `\|`.
  - The writer writes no link reference definition: one applies file-wide (R4.1).
- **R6.8 Failure.** If a `clean` body cannot be written, the writer **aborts**, names the entry, and points to
  `html` mode. It leaves no partial output (it writes to a temporary file and renames it on success). A body
  cannot be written when:
  - parsed as a reader will parse it, it holds a top-level level-1 or level-2 heading, which would end the
    entry;
  - it exceeds a limit of the writer (wudict: 512 open elements).
- **R6.9 Destinations.** A stock renderer percent-encodes destinations (R5.3), so a destination is written in a
  canonical form that reads the same after a round trip, and stays readable:
  - a lookup link: `entry://` + the target percent-decoded (R5.2) and encoded again by R5.1, so
    `[вода](entry://вода)`, not `entry://%D0%B2…`; a sub-entry keeps its raw `@` (`entry:@sub`); `bword:`, `d:`
    and `x:` become `entry:`;
  - a link's relative `href` that names a headword (R5.4): written as the lookup link, `entry://` + the target,
    and `#` + the fragment when there is one (`<a href="cooking apple#s2">` → `[…](<entry://cooking apple#s2>)`);
  - any other relative path (a resource): percent-decoded, with only `%`, `#`, `?` and control characters
    encoded; a `?…` or `#…` suffix is kept, encoded as a URL is;
  - any other URL: every byte percent-encoded except the ASCII letters and digits and
    ``!#$%&'()*+,-./:;=?@_~``, the characters the renderer keeps, so it changes nothing;
  - in markdown, a destination that is empty or holds SP or a control character is written in angle brackets,
    with `\`, `<`, `>` and `&` backslash-escaped and LF and CR written `%0A` and `%0D`; any other is written
    bare, with `\`, `(`, `)`, `<` and `&` backslash-escaped.
- **R6.10 Lists.**
  - Markers are `-` and `N.`, numbered from `start`. A list right after a list of the same kind and marker
    switches to `*` or `N)`, so the two stay two.
  - An empty item is left out: a bare marker is where parsers disagree.
  - A list is tight when each item is one block, or a paragraph followed by sub-lists that can interrupt it (a
    first item that is not empty and, if ordered, starts at 1). Otherwise it is loose: its items and their blocks
    are separated by blank lines.
  - Continuation lines are indented by the marker's width plus one.
  - Lists and quotes nest at most 16 deep; deeper levels join their parent's flow.

## 7. Guarantees

- **R7.1** `html` mode keeps every article's HTML, except for the wrapper `<div>`, the blank lines removed outside
  `pre`, `textarea` and `listing`, and lookup links respelled to `entry://` (R6.5).
- **R7.2** `clean` mode is lossy by design. It keeps text, the allowlisted structure, links and media.
- **R7.3** For a file F that a writer produced in mode M, exporting the import of F in mode M SHOULD give F back.
  Conformance tests check this; it is not a proof obligation of the writer.
- **R7.4** The writer is deterministic: the same input gives the same bytes. Two conforming writers given the
  same dictionary and mode write the same bytes, except where one reads stylesheets (R6.6) and in gzip framing.

## 8. Diagnostics

| Code | Condition | Effect |
|---|---|---|
| E-format | line 1 is not `# title`; line 2 is not the `wudict` field; the header is a setext heading; a compressed file that is not gzip or unpacks past the reader's bound (R2.4) | not read |
| E-version | unsupported MAJOR, or a malformed or near-miss line 2 | not read |
| W-entry | a heading group without a body, a malformed header line, an entry line inside an HTML block (R3.5) | skipped (the line: read as content), reading continues |

The writer reports counts: dropped empty entries, entries without a name, names repaired by R6.2. A `clean`
failure is the only writer error (R6.8).

## 9. Security

- **R9.1** Files are untrusted. Raw HTML, event attributes and `javascript:` links pass through the reader, so
  hosts MUST sanitize or isolate article HTML before display.
- **R9.2** Decompression is bounded, and resource resolution never leaves the container (R2.3).
- **R9.3** Parsing time is not bounded by this spec. Stock CommonMark parsers can be superlinear on hostile shapes
  (measured on goldmark v2.1.5: list markers nested on one line, `- - - … x`, 40,000 deep in 2.3 s; 40,000
  reference definitions in one entry in 0.5 s, both quadratic). A reader MAY refuse such input; wudict does not
  guard against it.

## 10. Examples

The same entries in both modes.

**Source articles (HTML):**
- `run`, aliases `runs`: `<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="bword://walk">walk</a>`;
- `ran` → redirect to `run`;
- `C#`: `<p>A language.</p><h2>History</h2><p>2000.</p>`.

**`clean`:**

```
# Sample
wudict: 1

## run
## runs
## ran

v. **1** to move quickly on foot [walk](entry://walk)

## C#

A language.

### History

2000\.
```

**`html`:**

```
# Sample
wudict: 1

## run
## runs
## ran

<div>
<span class="pos">v.</span> <b>1</b> to move quickly on foot <a href="entry://walk">walk</a>
</div>

## C#

<div>
<p>A language.</p><h2>History</h2><p>2000.</p>
</div>
```

In both modes the source's `bword://walk` is written `entry://walk`. That respelling is the only change `html`
mode makes inside a body.

**Import of either:** three entries with these names:
- `run` / `runs` / `ran`;
- `C#`.

The `C#` body is `<p>A language.</p><h3>History</h3><p>2000.</p>` (clean; `2000\.` escapes what would otherwise be
an ordered list starting at 2000, R6.7) or the `<div>` block (html).

## Appendix A. Implementer notes (informative)

**goldmark's HTML block starts** (§1), where they differ from CommonMark 4.6:
- `<pre/>`, `<script/>`, `<style/>` and `<textarea/>` start type 1;
- `</textarea>` and `</ span>` (spaces after `</`) start type 7;
- `meta` is a type-6 name;
- a type-6 name followed by a TAB starts no block, nor does one followed by the line end, except on the last line
  of the file;
- type 4 needs `<!` and an uppercase ASCII letter: `<!doctype html>` is a paragraph;
- type 7 allows only spaces, not TABs, before and after the closing `>`.

**Chunked reading.** A reader may parse the file in chunks cut before top-level `## ` lines: such a line ends every
open block except a fenced code block or an HTML block that runs across it, so the cut is confirmed by the parse.
Link reference definitions apply file-wide, so they are collected before bodies are rendered.

**Go.**
- goldmark with `extension.Table` and `html.WithUnsafe()`. There is no custom parser or renderer.
- Walk `doc.FirstChild()` siblings for R3.
- Render an entry by rendering its node range with the stock renderer.
- Verify the R5.3 behaviour of goldmark's destination escaping on `%HH` before relying on it.
- goldmark makes a table of a header row with fewer cells than its delimiter row, padding it (GFM Example 203
  says it is no table). Writers never write one.

**Python.**
- `MarkdownIt("commonmark", {"html": True}).enable("table")`, with its block rules adjusted to read as goldmark:
  HTML blocks start as listed above; headings and paragraphs are trimmed of WS only; the lines after a link
  reference definition stay in its paragraph, and a definition ends before a setext underline; an autolink's
  text is its URL as written.
- Walk the token stream's top-level `heading_open` tokens (`level == 0`) with markup `##`.
- `md.normalizeLink` percent-encodes non-ASCII, which R5.3 permits.

**pyglossary.**

| wudict | pyglossary |
|---|---|
| heading group | `newEntry([headword, *aliases], body, defiFormat="h")` |
| title, `from`, `to`, header fields, description | the `name`, `sourceLang`, `targetLang`, other and `description` info keys |
| resources | `newDataEntry` |

On write, pyglossary has no redirects: its terms list is already the folded form of R6.4.

## Appendix B. File extensions

The extensions of R5.4, which a later MINOR may extend: `.css .js .html .htm .png .jpg .jpeg .gif .webp .svg .bmp
.ico .avif .mp3 .ogg .oga .wav .spx .m4a .opus .flac .aac .mp4 .webm .ogv .mov .m4v .3gp .avi .wmv .mkv .mpg .mpeg
.asf .flv .pcx .dcx .wmf .emf .tif .tiff .pdf .woff .woff2 .ttf .otf .eot .json .xml .txt`.

## Appendix C. The wudict command line (informative)

On a `clean` failure (R6.8), `wudict dump` writes the whole raw entry (names and source HTML) to stdout, and to
stderr:

```
error: entry "run" (#1234): <construct> cannot be written as clean markdown: <reason>
hint: keep the dictionary's HTML instead:
  wudict dump -format md -mode html -o <outdir> <dictfile>
```

## Revisions

- 2026-10-02: only an ATX `## ` heading starts an entry (R3.5); goldmark's HTML block starts are the reference
  (§1); an entry line swallowed by an HTML block is reported (R3.5); R2.1 repairs per byte; header, heading-text,
  case-folding, destination-escaping and allowlist details made exact; stylesheet reading is a MAY; the
  extension list and the command line move to appendices.
