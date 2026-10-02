# DSL differential comparison

This is a development tool, not part of the Android build. It runs the original
GoldenDict C++ `ArticleDom` and heading functions against the experimental Go GD
parser. No source dictionaries or application indexes are modified.

## Scope

The C++ algorithms in `vendor/parser.inc` and `vendor/folding.inc` are extracted
unchanged from the local GoldenDict sources. Copyright notices and GPLv3 license
are retained; `vendor/provenance.json` records SHA256 of the input files.
`prepare.py` regenerates the extracts using function boundaries, not line numbers.
The Qt6 adapter uses UCS-4 strings and replaces only the QRegExp API with an
equivalent anchored QRegularExpression. Scanner and index builder are NOT included.
The reference now also extracts `nodeToHtml`, HTML escaping, link trimming,
numeric/named language tables and their original Unicode folding helpers.
Only the resource/media arm of `nodeToHtml` is replaced by an explicit counted
placeholder. Qt6 string/UTF-8 and URL-query adapters supply the surrounding API;
the other renderer branches remain unchanged. The preparation script records
hashes for every original source and retains GPL notices.

Input is a JSON array of `{id, key, headings, body}` cases. Both adapters produce
heading keys, a DSL tree and raw HTML. Optional `abbreviations` supplies the same
explicit expansion map to both renderers; this tests rendering, not the native
companion loaders. Optional `source` supplies GD's main filename for file URLs.
Heading order/duplicate keys, attribute order and adjacent text-node boundaries
are ignored. Text whitespace, tag names, nesting and attribute values are not.
The Go adapter applies the existing legacy IPA conversion to exported text,
because Go does this at HTML rendering time while GoldenDict does it in ArticleDom.
It exports retained DSL media payloads instead of the prepared media HTML. Escaped
literal spaces are kept distinct until the IPA conversion is done. Closed empty
tags are now removed in the Go tree as in GoldenDict. The report does not claim
visual or full-index parity; complex media payloads still need dedicated cases.

## Optional Enhancer (2026-10-02)

Production GD reader v4 enables a separate HTML preparation pass after the base
parser. `GDOptions{}` / `NewGDReaderWithOptions` disables it for direct reading;
ordinary DSL is unchanged. `compare.py` and every corpus/reference comparison
default to OFF regardless of inherited environment. Add `--enhance` to compare
the optional Go output; the original C++ oracle never runs the Enhancer.
Both the selected mode and raw outputs are recorded. `--enhance` does not add
the bundled stylesheet to test dumps; styling is a separate reader option.

The user's `GoldenDictEnhancer/GoldenDict Enhancer src v2.3.js` supplies the
reference behaviour: remove empty paragraph separators before blocks/end, unwrap
obsolete glyph spans, and merge pure wrapper chains. This is a conservative Go
HTML-tree implementation: native bold/italic tags stay intact, conflicting
attributes/roles stay nested, and links, titles, languages, media and block
semantics are retained. GoldenDict-specific audio scripts, buttons, article
folding, lazy loading, scrolling and image click handlers are not ported.

GD HTML also marks example-only paragraphs with `wu-xonly` during generation,
including paragraphs whose example role was moved onto the paragraph by cleanup.
Mixed example/translation lines are not marked. `Examples Show|Hide` remains the
existing CSS-layer command; no control or script is embedded in an article.
The browser marker stays as an idempotent fallback for ordinary/older articles.

Bundled GD presentation uses scoped `wu-gd` selectors and unique font families,
with TTF files from `internal/server/web/fonts` served at `/assets/gd/fonts/`.
The original CSS's Android `file://` paths are replaced by application URLs.
The main document registers font faces for Shadow DOM; generated GD HTML imports
the scoped stylesheet. Its application URL is exempt from dictionary resource
rewriting. Presentation asset URLs are pinned to GD reader v4: change their
version with a future bundled style/font update. Existing GD indexes need a
rebuild; ordinary indexes do not. Disable both options for byte-stable oracle
tests, not by changing the production index configuration.

