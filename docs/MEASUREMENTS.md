# Measurements on this machine

What a build costs, what an index occupies, and how long things take - measured on
this machine's own library and hardware, so the numbers are anchored rather than
guessed. Moved out of `HANDOFF.md` on 2026-11-03: this is the record of the
numbers, while `HANDOFF.md` carries the rules that came out of them.

Read a table here as evidence for a decision, not as a promise: the four archives
and the 38 dz/mdx dictionaries in the library at the time are the sample.

## What a read-through ZIM costs, measured on the user's four archives

Measured 2026-10-31 by walking only the ZIM directory (header, MIME list, cluster pointers, dirents -
never a cluster payload) and, for the prepared databases, `sum(length(entry.m))` plus `dbstat` per
table. The directory walk agrees with the prepared database's entry count to a handful of rows on all
four (the readers drop articles whose body is empty, which is the difference), so the parse is checked
against the library, not assumed. All figures bytes.

| archive | source | text clusters | articles | redirects | media entries | text db | stored markup | article FTS |
|---|---|---|---|---|---|---|---|---|
| wiktionary_ru nopic | 2.15 G | 1.079 G | 1 528 188 | 999 439 | 182 | 9.64 G | 6.90 G | 1.29 G |
| wiktionary_en nopic | 9.16 G | 4.363 G | 9 087 944 | 42 006 | 56 | 25.26 G | 15.64 G | 4.56 G |
| wikipedia_ru maxi | 37.1 G | 13.775 G | 2 199 375 | 2 825 093 | 2.88 M | 33.44 G | 22.78 G | 7.41 G |
| wikipedia_de maxi | 48.6 G | 13.624 G | 3 275 258 | 1 878 526 | 4.84 M | 35.04 G | 22.29 G | 9.21 G |

Three things the numbers settle. **Bodies dominate, the FTS does not**: the article index is 19, 29,
33 and 41 % of the stored markup, so the body copy is 69-97 % of everything a read-through build adds
and the searchable text is the smaller part of the bill. **The container is mostly not text**: Kiwix's
own Xapian index sits in namespace X as three blobs and is 8-48 % of the file (0.91 G, 4.13 G, 3.48 G,
3.76 G), and in the maxi Wikipedias the images are another half - so estimating from the source file
size is wrong twice over. **Expansion is a family constant, not a number**: compressed text clusters
become 1.65x stored markup on both Wikipedias (1.653 and 1.636 - the two agree to 1 %) but 3.58x on
the English and 6.39x on the Russian Wiktionary, so one coefficient cannot serve both families. The
free classifier is compressed bytes per article: 480 B and 706 B for the wiktionaries, 4.2 KB and
6.3 KB for the Wikipedias, which matches the observed split.

## What each index costs on the non-ZIM library (three passes)

38 dz/mdx dictionaries, measured with `text.db` size, `sum(length(entry.m))` and a full `dbstat`
breakdown after each step; the entry count never moved, and `entry` was byte-identical before and
after full text - the control that the bodies were not re-encoded.

| step | text.db | delta | what it was |
|---|---|---|---|
| base (index only) | 2 988 MiB | - | 4 010 535 entries, bodies 2 126 MiB deflate (69 % of the db) |
| + contains | 3 058 MiB | +2.3 MiB | `entry_fts` + `entry_trigram`: 88.6 MiB = 2.9 % = **23 bytes per entry** |
| + full text | 4 565 MiB | +1 507 MiB | `article_fts`: 1 353 MiB, the rest free pages (VACUUM would return ~150 MiB) |

So contains is priced per headword and full text per body byte: contains is flat at 19.8-42.4 B per
entry (23.2 average) whatever the dictionary weighs, while full text added 16.5-117.6 % of the file
before it (median 45 %), i.e. 0.49x the database that existed. The spread is deflate's: the index is
built over `StripHTML` of the *uncompressed* body while the file holds it compressed, so a small
dictionary whose markup compressed badly pays more for the index than the bodies take. ZIMs are not in
this series - their full text is the read-through form above.

## What removing full text frees, measured without removing anything (2026-11-01)

SQLite's own page accounting (`dbstat`, read-only) gives the sizes inside the four prepared ZIM
databases, and the article index is exactly the space a removal frees - `ClearSearchIndexesContext`
drops `article_fts` and vacuums:

| archive | text.db | our article_fts | headword fts | trigram | bodies (entry + its indexes) |
| --- | --- | --- | --- | --- | --- |
| wiktionary_ru 2026-09 | 9 645 MB | **1 291 MB** | 20 MB | 22 MB | 8 018 MB |
| wikipedia_ru 2026-10 | 33 439 MB | **7 414 MB** | 33 MB | 61 MB | 24 830 MB |
| wiktionary_en 2026-08 | 25 265 MB | **4 556 MB** | 101 MB | 128 MB | 19 946 MB |
| wikipedia_de 2026-10 | 35 044 MB | **9 213 MB** | 44 MB | 82 MB | 25 059 MB |

