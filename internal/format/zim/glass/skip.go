// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
)

// floorKey seeks the last key not greater than target. Tags are fetched through
// keyTag afterwards, so a split tag is assembled from its first component.
func (db *DB) floorKey(table int, target []byte) ([]byte, error) {
	type step struct {
		block []byte
		items []int
		at    int
	}
	var path []step
	n := db.Roots[table].Block
	if n == 0 {
		return nil, nil
	}
	for depth := 0; depth <= db.Roots[table].Level+1; depth++ {
		b, err := db.block(n)
		if err != nil {
			return nil, err
		}
		items, err := directoryItems(b)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("glass: empty block during seek")
		}
		branch := blockLevel(b) > 0
		i := searchItems(b, items, target, branch)
		if !branch {
			if i >= 0 {
				return append([]byte(nil), leafKey(b, items[i])...), nil
			}
			for k := len(path) - 1; k >= 0; k-- {
				s := path[k]
				if s.at == 0 {
					continue
				}
				n = binary.BigEndian.Uint32(s.block[s.items[s.at-1]:])
				for d := 0; d <= db.Roots[table].Level+1; d++ {
					b, err = db.block(n)
					if err != nil {
						return nil, err
					}
					items, err = directoryItems(b)
					if err != nil {
						return nil, err
					}
					if len(items) == 0 {
						return nil, fmt.Errorf("glass: empty predecessor block")
					}
					if blockLevel(b) == 0 {
						return append([]byte(nil), leafKey(b, items[len(items)-1])...), nil
					}
					n = binary.BigEndian.Uint32(b[items[len(items)-1]:])
				}
				return nil, fmt.Errorf("glass: predecessor tree exceeds its depth")
			}
			return nil, nil
		}
		if i < 0 {
			i = 0
		}
		path = append(path, step{b, items, i})
		n = binary.BigEndian.Uint32(b[items[i]:])
	}
	return nil, fmt.Errorf("glass: seek exceeds tree depth")
}

// Sparse seeks trade a tree descent per occupied chunk for a sequential walk.
// The stricter length-list threshold avoids regressions on moderately common
// terms (the real-archive comparison is in docs/MEASUREMENTS.md).
const sparsePostingRatio = 32
const sparseLengthRatio = 512

type postingCursor struct {
	db               *DB
	ctx              context.Context
	prefix, key, tag []byte
	p                int
	did, wdf, last   uint32
	final            bool
}

func (db *DB) postingCursor(ctx context.Context, term string, prefix []byte) (*postingCursor, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	tag, found, err := db.keyTag(TablePostlist, []byte(term))
	if err != nil || !found {
		return nil, found, err
	}
	p := 0
	_, p, err = unpackUint(tag, p)
	if err != nil {
		return nil, false, err
	}
	_, p, err = unpackUint(tag, p)
	if err != nil {
		return nil, false, err
	}
	did, p, err := unpackUint(tag, p)
	if err != nil {
		return nil, false, err
	}
	c := &postingCursor{db: db, ctx: ctx, prefix: prefix, key: []byte(term), tag: tag, p: p}
	err = c.chunk(did + 1)
	return c, true, err
}
func (c *postingCursor) chunk(first uint32) error {
	var span uint32
	var err error
	c.final, span, c.p, err = chunkHeader(c.tag, c.p)
	if err != nil {
		return err
	}
	if first == 0 || span > ^uint32(0)-first {
		return fmt.Errorf("glass: posting chunk overflows")
	}
	c.did, c.last = first, first+span
	c.wdf, c.p, err = unpackUint(c.tag, c.p)
	if err == nil && c.db.Progress != nil {
		c.db.Progress(c.did)
	}
	return err
}
func (c *postingCursor) load(key []byte) error {
	if !bytes.HasPrefix(key, c.prefix) || len(key) <= len(c.prefix) {
		return fmt.Errorf("glass: seek left posting list")
	}
	first, _, err := unpackPreservingSort(key, len(c.prefix))
	if err != nil {
		return err
	}
	tag, found, err := c.db.keyTag(TablePostlist, key)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("glass: missing posting chunk")
	}
	c.key, c.tag, c.p = key, tag, 0
	return c.chunk(first)
}
func (c *postingCursor) next() error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.did == 0 {
		return nil
	}
	if c.did >= c.last {
		if c.final {
			c.did = 0
			return nil
		}
		key, _, found, err := c.db.nextKey(TablePostlist, c.key)
		if err != nil {
			return err
		}
		if !found || !bytes.HasPrefix(key, c.prefix) {
			return fmt.Errorf("glass: posting list ends early")
		}
		if err := c.load(key); err != nil {
			return err
		}
	} else {
		gap, p, err := unpackUint(c.tag, c.p)
		if err != nil {
			return err
		}
		if gap >= ^uint32(0)-c.did {
			return fmt.Errorf("glass: posting gap overflows")
		}
		c.did += gap + 1
		if c.did > c.last {
			return fmt.Errorf("glass: posting passes chunk end")
		}
		c.wdf, c.p, err = unpackUint(c.tag, p)
		if err != nil {
			return err
		}

		if c.db.Progress != nil {
			c.db.Progress(c.did)
		}
	}
	return nil
}

// skipTo moves to the first posting at or beyond target. Whole chunks before
// target are bypassed through the tree; bytes within its chunk are decoded.
func (c *postingCursor) skipTo(target uint32) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.did == 0 {
		return nil
	}
	if target > c.last && !c.final {
		key, err := c.db.floorKey(TablePostlist, appendPreservingSort(append([]byte(nil), c.prefix...), target))
		if err != nil {
			return err
		}
		if bytes.Compare(key, c.key) > 0 {
			if err := c.load(key); err != nil {
				return err
			}
		}
	}
	for c.did != 0 && c.did < target {
		if err := c.next(); err != nil {
			return err
		}
	}
	return nil
}

// selectedPostings reads frequencies only for the sorted candidate ids. It is
// used when the candidate set is sparse relative to the posting list.
func (db *DB) selectedPostings(ctx context.Context, term string, prefix []byte, candidates []Hit) ([]Hit, error) {
	c, found, err := db.postingCursor(ctx, term, prefix)
	if err != nil || !found {
		return nil, err
	}
	out := make([]Hit, 0, len(candidates))
	for _, h := range candidates {
		if err := c.skipTo(h.DocID); err != nil {
			return nil, err
		}
		if c.did == h.DocID {
			out = append(out, Hit{DocID: c.did, WDF: c.wdf})
		}
	}
	return out, ctx.Err()
}
