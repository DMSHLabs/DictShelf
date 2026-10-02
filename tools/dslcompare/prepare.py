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
    sources = {}
    for name in ("dsl.cc", "htmlescape.cc", "filetype.cc", "utf8.cc", "langcoder.cc",
                 "langcoder.hh", "language.cc", "language.hh", "inc_case_folding.hh",
                 "inc_diacritic_folding.hh"):
        sources[name] = (args.source / name).read_text(encoding="utf-8-sig")
    render = section(sources["dsl.cc"], "string DslDictionary::processNodeChildren",
                     "QString const& DslDictionary::getDescription")
    media_start = render.index('  if ( node.tagName == GD_NATIVE_TO_WS( L"s" )')
    media_end = render.index('  if ( node.tagName == GD_NATIVE_TO_WS( L"url" )', media_start)
    # Stage 1 deliberately does not emulate resource lookup or media HTML.
    render = render[:media_start] + '''  if ( node.tagName == GD_NATIVE_TO_WS( L"s" ) || node.tagName == GD_NATIVE_TO_WS( L"video" ) )
  {
    ++excludedMedia;
    result += "<dsl-media></dsl-media>";
  }
  else
''' + render[media_end:]
    ws = section(sources["dsl.cc"], "bool isDslWs( wchar ch )", "void DslDictionary::loadArticle")
    (out / "renderer.inc").write_text(notice + ws + render, encoding="utf-8")
    helpers = "namespace Html {\n" + section(sources["htmlescape.cc"], "string escape(", "static void storeLineInDiv") + "}\n"
    helpers += "namespace Filetype {\n" + section(sources["filetype.cc"], "string simplifyString(", "bool isNameOfSound") + "}\n"
    (out / "html_helpers.inc").write_text(notice + helpers, encoding="utf-8")
    folding_helpers = section(folding, "bool isCombiningMark", "wstring applySimpleCaseOnly")
    punct = section(folding, "bool isPunct( wchar ch )", "wstring trimWhitespaceOrPunct")
    (out / "language_folding.inc").write_text(notice + folding_helpers + punct, encoding="utf-8")
    for name in ("inc_case_folding.hh", "inc_diacritic_folding.hh"):
        (out / name).write_text(sources[name], encoding="utf-8")
    language = "namespace Language {\nusing Id = quint32;\n" + section(sources["language.hh"], "struct BabylonLang", "BabylonLang getBabylonLangByIndex")
    language += section(sources["language.cc"], "#ifndef blgCode2Int", "BabylonLang getBabylonLangByIndex")
    language += section(sources["language.cc"], "quint32 findBlgLangIDByEnglishName", "QString englishNameForId") + "}\n"
    language += section(sources["langcoder.hh"], "struct GDLangCode", "template <typename")
    language += section(sources["langcoder.cc"], "static GDLangCode LangCodes[]", "LangCoder::LangCoder()")
    language += section(sources["langcoder.cc"], "QString LangCoder::intToCode2", "quint32 LangCoder::findIdForLanguageCode3")
    (out / "language.inc").write_text(notice + language, encoding="utf-8")
    dsl_language = section(hh, "struct DSLLangCode", "string findCodeForDslId")
    dsl_language += section(cc, "static DSLLangCode LangCodes[]", "bool isAtSignFirst")
    dsl_language += cc[cc.index("namespace\n{\n  void cutEnding"):].rsplit("}\n}", 1)[0]
    (out / "dsl_language.inc").write_text(notice + dsl_language, encoding="utf-8")
    license_file = next(p for p in (args.source / "LICENSE", args.source / "LICENSE.txt", args.source / "COPYING") if p.exists())
    (out / "LICENSE").write_bytes(license_file.read_bytes())
    provenance = {name: hashlib.sha256((args.source / name).read_bytes()).hexdigest()
                  for name in ("dsl_details.cc", "dsl_details.hh", "folding.cc", license_file.name, *sources)}
    (out / "provenance.json").write_text(json.dumps(provenance, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
