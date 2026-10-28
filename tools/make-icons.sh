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
brand="$root/packaging/brand/dictshelf.svg"
out="$root/internal/tray/icons"

[ -f "$brand" ] || { echo "make-icons: missing $brand" >&2; exit 1; }

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
			if case "$3" in *.svg) true ;; *) false ;; esac; then
				svg_width=$(sed -n 's/.*<svg[^>]*width="\([0-9][0-9]*\)".*/\1/p' "$3" | head -n 1)
				[ -n "$svg_width" ] || { echo "make-icons: cannot read SVG width from $3" >&2; exit 1; }
				target=$1
				[ "$2" -le "$target" ] || target=$2
				density=$(( (72 * target + svg_width - 1) / svg_width ))
				"$magick_bin" -background none -density "$density" "$source_path" -resize "${1}x${2}!" "PNG32:$output_path"
			else
				"$magick_bin" -background none "$source_path" -resize "${1}x${2}!" "PNG32:$output_path"
			fi
		else
			if case "$3" in *.svg) true ;; *) false ;; esac; then
				svg_width=$(sed -n 's/.*<svg[^>]*width="\([0-9][0-9]*\)".*/\1/p' "$3" | head -n 1)
				[ -n "$svg_width" ] || { echo "make-icons: cannot read SVG width from $3" >&2; exit 1; }
				target=$1
				[ "$2" -le "$target" ] || target=$2
				density=$(( (72 * target + svg_width - 1) / svg_width ))
				"$magick_bin" -background none -density "$density" "$3" -resize "${1}x${2}!" "PNG32:$4"
			else
				"$magick_bin" -background none "$3" -resize "${1}x${2}!" "PNG32:$4"
			fi
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

# ---- DictShelf brand mark --------------------------------------------------
# This source is the product mark for Android, Windows, macOS, Linux and the
# Play listing. Keep the upstream browser favicon independent.
tile=$(sed -n 's/.*<rect[^>]*fill="\(#[0-9a-fA-F]*\)".*/\1/p' "$brand")
[ -n "$tile" ] || { echo "make-icons: no tile colour in $brand" >&2; exit 1; }
fork="$brand"
mark=$(sed -e '/<rect /d' "$brand")

# Render the shared Linux mark and the platform-specific desktop variants.
sed -e '/<rect /d' -e 's/#fff4df/#000000/g' -e 's/#f2a93b/#000000/g' "$fork" > "$tmp/template.svg"
render 32 32 "$fork" "$out/tray.png"
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
	render "$1" "$1" "$fork" "$tmp/dictshelf.iconset/$2.png"
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
# Same DictShelf mark, same rule. Put 48/32/16px paletted DIBs first, as in
# well-supported Windows icons, then 256px PNG and the remaining 32bpp DIBs
# from largest to smallest. Some file managers choose the first suitable frame.
#
# The installer uses this for Setup, shortcuts and dictionary files. The same
# ICO is embedded into the Windows/amd64 executable by packaging/windows/dictshelf.rc.
ico="$root/packaging/windows/dictshelf.ico"

mkdir -p "$tmp/ico" "$(dirname "$ico")"
# Include common Windows shell and DPI sizes; the 256-pixel image remains the
# high-resolution Vista+ entry. Render each size from SVG, never from a bitmap.
for s in 16 20 24 32 40 48 64 96 128 256; do
	render "$s" "$s" "$fork" "$tmp/ico/$s.png"
done

run_python3 - "$tmp/ico" "$ico" <<'PY'
import os, struct, sys, zlib
srcdir, out = sys.argv[1], sys.argv[2]
sizes = [16, 20, 24, 32, 40, 48, 64, 96, 128]

