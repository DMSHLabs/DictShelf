# Plan for transferring upstream format and resource changes

## Проверка

Перед тем как считать перенос проверенным, нужно завершить ручную проверку уже
перенесённых изменений на реальных словарях и в целевых вариантах приложения:

- **Большой ZIM (пункт 3):** открыть реальный крупный ZIM, выполнить поиск и
  последовательный просмотр, затем загрузить большую картинку и проверить, что
  ресурсы из несжатого кластера читаются напрямую, а сжатый кластер остаётся в
  установленном ограничении распаковки.
- **Разделённый ZIM (пункт 4):** открыть реальный архив из нескольких частей,
  проверить поиск и ресурсы, в том числе когда граница частей попадает внутрь
  данных; убедиться, что архив с отсутствующей частью и одиночная часть после
  первой отвергаются с понятной ошибкой.
- **Windows и Android без сервера:** повторить загрузку картинки и аудио из
  исходного словаря через существующие in-process/WebView и Go/JNI пути; для
  большого ресурса проверить Range/перемотку и убедиться, что адаптер не читает
  весь ответ в память.
- **Оба режима хранения ресурсов:** проверить словарь с подготовленным
  `media.db` и словарь, ресурсы которого читаются напрямую из исходного файла.
- После ручной проверки повторно выполнить Go-тесты и сборки Windows/Android;
  при сбое исправить его до завершения переноса.

P3 и P4 пока ожидают ручной проверки; P5 начат по решению пользователя до её
завершения. Это не мешает продолжать независимые пункты, но перенос нельзя
считать полностью проверенным, а upstream review marker — обновлять, пока эти
проверки не пройдены. Ограничения и ожидаемое поведение описаны в разделе
«Обязательные инварианты» ниже.

## Scope

Transfer selected behavior from upstream commits `f1c5fb27` and `c8b92dda` into
the development line, while preserving this fork's in-process Android and
Windows media delivery. These commits are sequential; the review cursor before
them is `upstream-reviewed-2026-10-18`.

This is a selective port, not a merge or cherry-pick of either whole commit.
Upstream has substantial divergence in shared files such as
`internal/server/server.go`, `internal/store/media.go`, and Android intake.
The intended integration branch is `dev`, as required by `AGENTS.md`. At the
start of planning `dev` and `dev2` both pointed to `2183ed90`; keep working in
the current checkout unless the user directs otherwise. Do not switch branches
or discard user changes silently.

## Baseline recorded — 2026-10-20

- Development base: `dev` / `dev2`, `2183ed90f9944b69bd2bea55c11f95bbd17c202b`
  (`rename dictionary`). The common ancestor with both target commits is
  `bf4e83cdc5e1204dcc8d8c2b63e58d50be00979a`.
- Upstream source: `f1c5fb275a560df645cd41eb9c5ba5e53ad7073a`, followed by
  `c8b92ddafa2075e6fd5dfd8f12ddd228b9702c90`. Both are after
  `upstream-reviewed-2026-10-18`; upstream `master` currently points later at
  `bcc116e3`.
- No uncommitted source changes were present before writing this plan. The
  current uncommitted changes are this plan and its `HANDOFF.md` pointer.
- `dev` has fork-specific resource files absent from upstream's tree:
  `internal/server/desktop_resource.go`,
  `internal/server/desktop_resource_stream.go`,
  `internal/server/resource_source.go`, Android `GoAppResource.java`, and
  `android/nativebridge/runtime_android.go`. Preserve these while porting.
- Existing coverage includes `internal/store/media_stream_test.go`
  (`TestLargePackedResourceStreams`) and resource/range serving paths in
  `internal/server/server_test.go`. This baseline review did not run tests.
- The material code conflict confirmed for the next stages is upstream's
  `internal/store/media.go`: `f1c5fb27` removes the chunked SQLite `blobReader`
  and reads the whole blob. Retain the fork implementation and selectively
  adopt only compatible seekability and size-guard behavior.

## Progress

