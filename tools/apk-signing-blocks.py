#!/usr/bin/env python3
# Copyright (C) 2026 DmShAl (Shepeta Dmitry)
# SPDX-License-Identifier: GPL-3.0-or-later

"""List the entries of an APK's signing block, by id, with the ones we know named.

The signing block is where APK Signature Scheme v2/v3 live, and it is also where
AGP puts things that are not signatures at all - most notably the **Dependency
metadata** (`0x504b4453`, "PKDS"): an opaque, Google-only-readable record of the
dependency list. Play reads it from the AAB; an APK handed to a person has no
business carrying it, which is why `android/app/build.gradle` sets
`dependenciesInfo { includeInApk = false }`.

Nothing else can show it: the block sits between the file data and the ZIP
central directory, so `unzip -l` never lists it and `apksigner verify` does not
print it. An UNSIGNED apk has no block at all, so this needs a signed one.

Run it with this machine's Python 3 (the name is in `build-tools.md`):

    tools/apk-signing-blocks.py android/app/build/outputs/apk/foss/release/*.apk
"""

from __future__ import annotations

import struct
import sys

KNOWN = {
    0x7109871A: "v2 signature",
    0xF05368C0: "v3 signature",
    0x1B93AD61: "v3.1 signature",
    0x42726577: "padding",
    0x6DFF800D: "source stamp (v2)",
    0x504B4453: "Dependency metadata (AGP) - should NOT be in a released APK",
}

MAGIC = b"APK Sig Block 42"


def u64(data: bytes, offset: int) -> int:
    return struct.unpack_from("<Q", data, offset)[0]


def look(path: str) -> None:
    data = open(path, "rb").read()
    eocd = data.rfind(b"PK\x05\x06")
    if eocd < 0:
        print(f"{path}: not a ZIP")
        return
    cd = struct.unpack_from("<I", data, eocd + 16)[0]
    if data[cd - 16 : cd] != MAGIC:
        print(f"{path}: no signing block (the apk is unsigned)")
        return
    size = u64(data, cd - 24)
    start = cd - 8 - size
    print(f"{path}\n  signing block: {size} bytes")
    offset = start + 8
    end = start + 8 + size - 24
    while offset < end:
        length = u64(data, offset)
        entry_id = struct.unpack_from("<I", data, offset + 8)[0]
        name = KNOWN.get(entry_id, "unknown")
        print(f"    id=0x{entry_id:08x}  {length - 4:>6} bytes  {name}")
        offset += 8 + length


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    for argument in sys.argv[1:]:
        look(argument)
