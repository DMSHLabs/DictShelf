# Xapian "glass" inside a ZIM: the spike, and the format

Kiwix ships its own full-text index inside every modern ZIM, in namespace `X`, and it is roughly 2.5-2.9x
smaller than the one this app builds for the same archive (see `MEASUREMENTS.md`). Reading it in place
would answer full text without a read-through build. This document is what that takes: first the spike
that found the way in, then the format as read off the bytes, written down before the code so a reader
can be written from it alone.

Moved out of `HANDOFF.md` on 2026-11-03. The code is `internal/format/zim/glass` (pure Go, no cgo) on
branch `xapian-glass`, and the check is `tools/xapian-glass-check <archive.zim> [query ...]`.

## Reading Kiwix's own full-text index (spike findings)

`X/fulltext/xapian` is stored **uncompressed**, one blob per cluster, starting with the Xapian magic
`0f 0d "Xapian Glass"`, so it can be read in place from a mapped archive - no copying. Sizes 518 MB,
2.52 GB, 3.18 GB for three of the four archives, against our own built article index of 1.29 GiB,
7.41 GiB, 9.21 GiB (**2.4-2.9x larger**); `X/title/xapian` and the title listing are uncompressed too.
libzim (cloned at `D:/Projects/Android/libzim`) documents the whole scheme: the query is unaccented
first - via **ICU**, in libzim, not Xapian - then parsed with `OP_AND`, the stemmer and stopwords taken
from the index metadata, `STEM_ALL`, `FLAG_CJK_NGRAM` (`src/search.cpp`); the index side wrote
`index_text_without_positions` with `STOP_ALL`, so **no positions exist and only stemmed forms do**.
libzim opens the index by seeking the archive and handing the fd to `Xapian::Database(fd)`.
The pure-Go reader (gozim, MIT) parses that real version header correctly - `DocCount` = 1 528 188,
exactly our entry count - but fails on every table access with `root block 571 not found`. Not the block
size (both sides use 8192); the likely cause is the single-file layout, where all six tables are opened
at `version_file.get_offset()` and block numbers are global to the area, while gozim assumes the
multi-file shape. If we take this path the code belongs in our own package beside the container, reading
a byte range rather than a `[]byte`, with Xapian built **locally as a test oracle** and never shipped,
because a reader of our own makes compatibility our burden and its failures silent.

## The glass format, as read off Xapian 1.4.32 (written down before the code)

Everything below is from `D:/Projects/Android/xapian-core` and was checked byte by byte against the
`fulltext/xapian` blob of the Russian Wiktionary, so a reader can be written from this section alone.

**Integers.** `pack_uint` is LEB128 (7 bits per byte, high bit continues). `pack_string` is a packed
length then the bytes. All values *inside blocks* are **big-endian** (`aligned_read4`, `unaligned_read2`).

**Version file, at the start of the blob** (`glass_version.cc`): magic `0f 0d "Xapian Glass"` (14 bytes),
then the format version (2 bytes, big-endian, `GLASS_FORMAT_VERSION`), then the database uuid (16 bytes),
then `pack_uint(revision)`, then **six** `RootInfo` records in table order (postlist, docdata, termlist,
position, spelling, synonym), then the statistics. A `RootInfo` is: `pack_uint(root)`, `pack_uint(level << 2
| sequential << 1 | root_is_fake)`, `pack_uint(num_entries)`, `pack_uint(blocksize >> 11)`,
`pack_uint(compress_min)`, `pack_string(free_list)`. The statistics are eight packed integers: doccount,
last_docid - doccount, doclen_lbound, wdf_ubound, doclen_ubound - wdf_ubound, oldest_changeset,
total_doclen, spelling_wordfreq_ubound. Each `RootInfo` carries the block size, so the version file needs
no separate field.

**Blocks** (`glass_table.cc`): `read_block(n)` reads at `offset + n * block_size`, where `offset` is the
descriptor position when the database was opened - for libzim that is the blob's offset inside the ZIM
(`src/tools.cpp:320` seeks and hands the fd to `Xapian::Database(fd)`), and for our own reader it is 0 at
the blob's first byte. **Block numbers are global to the file** and shared by all six tables: that is the
single-file layout, and it is exactly what a multi-file-shaped reader gets wrong (gozim's
"root block 571 not found"). Block 0 is unused (`blockNone`), so block 1 starts at `block_size`.
A block's header is 11 bytes: revision (4, BE), level (1 - 254 means freelist), MAX_FREE (2, BE),
TOTAL_FREE (2, BE), DIR_END (2, BE). The directory follows the header: 2-byte big-endian item offsets at
11, 13, 15, ... below `DIR_END`, ascending by key, each giving the item's offset in the block. Items are
packed downward from the end of the block.

