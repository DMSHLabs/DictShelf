// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// A tag that does not fit in one item is split into components: items with the
// same key, counter 1, 2, 3 ... in that order, the last one flagged. A posting
// list of a common word is one of these, so reading only the first component
// returns a list that ends in the middle of a document - see docs/GLASS.md, "The
// glass format", and Xapian's make_start_of_chunk / component_of.
func (db *DB) keyTag(table int, key []byte) ([]byte, bool, error) {
	var out []byte
	for component := uint32(1); ; component++ {
		b, off, found, err := db.findComponent(table, key, component)
		if err != nil {
			return nil, false, err
		}
		if !found {
			if component == 1 {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("glass: the tag of %q ends without its last component", key)
		}
		piece, flags, err := itemTag(b, off)
		if err != nil {
			return nil, false, err
		}
		out = append(out, piece...)
		if flags&flagLast != 0 {
			return out, true, nil
		}
		if component > 1<<16 {
			return nil, false, fmt.Errorf("glass: %q has an implausible number of components", key)
		}
	}
}

// itemTag returns a leaf item's tag, inflating it when the item says so, and the
// item's flags. A component other than the first carries its counter before the
// tag; the first does not.
func itemTag(b []byte, off int) ([]byte, byte, error) {
	size := int(binary.BigEndian.Uint16(b[off:])) & itemSizeMask
	flags := b[off] & (flagCompressed | flagLast | flagFirst)
	kl := int(b[off+2])
	p := off + 3 + kl
	if flags&flagFirst == 0 {
		p += 2
	}
	// The item's size covers the key and the tag, not the 2-byte size field
	// or the key length byte in front of them (Xapian's "I K key X tag", where
	// I is the length of the key-tag pair). Reading it as the whole item cut
	// every tag three bytes short.
	end := off + 3 + size
	if end > len(b) || p > end {
		return nil, flags, fmt.Errorf("glass: item at %d has a broken size %d", off, size)
	}
	piece := b[p:end]
	if flags&flagCompressed == 0 {
		return piece, flags, nil
	}
	plain, err := inflate(piece)
	return plain, flags, err
}

// findComponent descends to the item holding (key, component) and returns its
// block and offset. Items are ordered by key and then by counter, so the
// comparison in the descent has to be the pair, not the key alone.
func (db *DB) findComponent(table int, key []byte, component uint32) ([]byte, int, bool, error) {
	n := db.Roots[table].Block
	if n == 0 {
		return nil, 0, false, nil
	}
	for depth := 0; depth <= db.Roots[table].Level+1; depth++ {
		b, err := db.block(n)
		if err != nil {
			return nil, 0, false, err
		}
		items, err := directoryItems(b)
		if err != nil {
			return nil, 0, false, fmt.Errorf("glass: block %d: %w", n, err)
		}
		if len(items) == 0 {
			return nil, 0, false, nil
		}
		branch := blockLevel(b) > 0
		i := searchTuple(b, items, key, component, branch)
		if !branch {
			if i < 0 {
				return nil, 0, false, nil
			}
			if !bytes.Equal(itemKey(b, items[i], false), key) || itemComponent(b, items[i]) != component {
				return nil, 0, false, nil
			}
			return b, items[i], true, nil
		}
		if i < 0 {
			i = 0
		}
		n = binary.BigEndian.Uint32(b[items[i]:])
	}
	return nil, 0, false, fmt.Errorf("glass: table %d: the tree is deeper than its root claims", table)
}

// itemComponent reads the counter of a leaf item; the first component has none.
func itemComponent(b []byte, off int) uint32 {
	if b[off]&flagFirst != 0 {
		return 1
	}
	kl := int(b[off+2])
	return uint32(binary.BigEndian.Uint16(b[off+3+kl:]))
}

// searchTuple is searchItems with the counter taken into account: the last item
// whose (key, counter) is not greater than the target.
func searchTuple(b []byte, items []int, key []byte, component uint32, branch bool) int {
	lo, hi := 0, len(items)-1
	res := -1
	for lo <= hi {
		mid := (lo + hi) / 2
		cmp := bytes.Compare(itemKey(b, items[mid], branch), key)
		if cmp == 0 && !branch {
			if c := itemComponent(b, items[mid]); c != component {
				if c > component {
					cmp = 1
				} else {
					cmp = -1
				}
			}
		}
		if cmp <= 0 {
			res = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return res
}
