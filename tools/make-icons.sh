#!/bin/sh
# Copyright (C) 2026 glowinthedark
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Render the tray icons and DictShelf brand assets from their source marks.
#
# Outputs are COMMITTED, so `make build` needs no image toolchain. Run
# `make icons` by hand when the mark changes.
#
#   internal/tray/icons/tray.png           32x32 colour  — Linux (DictShelf)
#   internal/tray/icons/tray-windows.png   32x32 colour  — Windows (DictShelf)
#   internal/tray/icons/tray-template.png  44x44 mono    — macOS (DictShelf @2x)
#
# …and the mark used by the Android app and its Play listing:
#
#   fastlane/metadata/android/en-US/images/icon.png            512x512
#   fastlane/metadata/android/en-US/images/featureGraphic.png  1024x500
#
# The icons live under internal/tray/ rather than packaging/ because go:embed
# cannot reach outside its own package directory.
#
# macOS template images are drawn by the system from their ALPHA channel alone;
# colour is discarded. So the template variant drops the rounded-rect tile (a
# solid tile would render as a solid black blob in the menu bar) and keeps only
# the ink, which is what a menu-bar glyph is supposed to be.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
brand="$root/packaging/brand/source/icon2_transparent/DictShelf2_transparent.svg"
out="$root/internal/tray/icons"

[ -f "$brand" ] || { echo "make-icons: missing $brand" >&2; exit 1; }

inkscape_bin=${INKSCAPE:-}
if [ -z "$inkscape_bin" ]; then
	if command -v inkscape >/dev/null 2>&1; then inkscape_bin=$(command -v inkscape)
	elif [ -x /c/Tools/inkscape/bin/inkscape.exe ]; then inkscape_bin=/c/Tools/inkscape/bin/inkscape.exe
	else echo "make-icons: set INKSCAPE to Inkscape executable" >&2; exit 1; fi
fi

mkdir -p "$out"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

render() { # width height source output
	if case "$inkscape_bin" in *.exe) true ;; *) false ;; esac; then
		source_path=$(cygpath -w "$3")
		output_path=$(cygpath -w "$4")
		"$inkscape_bin" "$source_path" --export-type=png --export-filename="$output_path" --export-width="$1" --export-height="$2" >/dev/null
	else
		"$inkscape_bin" "$3" --export-type=png --export-filename="$4" --export-width="$1" --export-height="$2" >/dev/null
	fi
}

run_python3() {
	if [ -n "${PYTHON3:-}" ]; then
		"$PYTHON3" "$@"
	elif command -v python3 >/dev/null 2>&1; then
		python3 "$@"
	elif command -v py.exe >/dev/null 2>&1; then
		py.exe -3 "$@"
	elif command -v py >/dev/null 2>&1; then
		py -3 "$@"
	else
		echo "make-icons: Python 3 is required to package .icns and .ico files" >&2
		exit 1
	fi
}

# ---- DictShelf icon family -------------------------------------------------
icon_sources() {
	case "$1" in
		icon1) echo 'icon1/DictShelf_small.svg|icon1/DictShelf.svg' ;;
		icon1_transparent) echo 'icon1_transparent/DictShelf-transparent_small.svg|icon1_transparent/DictShelf-transparent.svg' ;;
		icon1_transparent_light) echo 'icon1_transparent_light/DictShelf-transparent_light_small.svg|icon1_transparent_light/DictShelf-transparent_light.svg' ;;
		icon2) echo 'icon2/DictShelf2_small.svg|icon2/DictShelf2.svg' ;;
		icon2_transparent) echo 'icon2_transparent/DictShelf2_transparent_small.svg|icon2_transparent/DictShelf2_transparent.svg' ;;
		icon2_transparent_light) echo 'icon2_transparent_light/DictShelf-transparent_light_small.svg|icon2_transparent_light/DictShelf_transparent_light.svg' ;;
	esac
}
mkdir -p "$root/packaging/windows/icons" "$root/internal/server/web/icon-previews"
for variant in icon1 icon1_transparent icon1_transparent_light icon2 icon2_transparent icon2_transparent_light; do
	pair=$(icon_sources "$variant"); small=${pair%%|*}; large=${pair#*|}
	[ -f "$root/packaging/brand/source/$small" ] && [ -f "$root/packaging/brand/source/$large" ] || { echo "make-icons: missing SVG pair for $variant" >&2; exit 1; }
	render 32 32 "$root/packaging/brand/source/$small" "$out/tray-windows-$variant.png"
	cp "$out/tray-windows-$variant.png" "$root/internal/server/web/icon-previews/$variant.png"
	mkdir -p "$tmp/$variant"
	for size in 16 24 32; do render "$size" "$size" "$root/packaging/brand/source/$small" "$tmp/$variant/$size.png"; done
	for size in 48 96 128 256; do render "$size" "$size" "$root/packaging/brand/source/$large" "$tmp/$variant/$size.png"; done
	cp "$tmp/$variant/48.png" "$root/internal/server/web/icon-previews/$variant-48.png"
	python3 - "$tmp/$variant" "$root/packaging/windows/icons/$variant.ico" <<'PY'
import os, struct, sys
src, out = sys.argv[1:]
sizes = [16, 24, 32, 48, 96, 128, 256]
blobs = [open(os.path.join(src, f"{s}.png"), "rb").read() for s in sizes]
head = struct.pack("<HHH", 0, 1, len(sizes)); offset = len(head) + 16 * len(sizes); entries = b""
for s, blob in zip(sizes, blobs):
    entries += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(blob), offset)
    offset += len(blob)
