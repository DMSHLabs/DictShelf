#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Recheck every retained failing batch with the current Go parser."""
import argparse
import json
from pathlib import Path
from compare import HERE, ROOT, compare_cases, execute


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run", type=Path)
    parser.add_argument("--qt-bin", type=Path, required=True)
    args = parser.parse_args()
    binary = args.run.resolve() / "go-parser.recheck.test.exe"
    execute(["go", "test", "-c", "./internal/format/dsl", "-o", str(binary)], cwd=ROOT)
    results = json.loads((args.run / "corpus.json").read_text(encoding="utf-8"))
    for result in results:
        if result["status"] != "complete":
            continue
        remaining = 0
        checked = 0
        for batch in sorted(Path(result["output"]).glob("diff-*")):
            cases = json.loads((batch / "input.json").read_text(encoding="utf-8"))
            summary = compare_cases(cases, batch / "recheck", HERE / "build" / "dsl-oracle.exe", args.qt_bin, binary)
            remaining += len(summary["differences"])
            checked += len(cases)
        result.update(rechecked_cases=checked, remaining_differences=remaining)
        print(f"{Path(result['path']).name}: {result['differences']} original, {remaining} remaining", flush=True)
    (args.run / "rechecked.json").write_text(json.dumps(results, indent=2, ensure_ascii=False), encoding="utf-8")
    lines = ["# Full DSL Corpus Comparison", "",
             "Keys and normalized DSL trees only; not original HTML or native scanner/index parity.", "",
             "Completed files: " + str(sum(r["status"] == "complete" for r in results)),
             "Compared cards: " + str(sum(r["cases"] for r in results)), "",
             "| Dictionary | Status | Cards | Remaining Differences |",
             "| --- | --- | ---: | ---: |"]
    for result in results:
        remaining = str(result.get("remaining_differences", "not rechecked")) if result["status"] == "complete" else "incomplete"
        lines.append(f"| {Path(result['path']).name} | {result['status']} | {result['cases']} | {remaining} |")
    (args.run / "report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    return 1 if any(r["status"] != "complete" or r.get("remaining_differences", 0) for r in results) else 0


if __name__ == "__main__":
    raise SystemExit(main())
