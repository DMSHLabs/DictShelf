// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"errors"
	"io"
	"os"
	"sort"
)

// A split archive (".zimaa", ".zimab", …: dict.ZIMParts) is one ZIM cut into
// pieces at arbitrary byte offsets - `split -b`, to fit FAT32's 4 GiB per
// file - so no structure respects a part boundary: a dirent, a cluster, a
// blob can each start in one part and end in the next. It is therefore read
// as the single file it was, and nothing above this layer knows it was split:
// a stored video is still a window onto "the file", served by range across
// parts.

// joined is the parts of an archive read as one file. starts[i] is part i's
// offset in the whole; starts[len(files)] is the whole's size, fixed at open.
type joined struct {
	files  []*os.File
	starts []int64
}

// openJoined opens the parts in order. One part is the unsplit case, and goes
// straight to that file.
func openJoined(paths []string) (source, int64, error) {
	j := &joined{}
	var off int64
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			j.Close()
			return nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			j.Close()
			return nil, 0, err
		}
		j.files = append(j.files, f)
		j.starts = append(j.starts, off)
		off += st.Size()
	}
	j.starts = append(j.starts, off)
	if len(j.files) == 1 {
		return j.files[0], off, nil
	}
	return j, off, nil
}

// ReadAt reads from as many parts as the range covers. Safe for concurrent
// use, as os.File.ReadAt is. A part that has shrunk since open reads short,
// as a single truncated file would: io.ErrUnexpectedEOF inside the whole,
// io.EOF past its end.
func (j *joined) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("zim: negative offset")
	}
	total := j.starts[len(j.files)]
	n := 0
	for len(b) > 0 {
		if off >= total {
			return n, io.EOF
		}
		// the part holding off; an empty part ends where it starts and is passed over
		i := sort.Search(len(j.files), func(i int) bool { return j.starts[i+1] > off })
		want := min(int64(len(b)), j.starts[i+1]-off)
		k, err := j.files[i].ReadAt(b[:want], off-j.starts[i])
		n, off, b = n+k, off+int64(k), b[k:]
		if int64(k) < want {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return n, err
		}
	}
	return n, nil
}

func (j *joined) Close() error {
	var first error
	for _, f := range j.files {
		if err := f.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
