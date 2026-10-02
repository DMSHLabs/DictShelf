#!/usr/bin/env python3
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
"""Classify saved HTML corpus outputs without rerunning or replacing adapters."""
import argparse
import json
from pathlib import Path
from html_compare import normalize_html, without_spacing


def summarize(directory):
    manifest = json.loads((directory / "corpus.json").read_text(encoding="utf-8"))
    rows = []
    for item in manifest:
        path = Path(item["output"])
        if item["exit"] not in (0, 1):
            raise ValueError(f"incomplete sample: {item['path']}")
        gd = json.loads((path / "oracle.json").read_text(encoding="utf-8"))
        go = json.loads((path / "go.json").read_text(encoding="utf-8"))
        if [v["id"] for v in gd] != [v["id"] for v in go]:
            raise ValueError(f"adapter ids differ: {path}")
        row = {"path": item["path"], "cases": len(gd), "html_compared": 0,
               "html_matching": 0, "html_spacing_only": [], "html_other": [],
               "html_excluded_media": [], "keys_or_tree_differences": [d["id"] for d in item["differences"] if any(f != "html" for f in d["fields"])]}
        for a, b in zip(gd, go):
            if a["excluded_media"]:
                row["html_excluded_media"].append(a["id"])
                continue
            row["html_compared"] += 1
            left, right = normalize_html(a["html"]), normalize_html(b["html"])
            if left == right:
                row["html_matching"] += 1
            elif without_spacing(left) == without_spacing(right):
                row["html_spacing_only"].append(a["id"])
            else:
                row["html_other"].append(a["id"])
        rows.append(row)
    totals = {key: sum(r[key] if isinstance(r[key], int) else len(r[key]) for r in rows)
              for key in rows[0] if key != "path"}
    return {"scope": "bounded top-level samples; explicit HTML vocabulary mapping, not layout parity",
            "spacing_policy": "diagnostic only; spacing-only IDs are still strict differences",
            "totals": totals, "files": rows}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", type=Path)
    args = parser.parse_args()
    report = summarize(args.directory)
    (args.directory / "html_report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
    lines = ["# Original GoldenDict Text HTML Comparison", "", report["scope"], "",
             "Media articles are excluded. Spacing-only differences remain failures, not accepted matches.", "",
             "| Dictionary | Cases | HTML compared | Matches | Spacing only | Other | Media excluded |",
             "| --- | ---: | ---: | ---: | ---: | ---: | ---: |"]
    for r in report["files"]:
        lines.append(f"| {Path(r['path']).name} | {r['cases']} | {r['html_compared']} | {r['html_matching']} | {len(r['html_spacing_only'])} | {len(r['html_other'])} | {len(r['html_excluded_media'])} |")
    lines += ["", "Totals:", "```json", json.dumps(report["totals"], indent=2), "```",
              "", "Original per-case raw HTML and strict diffs are retained in numbered folders."]
    (args.directory / "report.md").write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(json.dumps(report["totals"], indent=2))


if __name__ == "__main__":
    main()
