#!/bin/sh
# Copyright (C) 2026 glowinthedark
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Render the tray icons — and this fork's own launcher mark — from the one
# source mark (D70: the mark is generated from
# internal/server/web/favicon.svg, never redrawn — it already exists in five
# unlinked places and this must not make a sixth; the digit added below is the
# fork's one deliberate exception, and it is derived from that mark, not drawn
# beside it).
#
# Outputs are COMMITTED, so `make build` needs no image toolchain. Run
# `make icons` by hand when the mark changes.
#
#   internal/tray/icons/tray.png           32x32 colour  — Linux (upstream mark)
#   internal/tray/icons/tray-windows.png   32x32 colour  — Windows (wuDict2)
#   internal/tray/icons/tray-template.png  44x44 mono    — macOS (wuDict2 @2x)
#
# …and the mark the Android app and its Play listing wear, which is the same
# mark with this fork's digit in it:
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
src="$root/internal/server/web/favicon.svg"
out="$root/internal/tray/icons"

[ -f "$src" ] || { echo "make-icons: missing $src" >&2; exit 1; }

mkdir -p "$out"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

magick_bin=${MAGICK:-}
if [ -z "$magick_bin" ]; then
	if command -v magick >/dev/null 2>&1; then
		magick_bin=$(command -v magick)
	elif command -v magick.exe >/dev/null 2>&1; then
		magick_bin=$(command -v magick.exe)
	fi
fi

render() { # width height source output
	if command -v rsvg-convert >/dev/null 2>&1; then
		rsvg-convert -w "$1" -h "$2" -o "$4" "$3"
	else
		[ -n "$magick_bin" ] || {
			echo "make-icons: install librsvg (rsvg-convert) or ImageMagick (magick)" >&2
			exit 1
		}
		if case "$magick_bin" in *.exe) true ;; *) false ;; esac && command -v cygpath >/dev/null 2>&1; then
			source_path=$(cygpath -w "$3")
			output_path=$(cygpath -w "$4")
			"$magick_bin" -background none "$source_path" -resize "${1}x${2}!" "PNG32:$output_path"
		else
			"$magick_bin" -background none "$3" -resize "${1}x${2}!" "PNG32:$4"
		fi
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

# ---- this fork's own launcher mark (wuDict2) ------------------------------
# wuDict2 installs BESIDE the upstream app, so its icon has to say which of the
# two it is: the same mark with a "2" in the free bottom-left corner, next to
# the lens. Both outputs below are derived from the source by substitution,
# exactly as the template variant above is, so the mark's three geometry rules
# survive untouched and an upstream change to the mark reaches them without a
# second copy of it existing anywhere.
#
# The digit, in the mark's own 32-unit space:
#
#   M4 20 A3 3 0 1 1 10 20 L4 25 H11
#
# one semicircle (r=3, its diameter on the y=20 line), one diagonal, one foot,
# at the text lines' stroke width (2), round caps and joins. Its ink is x[3,12]
# y[16,26]: the left edge is the text lines' own margin (3), the foot sits on
# odd y=25 so at 16px its edges are whole pixels (rule 3), and it stays inside
# the ink box the mark already occupies — x[3,29] y[6,27] — which leaves rule 1
# (centred ink) and the adaptive icon's safe-zone margin untouched. Clearances:
# 2 units to the second text line above, 2 to the lens on the right.
#
# android/app/src/main/res/drawable/ic_launcher_foreground.xml carries the same
# path by hand, and THAT is the icon the phone shows — a launcher icon is only
# ever a VectorDrawable, so it cannot be rendered from here. What is rendered
# here is the Play listing's pair, on canvases and at mark scales taken from the
# images they replace (the glyph box is 2/3 of the 512 icon and 300px wide in
# the 1024-wide feature graphic, both centred, both on the tile colour
# full-bleed — the store applies the only corner either of them has).
#
# The tile colour and the mark are read out of the source rather than repeated
# here; the digit is the one thing written down twice in this repository, here
# and in that VectorDrawable.
#
# On a machine without rsvg-convert, ImageMagick's librsvg delegate renders
# these two identically (`magick -background none x.svg -alpha off PNG24:x.png`)
# — that is how the committed pair was rendered.
tile=$(sed -n 's/.*<rect[^>]*fill="\(#[0-9a-fA-F]*\)".*/\1/p' "$src")
[ -n "$tile" ] || { echo "make-icons: no tile colour in $src" >&2; exit 1; }
digit='M4 20A3 3 0 1 1 10 20L4 25H11'
fork="$tmp/wudict2.svg"
sed -e "s|</g>|<path d=\"$digit\" stroke-width=\"2\" stroke-linejoin=\"round\"/></g>|" "$src" > "$fork"
mark=$(sed -e '/<rect /d' "$fork")

# Windows and macOS are fork-specific products. Keep Linux's shared image
# untouched; render the digit-bearing mark into the platform assets instead.
sed -e '/<rect /d' -e 's/stroke="#fff"/stroke="#000"/' "$fork" > "$tmp/template.svg"
render 32 32 "$src" "$out/tray.png"
render 32 32 "$fork" "$out/tray-windows.png"
render 44 44 "$tmp/template.svg" "$out/tray-template.png"
for f in tray.png tray-windows.png tray-template.png; do
	[ -s "$out/$f" ] || { echo "make-icons: $f is empty" >&2; exit 1; }
	printf '%s  ' "$f"
	ls -l "$out/$f" | awk '{print $5 " bytes"}'
