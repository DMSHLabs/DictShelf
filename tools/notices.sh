#!/bin/sh
# Regenerate THIRD-PARTY-NOTICES.md - the notices that must travel WITH the
# binaries, not merely with the source tree.
#
# Every third-party licence here (BSD-3, BSD-2, MIT, ISC, Apache-2.0) obliges us to
# reproduce its text "in the documentation or other materials provided with the
# distribution" of a BINARY. A release is a bare executable, so the file is
# embedded into it: `wudict licenses` prints exactly what this script wrote.
#
# The dependency set is the UNION over every shipped build configuration - the
# tray pulls different ones per OS - so a notice can never go missing because
# the release that needed it was built somewhere else.
set -eu
cd "$(dirname "$0")/.."

head=tools/notices.head.md
out=THIRD-PARTY-NOTICES.md
tmp=$(mktemp)
trap 'rm -f "$tmp" "$tmp.mods"' EXIT

: > "$tmp.mods"
# Every shipped configuration: the cgo driver (sqlite_fts5,sqlite_dbstat), which
# is the only one since the pure-Go driver (modernc.org/sqlite, `-tags purego`)
# was removed on 2026-11-03. The tag loop stays because it is the seam a second
# configuration would come back through.
for goos in darwin linux windows; do
  for tags in sqlite_fts5,sqlite_dbstat; do
    # CGO_ENABLED=1 is required, or `go list` drops the C-backed SQLite driver
    # from the listing and loses its notice.
    cgo=1
    CGO_ENABLED=$cgo GOOS=$goos GOARCH=amd64 go list -deps \
      -tags "$tags" \
      -f '{{if .Module}}{{.Module.Path}}	{{.Module.Version}}	{{.Module.Dir}}{{end}}' ./... \
      2>/dev/null >> "$tmp.mods" || true
  done
done

cat "$head" > "$tmp"

sort -u "$tmp.mods" | grep -v '^github.com/DMSHLabs/DictShelf	' | grep -v '^	' |
while IFS='	' read -r path version dir; do
  [ -n "$dir" ] || continue
  lic=""
  for c in LICENSE LICENSE.md LICENSE.txt LICENCE COPYING LICENSE-BSD LICENSE.BSD; do
    [ -f "$dir/$c" ] && { lic="$dir/$c"; break; }
  done
  printf '\n### %s %s\n\n' "$path" "$version" >> "$tmp"
  if [ -n "$lic" ]; then
    printf '```\n' >> "$tmp"
    sed 's/\r$//' "$lic" >> "$tmp"
    printf '```\n' >> "$tmp"
  else
    printf 'No licence file in the published module; see <https://%s>.\n' "$path" >> "$tmp"
  fi
done

mv "$tmp" "$out"
echo "wrote $out"