with open(out, "wb") as f: f.write(head + entries + b"".join(blobs))
PY
done
cp "$root/packaging/windows/icons/icon2_transparent.ico" "$root/packaging/windows/dictshelf.ico"
cp "$root/internal/server/web/icon-previews/icon2_transparent.png" "$out/tray.png"
cp "$root/internal/server/web/icon-previews/icon2_transparent.png" "$out/tray-windows.png"
sed -E 's/fill="#[[:xdigit:]]{6}"/fill="#000000"/g' "$root/packaging/brand/source/icon2_transparent/DictShelf2_transparent_small.svg" > "$tmp/template.svg"
render 44 44 "$tmp/template.svg" "$out/tray-template.png"
for f in tray.png tray-windows.png tray-windows-icon1.png tray-windows-icon1_transparent.png tray-windows-icon1_transparent_light.png tray-windows-icon2.png tray-windows-icon2_transparent.png tray-windows-icon2_transparent_light.png tray-template.png; do
	[ -s "$out/$f" ] || { echo "make-icons: $f is empty" >&2; exit 1; }
	printf '%s  ' "$f"
	ls -l "$out/$f" | awk '{print $5 " bytes"}'
done

play="$root/fastlane/metadata/android/en-US/images"
mkdir -p "$play"

render 512 512 "$brand" "$play/icon.png"

run_python3 - "$brand" "$tmp/feature.svg" <<'PY'
import copy, sys
import xml.etree.ElementTree as ET
source, out = sys.argv[1:]
ns = "http://www.w3.org/2000/svg"
ET.register_namespace("", ns)
original = ET.parse(source).getroot()
feature = ET.Element("{%s}svg" % ns, {
    "width": "1024", "height": "500", "viewBox": "0 0 1024 500"})
ET.SubElement(feature, "{%s}rect" % ns, {
    "width": "1024", "height": "500", "fill": "#FFF5E0"})
icon = ET.SubElement(feature, "{%s}svg" % ns, {
    "x": "312", "y": "50", "width": "400", "height": "400",
    "viewBox": original.get("viewBox", "0 0 1024 1024")})
for child in original:
    if child.tag != "{%s}metadata" % ns:
        icon.append(copy.deepcopy(child))
ET.ElementTree(feature).write(out, encoding="utf-8", xml_declaration=True)
PY
render 1024 500 "$tmp/feature.svg" "$play/featureGraphic.png"

for f in icon.png featureGraphic.png; do
	[ -s "$play/$f" ] || { echo "make-icons: $f is empty" >&2; exit 1; }
	printf '%s  ' "$f"
	ls -l "$play/$f" | awk '{print $5 " bytes"}'
done

