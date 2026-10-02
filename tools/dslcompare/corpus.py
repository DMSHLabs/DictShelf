#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Run bounded differential samples from every DSL dictionary in a folder."""
import argparse
import json
from pathlib import Path
import subprocess
import sys

HERE = Path(__file__).resolve().parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("folder", type=Path)
    parser.add_argument("--qt-bin", type=Path, required=True)
    parser.add_argument("--limit", type=int, default=100)
    parser.add_argument("--output", type=Path, default=HERE / "results" / "corpus")
    parser.add_argument("--html", action="store_true")
    parser.add_argument("--enhance", action="store_true")
    args = parser.parse_args()
    if not args.folder.is_dir() or args.limit < 1:
        parser.error("folder must exist and limit must be positive")
    files = sorted(p for p in args.folder.rglob("*") if p.is_file()
                   and str(p).lower().endswith((".dsl", ".dsl.dz")))
    args.output.mkdir(parents=True, exist_ok=True)
    results = []
    for index, path in enumerate(files):
        output = args.output / f"{index + 1:03d}"
        print(f"[{index + 1}/{len(files)}] {path.name}", flush=True)
        run = subprocess.run([sys.executable, str(HERE / "compare.py"),
                              "--dsl", str(path), "--qt-bin", str(args.qt_bin),
                              "--limit", str(args.limit), "--output", str(output)] + (["--html"] if args.html else []) + (["--enhance"] if args.enhance else []),
                             stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=360)
        output.mkdir(exist_ok=True)
        (output / "run.log").write_bytes(run.stdout)
        result = {"path":str(path), "exit":run.returncode, "output":str(output.resolve())}
        summary = output / "summary.json"
        if summary.exists() and run.returncode in (0, 1):
            result.update(json.loads(summary.read_text(encoding="utf-8")))
        results.append(result)
        (args.output / "corpus.json").write_text(json.dumps(results, indent=2, ensure_ascii=False), encoding="utf-8")
        print(f"  exit={run.returncode}, cases={result.get('cases', '?')}, matches={result.get('matching', '?')}", flush=True)
    return 1 if any(r["exit"] for r in results) else 0


if __name__ == "__main__":
    sys.exit(main())
