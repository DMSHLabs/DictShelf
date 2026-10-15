#!/bin/sh
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# The tag a publishing run should use when the dispatch form's `tag` field was
# left empty (D168). Two rules:
#
#   pre-release   the last version's patch + 1     wudict2-v0.11.1 → wudict2-v0.11.2
#   release       the last version's minor + 1     wudict2-v0.11.2 → wudict2-v0.12.0
#                 (the patch resets to 0)
#
# and one exception that keeps ONE release out of several runs. Android,
# Windows and the native trio each publish into the same tag, and the ones
# that follow the first on the same commit add their assets to its release
# instead of walking the number forward under it. So a run reuses the last
# version tag when both hold:
#
#   - HEAD has not moved past that tag's commit - new commits mean new work,
#     and new work means a new number;
#   - the tag is of the same kind as this run. A release tag is an x.y.0 and
#     a pre-release tag is not, which is exactly what the two rules above
#     keep true, so the kind is readable from the tag itself and no network
#     call is involved.
#
# stdout carries the tag alone - the callers capture it. stderr explains the
# decision, so a number that looks wrong in the log is readable after the
# fact.
#
# Usage: sh tools/next-tag.sh --prerelease|--release
#
# A `tag` typed into the dispatch form still wins: this script is only the
# answer for a blank field. To break the rules on purpose - a fresh number
# for a rebuild of the same commit - type the tag in the form.

set -u

usage() {
    echo "usage: sh tools/next-tag.sh --prerelease|--release" >&2
}

CHANNEL=""
for arg in "$@"; do
    case "$arg" in
        --prerelease) CHANNEL=prerelease ;;
        --release)    CHANNEL=release ;;
        -h|--help)    usage; exit 0 ;;
        *) echo "next-tag: unknown argument: $arg" >&2; usage; exit 2 ;;
    esac
done
if [ -z "$CHANNEL" ]; then
    echo "next-tag: say which channel this run publishes: --prerelease or --release" >&2
    usage
    exit 2
fi

# Version sort, so 0.10.0 outranks 0.9.0. Only bare x.y.z count: a suffixed
# tag (wudict2-v0.6.0-ru.1) names a variant nobody bumps. Upstream's own
# vX.Y.Z is accepted too, so a sync does not lose the line, and the tag is
# rebuilt with whatever prefix the last one used.
LAST="$(git tag --list --sort=-v:refname \
    | grep -E '^(wudict2-)?v[0-9]+\.[0-9]+\.[0-9]+$' \
    | head -n 1)"
if [ -z "$LAST" ]; then
    echo "next-tag: no wudict2-vX.Y.Z tag in this clone - tag the first release by hand and this script follows it" >&2
    exit 1
fi

NUM="${LAST#wudict2-}"
NUM="${NUM#v}"
MAJOR="${NUM%%.*}"
REST="${NUM#*.}"
MINOR="${REST%%.*}"
PATCH="${REST#*.}"
PREFIX="${LAST%v$NUM}"

# Commits HEAD has that the tag does not. Zero also covers HEAD being an
# ancestor of the tag, which is what building one release from two branches
# looks like: the same commit, the same version.
AHEAD="$(git rev-list --count "$LAST..HEAD" 2>/dev/null || echo 1)"

KIND=prerelease
[ "$PATCH" -eq 0 ] && KIND=release
echo "next-tag: last version ${LAST} (${KIND}, ${AHEAD} commit(s) after it); this run publishes a ${CHANNEL}" >&2

if [ "$AHEAD" -eq 0 ] && [ "$KIND" = "$CHANNEL" ]; then
    echo "next-tag: nothing committed since it and the kinds match - reusing ${LAST}" >&2
    printf '%s\n' "$LAST"
    exit 0
fi

if [ "$CHANNEL" = "release" ]; then
    MINOR=$((MINOR + 1))
    PATCH=0
    echo "next-tag: a release takes the next minor, and the patch resets" >&2
else
    PATCH=$((PATCH + 1))
    echo "next-tag: a pre-release takes the next patch" >&2
fi

TAG="${PREFIX}v${MAJOR}.${MINOR}.${PATCH}"
if git rev-parse -q --verify "refs/tags/${TAG}" > /dev/null; then
    echo "next-tag: ${TAG} already exists - refusing to guess; type the tag in the dispatch form" >&2
    exit 1
fi

echo "next-tag: ${LAST} -> ${TAG}" >&2
printf '%s\n' "$TAG"
