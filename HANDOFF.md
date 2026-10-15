# Agent handoff — current state
Checkout: branch `dev2` at `a1d0ce5`, follow-up to merge commit `406a8f2` (`dev` `f54e3a2` into `dev2` `b35377f`).

## Current work
The merge keeps the Windows desktop build as one GUI `wuDict2.exe`; `build-windows.cmd console` builds the same executable with a console subsystem for diagnostics. Windows tray launch detection uses the executable subsystem and falls back to the browser for console runs. The native WebView bridge shares Go services for search, dictionary listing, indexing, reindexing, ingest and progress; HTTP remains available to the server and browser UI.

Newly added dictionaries always queue the mandatory headword index, even before optional index defaults are saved. Saved defaults select only Contains and full-text. Search does not build a missing index; it skips that dictionary. The focused tests cover both rules.

Fork-specific dictionary groups, mask search, migration and Android identity remain enabled. Upstream how-to and wildcard features remain unwired as recorded in `AGENTS.md`.

## Verification
`go build ./...`, the Windows debug build, and focused server tests for work admission, native search parity, reindex start/stop, new-dictionary indexing and search-without-index pass. The full suite was not rerun after the latest fix; the current server and store package runs still show failures. Server failures include tests that start asynchronous index queues without waiting, plus older assumptions about direct DSL access. Two store tests disagree with prefix/contains results. A clean pre-merge `dev2` snapshot also failed `TestWorkAdmissionIndividualAndExclusive`.

## Remaining manual check
Launch the built Windows GUI and verify native WebView search and indexing progress. No Android build/device verification is planned for this merge.