The first three match the figures the earlier measurement passes recorded (1.29, 7.41 and 9.21 - GB,
not GiB as that note said), so the estimate the window shows for a ZIM is anchored to measurements that
a second, independent method reproduces. Two things the read-through cost table had wrong, both worth
knowing before trusting the button: the **bodies are NOT freed** by removing full text - 8-25 GB of
stored articles stay, and they go only when the request asks for a rebuild (`index: true`, which
re-ingests the archive in its headwords-only form, `internal/server/clear_database.go` takes the
`ClearSearchIndexes` branch for `fullText` alone and the `reconcile` branch only for `index`) - and the
archive's own Kiwix index (0.52-3.18 GB, already inside the ZIM) is 2.5-2.9x smaller than the one we
build, which is what makes it worth reading instead of building.

## Size estimates: measured, predicted, not yet shown

Stages 1-4 are in the tree. The ingest records `body_bytes`, `stored_body_bytes`, `headword_bytes`
and `stats_version` in the meta of every dictionary; a ZIM reader adds what its directory held
(`zim_articles`, `zim_redirects`, `zim_media`, `zim_other`, `zim_clusters`, `zim_text_bytes`,
`zim_mixed_bytes`, `zim_other_bytes`, from `internal/format/zim/dmsh_stats.go`, also usable alone as
`zim.MeasureDirectory`). Every one of those keys is in `volatileMeta`, so recording them moved no
fingerprint and the golden tests still pass; `TestStatsMetaIsVolatile` keeps it that way.

`internal/estimate` holds the coefficients (`Version = 1`) and predicts a band: contains 18-45 bytes
per entry; full text a share of the current file for dz/mdx (0.16-1.18, median 0.45), a share of the
stored bodies for a ZIM that has them (0.18-0.42), or a multiple of the archive's compressed article
text for a read-through build (2.4-2.6 encyclopedia, 5.7-9.0 dictionary, split at 1500 compressed
bytes per article). `estimate_test.go` carries the measured corpus as a table - 38 dz/mdx rows with
their real growth, four ZIM rows with their real builds - and asserts the measurement falls inside the
predicted band. It earned its keep on the first run: three constants were set from rounded numbers and
were wrong (contains 20 against 19.8, full-text 0.17 against 16.5 %, wiktionary 5.8 against 5.79).

`stats.cache` beside `text.db` (shape: `version`, `index_uuid`, source fingerprint, numbers) caches it.
It is written in place rather than through a temporary - a cache whose reader validates its content
needs no atomicity, and a stray temp file is something every listing of that folder would have to know
about - and it is accepted only when `index_uuid` matches `dict_uuid`, the version is current and the
source fingerprint still matches, while a source that is GONE is accepted (folders outlive sources).
`Registry.warmStatsStart` fills it once per process from `Warm()`: two workers, one dictionary
committed before the next, no prompt and no panel row, so a mid-flight close leaves the finished ones
measured and the rest for the next start.

The numbers now ride on each dictionary list row (`dictInfo.Estimate`, `internal/server/dmsh_estimate.go`),
attached AFTER the row cache - the cache key knows nothing about the sidecar, and a cached row would
otherwise keep saying "unknown" until something unrelated invalidated it. That means the page needs no
new endpoint, no bridge method and no `openapi.yaml` entry: it reads what `/api/dicts` already sends.
Two facts the first test run settled: a dz/mdx row gets its band from the file's own size and needs no
measurement at all (`Basis: database`), so an old library is estimated out of the box; a ZIM row gets
none until its directory has been measured (`Stats.ZIMForm`, which the sidecar now records beside the
numbers), and that absence is exactly what the page shows as "Calculating".

The window is in as `#fullTextPicker` (index.html), opened from a `full-text search` row in the
Settings drawer - the owner's decision is that full text is its own door, not a chip beside three
cheap ones. It lists every dictionary alphabetically with its band (or "already built", or
"calculating" when nothing is measured yet), a checkbox only where there is something to build, a
sticky summary with the selection's total and what the volume will have left, a warning when the high
end does not fit, and the ZIM part of the total called out separately because that one is a rebuild
that copies article texts. The build goes through the existing work API (`feature: "fts"`,
`action: "create"`) so progress, Stop and the task panel are the ones already there. Free space rides
on the list response's `end` message (`freeBytes`, `libraryVolume`, `fsx.FreeSpace` on the library
volume) rather than on a route of its own.

