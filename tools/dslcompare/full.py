#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Full corpus comparison in bounded batches; retain complete failing batches."""
import argparse
from itertools import islice
import json
from pathlib import Path
import shutil
import subprocess
import sys
import time
import traceback
from compare import HERE, ROOT, compare_cases, execute, iter_dictionary_cases


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("folder", type=Path)
    parser.add_argument("--qt-bin", type=Path, required=True)
    parser.add_argument("--batch", type=int, default=1000)
    parser.add_argument("--output", type=Path, default=HERE / "results" / "full")
    args = parser.parse_args()
    if not args.folder.exists() or args.batch < 1:
        parser.error("input folder/file must exist and batch must be positive")
    args.output = args.output.resolve()
    if (args.output / "corpus.json").exists():
        parser.error("output already contains a run; choose a new output directory")
    args.output.mkdir(parents=True, exist_ok=True)
    binary = args.output / "go-parser.test.exe"
    execute(["go", "test", "-c", "./internal/format/dsl", "-o", str(binary)], cwd=ROOT)
    candidates = [args.folder] if args.folder.is_file() else args.folder.rglob("*")
    files = sorted(p for p in candidates if p.is_file()
                   and str(p).lower().endswith((".dsl", ".dsl.dz")))
    if not files:
        parser.error("no DSL files found")
    results = []
    start = time.monotonic()
    for index, path in enumerate(files):
        directory = args.output / f"{index + 1:03d}"
        work = directory / "work"
        result = {"path":str(path), "cases":0, "matching":0, "differences":0,
                  "status":"running", "output":str(directory)}
        results.append(result)
        print(f"[{index + 1}/{len(files)}] {path.name}", flush=True)
        cases = iter_dictionary_cases(path)
        try:
            batch_number = 0
            while True:
                batch = list(islice(cases, args.batch))
                if not batch:
                    result["status"] = "complete"
                    break
                batch_number += 1
                summary = compare_cases(batch, work, HERE / "build" / "dsl-oracle.exe", args.qt_bin, binary)
                result["cases"] += summary["cases"]
                result["matching"] += summary["matching"]
                result["differences"] += len(summary["differences"])
                if summary["differences"]:
                    shutil.copytree(work, directory / f"diff-{batch_number:05d}")
                (args.output / "corpus.json").write_text(json.dumps(results, indent=2, ensure_ascii=False), encoding="utf-8")
                if batch_number % 10 == 0:
                    print(f"  {result['cases']} cards, {result['differences']} differences", flush=True)
        except Exception as error:
            result["status"] = "error"
            result["error"] = str(error)
            directory.mkdir(parents=True, exist_ok=True)
            (directory / "error.txt").write_text(traceback.format_exc(), encoding="utf-8")
            if isinstance(error, subprocess.CalledProcessError):
                (directory / "adapter-error.log").write_bytes((error.stdout or b"") + (error.stderr or b""))
        finally:
            cases.close()
        (args.output / "corpus.json").write_text(json.dumps(results, indent=2, ensure_ascii=False), encoding="utf-8")
        print(f"  {result['status']}: {result['cases']} cards, {result['differences']} differences", flush=True)
    print(f"Finished in {time.monotonic()-start:.1f}s; report: {args.output / 'corpus.json'}", flush=True)
    return 1 if any(r["status"] != "complete" or r["differences"] for r in results) else 0


if __name__ == "__main__":
    sys.exit(main())
