#!/usr/bin/env python3
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
"""Compare GoldenDict ArticleDom with the experimental Go DSL tree parser."""
import argparse
import difflib
import gzip
import io
import json
import os
from pathlib import Path
import shlex
import subprocess
import sys
from html_compare import normalize_html, without_spacing

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent


def iter_dictionary_cases(path):
    stream = gzip.open(path, "rb") if str(path).endswith(".dz") else path.open("rb")
    data = stream.peek(4)[:4]
    if data.startswith((b"\xff\xfe", b"\xfe\xff")):
        encoding = "utf-16"
    elif data.startswith(b"\xef\xbb\xbf"):
        encoding = "utf-8-sig"
    else:
        # Never silently guess a legacy encoding for an oracle comparison.
        encoding = "utf-8"
    headings, body, count = [], [], 0
    with io.TextIOWrapper(stream, encoding=encoding) as text:
        for line in text:
            line = line.rstrip("\r\n")
            if line.startswith("#") or not line.strip():
                continue
            if line.startswith((" ", "\t")):
                body.append(line.lstrip(" \t"))
            else:
                if body:
                    if headings:
                        count += 1
                        yield {"id":str(count), "key":headings[0],
                               "headings":headings[:], "body":"\n".join(body)}
                    headings.clear()
                    body.clear()
                headings.append(line)
    if headings and body:
        yield {"id":str(count + 1), "key":headings[0],
               "headings":headings[:], "body":"\n".join(body)}


def dictionary_cases(path, limit):
    from itertools import islice
    cases = iter_dictionary_cases(path)
    try:
        return list(islice(cases, limit)) if limit else list(cases)
    finally:
        cases.close()


def normalize(n):
    if "tag" not in n and "children" not in n:
        return {"text":n.get("text", "")}
    tag = n.get("tag", "")
    if tag == "literal":
        return {"text":n["text"]}
    attrs = n.get("attrs") or {}
    if isinstance(attrs, str):
        lexer = shlex.shlex(attrs, posix=True, punctuation_chars="=")
        lexer.whitespace_split = True
        lexer.commenters = ""
        values = list(lexer)
        attrs = {}
        index = 0
        while index < len(values):
            name = values[index]
            index += 1
            value = ""
            if index < len(values) and values[index] == "=":
                index += 1
                if index < len(values):
                    value = values[index]
                    index += 1
            attrs[name] = value
    children = []
    for child in n.get("children") or []:
        child = normalize(child)
        if children and "text" in child and "text" in children[-1]:
            children[-1]["text"] += child["text"]
        else:
            children.append(child)
    return {"tag":tag, "attrs":attrs, "children":children}


def execute(command, **kwargs):
    return subprocess.run(command, check=True, timeout=300, **kwargs)


