#!/bin/sh
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later

# Publish only the generated JSON to a dedicated metadata branch. This branch
# is replaced as one snapshot and never contributes commits to release builds.
set -eu

CATALOG=${1:-update-catalog.json}
if [ ! -f "$CATALOG" ]; then
    echo "publish-update-catalog: missing catalogue: $CATALOG" >&2
    exit 1
fi
if [ -z "${GH_TOKEN:-}" ]; then
    echo "publish-update-catalog: GH_TOKEN is required" >&2
    exit 1
fi

OUT="${RUNNER_TEMP:-${TMPDIR:-/tmp}}/dictshelf-update-catalog"
TEMP_ROOT="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
OUT_PARENT="$(cd "$(dirname "$OUT")" && pwd)"
TEMP_ROOT="$(cd "$TEMP_ROOT" && pwd)"
if [ "$OUT_PARENT" != "$TEMP_ROOT" ]; then
    echo "publish-update-catalog: unsafe temporary path" >&2
    exit 1
fi
rm -rf "$OUT"
mkdir -p "$OUT"
git -C "$OUT" init -q
cp "$CATALOG" "$OUT/update-catalog.json"
git -C "$OUT" config user.name "github-actions[bot]"
git -C "$OUT" config user.email "github-actions[bot]@users.noreply.github.com"
git -C "$OUT" add update-catalog.json
git -C "$OUT" commit -q -m "Publish update catalogue"
git -C "$OUT" remote add origin https://github.com/DMSHLabs/DictShelf.git
AUTH="$(printf 'x-access-token:%s' "$GH_TOKEN" | base64 | tr -d '\n')"
git -C "$OUT" -c "http.https://github.com/.extraheader=AUTHORIZATION: basic $AUTH" \
    push --force origin HEAD:update-catalog
