#!/usr/bin/env python3
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later

"""Build the public, static update catalogue from published GitHub releases."""

import json
import os
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


TAG_RE = re.compile(r"^dictshelf-v(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$")
CHANNELS = ("release", "prerelease", "beta")


def main() -> None:
    raw = sys.stdin.read()
    releases = json.loads(raw)
    if not isinstance(releases, list):
        raise SystemExit("GitHub release response must be an array")

    catalog = {
        "schema": 1,
        "generated": datetime.now(timezone.utc).replace(microsecond=0).isoformat(),
        **{channel: [] for channel in CHANNELS},
    }
    for release in releases:
        if not isinstance(release, dict) or release.get("draft"):
            continue
        match = TAG_RE.fullmatch(str(release.get("tag_name", "")))
        url = release.get("html_url")
        if not match or not isinstance(url, str) or not url.startswith(
            "https://github.com/DMSHLabs/DictShelf/releases/tag/"
        ):
            continue

        version = match.group(1)
        is_beta = "beta" in version.lower()
        channel = "beta" if is_beta else "prerelease" if release.get("prerelease") else "release"
        entry = {"version": version, "url": url}
        assets = release.get("assets", [])
        android_apks = {}
        if isinstance(assets, list):
            for asset in assets:
                if not isinstance(asset, dict):
                    continue
                name, asset_url = asset.get("name"), asset.get("browser_download_url")
                digest = asset.get("digest", "")
                if not isinstance(asset_url, str) or not asset_url.startswith(
                    "https://github.com/DMSHLabs/DictShelf/releases/download/"
                ):
                    continue
                if name == "DictShelf.exe":
                    entry["windowsPortable"] = {"url": asset_url}
                    if isinstance(digest, str) and digest.startswith("sha256:"):
                        entry["windowsPortable"]["sha256"] = digest.removeprefix("sha256:")
                elif isinstance(name, str) and re.fullmatch(
                    r"dictshelf-windows-x64-setup-.+\.exe", name, re.IGNORECASE
                ):
                    entry["windowsInstaller"] = {"url": asset_url}
                    if isinstance(digest, str) and digest.startswith("sha256:"):
                        entry["windowsInstaller"]["sha256"] = digest.removeprefix("sha256:")
                elif name in {
                    "dictshelf-android-arm64-v8a-foss.apk",
                    "dictshelf-android-armeabi-v7a-foss.apk",
                    "dictshelf-android-universal-foss.apk",
                }:
                    abi = {
                        "dictshelf-android-arm64-v8a-foss.apk": "arm64-v8a",
                        "dictshelf-android-armeabi-v7a-foss.apk": "armeabi-v7a",
                        "dictshelf-android-universal-foss.apk": "universal",
                    }[name]
                    android_apks[abi] = {"url": asset_url}
                    if isinstance(digest, str) and digest.startswith("sha256:"):
                        android_apks[abi]["sha256"] = digest.removeprefix("sha256:")
        if android_apks:
            entry["androidApks"] = android_apks
        catalog[channel].append(entry)

    version_key = lambda item: tuple(
        (0, int(part)) if part.isdigit() else (1, part.lower())
        for part in re.split(r"[.-]", item["version"])
    )
    for channel in CHANNELS:
        catalog[channel].sort(key=version_key, reverse=True)

    path = Path(os.environ.get("UPDATE_CATALOG_PATH", "update-catalog.json"))
    temp = path.with_suffix(path.suffix + ".tmp")
    temp.write_text(json.dumps(catalog, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    temp.replace(path)


if __name__ == "__main__":
    main()
