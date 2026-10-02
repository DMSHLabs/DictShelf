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
equivalent anchored QRegularExpression. Scanner, index builder, language metadata,
abbreviations and `DslDictionary::nodeToHtml` are NOT included.

Input is a JSON array of `{id, key, headings, body}` cases. Both adapters produce
heading keys and a DSL tree. Go also exports its HTML for inspection, but there is
currently no original GoldenDict HTML output to compare it against.
Heading order/duplicate keys, attribute order and adjacent text-node boundaries
are ignored. Text whitespace, tag names, nesting and attribute values are not.
The Go adapter applies the existing legacy IPA conversion to exported text,
because Go does this at HTML rendering time while GoldenDict does it in ArticleDom.
It exports retained DSL media payloads instead of the prepared media HTML. Escaped
literal spaces are kept distinct until the IPA conversion is done. Closed empty
tags are now removed in the Go tree as in GoldenDict. The report does not claim
visual or full-index parity; complex media payloads still need dedicated cases.

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
