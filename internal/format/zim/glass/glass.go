// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package glass reads the Xapian "glass" database that Kiwix archives carry as
// their full-text index (the X/fulltext/xapian entry, stored uncompressed), so a
// dictionary can be searched without building an index of our own.
//
// It is written from the format, not ported from a reader: the layout was read
// off Xapian 1.4.32 (backends/glass: glass_version.cc, glass_table.cc,
// glass_cursor.cc, glass_defs.h) and is written down in HANDOFF.md under "The
// glass format", together with the byte-level check that confirmed it against a
// real archive. The one thing a reader written for the multi-file layout gets
// wrong, and this one gets right: a single-file database keeps its six tables in
// ONE block space, addressed from the version file's end (see blockOffset).
//
// Only reading exists here, and only what a search needs: the version, blocks,
// and a key lookup in a table.
package glass

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
)

// Tables in the order the version file lists their roots (glass_defs.h).
const (
	TablePostlist = iota
	TableDocdata
	TableTermlist
	TablePosition
	TableSpelling
	TableSynonym
	numTables
)

const (
	versionMagicLen = 14 // "\x0f\x0dXapian Glass"
	headerSize      = 11 // revision 4, level 1, max free 2, total free 2, dir end 2
	dirStart        = headerSize
	blocksPerNumber = 4 // BYTES_PER_BLOCK_NUMBER: a branch item's child block number

	flagCompressed = 0x80 // leaf item: the tag is deflated
	flagLast       = 0x40
	flagFirst      = 0x20
	itemSizeMask   = 0x1fff // the low 13 bits of the 2-byte size

	levelFreelist = 254
)

var magic = []byte{0x0f, 0x0d, 'X', 'a', 'p', 'i', 'a', 'n', ' ', 'G', 'l', 'a', 's', 's'}

// Root describes one table: where its tree starts and how big its blocks are.
type Root struct {
	Block      uint32
	Level      int
	Entries    uint32
	Sequential bool
	Fake       bool
	BlockSize  uint32
	CompressAt uint32
}

// Stats are the database-wide counters from the version file.
type Stats struct {
	DocCount    uint32
	LastDocID   uint32
	DocLenLow   uint32
	WDFBound    uint32
	DocLenBound uint32
	Oldest      uint32
	TotalDocLen uint64
	SpellingMax uint32
}

// DB is an open database. It reads the bytes it was given and never writes: the
// caller keeps the archive mapped (or the blob in memory) for its lifetime.
type DB struct {
	data         []byte
	FormatDate   uint16
	UUID         [16]byte
	Revision     uint32
	Roots        [numTables]Root
	Stats        Stats
	versionBytes int // where the version file ends

	// The index's own description - its language and stopwords - read on first
	// search and kept, because it decides how every query is stemmed.
	metaOnce sync.Once
	meta     *indexMeta
	metaErr  error
}

// Open parses the version file at the start of data. data must be exactly one
// database, i.e. the X/fulltext/xapian blob and nothing else.
func Open(data []byte) (*DB, error) {
	if len(data) < versionMagicLen+2 {
		return nil, fmt.Errorf("glass: %d bytes is too short for a database", len(data))
	}
	if !bytes.Equal(data[:versionMagicLen], magic) {
		return nil, fmt.Errorf("glass: not a Xapian Glass database (magic % x)", data[:versionMagicLen])
	}
	db := &DB{data: data}
	db.FormatDate = binary.BigEndian.Uint16(data[versionMagicLen:])
	p := versionMagicLen + 2
	copy(db.UUID[:], data[p:p+16])
	p += 16
	var err error
	if db.Revision, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	for i := 0; i < numTables; i++ {
		r := Root{}
		if r.Block, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d root: %w", i, err)
		}
		var flags uint32
		if flags, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d level: %w", i, err)
		}
		r.Level = int(flags >> 2)
		r.Sequential = flags&0x02 != 0
		r.Fake = flags&0x01 != 0
		if r.Entries, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d entries: %w", i, err)
		}
		var sizeShift uint32
		if sizeShift, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d block size: %w", i, err)
		}
		r.BlockSize = sizeShift << 11
		if r.BlockSize < 2048 || r.BlockSize > 65536 || r.BlockSize&(r.BlockSize-1) != 0 {
			return nil, fmt.Errorf("glass: table %d: implausible block size %d", i, r.BlockSize)
		}
		if r.CompressAt, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d compress_min: %w", i, err)
		}
		// The free list is a string; we do not need it, only to skip it.
		var freeLen uint32
		if freeLen, p, err = unpackUint(data, p); err != nil {
			return nil, fmt.Errorf("glass: table %d free list: %w", i, err)
		}
		if p+int(freeLen) > len(data) {
			return nil, fmt.Errorf("glass: table %d free list runs past the file", i)
		}
		p += int(freeLen)
		db.Roots[i] = r
	}
	if db.Stats.DocCount, p, err = unpackUint(data, p); err != nil {
		return nil, fmt.Errorf("glass: stats: %w", err)
	}
	// last_docid is stored as the difference from doccount, and doclen_ubound as
	// the difference from wdf_ubound, both to pack smaller.
	var d uint32
	if d, p, err = unpackUint(data, p); err != nil {
		return nil, fmt.Errorf("glass: stats: %w", err)
	}
	db.Stats.LastDocID = db.Stats.DocCount + d
	if db.Stats.DocLenLow, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	if db.Stats.WDFBound, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	if d, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	db.Stats.DocLenBound = db.Stats.WDFBound + d
	if db.Stats.Oldest, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	var total uint32
	if total, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	db.Stats.TotalDocLen = uint64(total)
	if db.Stats.SpellingMax, p, err = unpackUint(data, p); err != nil {
		return nil, err
	}
	db.versionBytes = p
	return db, nil
}