Verified: GD and ordinary DSL package tests, targeted server asset/resource/style
tests, baseline 19-case HTML comparison with cleanup OFF, and 19-case tree
comparison with cleanup ON. Chromium at 1100/390px loads all five font faces,
applies italic/gray optional-example styles even after wrapper merging, and the
actual existing Examples commands hide/restore only example lines. Browser
report/screenshots are local ignored outputs in `results/enhancer-preview`.
No Android/device/APK check. Browser verification tool:
`verify_enhancer.cjs` requires Playwright; point
`PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` at an available Chromium browser.

```bat
"%PY3%" tools/dslcompare/compare.py --html --qt-bin "%QT%\bin" --output tools/dslcompare/results/enhancer-off
"%PY3%" tools/dslcompare/compare.py --html --enhance --qt-bin "%QT%\bin" --output tools/dslcompare/results/enhancer-on
```

## Text HTML (Stage 1)

`--html` additionally compares parsed HTML against original GoldenDict output.
Raw outputs remain in `oracle.json` / `go.json`. The explicit vocabulary mapping
in `html_compare.py` translates known role classes, margin/color parameters,
abbreviation wrappers, entry URL encoding and cross-dictionary targets. It ignores
only generated optional-span IDs and wudict's author-token `data-lang` selector.
The stress wrapper is translated only after verifying its second branch is the
first branch plus the accent. All other attributes, text whitespace, break nodes,
language codes and tooltip values are compared, not silently discarded.

Articles containing media are excluded from HTML comparison altogether (IDs and
counts are reported); their keys/trees are still compared. Media output, resource
resolution, CSS, browser layout, optional-section controls and actual navigation
are outside Stage 1. A vocabulary match is NOT visual equivalence.

`html_spacing_only` is a diagnostic category, NOT an accepted match: these IDs
remain differences and cause exit 1. It helps distinguish break/ASCII-whitespace
differences from differences in tags, links, tooltips or non-whitespace text.
Known retained differences: GD uses empty paragraphs for newlines; wudict trims
block-edge spacing and uses breaks. GD's numeric Chinese code is literally `ch`,
while wudict retains valid `zh-Hans` / `zh-Hant` and other script-specific BCP-47
codes. Neither difference is hidden by the comparator.

```bat
"%PY3%" tools/dslcompare/compare.py --html --cases tools/dslcompare/html_cases.json --qt-bin "%QT%\bin" --output tools/dslcompare/results/html-fixtures
"%PY3%" tools/dslcompare/corpus.py D:/Dictionaries/wuDict/Dictionaries --html --limit 1000 --qt-bin "%QT%\bin" --output tools/dslcompare/results/html-corpus
```

Go GD reader version 3 fixes reference-target ASCII-space normalization, external
URL trimming, exact abbreviation lookup and short nonbreaking tooltips, NFC text,
IPA text in link targets, unknown-tag attribute retention and GD's strict language
attribute syntax. Tags `trs`, `!trn` and stray `preview` are unknown in this GD
renderer and retain their visible markers. Ordinary DSL parsing is unchanged.
Existing GD indexes are invalidated by the reader-version change.

Stage 1 sample (2026-10-02): all 28 files, up to 1000 top-level cards per file,
27,002 cards total. HTML compared for 24,667; 2,335 media-bearing cards excluded.
2,635 match the explicit vocabulary mapping; 22,032 differ only in break nodes
or ASCII whitespace; no other HTML or key/tree differences in this sample.
This is NOT the earlier exhaustive 3,259,925-card key/tree pass. The 24 focused
HTML cases leave three strict differences: two spacing cases and Chinese language
codes; one media case is excluded. All 19 original cases match (18 HTML compared).
Go package tests, 9 Python tests, vet and build pass. No browser/device check.
Generate the per-file classification without modifying raw adapter results:

```bat
"%PY3%" tools/dslcompare/html_report.py tools/dslcompare/results/html-corpus
```

`--dsl` splits ordinary top-level cards in Python and supplies the SAME fragments
to both parsers. It supports UTF-8, BOM UTF-16 and `.dsl.dz`. This is not a test of
either engine's file scanner: includes, header code pages, multiline comments
across cards, subentry indexing and blank-line preservation need a future native
scanner adapter. Use explicit JSON cases for exact whitespace and malformed input.

