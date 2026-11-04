# DictShelf — where things are

This repository is the public half of DictShelf. It holds the product and the
documents a person needs to build it. Upstream is `wuweidict/wudict`; the branch
`master` here mirrors it.

## The working notes are not here

The fork's working notes — plans, handoffs, session logs, the document gate, and
the rules that govern them — live outside this repository, in `../DictShelf_Priv`.

Before you start work in this checkout, read these two files:

1. `../DictShelf_Priv/AGENTS.md` — the rules the work follows.
2. `../DictShelf_Priv/HANDOFF.md` — the state of the fork.

If `../DictShelf_Priv` is absent, this is the public copy only. Build and test as
`Makefile` and `README.md` describe, and expect no working notes.

## The product

`README.md` describes the program. `docs/` holds the format contracts
(`DSL.md`, `FORMATS.md`, `DICTZIP-V2.md`, `WUDICT-MARKDOWN.md`), the architecture
and schema (`SPEC.md`), the writing rules (`STYLE.md`), the client API
(`CLIENT-API.md`), the translation policy (`I18N.md`) and the open questions
(`OPEN.md`).