- **P1 — baseline:** complete. The target branches, source hashes, fork-only
  resource adapters, and existing resource streaming implementation are
  recorded above.
- **P2 — seekable readers:** implemented on `dev`/`dev2`. Added
  `resource.Bytes` and used it for in-memory MDict/MDD, BGL, SLOB, ZIM, locator,
  and small `media.db` resources. The large SQLite `blobReader` and oversized
  media skip behavior remain intact. `go build ./...` and `git diff --check`
  pass; tests were not run.
- **P3 — ZIM on-demand indexing and direct resources:** implementation complete;
  runtime verification with real large ZIM files is still pending. The full Go
  test suite and Go build pass on the current worktree.

- **P4 — split-ZIM support:** implementation complete; runtime verification with
  a real multi-part ZIM is pending. The joined `ReaderAt`, `.zimaa` discovery,
  intake staging, required URL-part selection, source revision, cleanup listing,
  64-part cap, and 64 GiB per-dictionary cap are implemented. Gaps are rejected,
  uppercase suffix runs are preserved, and a later part alone is never offered
  as a dictionary. Synthetic split-boundary and intake tests, the full Go test
  suite, Go build, Android FOSS Java compilation, and `git diff --check` pass.

- **P5 — SLOB lazy reference access:** implementation complete; package tests
  pass. Open now validates the ref-list bounds without decoding every key;
  browse, resource listing, and ingest read references from disk in bounded
  windows. Direct lookup lazily caches the decoded refs because its exact and
  folded indexes need the complete key set. The full Go test suite, Go build,
  and `git diff --check` pass. Large real-SLOB runtime verification remains
  pending.

## Required invariants

- Android stays in-process: Go/JNI serves `/res/` and `/files/`, with no TCP
  listener. Range requests keep the existing Android WebView workaround.
- Windows WebView keeps the existing in-process resource handler and spill to
  a seekable temporary file for responses that exceed its memory threshold.
- A resource source that supports seeking remains an `io.ReadSeeker` through
  the common `/res/` handler, so `http.ServeContent` can answer Range requests.
- Large resources in `media.db` keep the chunked `blobReader`; do not replace
  it with `io.ReadAll` or a `[]byte` response. Resources too large for SQLite
  remain available from their original source.
- IDs and stored group membership remain independent of a dictionary's
  display name.
- Existing source override lookup, media playback, and resource lifecycle
  behavior remain intact.

## Transfer sequence

### 1. Establish the baseline

Record the selected development base and upstream source hashes. Inspect the
current diff and tests for `internal/server/desktop_resource.go`,
`internal/server/desktop_resource_stream.go`, Android `GoAppResource`, the JNI
media bridge, `internal/server/resource_source.go`, and
`internal/store/media.go`. Keep the transfer isolated from unrelated working
changes.

### 2. Add seekable resource support without losing SQLite streaming

**Status: implemented.**

Adopt the small `resource.Bytes` helper from `f1c5fb27` where it preserves a
`bytes.Reader`'s `Seek` method. Keep the fork's large-blob SQLite reader and
chunked ingestion. Retain the SQLite size guard that skips un-packable blobs,
so the source dictionary can continue serving those assets. Review every
resource backend that currently wraps a reader with `io.NopCloser`; change only
the paths where seek support is correct and useful.

### 3. Port ZIM on-demand indexing and direct resources