## Build on this Windows machine

Requires Python 3, Go, CMake, Ninja, MinGW and Qt6 Core. The default `python` here
is Python 2; use the bundled Python 3 path, or your own Python 3 interpreter.
These commands are for cmd, run from the repository root:

```bat
set "PY3=%USERPROFILE%\.cache\codex-runtimes\codex-primary-runtime\dependencies\python\python.exe"
set "CMAKE=C:\Qt\Tools\CMake_64\bin\cmake.exe"
set "QT=C:\Qt\6.11.2\mingw_64"
"%CMAKE%" -S tools/dslcompare -B tools/dslcompare/build -G Ninja -DCMAKE_MAKE_PROGRAM=C:/Qt/Tools/Ninja/ninja.exe -DCMAKE_CXX_COMPILER=C:/Qt/Tools/mingw1310_64/bin/g++.exe -DCMAKE_PREFIX_PATH="%QT%"
"%CMAKE%" --build tools/dslcompare/build
"%PY3%" tools/dslcompare/compare.py --qt-bin "%QT%\bin"
"%PY3%" tools/dslcompare/compare.py --qt-bin "%QT%\bin" --dsl "test_data/Oxford (En-Ru).dsl.dz" --limit 100 --output tools/dslcompare/results/oxford
```

Only when updating the original reference extraction:

```bat
"%PY3%" tools/dslcompare/prepare.py D:/Projects/Android/goldendict
```

Outputs: `input.json`, `oracle.json`, `oracle.log`, `go.json`, `summary.json`,
`diff.txt` under the ignored results directory. Exit 0 means all compared fields
match; 1 means differences; other errors (including subprocess failure) abort.
Each subprocess has a five-minute timeout. `--limit 0` scans all top-level cards;
both adapters currently buffer their case lists, so begin with a bounded sample.

For complete dictionaries, use `full.py`, which streams all cards in batches and
builds the Go adapter once. The output directory must be new. Complete failing
batches are retained as `diff-NNNNN`; successful batches reuse one `work` folder.
The corpus report distinguishes completed files from partial/error results.
After fixing parser errors, `recheck.py` rebuilds Go and reruns every retained
failing batch, recording remaining differences in `rechecked.json` without
overwriting the original report.

```bat
"%PY3%" tools/dslcompare/full.py D:/Dictionaries/wuDict/Dictionaries --qt-bin "%QT%\bin" --batch 1000 --output tools/dslcompare/results/full
"%PY3%" tools/dslcompare/recheck.py tools/dslcompare/results/full --qt-bin "%QT%\bin"
```

## Verification (2026-10-02)

Qt6/MinGW reference builds and runs. All 19 curated cases match after fixing Go's
`^~` alternate/subentry headings, closed empty-node cleanup, malformed `lang id`,
literal carets and escaped/tilde media filenames. GD reader version is 2;
ordinary DSL reader behaviour is unchanged.
Comparator fixes cover spaced assignments (`id= 2`) and media/IPA stage alignment.
`corpus.py` samples every DSL file recursively; file decoding streams rather than
decompressing entire large files before taking a sample. Example:

```bat
"%PY3%" tools/dslcompare/corpus.py D:/Dictionaries/wuDict/Dictionaries --qt-bin "%QT%\bin" --limit 1000
```

The subsequent complete run reads all 28 user corpus files, including both new
Urban parts, to EOF: 3,259,925 top-level cards. The frozen original Go adapter
found 134 differences; all retained failing batches pass after fixes for literal
`^` (including mathematical powers) and DSL escaping/tilde expansion in media
filenames. Urban p1: 702,374 cards; p2: 716,177 cards, both match and decompress
fully. Current curated cases: 19/19 match. `results/full/report.md` and
`rechecked.json` contain the final per-file results; `corpus.json` retains the
initial differences, and every failing batch retains both original and rechecked
output. These are top-level fragments, not native scanner/subentry-index or
original HTML checks. DSL/Python tests, targeted race/server checks, vet and build
pass. No APK/device/visual verification.
