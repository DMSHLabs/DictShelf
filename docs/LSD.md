# Lingvo LSD — reader contract

This document records the application-facing contract for a native `.lsd`
reader. The reader is being developed in `internal/format/lsd`; the existing
`internal/format/dsl` remains the text DSL backend. The two formats can share
the ingest pipeline and its `BodyDSL` normalization, but the binary container
reader must not be added as a mode inside the text DSL scanner.

## wuDict2 operations to support

| Application operation | Existing contract | Data the LSD backend must provide |
| --- | --- | --- |
| Format discovery and probing | `dict.RegisterFormat`, optional `dict.RegisterProber`; `dict.Probe` falls back to `Open` | Recognize `.lsd`; validate the signature, supported version and bounded header fields; report `Meta` without decoding every article when feasible |
| Initial index preparation / rebuild | `dict.RegisterReader`; `dict.OpenReader`; sequential `dict.Reader` (`Meta`, `Next`, `Close`) consumed by `store.Reconcile` | Stream entries in source order. `Next` returns headwords/aliases, DSL article markup as `dict.BodyDSL`, and `io.EOF` at the end. Keep the file and decoding state open only for the reader lifetime |
| Metadata shown in the app and CLI | `dict.Meta` | Name, declared index/content languages, entry count, and useful remaining header fields; format id is `lsd` and path is the source `.lsd` |
| Optional media packing | `dict.ResourceLister` plus `Dictionary.Resource`; `store.MediaNames` and `store.IngestMedia` | Enumerate embedded overlay/resource names and open their bytes by name. Preserve lazy reads; do not extract files beside the source |
| Runtime media after preparation | `resource.Provider` (`Sources` and, if needed, `Open`); `server.upgraded` tries packed media, then the source provider | Reopen the LSD resource area from the recorded source path without scanning article bodies. Use the same resource-name matching rules as other formats |
| About/annotation | `dict.RegisterAbout` and the server About surface | If an LSD embeds an annotation, expose it through the About provider, not by changing the article reader contract |
| Direct search and Browse before preparation | `dict.Dictionary` | The current DSL backend advertises no direct search or Browse and relies on the prepared index. LSD can follow that same minimum behavior unless a cheap native operation is demonstrated |

`dict.Dictionary` also requires `Meta`, `Caps`, `Exact`, `Prefix`, `Keywords`,
`Resource`, and `Close`. A minimal direct LSD backend may return unsupported for
`Exact`/`Prefix` and no keyword list, like DSL; the user-facing search path
becomes available after the mandatory prepared headword index is built. Do not
claim `Browser`, `Searcher`, `SelfIndexed`, or `PreviewBytes` capabilities
without a measured implementation and contract.

## Physical reads identified from the reference implementations

The LSD container keeps its header, encoded dictionary alphabet/decoder data,
fixed-size index pages, article payloads, optional annotation, optional icon,
and an optional resource overlay in one file. The current Go reader decodes
user-dictionary articles, embedded annotations, and lazy overlay resources for
the 12/X3/X5 fixtures. `lsd2dsl` writes overlay entries
to a sibling `.dsl.files.zip`, writes the annotation as `.ann`, and writes the
icon as `.bmp`; those are outputs of its converter, not separate inputs that
the native wuDict2 reader should require.

The Go implementation keeps these reads behind a small set of private
operations: open/validate header; read or initialize version-specific decoder
tables; traverse index pages to produce headword groups and article offsets;
decode one article body; enumerate and decode one overlay resource on demand;
read annotation/icon metadata when requested. It should not materialize the
entire LSD file or all article bodies in memory. The version-specific bitstream
and Huffman details remain internal to `internal/format/lsd`.

## Reference projects and validation limits

The local references are `D:\Projects\Android\lsdreader` (Python, reports
Lingvo 11/12/X3/X5/X6) and `D:\Projects\Android\lsd2dsl` (C++ converter,
including overlay extraction). Use them to compare observable output and
container behavior, not as source files to port line by line. The current
`lsdreader` implementation reads the whole file and materializes decoded
articles; that is not an appropriate memory model for wuDict2's ingest reader.

The small `lsd2dsl/tests/simple_testdict1` fixture set is copied under this
package at `internal/format/lsd/testdata/simple_testdict1`; its upstream MIT
license and provenance note stay alongside it. Cases cover header/article/
annotation decoding (`test.lsd`), headings for versions 12/X3/X5, overlay
resource extraction for 12/X3/X5, extended and unsorted headings, and variant
collapse. Heading extension pairs are converted back to DSL title syntax, and
the text DSL title parser supplies searchable keys. Lingvo 11 and X6,
system/abbreviation decoders, real-world large files, malformed-file handling,
title display rendering and wuDict2-specific resource-name behavior still
need coverage before support is marked complete.
