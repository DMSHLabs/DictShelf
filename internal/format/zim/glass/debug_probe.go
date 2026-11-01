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

// RawChunks returns the raw bytes of the items that continue a term's posting
// list, and the keys they live under. Temporary: it is how the chunk layout was
// pinned against a real archive, and it goes when the decoder is settled.
func (db *DB) RawChunks(term string, n int) (raws, keys [][]byte, err error) {
	key := []byte(term)
	for i := 0; i < n; i++ {
		k, _, found, err := db.nextKey(TablePostlist, key)
		if err != nil || !found {
			return raws, keys, err
		}
		key = k
		raw, _, _, _, err := db.RawItem(TablePostlist, k)
		if err != nil {
			return raws, keys, err
		}
		raws = append(raws, raw)
		keys = append(keys, k)
	}
	return raws, keys, nil
}
