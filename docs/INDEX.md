# What to read for what

One step from a question to the file that answers it. Read the row, then the file -
and read only the part of it the row names: `HANDOFF.md` and the archive are not
documents to read end to end.

| The question in front of you | File |
| --- | --- |
| Where does this fork stand, what is left to do, what are the rules for search, indexes and removal? | [`HANDOFF.md`](../HANDOFF.md) - its own contents list is at the top |
| How do I build, test, run or preview it **on this Windows machine**, and which recipes are known to be broken here? | [`WINDOWS-VERIFY.md`](WINDOWS-VERIFY.md) |
| Which tools are installed here and what is each one called? (**no `make` on PATH** - `mingw32-make`; gcc, Inkscape, Inno Setup, node) | [`thirdpartytools.md`](../thirdpartytools.md) |
| Release channels and version suffixes (`--channel release/pre-release/beta`) | `#build.md` at the repository root (the leading `#` is part of the file name) |
| Android: app identity, package names, build variants, App Links, the emulator recipe | [`ANDROID-FORK.md`](ANDROID-FORK.md) |
| Android and Windows UI: what each window does today, and what is still unverified | [`ANDROID-UI-HANDOFF.md`](ANDROID-UI-HANDOFF.md) |
| Architecture, database schema, query engine, ingest - read before writing code | [`SPEC.md`](SPEC.md) |
| One format's facts and the exact reference-code pointers (MDX, StarDict, Slob, DSL, LSD, BGL, ZIM, wudict markdown) | [`FORMATS.md`](FORMATS.md), [`DSL.md`](DSL.md), [`LSD.md`](LSD.md), [`WUDICT-MARKDOWN.md`](WUDICT-MARKDOWN.md) |
| Any text a user reads: manual, README, UI strings, `--help`, howto | [`STYLE.md`](STYLE.md) |
| Translating the interface, or a locale bug | [`translation.md`](../translation.md), [`translation_todo.md`](../translation_todo.md), [`I18N.md`](I18N.md) |
| How much something costs or occupies: measured index sizes, read-through builds, the ingest write path | [`MEASUREMENTS.md`](MEASUREMENTS.md) |
| Xapian's own full-text index inside a ZIM ("glass"), and the reader for it (branch `xapian-glass`) | [`GLASS.md`](GLASS.md) |
| How the API works for a client (NDJSON, `/api/search`, `/res/`, link grammar) | [`CLIENT-API.md`](CLIENT-API.md) |
| Dictionary groups | [`DICTIONARY-GROUPS.md`](DICTIONARY-GROUPS.md) |
| The Browse/Wildcards headword ribbon and dock | [`BROWSE-WILDCARDS-CONTROL.md`](BROWSE-WILDCARDS-CONTROL.md) |
| The workspace-dialog refactor (iframe pages into in-document dialogs) | [`DIALOG-REFACTOR.md`](DIALOG-REFACTOR.md) |
| Portable-mode write paths | [`PORTABLE-WRITES-AUDIT.md`](PORTABLE-WRITES-AUDIT.md) |
| Porting upstream format/resource work into this fork | [`UPSTREAM-TRANSFER-PLAN.md`](UPSTREAM-TRANSFER-PLAN.md) |
| Researched but **not scheduled** - do not start any of it unasked | [`OPEN.md`](OPEN.md) |
| What happened in earlier sessions (grep, never read whole) | [`handoff-archive.md`](handoff-archive.md) |
| Licence notices for the vendored code and the modules in the binaries | [`../THIRD-PARTY-NOTICES.md`](../THIRD-PARTY-NOTICES.md) |
| The user-facing manual (the published site, `pages/`) | [`../pages/docs/index.md`](../pages/docs/index.md) |

Two rules that keep this map true:

- **A fact lives in exactly one of these files.** When you move one, update the row
  here and the pointer in the file you took it from (`grep -rn "<name>.md"` finds
  them). A doc nothing links to is a doc nobody reads - `thirdpartytools.md` sat
  unread for months while every session re-derived that `make` does not exist here.
- **Code citations count as links.** A Go comment that names a document names one
  that exists, with the section it means.