done

play="$root/fastlane/metadata/android/en-US/images"
mkdir -p "$play"

render_mark() { # width height scale tx ty out
	cat > "$tmp/play.svg" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 $1 $2" width="$1" height="$2">
  <rect width="$1" height="$2" fill="$tile"/>
  <g transform="translate($4,$5) scale($3)">
$mark
  </g>
</svg>
EOF
	render "$1" "$2" "$tmp/play.svg" "$6"
}

render_mark 512  512 10.66667 85.3333 85.3333 "$play/icon.png"
render_mark 1024 500 9.375    362     100    "$play/featureGraphic.png"

for f in icon.png featureGraphic.png; do
	[ -s "$play/$f" ] || { echo "make-icons: $f is empty" >&2; exit 1; }
	printf '%s  ' "$f"
	ls -l "$play/$f" | awk '{print $5 " bytes"}'
done

# ---- the macOS app icon (P85) -------------------------------------------
# Same wuDict2 mark, same rule: rendered, never redrawn. Full-bleed, keeping the tile's
# own rx=7/32 corner — Apple's squircle-with-margins grid would mean moving
# coordinates, which D70 forbids, and the tile already reads as an app icon at
# every size.
#
# Prefer Apple's iconutil where available. Else pack the same PNG iconset into
# the standard ICNS container; this lets Windows/Linux maintainers regenerate
# the committed app icon from the shared mark too.
icns="$root/packaging/darwin/wudict.icns"
set -- 16 icon_16x16 32 icon_16x16@2x 32 icon_32x32 64 icon_32x32@2x \
       128 icon_128x128 256 icon_128x128@2x 256 icon_256x256 512 icon_256x256@2x \
       512 icon_512x512 1024 icon_512x512@2x
mkdir -p "$tmp/wudict.iconset" "$(dirname "$icns")"
while [ $# -gt 0 ]; do
	render "$1" "$1" "$fork" "$tmp/wudict.iconset/$2.png"
	shift 2
done

if command -v iconutil >/dev/null 2>&1; then
	iconutil -c icns -o "$icns" "$tmp/wudict.iconset"
else
	run_python3 - "$tmp/wudict.iconset" "$icns" <<'PY'
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
[ -s "$icns" ] || { echo "make-icons: wudict.icns is empty" >&2; exit 1; }
printf 'wudict.icns  '
ls -l "$icns" | awk '{print $5 " bytes"}'

# ---- the Windows app icon (P86) -----------------------------------------
# Same wuDict2 mark, same rule. An .ico is a directory of images; since Vista each
# entry may be a PNG verbatim, so the container is a 6-byte header plus one
# 16-byte record per size and needs no image library to assemble — python3
# (already required by nothing else here, but present on every dev machine
# that has the Xcode tools) packs it in a dozen lines.
#
# The installer uses this for Setup, shortcuts and dictionary files. The same
# ICO is embedded into the Windows/amd64 executable by packaging/windows/wudict.rc.
ico="$root/packaging/windows/wudict.ico"

mkdir -p "$tmp/ico" "$(dirname "$ico")"
for s in 16 32 48 64 128 256; do
	render "$s" "$s" "$fork" "$tmp/ico/$s.png"
done

run_python3 - "$tmp/ico" "$ico" <<'PY'
import struct, sys, os
srcdir, out = sys.argv[1], sys.argv[2]
sizes = [16, 32, 48, 64, 128, 256]
blobs = [open(os.path.join(srcdir, "%d.png" % s), "rb").read() for s in sizes]
# ICONDIR: reserved, type=1 (icon), count. Then one 16-byte ICONDIRENTRY each.
head = struct.pack("<HHH", 0, 1, len(sizes))
offset = len(head) + 16 * len(sizes)
entries = b""
for s, b in zip(sizes, blobs):
    # 256 is stored as 0: the field is one byte wide.
    entries += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(b), offset)
    offset += len(b)
with open(out, "wb") as f:
    f.write(head + entries + b"".join(blobs))
PY
[ -s "$ico" ] || { echo "make-icons: wudict.ico is empty" >&2; exit 1; }
printf 'wudict.ico   '
ls -l "$ico" | awk '{print $5 " bytes"}'

# Keep the app/window PE icon in sync with the ICO. The resource compiler is
# only available on Windows; the committed, OS-filtered .syso is the build
# input on every Windows build and is also what Android builds safely ignore.
windres_bin=${WINDRES:-}
if [ -z "$windres_bin" ] && command -v windres >/dev/null 2>&1; then
	windres_bin=$(command -v windres)
fi
if [ -z "$windres_bin" ] && [ -n "${GCC_PATH:-}" ]; then
	compiler_dir=$(dirname -- "$GCC_PATH")
	for candidate in "$compiler_dir/windres.exe" "$compiler_dir/windres"; do
		[ -x "$candidate" ] && { windres_bin=$candidate; break; }
	done
fi
if [ -n "$windres_bin" ]; then
	(cd "$root" && "$windres_bin" -F pe-x86-64 -i packaging/windows/wudict.rc -o wudict_windows_amd64.syso)
	echo "wudict_windows_amd64.syso regenerated"
else
	echo "make-icons: windres not found — run the resource command in packaging/windows/wudict.rc to refresh the Windows executable icon" >&2
fi
