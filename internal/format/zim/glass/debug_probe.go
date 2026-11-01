// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

// Temporary: raw view of the item a key resolves to, for pinning the byte
// layout against a real archive. Removed once the decoder is settled.
func (db *DB) RawItem(table int, key []byte) (raw []byte, size int, flags byte, keyLen int, err error) {
	b, off, found, err := db.findComponent(table, key, 1)
	if err != nil || !found {
		return nil, 0, 0, 0, err
	}
	end := off + 32
	if end > len(b) {
		end = len(b)
	}
	return b[off:end], int(bigEndianUint16(b[off:]) & itemSizeMask), b[off] & 0xe0, int(b[off+2]), nil
}

func bigEndianUint16(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