def read_png(path):
    data = open(path, "rb").read()
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise ValueError("not a PNG: " + path)
    pos, compressed = 8, bytearray()
    while pos < len(data):
        length = struct.unpack_from(">I", data, pos)[0]
        kind = data[pos + 4:pos + 8]
        chunk = data[pos + 8:pos + 8 + length]
        pos += 12 + length
        if kind == b"IHDR":
            width, height, depth, color, _, _, interlace = struct.unpack(">IIBBBBB", chunk)
        elif kind == b"IDAT":
            compressed.extend(chunk)
        elif kind == b"IEND":
            break
    if depth != 8 or color not in (2, 6) or interlace != 0:
        raise ValueError("expected non-interlaced 8-bit RGB/RGBA PNG: " + path)
    bpp = 4 if color == 6 else 3
    raw, stride = zlib.decompress(compressed), width * bpp
    rows, previous, at = [], bytearray(stride), 0
    for _ in range(height):
        method = raw[at]
        row = bytearray(raw[at + 1:at + 1 + stride])
        at += stride + 1
        for i in range(stride):
            left = row[i - bpp] if i >= bpp else 0
            up = previous[i]
            upper_left = previous[i - bpp] if i >= bpp else 0
            if method == 0:
                predictor = 0
            elif method == 1:
                predictor = left
            elif method == 2:
                predictor = up
            elif method == 3:
                predictor = (left + up) // 2
            elif method == 4:
                p = left + up - upper_left
                distances = (abs(p - left), abs(p - up), abs(p - upper_left))
                predictor = (left, up, upper_left)[distances.index(min(distances))]
            else:
                raise ValueError("unknown PNG filter %d" % method)
            row[i] = (row[i] + predictor) & 255
        rows.append(bytes(row))
        previous = row
    if color == 2:
        rows = [b"".join(row[i:i + 3] + b"\xff" for i in range(0, stride, 3)) for row in rows]
    return width, height, rows

def rgba_image(size):
    width, height, rows = read_png(os.path.join(srcdir, "%d.png" % size))
    if width != size or height != size:
        raise ValueError("unexpected rendered size %dx%d for %dpx icon" % (width, height, size))
    # The source SVG is clean geometry, but ordinary rasterization introduces
    # partially covered edge pixels. ICO layers are pixel artwork: snap alpha
    # to the transparency mask and every visible pixel to the brand palette.
    brand = ((255, 244, 223), (7, 28, 51), (242, 169, 59))
    crisp = []
    for row in rows:
        out_row = bytearray()
        for i in range(0, len(row), 4):
            pixel = row[i:i + 4]
            if pixel[3] < 128:
                out_row.extend((0, 0, 0, 0))
                continue
            rgb = pixel[:3]
            color = min(brand, key=lambda c: sum((a - b) * (a - b) for a, b in zip(rgb, c)))
            out_row.extend((*color, 255))
        crisp.append(bytes(out_row))
    return width, height, crisp

def png_chunk(kind, payload):
    return (struct.pack(">I", len(payload)) + kind + payload +
            struct.pack(">I", zlib.crc32(kind + payload) & 0xffffffff))

def crisp_png(size):
    width, height, rows = rgba_image(size)
    raw = b"".join(b"\0" + row for row in rows)
    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    return (b"\x89PNG\r\n\x1a\n" + png_chunk(b"IHDR", ihdr) +
            png_chunk(b"IDAT", zlib.compress(raw, 9)) + png_chunk(b"IEND", b""))

