// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import "context"

// sortHits keeps the stable ordering of sort.SliceStable, with cancellation
// between small batches of comparisons even for a common word's million hits.
func sortHits(ctx context.Context, hits []Hit, less func(i, j int) bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Sort indices so less always sees the original hits until sorting ends.
	order := make([]int, len(hits))
	scratch := make([]int, len(hits))
	for i := range order {
		order[i] = i
	}
	for width := 1; width < len(order); width *= 2 {
		for start := 0; start < len(order); start += 2 * width {
			mid, end := min(start+width, len(order)), min(start+2*width, len(order))
			i, j := start, mid
			for k := start; k < end; k++ {
				if k&1023 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if i < mid && (j >= end || !less(order[j], order[i])) {
					scratch[k] = order[i]
					i++
				} else {
					scratch[k] = order[j]
					j++
				}
			}
		}
		order, scratch = scratch, order
	}
	// Apply the permutation in place; scratch is no longer needed for merging.
	for i := range order {
		if i&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		scratch[order[i]] = i
	}
	for i := range hits {
		if i&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		for scratch[i] != i {
			if err := ctx.Err(); err != nil {
				return err
			}
			j := scratch[i]
			hits[i], hits[j] = hits[j], hits[i]
			scratch[i], scratch[j] = scratch[j], scratch[i]
		}
	}
	return ctx.Err()
}
