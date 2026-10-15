# Agent handoff — current state
Checkout: branch `dev2`; merging `dev` commit `f54e3a2` into `dev2` commit `b35377f`.

## Current work
The merge keeps the Windows desktop build as one GUI `wuDict2.exe`; `build-windows.cmd console` builds the same executable with a console subsystem for diagnostics. Windows tray launch detection uses the executable subsystem and falls back to the browser for console runs. The native WebView bridge shares Go services for search, dictionary listing, indexing, reindexing, ingest and progress; HTTP remains available to the server and browser UI.

Fork-specific dictionary groups, mask search, migration and Android identity remain enabled. Upstream how-to and wildcard features remain unwired as recorded in `AGENTS.md`.

## Verification for this merge
`go build ./...`, the focused server tests for work admission, native search parity and reindex start/stop, and `build-windows.cmd debug` pass. The full `go test ./...` does not pass on the merged tree. A clean source snapshot of the pre-merge `dev2` also fails one test (`TestWorkAdmissionIndividualAndExclusive`); the merged tree additionally exposes server tests that assumed DSL indexes were created when a dictionary was opened, while `dev` now requires the add queue or an explicit index action. Store tests also report mismatches in prefix/contains expectations. These need review before treating the full suite as green.

## Remaining manual check
Launch the built Windows GUI and verify native WebView search and indexing progress. No Android build/device verification is planned for this merge.
