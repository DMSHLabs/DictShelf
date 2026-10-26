#!/bin/sh
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
#
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Resolve the tag for a blank release-dispatch tag field. New fork releases use
# dictshelf-vMAJOR.MINOR.PATCH. Legacy upstream and wudict2 tags remain valid
# when finding the current version.

set -u

usage() { echo "usage: sh tools/next-tag.sh --release|--pre-release|--beta" >&2; }
CHANNEL=""
for arg in "$@"; do
    case "$arg" in
        --release) CHANNEL=release ;;
        --prerelease|--pre-release) CHANNEL=prerelease ;;
        --beta) CHANNEL=beta ;;
        -h|--help) usage; exit 0 ;;
        *) echo "next-tag: unknown argument: $arg" >&2; usage; exit 2 ;;
    esac
done
if [ -z "$CHANNEL" ]; then
    echo "next-tag: choose --release, --pre-release or --beta" >&2
    usage
    exit 2
fi

# Suffixed preview tags do not advance the stable version line. Version-sort
# all supported plain tags, then emit only the new DictShelf prefix.
LAST="$(git tag --list --sort=-v:refname \
    | grep -E '^(dictshelf-|wudict2-)?v[0-9]+\.[0-9]+\.[0-9]+$' \
    | head -n 1)"
if [ -z "$LAST" ]; then
    echo "next-tag: no version tag found; create the first release tag manually" >&2
    exit 1
fi

NUM="${LAST#dictshelf-}"
NUM="${NUM#wudict2-}"
NUM="${NUM#v}"
MAJOR="${NUM%%.*}"
REST="${NUM#*.}"
MINOR="${REST%%.*}"
MINOR="$(printf '%s' "$MINOR" | sed 's/^0*//')"
[ -n "$MINOR" ] || MINOR=0

# Each channel uses the next minor's .01 version. Preview suffixes make tags
# distinct from the stable release while keeping all channels on one version
# line; the suffix number advances only when that channel has new commits.
MINOR=$((MINOR + 1))
PATCH=1
CORE="dictshelf-v${MAJOR}.$(printf '%02d' "$MINOR").$(printf '%02d' "$PATCH")"
case "$CHANNEL" in
    release)
        TAG="$CORE"
        if git rev-parse -q --verify "refs/tags/${TAG}" > /dev/null; then
            TAG_COMMIT="$(git rev-parse "${TAG}^{commit}")"
            HEAD_COMMIT="$(git rev-parse HEAD)"
            if [ "$TAG_COMMIT" = "$HEAD_COMMIT" ]; then
                echo "next-tag: same commit - reusing ${TAG}" >&2
                printf '%s\n' "$TAG"
                exit 0
            fi
            echo "next-tag: ${TAG} exists on an older commit - supply the desired tag explicitly" >&2
            exit 1
        fi
        ;;
    prerelease) SUFFIX="-pre-release." ;;
    beta) SUFFIX="-beta." ;;
esac

if [ "$CHANNEL" != release ]; then
    PREFIX="${CORE}${SUFFIX}"
    LAST_PREVIEW="$(git tag --list "${PREFIX}*" --sort=-v:refname | head -n 1)"
    NUMBER=1
    if [ -n "$LAST_PREVIEW" ]; then
        TAG_COMMIT="$(git rev-parse "${LAST_PREVIEW}^{commit}")"
        HEAD_COMMIT="$(git rev-parse HEAD)"
        if [ "$TAG_COMMIT" = "$HEAD_COMMIT" ]; then
            echo "next-tag: same commit - reusing ${LAST_PREVIEW}" >&2
            printf '%s\n' "$LAST_PREVIEW"
            exit 0
        fi
        if ! git merge-base --is-ancestor "$LAST_PREVIEW" HEAD; then
            echo "next-tag: ${LAST_PREVIEW} is not an ancestor of HEAD - supply the desired tag explicitly" >&2
            exit 1
        fi
        NUMBER="${LAST_PREVIEW##*.}"
        NUMBER="$(printf '%s' "$NUMBER" | sed 's/^0*//')"
        [ -n "$NUMBER" ] || NUMBER=0
        NUMBER=$((NUMBER + 1))
    fi
    TAG="${PREFIX}${NUMBER}"
fi

if git rev-parse -q --verify "refs/tags/${TAG}" > /dev/null; then
    echo "next-tag: ${TAG} already exists - supply the desired tag explicitly" >&2
    exit 1
fi
echo "next-tag: ${LAST} -> ${TAG}" >&2
printf '%s\n' "$TAG"
