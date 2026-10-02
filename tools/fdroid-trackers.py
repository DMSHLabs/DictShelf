#!/usr/bin/env python3
# Copyright (C) 2026 glowinthedark
#
# SPDX-License-Identifier: GPL-3.0-or-later

"""Check a Gradle dependency report against the signature lists F-Droid scans with.

    ./gradlew -q :app:dependencies | tools/fdroid-trackers.py

fdroidserver's scanner (fdroidserver/scanner.py) loads three lists, and a
coordinate matching any of them gets an app flagged:

  SUSS    https://fdroid.gitlab.io/fdroid-suss/suss.json - F-Droid's own; gradle
          signatures (regexes over group:artifact) and code signatures, with
          the anti-features they carry (Tracking, Ads, NonFreeComp, ...)
  Exodus  https://reports.exodus-privacy.eu.org/api/trackers
  ETIP    https://etip.exodus-privacy.eu.org/api/trackers/?format=json
          tracker code signatures: package regexes such as
          "io.opencensus|io.opentelemetry" - the pattern that flagged wudict.

None of these is a fixed list worth copying into a Makefile: they grow, so
they are fetched and cached for a day, as the scanner itself does. Every
signature is applied to every coordinate of every configuration in the report,
test-platform ones included, because that is the graph the scanner reads.

Exit status: 0 clean, 1 something matched, 2 the lists could not be had.
"""

import json
import os
import re
import sys
import time
import urllib.request

SOURCES = {
    "SUSS": "https://fdroid.gitlab.io/fdroid-suss/suss.json",
    "Exodus": "https://reports.exodus-privacy.eu.org/api/trackers",
    "ETIP": "https://etip.exodus-privacy.eu.org/api/trackers/?format=json",
}
CACHE_SECONDS = 24 * 3600
CACHE_DIR = os.environ.get("FDROID_SIG_CACHE") or os.path.join(
    os.path.dirname(os.path.abspath(__file__)), "..", "android", "build", "fdroid-signatures")

# group:artifact[:version] as the dependency report prints it, after the tree
# drawing ("+--- ", "\--- ", "|    ").
COORD = re.compile(r"[-\\+]--- ([\w.\-]+):([\w.\-]+)(?::([\w.\-+\[\],() ]+?))?(?: -> | \(|$)")


def fetch(name, url):
    """The list, from the cache while it is fresh, else from the network; a
    stale cache is still better than nothing when the network is down."""
    os.makedirs(CACHE_DIR, exist_ok=True)
    path = os.path.join(CACHE_DIR, name.lower() + ".json")
    fresh = os.path.exists(path) and time.time() - os.path.getmtime(path) < CACHE_SECONDS
    if not fresh:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "wudict-fdroid-check"})
            with urllib.request.urlopen(req, timeout=30) as r:
                body = r.read()
            json.loads(body)  # never cache something that is not the list
            with open(path + ".tmp", "wb") as f:
                f.write(body)
            os.replace(path + ".tmp", path)
        except Exception as e:  # noqa: BLE001 - any failure means "use the cache"
            if not os.path.exists(path):
                sys.exit(f"error: cannot fetch the {name} list ({e}) and nothing is cached")
            print(f"warning: {name} list not refreshed ({e}); using the cached copy", file=sys.stderr)
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def compile_sig(sig):
    try:
        return re.compile(sig, re.IGNORECASE)
    except re.error:
        return re.compile(re.escape(sig), re.IGNORECASE)


def signatures():
    """(source, name, flags, regex, kind) for every usable signature."""
    out = []
    suss = fetch("SUSS", SOURCES["SUSS"])["signatures"]
    for key, s in suss.items():
        flags = ", ".join(s.get("anti_features") or []) or s.get("license", "")
        name = s.get("name") or key
        for sig in s.get("gradle_signatures", []):
            out.append(("SUSS", name, flags, compile_sig(sig), "gradle"))
        for sig in s.get("code_signatures", []):
            # code signatures are class paths ("com/amazon/device"); the
            # coordinate's group is the same name written with dots
            out.append(("SUSS", name, flags, compile_sig(sig.replace("/", r"[./]")), "code"))
    for src in ("Exodus", "ETIP"):
        data = fetch(src, SOURCES[src])
        trackers = data["trackers"].values() if isinstance(data, dict) else data
        for t in trackers:
            sig = (t.get("code_signature") or "").strip()
            if sig:  # an empty pattern would match everything
                out.append((src, t.get("name", "?"), "Tracker", compile_sig(sig), "code"))
    return out


def coordinates(report):
    seen = set()
    for line in report.splitlines():
        m = COORD.search(line)
        if m:
            seen.add(f"{m.group(1)}:{m.group(2)}")
    return sorted(seen)


def main():
    report = sys.stdin.read() if len(sys.argv) < 2 else open(sys.argv[1], encoding="utf-8").read()
    coords = coordinates(report)
    if not coords:
        sys.exit("error: no coordinates in the input - is it a Gradle dependency report?")
    sigs = signatures()
    hits = {}
    for c in coords:
        dotted = c.replace(":", ".")
        for src, name, flags, rx, kind in sigs:
            # gradle signatures are written against "group:artifact"; code
            # signatures are package names, which the group spells. Searched
            # anywhere, not only at the start: a false alarm costs a look, a
            # miss costs a rejected submission.
            if rx.search(c) if kind == "gradle" else (rx.search(dotted) or rx.search(c)):
                hits.setdefault(c, set()).add(f"{src}: {name} [{flags}] /{rx.pattern}/")
    print(f"{len(coords)} coordinates checked against {len(sigs)} signatures "
          f"(SUSS, Exodus, ETIP)", file=sys.stderr)
    if not hits:
        print("no coordinate matches any F-Droid signature")
        return 0
    for c, why in sorted(hits.items()):
        print(c)
        for w in sorted(why):
            print("    " + w)
    return 1


if __name__ == "__main__":
    sys.exit(main())