The build direction is gone from every other window: the dictionary chip keeps only "off" (removal is
this dictionary's own state and cheap - the bodies are not decompressed again), and both the bulk
button and the single "add full text" action open this window. CLI flags are untouched: a script is
not a window. The ZIM skip and its `dictUI.bulkZimSkipped` string are now dead on that path.

Not verified: the dialog has never been rendered - see the browser-automation recipe in
`WINDOWS-VERIFY.md` - so the JS is only checked by `tools/i18n-test.cjs` (syntax + keys) and the layout
by eye in the source, and that the CSS block scoped inside the dialog is the right place for those
rules rather than app.css.

## The Wildcards cache benchmark (opt-in, on the user's dictionaries)

Transferred indexer work was verified in its source worktree: full Windows cgo/FTS5 tests, focused race tests, Android Java/arm64 bridge builds, and Android/macOS pure-Go cross-builds passed. The opt-in Wildcards cache benchmark used Longman MDX + Oxford 10: upper `*a*` took 7.82 s with FTS/trigram indexes and 2.98 s without; lower `*e*` was 2.94/2.78 s; Combined base was 2.70/1.10 s. This is one workload; these checks have not been rerun on the current DictShelf2 checkout. A later three-dictionary Wildcards run (Longman MDX, Oxford 10 MDX, Wiktionary ZIM; `*d*g*`/`*home*`) took 28.68 s for the upper stage with result-cache FTS/trigram and 23.99 s without; lower group filtering took 27/114 ms. Batching 128 index rows per SQL statement cut index insert calls from 675,492 to 6,309 but the upper stage still took 28.48 s on this small result set. On the 9.13M-word mask `*`, batching cut both-index creation from 4m23s to 2m52s and index insert calls from 27.39M to 219,168, so it is now retained. The real UI now defaults both Wildcards cache indexes off; explicit on choices persist as `1`. The opt-in benchmark now selects FTS and trigram separately: on the same three dictionaries, FTS only took 27.11 s upper / 115 ms lower, trigram only 28.61 s upper / 27 ms lower, and neither 24.91 s upper / 121 ms lower. The corresponding Combined-base times were 1.83, 2.13 and 1.19 s. The app defaults both Wildcards cache indexes off. The full-match mask `*` on Wiktionary ZIM returned 9,129,950 words: upper-cache creation took 4m23s with both indexes and 36.7s without (27,389,850 per-row index INSERTs; peak observed temporary cache was 1.55 GiB). This run stopped after upper-cache creation; lower filtering and Combined mode on all 9.1M rows were not measured. The performance harness and BAT now support `UPPER_ONLY=1` for this case. With batching, trigram-only creation took 2m21.5s, about 31s less than both indexes; parallel FTS construction within the one SQLite database cannot bypass its single writer. On the same ZIM, the lower `**home**` filter took 230ms with indexes (2,317 matches from 2,474 trigram candidates) versus 5.2s without; FTS prefix `h` took 4.8s versus 9.6s without (243,637 matches).

## Measuring the ingest write path on this machine

`internal/store/dmsh_ingest_bench_test.go` is a gated measurement (it skips
unless `WUDICT_BENCH_ARTICLES` is set and asserts nothing about time): it builds
one synthetic read-through database - bodies dropped, article text indexed, the
form a ZIM archive gets - and prints the phase timings, so a change to the write
path can be A/B'd on one machine without touching a real library:

    WUDICT_BENCH_ARTICLES=300000 go test -tags sqlite_fts5,sqlite_dbstat         -run TestIngestSplitThroughput -v -count=5 ./internal/store/

300000 articles of ~1.5 kB each is ~450 MB of text and about 10.5 s per run
here; five runs give a median with a spread under 1%. To compare against a
different revision, run the same command in a `git worktree` of that revision
(the test file is untracked, so copy it in).

For the real thing - one archive, alone, no other builds running - the CLI is
the driver and its own line is the number:

    DictShelf ingest -fulltext -contains -o <dir>/text.db <archive.zim>

Measured 2026-11-03 on the NVMe machine, `wiktionary_ru_all_nopic_2026-09.zim`
(2.15 GB, 1.53 M entries), four runs of the same command back to back:

    495.5 s   before the attached-temp pragmas and the ingest page cache
    431.3 s   the same binary again, an hour later
    430.7 s   with the two changes

Two runs of the SAME binary differed by 15 %, so a single pair of real builds
proves nothing: the bench above is the instrument (its spread is under 1 %), and
the real archive is for sanity checks - output bytes and recorded sizes must
match. What the real runs do settle is the comparison that matters for the
feeling of slowness: the same archive took **763 s** in the log of a
three-dictionary parallel batch, against 430-495 s alone.