// BlockSize is the size of a block, which all six tables share in a single-file
// database.
func (db *DB) BlockSize() int { return int(db.Roots[TablePostlist].BlockSize) }

// blockOffset is where block n lives. This is the whole difference from a
// multi-file reader: the block area starts at the end of the version file, and
// the six tables share one numbering, so a block number is an offset into the
// database, not into a table. Block 0 is unused.
func (db *DB) blockOffset(n uint32) (int64, error) {
	size := int64(db.BlockSize())
	if size == 0 || n == 0 {
		return 0, fmt.Errorf("glass: block %d does not exist", n)
	}
	off := int64(db.versionBytes)/size*size + int64(n)*size
	if off < 0 || off+size > int64(len(db.data)) {
		return 0, fmt.Errorf("glass: block %d at %d is outside the database (%d bytes)", n, off, len(db.data))
	}
	return off, nil
}

func (db *DB) block(n uint32) ([]byte, error) {
	off, err := db.blockOffset(n)
	if err != nil {
		return nil, err
	}
	return db.data[off : off+int64(db.BlockSize())], nil
}

// level of a block, and its directory bounds.
func blockLevel(b []byte) int  { return int(b[4]) }
func blockDirEnd(b []byte) int { return int(binary.BigEndian.Uint16(b[9:])) }

// Find returns the tag stored under key in one table, descending the tree from
// the table's root. found is false when the key is not there.
func (db *DB) Find(table int, key []byte) (tag []byte, found bool, err error) {
	if table < 0 || table >= numTables {
		return nil, false, fmt.Errorf("glass: no table %d", table)
	}
	n := db.Roots[table].Block
	if n == 0 {
		return nil, false, nil // an empty table has no root
	}
	for depth := 0; depth <= db.Roots[table].Level+1; depth++ {
		b, err := db.block(n)
		if err != nil {
			return nil, false, err
		}
		items, err := directoryItems(b)
		if err != nil {
			return nil, false, fmt.Errorf("glass: block %d: %w", n, err)
		}
		if len(items) == 0 {
			return nil, false, nil
		}
		branch := blockLevel(b) > 0
		i := searchItems(b, items, key, branch)
		if !branch {
			if i < 0 || !bytes.Equal(leafKey(b, items[i]), key) {
				return nil, false, nil
			}
			return leafTag(db, b, items[i])
		}
		if i < 0 {
			i = 0
		}
		n = binary.BigEndian.Uint32(b[items[i]:])
	}
	return nil, false, fmt.Errorf("glass: table %d: the tree is deeper than its root claims", table)
}

// directoryItems lists the item offsets of a block, ascending by key.
func directoryItems(b []byte) ([]int, error) {
	end := blockDirEnd(b)
	if end < dirStart || end > len(b) || (end-dirStart)%2 != 0 {
		return nil, fmt.Errorf("directory ends at %d", end)
	}
	out := make([]int, 0, (end-dirStart)/2)
	for c := dirStart; c < end; c += 2 {
		off := int(binary.BigEndian.Uint16(b[c:]))
		if off <= 0 || off+2 > len(b) {
			return nil, fmt.Errorf("item offset %d out of range", off)
		}
		out = append(out, off)
	}
	return out, nil
}

