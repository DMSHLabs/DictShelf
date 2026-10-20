# Plan for transferring upstream format and resource changes

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
  build and diff checks pass. Runtime verification with real large ZIM files is
  still pending; keep the transfer at P3 until that check is confirmed.

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

Port the `c8b92dda` joined `ReaderAt` source and part discovery. Trace the full
path from `.zimaa` selection through companion discovery, intake staging,
reconciliation, opening, and cleanup. Preserve gaps/case behavior and put a
clear cap on part count and total import size. Update the Android intake
extension filter to accept split parts while retaining the in-process runtime.
Do not treat a later part as a complete archive by itself.

### 5. Reduce SLOB open and scan memory

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

Review MDX header encodings, StarDict resources and `.idx.gz` bounds, XDXF link
escaping, dictzip handling, redirect resolution, and general intake bounds as
separate candidates. Upstream wildcard changes remain excluded: this fork's
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
