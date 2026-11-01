// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
)

// inflate decompresses an item tag that Xapian stored deflated (the item's
// compression flag). Bounded, because the size comes from the file.
func inflate(tag []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(tag))
	if err != nil {
		return nil, fmt.Errorf("glass: tag: %w", err)
	}
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, maxTagBytes))
	if err != nil {
		return nil, fmt.Errorf("glass: tag: %w", err)
	}
	return out, nil
}

// maxTagBytes caps what one tag may inflate to. Xapian's largest useful tag is a
// posting list chunk; a corrupt file must not be able to ask for all memory.
const maxTagBytes = 64 << 20