# ---- the macOS app icon (P85) -------------------------------------------
# Same DictShelf mark, same rule: rendered, never redrawn. Full-bleed, keeping the tile's
# own rx=7/32 corner — Apple's squircle-with-margins grid would mean moving
# coordinates, which D70 forbids, and the tile already reads as an app icon at
# every size.
#
# Prefer Apple's iconutil where available. Else pack the same PNG iconset into
# the standard ICNS container; this lets Windows/Linux maintainers regenerate
# the committed app icon from the shared mark too.
icns="$root/packaging/darwin/dictshelf.icns"
set -- 16 icon_16x16 32 icon_16x16@2x 32 icon_32x32 64 icon_32x32@2x \
       128 icon_128x128 256 icon_128x128@2x 256 icon_256x256 512 icon_256x256@2x \
       512 icon_512x512 1024 icon_512x512@2x
mkdir -p "$tmp/dictshelf.iconset" "$(dirname "$icns")"
while [ $# -gt 0 ]; do
	render "$1" "$1" "$brand" "$tmp/dictshelf.iconset/$2.png"
	shift 2
done

if command -v iconutil >/dev/null 2>&1; then
	iconutil -c icns -o "$icns" "$tmp/dictshelf.iconset"
else
	run_python3 - "$tmp/dictshelf.iconset" "$icns" <<'PY'
import os, struct, sys
src, out = sys.argv[1:]
images = [("icon_16x16.png", b"icp4"), ("icon_16x16@2x.png", b"icp5"),
          ("icon_32x32@2x.png", b"icp6"), ("icon_128x128.png", b"ic07"),
          ("icon_256x256.png", b"ic08"), ("icon_512x512.png", b"ic09"),
          ("icon_512x512@2x.png", b"ic10")]
chunks = []
for name, kind in images:
    data = open(os.path.join(src, name), "rb").read()
    chunks.append(kind + struct.pack(">I", len(data) + 8) + data)
body = b"".join(chunks)
with open(out, "wb") as f:
    f.write(b"icns" + struct.pack(">I", len(body) + 8) + body)
PY
fi
[ -s "$icns" ] || { echo "make-icons: dictshelf.icns is empty" >&2; exit 1; }
printf 'dictshelf.icns  '
ls -l "$icns" | awk '{print $5 " bytes"}'

# ---- the Windows app icon (P86) -----------------------------------------
# Use the supplied multi-size ICO to preserve its hand-tuned pixel edges. It is
# used by the installer and embedded into the Windows executable by the resource file.
ico="$root/packaging/windows/dictshelf.ico"
mkdir -p "$(dirname "$ico")"
cp "$root/packaging/windows/icons/icon2_transparent.ico" "$ico"
[ -s "$ico" ] || { echo "make-icons: dictshelf.ico is empty" >&2; exit 1; }
printf 'dictshelf.ico   '
ls -l "$ico" | awk '{print $5 " bytes"}'

# Keep the app/window PE icon in sync with the ICO. The resource compiler is
# only available on Windows; the committed, OS-filtered .syso is the build
# input on every Windows build and is also what Android builds safely ignore.
windres_bin=${WINDRES:-}
if [ -z "$windres_bin" ] && command -v windres >/dev/null 2>&1; then
	windres_bin=$(command -v windres)
fi
gcc_path=${GCC_PATH:-}
if [ -z "$gcc_path" ] && [ -f "$root/build-windows.local.bat" ]; then
	gcc_path=$(sed -n 's/^[[:space:]]*[Ss][Ee][Tt][[:space:]]*"GCC_PATH=\([^"]*\)".*/\1/p' "$root/build-windows.local.bat" | head -n 1)
	if [ -n "$gcc_path" ] && command -v cygpath >/dev/null 2>&1; then
		gcc_path=$(cygpath -u "$gcc_path")
	fi
fi
if [ -z "$windres_bin" ] && [ -n "$gcc_path" ]; then
	compiler_dir=$(dirname -- "$gcc_path")
	for candidate in "$compiler_dir/windres.exe" "$compiler_dir/windres"; do
		[ -x "$candidate" ] && { windres_bin=$candidate; break; }
	done
fi
if [ -n "$windres_bin" ]; then
	(cd "$root" && "$windres_bin" -F pe-x86-64 -i packaging/windows/dictshelf.rc -o dictshelf_windows_amd64.syso)
	echo "dictshelf_windows_amd64.syso regenerated"
else
	echo "make-icons: windres not found — run the resource command in packaging/windows/dictshelf.rc to refresh the Windows executable icon" >&2
fi
