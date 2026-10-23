# Working in this checkout

- Read `CLAUDE.md` for the existing project map and conventions; consult the relevant sections of `docs/SPEC.md` before implementation. Do not duplicate those documents here.
- `HANDOFF.md` is the current state of this fork — branch, what remains, architecture facts, what is not verified. It is short on purpose (~400 lines): read it whole before planning. Everything else is read only when the task needs it: `docs/handoff-archive.md` is the per-session history (grep it, never whole — its index lists every section by date, and the commit messages carry the same narratives), and `docs/WINDOWS-VERIFY.md` holds this machine's build, test and emulator recipes plus the tests that fail on clean HEAD. When a session ends, update `HANDOFF.md` in place — refresh the branch/commit state, move finished items out of "what remains", add facts worth carrying over — but never past that cap: a session's narrative belongs in its commit message (or the archive), and anything derivable from the code with a grep belongs in neither.
- For Android fork identity, server startup, builds, and current verification, start with `docs/ANDROID-FORK.md`. For the separate UI work and its device checks, use `docs/ANDROID-UI-HANDOFF.md`.
- For translation/localization tasks or bugs (including language switching, untranslated UI, or locale-related crashes), read [translation.md](translation.md) and [translation_todo.md](translation_todo.md) before changing code. Keep reusable translation rules in the former and current remaining work/verification in the latter.
- Check branch/status before editing; preserve uncommitted work and user changes. Branch names in handoff snapshots are historical context, not instructions to switch branches. Work directly in this checkout unless the user asks otherwise; do not apply old stashes or patches without inspecting them.
- The user builds from Windows cmd with `build-android.cmd`. Do not commit, publish, or install APKs unless requested. Do not read or print private signing settings/passwords.
- Current user-approved labels (`DictShelf` for this Android and Windows product, `Background color`, etc.) take precedence over older naming notes in general documentation.

## Git branch policy

- `dev` is the development integration branch. Create new feature, fix, and experimental branches from `dev`, not from `master` or an unrelated task branch, unless the user explicitly requests another base.
- `master` is reserved exclusively for synchronization with the upstream project from which this repository was forked. Do not develop fork-specific features or fixes on `master` or merge task branches into it.
- Before creating a branch, verify the base and working-tree status. Do not silently switch, reset, rebase, stash, or discard existing work to enforce this policy. Existing task branches are not retroactively rebased. If `dev` is missing or its intended state is unclear, clarify rather than falling back to `master`.
- When a task is ready, its integration target is `dev`; committing, merging, publishing, and upstream synchronization still require the user request specified above.
- **Upstream review markers:** after reviewing `master`, record the last upstream commit considered with an annotated tag named `upstream-reviewed-YYYY-MM-DD` on that exact upstream commit (not on a fork merge commit). To review the next update, compare the marker with `master`, for example `git log --oneline upstream-reviewed-YYYY-MM-DD..master`; classify changes against the dead-upstream registry before adopting them. The tag is a review cursor, not a claim that every upstream change was merged. Do not move an existing marker; create a newer dated marker.

## Dictzip v2

- `D:\Projects\Android\dictzip-win64` is the separate Windows dictzip project.
  Its `build-windows.local.bat` contains the configured GCC path; inspect only
  the `GCC_PATH` setting when locating the compiler.
- The dictzip utility must retain two explicit output formats: v1 is the
  existing format and remains the default unless the user selects otherwise;
  v2 is a distinct, intentionally incompatible format. Old readers are not
  required to recognize v2.
- V2 has a private `DZV2` preamble with 64-bit uncompressed length and a
  32-bit per-block compressed-length table; its compressed payload follows.
  Keep v1's block size exactly unchanged in v2. Do not enlarge blocks to fit
  an index or adapt v2 to gzip header limits.
- The wudict DSL reader must accept both v1 and v2. Keep v1 behavior intact and
  verify both formats with generated fixtures and the Windows GCC build.

## Dead upstream code

Upstream features this fork deliberately keeps but does not wire are dead code:
take their side verbatim on every upstream sync, never wire them, and record a
new decision here when one appears. The mirror convention is the fork's
`dmsh_` file prefix: a fork-owned parallel implementation of an upstream
surface is named `dmsh_<feature>` beside upstream's file (e.g. `dmsh_groups.go`
beside `groups.go`), so the two never merge and upstream's file is taken
verbatim. The registry:

- **The panel's pinned sorting** (upstream `a00be12`, D166): skipped. This
  fork's picker order is its own — the reader's arrangement, edited per group
  in Dictionary Groups with its own pin mode (`dmsh_groups.go`) — while
  upstream's model is pinned-first + A–Z and declares the saved array order no
  longer an order, which this fork's search order (D142) is built on. It lives
  inside shared files (`prefs.go`, `index.html`), so a sync resolves those to
  this fork's side for anything pin-related.
- **`internal/howto/`** — the built-in howto, kept unwired (see
  `internal/cli/cli.go`); revive by wiring `server.WithBuiltin`.
- **`ext/server/`** — upstream's share-link hosting asset; this fork's server
  does not serve it.
- **Upstream wildcard search** (`internal/wildcard/`, `internal/store/wildcard.go`,
  D167): kept unwired. This fork's Mask mode uses a paged headword list with
  Unicode case-insensitive matching and dictionary order; its index-range
  optimization lives separately in `internal/store/dmsh_mask_ranges.go`.
  Upstream's removal of Contains and its trigram index is not adopted.

Files kept verbatim stay verbatim: the marker is this section, not the files.

## License and copyright

- The licence is **GPL-3.0-or-later** everywhere, and the `SPDX-License-Identifier: GPL-3.0-or-later` line belongs in every file that carries a header at all. Never edit the text of `LICENSE`.
- **A file this fork authored carries the fork's notice**: `Copyright (C) 2026 DmShAl (Shepeta Dmitry)`, on the line above the SPDX one. The comment shape follows the file's own kind — `//` for Go and Java, the `/** … */` block for the web JS and CSS, `#` after a shebang for Python, `rem` after `@echo off` for the build scripts, an HTML comment after the doctype. The canonical statement of it is the License section of `README.md`.
- **A file inherited from upstream keeps upstream's notice untouched.** Do not rewrite the upstream name and do not add a fork line to those files: the fork's changes to them are covered by the README, and every line added to a shared file is merge-conflict surface on the next upstream sync. `app.css` and the preset CSS are upstream's TEXT in files this fork created — they keep `glowinthedark`, or no header at all, never the fork's name.
- Which is which is not the file's path but its CONTENT: compare with upstream (`git diff --name-status master dev`) and then measure the text — a new path often holds a moved upstream file (does the same text exist under `master`, e.g. inside its `index.html`?). Files with no header by convention keep none — preset CSS, JSON, XML and translations, images and fonts, vendored third-party sources, `.md` — do not invent one for them.
