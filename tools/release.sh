#!/bin/sh
# Copyright (C) 2026 glowinthedark
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Cut a release: `make release VERSION=v3.7.9` (or v3.7.9-beta.2, -alpha.N,
# -rc.N). The tag stays the release; this writes what Android stores read from
# the source at that tag (F-Droid reads android/app/build.gradle, not git
# history) and points the tag at the commit that holds it:
#
#   1. versionName and versionCode in android/app/build.gradle, as literals;
#   2. fastlane/metadata/android/en-US/changelogs/<versionCode>.txt from the
#      version's section of changelog.txt (a pre-release uses its release's);
#   3. one commit, "chore(release): <tag>", and the tag on it.
#
# versionCode follows from the version, so it is known before any build and
# does not move when history is rewritten:
#   major*1000000 + minor*10000 + patch*100 + rank
#   rank: alpha.N = N, beta.N = 20+N, rc.N = 40+N (N 1-19), release = 90
#   v3.7.9 -> 3070990, v3.7.9-beta.2 -> 3070922
#
# DRY_RUN=1 prints what it would write and changes nothing. `release.sh --code
# <tag>` only prints the tag's versionCode: build-android.yml gives it to a
# pre-release it tags from workflow_dispatch.
set -eu

only_code=
if [ "${1:-}" = --code ]; then
	only_code=1
	shift
fi
tag=${1:?usage: release.sh [--code] vMAJOR.MINOR.PATCH[-alpha.N|-beta.N|-rc.N]}
gradle=android/app/build.gradle
log=changelog.txt
changes=fastlane/metadata/android/en-US/changelogs

die() { echo "release: $*" >&2; exit 1; }

case $tag in v*) ;; *) die "$tag: a release tag starts with v" ;; esac
ver=${tag#v}
core=${ver%%-*}
pre=
[ "$core" = "$ver" ] || pre=${ver#*-}
echo "$core" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || die "$tag: want vMAJOR.MINOR.PATCH"
IFS=. read -r major minor patch <<EOF
$core
EOF
[ "$minor" -le 99 ] && [ "$patch" -le 99 ] || die "$tag: minor and patch go up to 99"
case $pre in
'') rank=90 ;;
alpha.* | beta.* | rc.*)
	n=${pre#*.}
	echo "$n" | grep -Eq '^[0-9]+$' && [ "$n" -ge 1 ] && [ "$n" -le 19 ] || die "$tag: pre-release number 1-19"
	case $pre in alpha.*) rank=$n ;; beta.*) rank=$((20 + n)) ;; rc.*) rank=$((40 + n)) ;; esac
	;;
*) die "$tag: pre-release is alpha.N, beta.N or rc.N" ;;
esac
code=$((major * 1000000 + minor * 10000 + patch * 100 + rank))
if [ -n "$only_code" ]; then
	echo "$code"
	exit 0
fi

cur=$(sed -n 's/^ *versionCode \([0-9][0-9]*\)$/\1/p' "$gradle")
[ -n "$cur" ] || die "$gradle: no literal versionCode line"
[ "$code" -gt "$cur" ] || [ "${FORCE:-}" = 1 ] || die "$tag: versionCode $code is not above $cur (FORCE=1 to rewrite the same one)"

# The release notes: changelog.txt's section for the release, cut to the 500
# bytes F-Droid shows, at a line end.
notes=$(awk -v h="v$core" '
	$0 == h { on = 1; next }
	on && /^v[0-9]/ { exit }
	on { print }' "$log" | sed -e '/^[[:space:]]*$/d')
[ -n "$notes" ] || die "$log: no section v$core"
# A file already written for this code is kept: it is the notes someone
# wrote by hand, which the cut below would replace with a fragment.
full=$(printf '%s\n' "$notes" | LC_ALL=C wc -c | tr -d ' ')
notes=$(printf '%s\n' "$notes" | LC_ALL=C awk '{ if (n + length($0) + 1 > 500) exit; n += length($0) + 1; print }')
keep=
[ -s "$changes/$code.txt" ] && keep=1

if [ "${DRY_RUN:-}" = 1 ]; then
	echo "tag $tag  versionName $ver  versionCode $code (was $cur)"
	if [ -n "$keep" ]; then
		echo "--- $changes/$code.txt (kept as written)"
		cat "$changes/$code.txt"
	else
		echo "--- $changes/$code.txt"
		printf '%s\n' "$notes"
		[ "$full" -le 500 ] || echo "(cut from $full bytes: write a summary in that file before releasing)"
	fi
	exit 0
fi

git diff --quiet && git diff --cached --quiet || die "commit or stash your changes first: the tag must hold exactly what is committed"
git rev-parse -q --verify "refs/tags/$tag" >/dev/null && die "$tag already exists"

sed -i.bak -e "s/^\( *versionCode \)[0-9][0-9]*$/\1$code/" -e "s/^\( *versionName \)\"[^\"]*\"$/\1\"$ver\"/" "$gradle"
rm -f "$gradle.bak"
grep -q "^ *versionCode $code$" "$gradle" && grep -q "^ *versionName \"$ver\"$" "$gradle" || die "$gradle: the version lines were not rewritten"
mkdir -p "$changes"
if [ -n "$keep" ]; then
	echo "release: keeping $changes/$code.txt as written"
else
	printf '%s\n' "$notes" >"$changes/$code.txt"
	[ "$full" -le 500 ] || echo "release: warning: v$core's notes are $full bytes; stores show 500, so $changes/$code.txt holds the first lines only - write a summary there (FORCE=1 make release reruns keep it)" >&2
fi

git add "$gradle" "$changes/$code.txt"
git commit -q -m "chore(release): $tag"
git tag "$tag"
echo "$tag: versionCode $code; committed and tagged. Push with: git push && git push origin $tag"