**Status: implementation complete; runtime verification pending.** The full
path-pointer list remains on disk, sequential scans use a bounded pointer page,
and stored resources in uncompressed clusters use bounded direct reads. A
compressed cluster is still decompressed as a whole under the existing bound.
The fork's Browse/A-Z and headword-only reader paths are retained. `go build
./...` and `git diff --check` pass; tests were not run.

Port the `f1c5fb27` ZIM pointer paging and stored-cluster direct reads into the
fork's current ZIM code. Preserve fork-specific lookup, indexing, and resource
resolution behavior. For uncompressed clusters, return a bounded seekable
reader over the source file. For compressed clusters, retain explicit
decompression bounds and document that the current implementation still
materializes the bounded cluster. Check that concurrent range reads and
registry shutdown do not close a source while an active resource is being
served.

### 4. Add split-ZIM support

**Status: implementation complete; real-file runtime verification pending.**
The fork joins at most 64 consecutive parts through `io.ReaderAt`, retains
seekable direct resource reads, and records all part sizes and modification
times in the source revision used by reconciliation. Intake recognizes
`.zimaa`, groups later parts as companions, refuses gaps and oversized sets,
stages and disposes every part with the main file, and makes discovered URL
parts required as one selection. A later part cannot become a standalone
candidate. Android's loose-file suffix filter accepts split-part names while
the Go/JNI intake path remains in-process. Synthetic tests split a valid ZIM
at arbitrary byte offsets, including inside the header and entries; they verify
lookup and resource reading across the joined source. Real multi-part ZIM
runtime verification remains an open verification gate.

Port the `c8b92dda` joined `ReaderAt` source and part discovery. Trace the full
path from `.zimaa` selection through companion discovery, intake staging,
reconciliation, opening, and cleanup. Preserve gaps/case behavior and put a
clear cap on part count and total import size. Update the Android intake
extension filter to accept split parts while retaining the in-process runtime.
Do not treat a later part as a complete archive by itself.

### 5. Reduce SLOB open and scan memory

**Status: implementation complete; Go tests and build pass, real-file
verification pending.** `Open` no longer loads the complete ref table. Browse pages, resource
listing, and ingest scanning read bounded windows from disk; direct lookup still
caches all decoded refs lazily when building its indexes.

Port the lazy reference-list reads from `f1c5fb27` and `c8b92dda` into the
current SLOB container. Preserve direct lookup semantics, resource naming,
reader output, and safe concurrent access. Keep large media serving seekable
where possible; in all cases, preserve the independent `media.db` streaming
path. Confirm that malformed offsets and oversized bins fail with bounded
memory use.

### 6. Introduce configurable limits and intake space checks

Review `c8b92dda`'s `internal/limits`, config parsing, decompression, upload,
download, archive extraction, and free-space checks as one feature. Preserve
generous defaults suitable for multi-gigabyte dictionaries and make sure
embedded Android startup applies the same limits as CLI/server startup. Confirm
that the limit is enforced during streaming, not only from declared lengths.
Do not lower current accepted import sizes accidentally.

### 7. Review other independent fixes individually

**Partial: low-risk fixes implemented.** MDict header encoding recognition now
accepts GB18030 and UTF-16LE spellings and trims surrounding whitespace. XDXF
`href` values are escaped as HTML attributes. Concurrent dictzip reads no
longer add a chunk to the FIFO cache more than once. Focused tests, the full Go
test suite, `go build ./...`, and `git diff --check` pass. The race detector
could not run because this Windows environment has no GCC/cgo toolchain.

Still separate candidates: StarDict resource-list (`r`) rendering (requires
checking generated media markup and Reader-version invalidation), configurable
`.idx.gz` bounds (coordinate with P6), and ZIM title/redirect search behavior.
Upstream wildcard changes remain excluded: this fork's
mask behavior is deliberately implemented in `internal/store/dmsh_mask_ranges.go`
and the dead-upstream rules in `AGENTS.md` say not to wire upstream wildcard
search. The lemma manifest/UI change is also independent of the resource work
and can be considered in a later pass.

## Verification gates

Run focused format and store tests after each relevant port, then build the
Windows and Android in-process variants. Verify in both apps that an image and
audio file load from the original dictionary, a large media file starts and
seeks using byte ranges, and a large image does not force the adapter to
materialize the complete response. Exercise both a prepared dictionary with
`media.db` and a source-only dictionary. Verify ordinary single-file ZIM and
multi-part ZIM, including a missing part; verify large SLOB open/resource
behavior. Recheck that dictionary groups still resolve by stable ID after a
display-name edit and reset.

Keep each stage reviewable and stop before the next stage if a verification
gate fails. Do not update the upstream review marker until all changes through
the intended upstream commit range have been reviewed; the marker records
review, not successful adoption.
