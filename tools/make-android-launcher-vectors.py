#!/usr/bin/env python3
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later
"""Convert the three Icon 2 SVG sources to Android launcher VectorDrawables."""

from html import escape
from pathlib import Path
import re
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1]
SOURCES = {
    "icon2": "icon2/DictShelf2.svg",
    "icon2_transparent": "icon2_transparent/DictShelf2_transparent.svg",
    "icon2_transparent_light": "icon2_transparent_light/DictShelf_transparent_light.svg",
}
OUT = ROOT / "android/app/src/main/res/drawable"


def group_attributes(transform):
    attributes = {}
    for name, raw in re.findall(r"([a-zA-Z]+)\(([^)]*)\)", transform):
        values = [float(v) for v in re.split(r"[\s,]+", raw.strip())]
        if name == "scale":
            attributes["scaleX"] = values[0]
            attributes["scaleY"] = values[-1]
        elif name == "translate":
            attributes["translateX"] = values[0]
            attributes["translateY"] = values[-1]
        else:
            raise ValueError(f"Unsupported SVG transform: {name}")
    return " ".join(f'android:{key}="{value:g}"' for key, value in attributes.items())


def convert_element(node, indent="    "):
    kind = node.tag.rsplit("}", 1)[-1]
    if kind == "g":
        attrs = group_attributes(node.get("transform", ""))
        lines = [f"{indent}<group {attrs}>" if attrs else f"{indent}<group>"]
        for child in node:
            lines.extend(convert_element(child, indent + "    "))
        return lines + [f"{indent}</group>"]
    if kind == "path":
        if set(node.attrib) - {"d", "fill"}:
            raise ValueError(f"Unsupported SVG path attributes: {node.attrib.keys()}")
        return [f'{indent}<path android:fillColor="{escape(node.attrib["fill"])}"',
                f'{indent}    android:pathData="{escape(node.attrib["d"], quote=True)}" />']
    raise ValueError(f"Unsupported SVG element: {kind}")


def main():
    for variant, relative in SOURCES.items():
        source = ROOT / "packaging/brand/source" / relative
        svg = ET.parse(source).getroot()
        x, y, width, height = [float(v) for v in svg.attrib["viewBox"].split()]
        if width != height:
            raise ValueError(f"Non-square SVG viewBox: {source}")
        # Android reserves the outer 21 dp of a 108 dp icon layer for masking.
        inset = 21 / 108 * width
        scale = 66 / 108
        lines = [
            '<?xml version="1.0" encoding="utf-8"?>',
            '<!-- Copyright (C) 2026 DmShAl (Shepeta Dmitry) -->',
            '<!-- SPDX-License-Identifier: GPL-3.0-or-later -->',
            f'<!-- Generated from packaging/brand/source/{relative} by tools/make-android-launcher-vectors.py. -->',
            '<vector xmlns:android="http://schemas.android.com/apk/res/android"',
            '    android:width="108dp" android:height="108dp"',
            f'    android:viewportWidth="{width:g}" android:viewportHeight="{height:g}">',
            f'    <group android:scaleX="{scale:g}" android:scaleY="{scale:g}"',
            f'        android:translateX="{inset:g}" android:translateY="{inset:g}">',
        ]
        if x or y:
            lines.append(f'        <group android:translateX="{-x:g}" android:translateY="{-y:g}">')
        for child in svg:
            lines.extend(convert_element(child, "            " if x or y else "        "))
        if x or y:
            lines.append("        </group>")
        lines += ["    </group>", "</vector>", ""]
        (OUT / f"ic_launcher_{variant}_vector.xml").write_text("\n".join(lines), encoding="utf-8")


if __name__ == "__main__":
    main()
