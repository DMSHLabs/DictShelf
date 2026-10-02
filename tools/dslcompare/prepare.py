#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Extract unchanged GoldenDict parser functions for the isolated Qt harness."""
import argparse
import hashlib
import json
from pathlib import Path


def section(text, start, end):
    return text[text.index(start):text.index(end, text.index(start))]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    args = parser.parse_args()
    out = Path(__file__).parent / "vendor"
    out.mkdir(exist_ok=True)
    cc = (args.source / "dsl_details.cc").read_text(encoding="utf-8-sig")
    hh = (args.source / "dsl_details.hh").read_text(encoding="utf-8-sig")
    folding = (args.source / "folding.cc").read_text(encoding="utf-8-sig")
    notice = cc[:cc.index("#include")]
    declaration = section(hh, "struct ArticleDom", "/// A adapted version")
    functions = section(cc, "wstring ArticleDom::Node::renderAsText", "/////////////// DslScanner")
    titles = section(cc, "void processUnsortedParts", "namespace\n{\n  void cutEnding")
    whitespace = section(folding, "bool isWhitespace( wchar ch )", "bool isPunct( wchar ch )")
    trim = section(folding, "wstring trimWhitespace( wstring const & in )", "void normalizeWhitespace")
    (out / "parser.inc").write_text(notice + declaration + functions + titles, encoding="utf-8")
    (out / "folding.inc").write_text(notice + whitespace + trim, encoding="utf-8")
    license_file = next(p for p in (args.source / "LICENSE", args.source / "LICENSE.txt", args.source / "COPYING") if p.exists())
    (out / "LICENSE").write_bytes(license_file.read_bytes())
    provenance = {name: hashlib.sha256((args.source / name).read_bytes()).hexdigest()
                  for name in ("dsl_details.cc", "dsl_details.hh", "folding.cc", license_file.name)}
    (out / "provenance.json").write_text(json.dumps(provenance, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