def and_mask(width, height, rows):
    mask_stride = ((width + 31) // 32) * 4
    mask = bytearray(mask_stride * height)
    for y in range(height):
        row = rows[y]
        mask_y = height - 1 - y
        for x in range(width):
            if row[x * 4 + 3] < 128:
                mask[mask_y * mask_stride + x // 8] |= 0x80 >> (x % 8)
    return bytes(mask)

def dib_frame(size):
    width, height, rows = rgba_image(size)
    pixels = bytearray()
    for y in range(height - 1, -1, -1):
        row = rows[y]
        for x in range(width):
            r, g, b, a = row[x * 4:x * 4 + 4]
            pixels.extend((b, g, r, a))
    mask = and_mask(width, height, rows)
    header = struct.pack("<IiiHHIIiiII", 40, width, height * 2, 1, 32,
                         0, len(pixels), 0, 0, 0, 0)
    return 0, header + pixels + mask

def indexed_dib_frame(size):
    width, height, rows = rgba_image(size)
    # Classic 8bpp DIBs have a binary transparency mask. Composite partial
    # coverage against the light toolbar colour used by Windows file managers.
    background = (240, 240, 240)
    counts = {}
    for row in rows:
        for i in range(0, len(row), 4):
            r, g, b, a = row[i:i + 4]
            if a < 128:
                continue
            color = tuple((c * a + bg * (255 - a) + 127) // 255
                          for c, bg in zip((r, g, b), background))
            counts[color] = counts.get(color, 0) + 1

    boxes = [list(counts)]
    while len(boxes) < 256:
        candidates = [i for i, box in enumerate(boxes) if len(box) > 1]
        if not candidates:
            break
        def split_score(i):
            box = boxes[i]
            spans = [max(c[k] for c in box) - min(c[k] for c in box) for k in range(3)]
            return max(spans) * sum(counts[c] for c in box)
        index = max(candidates, key=split_score)
        box = boxes.pop(index)
        spans = [max(c[k] for c in box) - min(c[k] for c in box) for k in range(3)]
        channel = spans.index(max(spans))
        ordered = sorted(box, key=lambda c: c[channel])
        halfway, accumulated, cut = (sum(counts[c] for c in ordered) + 1) // 2, 0, 1
        for j, color in enumerate(ordered[:-1], 1):
            accumulated += counts[color]
            if accumulated >= halfway:
                cut = j
                break
        boxes[index:index] = [ordered[:cut], ordered[cut:]]

    palette, color_index = [], {}
    for box in boxes:
        total = sum(counts[c] for c in box)
        color = tuple((sum(c[k] * counts[c] for c in box) + total // 2) // total
                      for k in range(3))
        color_index.update((c, len(palette)) for c in box)
        palette.append(color)

    stride = (width + 3) & ~3
    pixels = bytearray()
    for y in range(height - 1, -1, -1):
        row = rows[y]
        encoded = bytearray()
        for x in range(width):
            r, g, b, a = row[x * 4:x * 4 + 4]
            if a < 128:
                encoded.append(0)
            else:
                color = tuple((c * a + bg * (255 - a) + 127) // 255
                              for c, bg in zip((r, g, b), background))
                encoded.append(color_index[color])
        pixels.extend(encoded)
        pixels.extend(b"\0" * (stride - width))

    table = b"".join(struct.pack("<BBBB", b, g, r, 0) for r, g, b in palette)
    mask = and_mask(width, height, rows)
    header = struct.pack("<IiiHHIIiiII", 40, width, height * 2, 1, 8,
                         0, len(pixels), 0, 0, len(palette), 0)
    return (0 if len(palette) == 256 else len(palette)), header + table + pixels + mask

# Match the ordering and legacy palette entries used by common Windows icons:
# file managers that take the first/best early frame get a native-size image.
frames = []
for size in (48, 32, 16):
    colors, blob = indexed_dib_frame(size)
    frames.append(((size, size), 8, colors, blob))
# Keep PNG encoding for 256px, with the same pixel-sharp palette as DIB layers.
frames.append(((256, 256), 32, 0, crisp_png(256)))
for size in reversed(sizes):
    colors, blob = dib_frame(size)
    frames.append(((size, size), 32, colors, blob))
head = struct.pack("<HHH", 0, 1, len(frames))
offset = len(head) + 16 * len(frames)
entries, blobs = [], []
for (width, height), bits, colors, blob in frames:
    entries.append(struct.pack("<BBBBHHII", width % 256, height % 256,
                               colors if colors < 256 else 0, 0, 1, bits,
                               len(blob), offset))
    blobs.append(blob)
    offset += len(blob)
with open(out, "wb") as f:
    f.write(head + b"".join(entries) + b"".join(blobs))
PY
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
if [ -z "$windres_bin" ] && [ -n "${GCC_PATH:-}" ]; then
	compiler_dir=$(dirname -- "$GCC_PATH")
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