**Items.** A *leaf* item (level 0) is `I2 K1 key [X2] tag`: `I2` is the item size in 2 bytes whose top
three bits are flags (`0x80` tag is compressed, `0x40` last component, `0x20` first component); a
component other than the first carries `X2`, its 2-byte counter. A *branch* item (level > 0) is
`block(4, BE) K1 key X2` where the leading four bytes are the child's block number and `size() = key_len +
1 + 2 + 4`. The compression flag means the tag is zlib-deflated above `compress_min`.

**Checked on the real file**: block 571 of the postlist table (at 571 * 8192 bytes into the blob) has
revision 1, level 2, directory from 11 to 323 (156 items) with item offsets 8185, 8172, 8159, ... spaced
by 13 bytes, and block 1 has level 0 with eight 13-byte leaf items - all consistent with the layout above.
So the addressing model is confirmed, not guessed.

**The reader works end to end** (`internal/format/zim/glass`, pure Go, no cgo), against the Russian
Wiktionary's 518 MB `X/fulltext/xapian` (format 1134, revision 1, block 8192, 1 528 188 documents,
postlist root 571, docdata root 59 228 with one record per document, the termlist/position/spelling/
synonym tables empty - libzim indexed without positions). What it reads: the version file, the
single-file block space, the B-tree descent, item keys and tags, **chunked** posting lists, the docdata
table and the query side (metadata, stopwords, stemming, AND). Nothing is copied: the spike
extracted the blob once, and the reader now takes either a `[]byte` (`glass.Open`) or, since the
byte-range API landed, the archive's own handle (`glass.OpenSource(io.ReaderAt, size)` -
`Dict.XapianIndex()` hands it a seekable window, so no blob is ever materialised).

`tools/xapian-glass-check <archive.zim> [query ...]` is the check: it prints the index's metadata, runs
each query, and opens every article a query returns to verify that a word of that article stems to each
term asked for - which is the index's own criterion, so index and reader agreeing is the whole result.
Numbers from it, on the Russian Wiktionary: `"кларнетист"` 11 documents / 32 occurrences, `"джаз"` 162 /
715, `"год"` 41 037 / 59 574, `"слов"` 1 449 345 / 5 081 953. The last two are the interesting ones:
their lists are split across **42** and **1 448** table entries, and every entry is read, equal to the
termfreq in the first chunk's header. Multi-word AND behaves: `"духовой инструмент"` gives бассетгорн,
бомбарда, альт-саксофон; `"музыкальный инструмент"` gives фортепиано, укулеле, ионика.

**It reads all four archives in place, in three languages** - `Dict.XapianIndex()` hands out a seekable
window onto the archive file (`io.NewSectionReader` over the container's own handle), and
`glass.OpenSource` reads blocks through it, so nothing is copied and the tool runs in well under a
second instead of copying 518 MB - 3.18 GB first:

| archive | index | documents | language | example |
| --- | --- | --- | --- | --- |
| wiktionary_ru 2026-09 | 518 MB | 1 528 188 | `rus` | `"духовой инструмент"` → 169 |
| wikipedia_ru 2026-10 | 2.52 GB | 2 199 375 | `rus` | `"духовой инструмент"` → 774 |
| wiktionary_en 2026-08 | 3.11 GB | 9 087 944 | `eng` | `"musical instrument"` → 7 197 (Porter2: *musical* → *music*) |
| wikipedia_de 2026-10 | 3.18 GB | 3 275 258 | `deu` | `"Jazzmusiker"` → 18 973 (*-iker* off, so the stem is `jazzmus`) |

Each row's query was checked the same way - every returned article opened and every term found in it.
The English row is the clearest sighting of the stemmer being the archive's own: no English index holds
"musical" as a term, and the query works anyway because both sides reduce it to "music".

**Terms are stemmed, and the query has to stem too** - an earlier note here said the opposite
("a query needs unaccenting and case folding but no snowball stemmer") and it was wrong, on evidence
that looked convincing and was a coincidence: the first term checked, `"кларнетист"`, stems to itself.
The index is built with Xapian's `STEM_ALL` and `STOP_ALL` (`src/writer/xapianWorker.cpp:49-58`), so it
holds stems and only stems: in this archive `"джазовый"` is **0** documents while `"джазов"`, what the
Russian stemmer makes of it, has 29, and `"джазов"` itself stems on to `"джаз"` with 162. The stemmer is
whichever the index's `language` metadata resolves to - libzim runs it through ICU to a two-letter code
and hands it to `Xapian::Stem` (`src/writer/xapianIndexer.cpp:44`, `src/tools.cpp`), which here is `rus`
-> `ru` -> Snowball Russian. `internal/format/zim/glass/stem.go` carries Snowball's own algorithms
compiled to Go (`github.com/blevesearch/snowballstem`, BSD-3-Clause, notices regenerated), keyed by the
codes and names Xapian accepts; a language Xapian stems but we do not (Armenian, Basque, Catalan,
Indonesian, Lithuanian, Nepali, and the legacy earlyenglish/german2/kraaij_pohlmann/lovins variants) is
reported as unstemmable - `ErrNoStemmer` - rather than searched with the wrong one, while a language
Xapian has no stemmer for (Japanese, Thai) was never stemmed and is searched plain.

Three more things only the bytes settled, each of which the code got wrong first:

- **A common term's list is several table entries, not more bytes under one key.** Xapian breaks it into
  ~2 KB chunks: the first under the term itself, each later one under the term, a zero byte and the
  chunk's first document id in the key's own encoding (`glass_postlist.h`, `make_key`). Reaching them
  needs a table cursor, not a lookup - `nextKey` keeps its descent path so it can climb out of a leaf -
  and the next chunk does **not** start at the previous chunk's last document plus one, because a term
  is not in every document: that gap is why a computed key finds nothing.
- **The document-id key encoding is glass's, not chert's.** Both are in `common/pack.h`; glass's is a
  unary length marker (leading set bits) and, below `0x8000`, **two plain big-endian bytes with no
  marker at all**. Ours was the chert reading, which agrees with glass above `0x8000` and differs below
  it - so every id an early check happened to use was above it and looked right, while ids below it
  resolved to a *different* document (`49 a8` is 18 856, not 2472) or to none. The symptom was a search
  whose hit count was short of the term's own list: 1 433 839 against 1 449 345, the difference being 16
  chunks re-read because the walk's document ids were decoded from the wrong form. Pinned against the
  archive's real bytes in `item_test.go` (`04 24` for 1060, `80 85 91` for 34 193).
- **An item's size field covers the key AND the tag** (Xapian's `I K key X tag` with I the length of the
  key-tag pair), so the tag ends three bytes past the size. Reading the size as the whole item cut every
  tag three bytes short, which showed up as paths truncated inside a character.
