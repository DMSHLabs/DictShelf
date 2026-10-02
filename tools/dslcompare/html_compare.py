# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
"""Explicit HTML vocabulary translation, not whitespace or layout equivalence."""
from html.parser import HTMLParser
from urllib.parse import parse_qsl, unquote, urlsplit

ROLES = {
    "dsl_b": "bold", "dsl_i": "italic", "dsl_u": "underline",
    "dsl_opt": "optional", "wu-sec": "optional",
    "dsl_trn": "translation", "wu-trn": "translation",
    "dsl_ex": "example", "wu-ex": "example",
    "dsl_com": "comment", "wu-com": "comment",
    "dsl_trs": "not-transcription", "wu-trs-not": "not-transcription",
    "dsl_p": "abbreviation", "wu-p": "abbreviation",
    "dsl_t": "transcription", "wu-ipa": "transcription",
    "dsl_lang": "language", "wu-lang": "language",
    "dsl_stress": "stress", "wu-acc": "stress",
    "dsl_unknown": "unknown", "wu-unknown": "unknown",
}
VOID = {"br", "img", "input", "hr", "meta", "link", "source", "wbr"}


class Document(HTMLParser):
    def __init__(self, source):
        super().__init__(convert_charrefs=True)
        self.root = {"tag": "root", "attrs": {}, "children": []}
        self.stack = [self.root]
        self.feed(source)
        self.close()

    def handle_starttag(self, tag, attrs):
        node = {"tag": tag, "attrs": dict(attrs), "children": []}
        self.stack[-1]["children"].append(node)
        if tag not in VOID:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.handle_starttag(tag, attrs)
        if tag not in VOID:
            self.handle_endtag(tag)

    def handle_endtag(self, tag):
        for i in range(len(self.stack) - 1, 0, -1):
            if self.stack[i]["tag"] == tag:
                del self.stack[i:]
                break

    def handle_data(self, data):
        children = self.stack[-1]["children"]
        if children and isinstance(children[-1], str):
            children[-1] += data
        else:
            children.append(data)


def canonical(node):
    if isinstance(node, str):
        return node
    attrs = node["attrs"].copy()
    tag = node["tag"]
    classes = attrs.pop("class", "").split()
    children = node["children"]
    role_class = next((c for c in classes if c in ROLES), None)
    if role_class:
        tag = ROLES[role_class]
        classes.remove(role_class)
    elif tag in ("b", "i", "u"):
        tag = {"b": "bold", "i": "italic", "u": "underline"}[tag]
    if tag == "optional":
        attrs.pop("id", None)  # Engine-generated DOM identity only.
    if tag == "language":
        attrs.pop("data-lang", None)  # wudict's author-token selector; lang is still compared.
    if "dsl_m" in classes or any(c.startswith("dsl_m") and c[5:].isdigit() for c in classes):
        margin = next(c for c in classes if c.startswith("dsl_m"))
        tag = "margin"
        attrs["level"] = margin[5:] or "0"
        classes.remove(margin)
    if "wu-m" in classes:
        tag = "margin"
        style = attrs.get("style", "")
        if not style or style.startswith("--wd-m:") and ";" not in style:
            attrs.pop("style", None)
            attrs["level"] = style.removeprefix("--wd-m:") or "0"
        classes.remove("wu-m")
    if tag == "font" and "color" in attrs:
        tag = "color"
        attrs["color"] = "default" if attrs["color"] == "c_default_color" else attrs["color"]
    if "wu-c" in classes:
        tag = "color"
        style = attrs.get("style", "")
        if not style or style.startswith("--wd-c:") and ";" not in style:
            attrs.pop("style", None)
            attrs["color"] = style.removeprefix("--wd-c:") or "default"
        classes.remove("wu-c")
    if tag == "abbreviation" and len(children) == 1 and isinstance(children[0], dict):
        abbr = children[0]
        if abbr["tag"] == "abbr" and abbr["attrs"].get("class") == "wu-abbr":
            attrs.update({k: v for k, v in abbr["attrs"].items() if k != "class"})
            children = abbr["children"]
    if tag == "stress" and len(children) == 2 and all(isinstance(c, dict) for c in children):
        first, second = children
        if (first["attrs"].get("class") == "dsl_stress_without_accent"
                and second["attrs"].get("class") == "dsl_stress_with_accent"
                and second["children"] == accented(first["children"])):
            children = first["children"]
        # Otherwise retain BOTH branches: unexpected accented text must be visible.
    if tag == "a":
        href = attrs.get("href", "")
        url = urlsplit(href)
        if url.scheme == "gdlookup" and url.netloc == "localhost":
            attrs.pop("href", None)
            attrs["entry"] = unquote(url.path.removeprefix("/"))
            query = dict(parse_qsl(url.query, keep_blank_values=True))
            if "dict" in query:
                attrs["dictionary"] = query.pop("dict")
            if query or url.fragment:
                attrs["query"] = query
                attrs["fragment"] = url.fragment
        elif url.scheme == "entry":
            attrs.pop("href", None)
            attrs["entry"] = unquote(href[len("entry://"):])
            if "data-dict" in attrs:
                attrs["dictionary"] = attrs.pop("data-dict")
                if attrs.get("title") == attrs["dictionary"]:
                    attrs.pop("title")
        classes = [c for c in classes if c not in ("dsl_ref", "dsl_url", "wu-xref")]
    if classes:
        attrs["class"] = sorted(classes)
    return {"tag": tag, "attrs": attrs, "children": [canonical(c) for c in children]}


def accented(children):
    result = children[:]
    if result and isinstance(result[-1], str):
        result[-1] += "\u0301"
    else:
        result.append("\u0301")
    return result


def normalize_html(source):
    return canonical(Document(source).root)


def without_spacing(node):
    """Diagnostic classification only. NEVER used to decide whether HTML matches."""
    if isinstance(node, str):
        return "".join(c for c in node if c not in " \t\r\n")
    children = []
    for child in node["children"]:
        if isinstance(child, dict) and child["tag"] in ("p", "br") and not child["attrs"] and not child["children"]:
            continue
        child = without_spacing(child)
        if child == "":
            continue
        if children and isinstance(child, str) and isinstance(children[-1], str):
            children[-1] += child
        else:
            children.append(child)
    return {**node, "children": children}
