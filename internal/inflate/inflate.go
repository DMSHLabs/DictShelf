// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

// Package inflate decodes compressed blocks - ZIM clusters, slob bins - into
// buffers reused across calls, and bounds what a decoder may produce.
//
// A decoder given no destination sizes its own and grows it by copying:
// zstd, whose frames from libzim carry no content size, allocates 4.6x the
// 2 MB cluster it produces, and io.ReadAll 2.8x a 530 kB slob bin (measured on
// wikipedia_en_climate_change_mini_2024-06 and 30 real slobs). Decoding into
// a reused buffer and copying out the result allocates the result alone.
package inflate

import (
	"bytes"
	"errors"
	"io"
)

// ErrTooLarge is AppendAll's refusal. Callers name the setting that bounds
// their data (limits.Exceeded), which this package does not know.
var ErrTooLarge = errors.New("unpacked data exceeds its limit")

// maxKept is the largest buffer kept for reuse; a rare larger block is
// decoded into one that is then dropped.
const maxKept = 16 << 20

// free holds the reusable buffers. A channel of two rather than a sync.Pool:
// a pool is emptied by every second garbage collection, and a server
// decompressing megabyte blocks collects often enough that a pooled buffer
// would rarely live to be reused. At most two buffers of maxKept stay
// resident; a decode that finds none starts from nil.
var free = make(chan []byte, 2)

// Into runs decode on a reused buffer and returns a copy of what it appended:
// the caller keeps the copy (in a cache, in a result), never the buffer.
func Into(decode func(dst []byte) ([]byte, error)) ([]byte, error) {
	var buf []byte
	select {
	case buf = <-free:
	default:
	}
	out, err := decode(buf[:0])
	var res []byte
	if err == nil {
		res = bytes.Clone(out)
		if res == nil {
			res = []byte{}
		}
	}
	if cap(out) <= maxKept {
		select {
		case free <- out[:0]:
		default:
		}
	}
	return res, err
}

// AppendAll reads r to its end onto dst, refusing more than max bytes: every
// codec can be made to expand a bounded input without limit, and the input is
// a downloaded file.
func AppendAll(dst []byte, r io.Reader, max int64) ([]byte, error) {
	for {
		if len(dst) == cap(dst) {
			dst = append(dst, 0)[:len(dst)]
		}
		n, err := r.Read(dst[len(dst):cap(dst)])
		dst = dst[:len(dst)+n]
		if int64(len(dst)) > max {
			return dst, ErrTooLarge
		}
		if err == io.EOF {
			return dst, nil
		}
		if err != nil {
			return dst, err
		}
	}
}