// searchItems returns the index of the last item whose key is not greater than
// key, or -1 when every item's key is greater. The directory is ascending, so a
// binary chop is enough; Xapian's own find_in_leaf does the same.
func searchItems(b []byte, items []int, key []byte, branch bool) int {
	lo, hi := 0, len(items)-1
	res := -1
	for lo <= hi {
		mid := (lo + hi) / 2
		if bytes.Compare(itemKey(b, items[mid], branch), key) <= 0 {
			res = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return res
}

// itemKey reads an item's key. The two kinds of item start differently - a leaf
// item begins with a 2-byte size, a branch item with its 4-byte child block
// number - and both put the 1-byte key length right after that, so the offset of
// the key itself differs by two. Reading a branch item's key with the leaf
// offsets silently compares garbage, which is why this takes the kind.
func itemKey(b []byte, off int, branch bool) []byte {
	at := off + 2
	if branch {
		at = off + 4
	}
	kl := int(b[at])
	return b[at+1 : at+1+kl]
}

func leafKey(b []byte, off int) []byte { return itemKey(b, off, false) }

// leafTag returns a leaf item's tag, inflating it when the item says so. The
// first component of a tag has no counter field, the others do - and a tag too
// long for one item continues in items whose keys carry the next counter, which
// Find does not follow yet: it returns what the first component holds.
func leafTag(db *DB, b []byte, off int) ([]byte, bool, error) {
	size := int(binary.BigEndian.Uint16(b[off:])) & itemSizeMask
	flags := b[off] & (flagCompressed | flagLast | flagFirst)
	kl := int(b[off+2])
	p := off + 3 + kl
	if flags&flagFirst == 0 {
		p += 2 // the component counter
	}
	// The first component's tag ends at the item's end: the flags that would say
	// where further components begin belong to them, not to us.
	// The item's size covers the key and the tag, not the 2-byte size field
	// or the key length byte in front of them (Xapian's "I K key X tag", where
	// I is the length of the key-tag pair). Reading it as the whole item cut
	// every tag three bytes short.
	end := off + 3 + size
	if end > len(b) || p > end {
		return nil, false, fmt.Errorf("item at %d has a broken size %d", off, size)
	}
	tag := b[p:end]
	if flags&flagCompressed == 0 {
		return tag, true, nil
	}
	plain, err := inflate(tag)
	if err != nil {
		return nil, false, err
	}
	return plain, true, nil
}

// unpackUint reads Xapian's packed unsigned integer: 7 bits per byte, high bit
// set means another byte follows (common/pack.h).
func unpackUint(data []byte, p int) (uint32, int, error) {
	var v uint32
	shift := 0
	for {
		if p >= len(data) {
			return 0, p, fmt.Errorf("glass: a packed integer runs past the end of the data")
		}
		b := data[p]
		p++
		if shift > 28 {
			return 0, p, fmt.Errorf("glass: a packed integer is longer than 32 bits")
		}
		v |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			return v, p, nil
		}
		shift += 7
	}
}

// nextKey returns the key and tag of the item that follows after in a table,
// which is how the later chunks of a posting list are reached: they are not
// more bytes under one key but entries of their own, and the table's order is
// the only thing that lists them (glass_postlist.cc, GlassPostList::next_chunk
// walks the cursor; the number in the key is the document the next chunk starts
// at, which is not the previous chunk's last document plus one - a term does not
// occur in every document, so there is a gap to skip).
//
// found is false at the end of the table. The descent keeps its path so the walk
// can climb back out of a leaf and into the item that follows it in the parent,
// which is what makes this a cursor rather than a lookup.
func (db *DB) nextKey(table int, after []byte) (key, tag []byte, found bool, err error) {
	if table < 0 || table >= numTables {
		return nil, nil, false, fmt.Errorf("glass: no table %d", table)
	}
	n := db.Roots[table].Block
	if n == 0 {
		return nil, nil, false, nil
	}
	type step struct {
		block []byte
		item  int // the item we went into
	}
	var path []step
	depth := 0
	for ; depth <= db.Roots[table].Level+1; depth++ {
		b, err := db.block(n)
		if err != nil {
			return nil, nil, false, err
		}
		items, err := directoryItems(b)
		if err != nil {
			return nil, nil, false, fmt.Errorf("glass: block %d: %w", n, err)
		}
		if len(items) == 0 {
			break // an empty block: nothing follows inside it
		}
		branch := blockLevel(b) > 0
		if !branch {
			// The leaf is where the answer is, or where the climb starts.
			i := searchItems(b, items, after, false)
			if i+1 < len(items) {
				return db.itemAt(b, items[i+1])
			}
			break
		}
		i := searchItems(b, items, after, true)
		if i < 0 {
			i = 0
		}
		path = append(path, step{block: b, item: i})
		n = binary.BigEndian.Uint32(b[items[i]:])
	}
	// Climb: the item after the last one we descended through leads to the
	// subtree whose leftmost leaf follows everything in the one below it.
	for k := len(path) - 1; k >= 0; k-- {
		b := path[k].block
		items, err := directoryItems(b)
		if err != nil {
			return nil, nil, false, err
		}
		next := path[k].item + 1
		if next >= len(items) {
			continue
		}
		child := binary.BigEndian.Uint32(b[items[next]:])
		for {
			leaf, err := db.block(child)
			if err != nil {
				return nil, nil, false, err
			}
			leafItems, err := directoryItems(leaf)
			if err != nil {
				return nil, nil, false, fmt.Errorf("glass: block %d: %w", child, err)
			}
			if len(leafItems) == 0 {
				return nil, nil, false, fmt.Errorf("glass: block %d is empty below a branch", child)
			}
			if blockLevel(leaf) == 0 {
				return db.itemAt(leaf, leafItems[0])
			}
			child = binary.BigEndian.Uint32(leaf[leafItems[0]:])
		}
	}
	return nil, nil, false, nil
}

// itemAt returns a leaf item's key and tag. The key is copied because the
// caller may keep it after the next block is read, and the tag is inflated when
// the item says so.
func (db *DB) itemAt(b []byte, off int) (key, tag []byte, found bool, err error) {
	k := itemKey(b, off, false)
	t, _, err := itemTag(b, off)
	if err != nil {
		return nil, nil, false, err
	}
	return append([]byte(nil), k...), t, true, nil
}
