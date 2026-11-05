# DictShelf — the code, and the rules of this tree

**This file is the fork's, not upstream's.** Upstream has its own `CLAUDE.md`
describing upstream's code — module `github.com/wuweidict/wudict`, pure-Go and
`purego` builds, `wudict` everywhere — and **none of that is true here**. On an
upstream sync, resolve `CLAUDE.md` to THIS side; it is the only file in the tree
that must not be taken from upstream.

The product is **DictShelf**, the command is **`dictshelf`**, and the module is
`github.com/DMSHLabs/DictShelf` (D27). The fork's **working notes are not in this
repository**: the state of the fork, the plans, the session records, the document
gate and the rules that govern them live beside it in `../DictShelf_Priv` —
start at `../DictShelf_Priv/AGENTS.md`. The **tool names of the machine you are
on** (the cgo compiler, the real name of `make`, Python, the SDKs) are in
`../DictShelf_Priv/build-tools.md`: read it before the first build. The rules
that must not be broken by not knowing them (branches, markers, licence, dead
upstream, monolith) are in `AGENTS.md` next to this file.

**The name `wudict` survives in exactly three kinds of place**, and nowhere else:
the **wudict markdown** format (its name, its `wudict:` front-matter gate, its
`.wudict.md` / `.wudict.files` naming, read by `internal/format/wmd` and written
by `dictshelf dump -format md`); **legacy identifiers that are on disk or in a
user's settings** — the config file `wudict.toml` (`config.LegacyName`), the data
folders `~/.wudict` and `~/.wuDict2` (`internal/cli/migration_windows.go`), the
env vars `WUDICT_HOME` / `WUDICT_PORTABLE_HOME`, the WebView bridge and
localStorage names (`wudictGet*`, `wudict_theme`, `window.wudictI18n`,
`internal/cli/desktop_window_windows.go`) and the Android `libwudictbridge.so`;
and one sentence in `README.md` recording that DictShelf is a fork of WuWeiDict.

## The build

One flavour: **cgo**, `-tags sqlite_fts5,sqlite_dbstat`. A tag-less build or
`CGO_ENABLED=0` **fails on purpose** (`internal/store/driver_missing.go`) — the
pure-Go driver was removed on 2026-11-03, so the failure is the feature, not a
bug to fix. `make check` before declaring work done (the Makefile is the
developer UI, D10: `make help` lists every target); `make api-test` keeps
`internal/server/web/openapi.yaml` matched to the routes, and `make api-lint`
validates it with Redocly (needs Node). This machine's command with the compiler
path is in `../DictShelf_Priv/build-tools.md`.

## Layout

- `internal/cli` — the CLI and the server entry point (all of it); one `package main` shim at the module root builds `dictshelf`. The version stamp targets `internal/cli.Version`.
- `internal/dict` — the format-agnostic core: interfaces, format registry, discovery.
- `internal/format/<fmt>` — `mdx`, `stardict`, `slob`, `dsl`, `lsd`, `bgl`, `wmd`, `zim`. Each implements both `Dictionary` (runtime lookup) and `Reader` (one-shot ingest scan).
- `internal/store` — the ingested SQLite backend: schema, ingest, library folders, media, `Reconcile`, and the fork's `dmsh_*` pieces (mask ranges, article results, articles split/rebuild).
- `internal/search` — the per-dictionary query and the streaming fan-out; `internal/ftsq` lowers a typed query into the readings; `internal/hilite` marks the matched spans; `internal/watchdog` reports a loop that is ours when it stops moving (a diagnostic, never a clock or a cancellation).
- `internal/server` — the persistent HTTP server, the API and the embedded UI (`web/index.html`, `web/setup.html`, `web/frame.js`, `web/openapi.yaml`).
- `internal/intake` — arbitrary bytes (archive, URL, folder) → candidate dictionaries → installed; `internal/config` — flag > env > toml layering; `internal/fsx` — the file handling every package shares.
- `internal/lang`, `internal/morph`, `internal/lemmas` — the language a dictionary is indexed in, inflection → dictionary form (O3), and the lemma data that morph reads (D88).
- `internal/limits` — the `MAX_*` bounds on how much a file, an import or a download may occupy; `internal/indexconst` — the shared sizes and intervals of prepared indexes.
- `internal/htmlref` — resource references inside one article; `internal/resource` — matching the name an article asks for; `internal/artmark` — the markup contract for articles this app writes.
- `internal/estimate` — what an index will cost before it is built; `internal/facet` — the groups a dictionary belongs to; `internal/logx` — the app-wide message channel; `internal/sysmem` — what memory the machine has and has free; `internal/tray` — the optional system-tray icon.
- `internal/gomdict` — the inlined MDX/MDD parser; `internal/speex` — the in-process `.spx` decoder (D18).
- `internal/howto` — the app's own guide, embedded as a wudict-markdown dictionary; edit it there, not in `pages/`.
- `internal/wildcard` — **upstream's wildcard queries, kept UNWIRED** (dead code: take upstream's side verbatim on a sync, never wire it; the registry and the `dmsh_` naming convention are in `../DictShelf_Priv/docs/DEAD-UPSTREAM.md`).
- `android/` — the Java WebView shell and the Go library it execs (no Go code of its own, no build tags); `pages/` — the published site; `packaging/` — installer and brand assets; `tools/` — the scripts the Makefile targets call.

## Conventions

- **Need-to-know (D102).** Internals and the UI are separate layers: a mechanism reaches the UI only when the user must *decide* or *act* on it, never as an explanation of how the app works. Applies to every label, hint and message.
- **Comments state the present (D161).** Invariants, reasons, measurements and D/P/§ citations — never "used to" narratives; history belongs to the commit.
- **Prepared data changes only through `store.Reconcile` (D157).** A new way to prepare, rebuild or pack states a `store.Target`, judges a prepared database through one `store.Inspect` read, and never calls `IngestPlan`/`IngestMedia` itself.
- **The build is monolithic**: the server runs inside the app (WebView2 on Windows, Go/JNI on Android). The "browser against a separate server" mode is kept in the code and **is not tested** — verify a server change on the in-process path.
- **Licence.** GPL-3.0-or-later; the `SPDX-License-Identifier` line belongs in every file that has a header at all. A file **this fork authored** carries `Copyright (C) 2026 DmShAl (Shepeta Dmitry)` above the SPDX line; a file **inherited from upstream keeps upstream's notice untouched** — no fork line, no rewritten name. Which is which follows the CONTENT, not the path (`git diff --name-status master dev`, then compare the text). Never edit `LICENSE`; the canonical statement is the Licence section of `README.md`.
- **`D-`, `P-` and `C-` numbers** cited in the code (`D15`, `docs.local/PERF.md §3.1`, `P88`) refer to the maintainer's decision log, phase record, performance audit and review backlog — working records kept outside the repository. The citations are provenance: treat one you cannot open as a note on *why* the code is the way it is, not as a missing file. The same goes for the reference projects this fork was built from (`mdict-go-web`, `draego`, `pyglossary`, the speex sources), which are **removed from disk**: `docs/FORMATS.md` still cites their file and function names as the provenance of a rule, not as paths.
- Before writing code: the section of `docs/SPEC.md` (architecture, schema, query engine) and of `docs/FORMATS.md` (per format) that you touch. Before writing any text a user reads: `docs/STYLE.md`.