def compare_cases(cases, output, oracle_path, qt_bin=None, go_binary=None, html=False, enhance=False):
    if not cases or len({c["id"] for c in cases}) != len(cases):
        raise ValueError("cases must be nonempty and have unique ids")
    output.mkdir(parents=True, exist_ok=True)
    input_path = (output / "input.json").resolve()
    go_path = (output / "go.json").resolve()
    input_path.write_text(json.dumps(cases, ensure_ascii=False), encoding="utf-8")
    env = os.environ.copy()
    if qt_bin:
        env["PATH"] = str(qt_bin.resolve()) + os.pathsep + env["PATH"]
    oracle = execute([str(oracle_path.resolve())] + (["--html"] if html else []), input=input_path.read_bytes(),
                     stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    (output / "oracle.json").write_bytes(oracle.stdout)
    (output / "oracle.log").write_bytes(oracle.stderr)
    env.update(WUDICT_DSL_COMPARE_INPUT=str(input_path), WUDICT_DSL_COMPARE_OUTPUT=str(go_path))
    env["WUDICT_DSL_COMPARE_ENHANCE"] = "1" if enhance else "0"
    go_path.unlink(missing_ok=True)
    command = ([str(go_binary.resolve()), "-test.run=^TestGDCompareDump$"] if go_binary else
               ["go", "test", "./internal/format/dsl", "-run", "^TestGDCompareDump$", "-count=1"])
    execute(command, cwd=ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    gd = json.loads(oracle.stdout)
    go = json.loads(go_path.read_text(encoding="utf-8"))
    if [v["id"] for v in gd] != [v["id"] for v in go] or len(gd) != len(cases):
        raise ValueError("adapter result ids/count differ")
    report, differences = [], []
    excluded, raw_equal, spacing_only = [], 0, []
    for c, a, b in zip(cases, gd, go):
        fields = []
        if html and a.get("excluded_media", 0):
            excluded.append(c["id"])
        if html and a["html"] == b["html"]:
            raw_equal += 1
        for field in ("keys", "tree", "html") if html and c["id"] not in excluded else ("keys", "tree"):
            convert = (lambda v: sorted(set(v))) if field == "keys" else normalize_html if field == "html" else normalize
            left, right = convert(a[field]), convert(b[field])
            if left == right:
                continue
            if field == "html" and without_spacing(left) == without_spacing(right):
                spacing_only.append(c["id"])
            fields.append(field)
            left = json.dumps(left, indent=2, ensure_ascii=False).splitlines()
            right = json.dumps(right, indent=2, ensure_ascii=False).splitlines()
            report.append("Case " + c["id"] + ": " + field + "\n" + "\n".join(
                difflib.unified_diff(left, right, fromfile="GoldenDict", tofile="Go", lineterm="")))
        if fields:
            differences.append({"id":c["id"], "fields":fields})
    summary = {"cases":len(cases), "matching":len(cases)-len(differences), "differences":differences,
               "scope":"heading functions and ArticleDom" + ("; original GD text HTML renderer" if html else "") + "; not native scanner/index/media/CSS layout"}
    summary["go_enhancer"] = enhance
    if html:
        summary.update(html_compared=len(cases)-len(excluded), html_excluded_media=excluded,
                       html_raw_equal=raw_equal, html_spacing_only=spacing_only,
                       html_other_differences=[d["id"] for d in differences if "html" in d["fields"] and d["id"] not in spacing_only],
                       html_policy="explicit vocabulary mapping; whitespace and other attributes preserved")
    (output / "summary.json").write_text(json.dumps(summary, indent=2), encoding="utf-8")
    (output / "diff.txt").write_text("\n\n".join(report), encoding="utf-8")
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--oracle", type=Path, default=HERE / "build" / "dsl-oracle.exe")
    parser.add_argument("--qt-bin", type=Path)
    group = parser.add_mutually_exclusive_group()
    group.add_argument("--cases", type=Path, default=None)
    group.add_argument("--dsl", type=Path)
    parser.add_argument("--limit", type=int, default=100, help="article limit; 0 means all")
    parser.add_argument("--output", type=Path, default=HERE / "results")
    parser.add_argument("--html", action="store_true", help="also compare original text HTML; exclude articles containing media")
    parser.add_argument("--enhance", action="store_true", help="enable optional Go HTML cleanup; off for reference comparisons")
    args = parser.parse_args()
    if args.limit < 0:
        parser.error("--limit must be nonnegative")
    cases = dictionary_cases(args.dsl, args.limit) if args.dsl else json.loads(
        (args.cases or HERE / "cases.json").read_text(encoding="utf-8"))
    summary = compare_cases(cases, args.output, args.oracle, args.qt_bin, html=args.html, enhance=args.enhance)
    print(f"Compared {len(cases)} cases: {summary['matching']} match, {len(summary['differences'])} differ.")
    print(f"Report: {args.output.resolve() / 'diff.txt'}")
    return 1 if summary['differences'] else 0


if __name__ == "__main__":
    sys.exit(main())
