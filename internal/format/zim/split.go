// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wuweidict/wudict/internal/dict"
)

// joined presents consecutive split-ZIM files as one ReaderAt. ZIM structures
// and blobs may cross any part boundary, so offsets always refer to the joined
// stream.
type joined struct {
	files  []*os.File
	starts []int64
}

func openJoined(paths []string) (io.ReaderAt, io.Closer, int64, error) {
	if len(paths) == 0 || len(paths) > dict.MaxZIMParts {
		return nil, nil, 0, fmt.Errorf("invalid split ZIM part count: %d", len(paths))
	}
	if len(paths) == 1 {
		f, err := os.Open(paths[0])
		if err != nil {
			return nil, nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, nil, 0, err
		}
		return f, f, st.Size(), nil
	}
	j := &joined{}
	var off int64
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			_ = j.Close()
			return nil, nil, 0, err
		}
		st, err := f.Stat()
		if err != nil {
			_ = f.Close()
			_ = j.Close()
			return nil, nil, 0, err
		}
		if st.Size() < 0 || off > int64(^uint64(0)>>1)-st.Size() {
			_ = f.Close()
			_ = j.Close()
			return nil, nil, 0, errors.New("split ZIM size overflows int64")
		}
		j.files = append(j.files, f)
		j.starts = append(j.starts, off)
		off += st.Size()
	}
	j.starts = append(j.starts, off)
	return j, j, off, nil
}

func (j *joined) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("zim: negative offset")
	}
	if len(b) == 0 {
		return 0, nil
	}
	total := j.starts[len(j.files)]
	if off >= total {
		return 0, io.EOF
	}
	n := 0
	for len(b) > 0 && off < total {
		i := sort.Search(len(j.files), func(i int) bool { return j.starts[i+1] > off })
		end := j.starts[i+1]
		want := min(int64(len(b)), end-off)
		k, err := j.files[i].ReadAt(b[:int(want)], off-j.starts[i])
		n += k
		off += int64(k)
		b = b[k:]
		if int64(k) < want {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return n, err
		}
	}
	if len(b) > 0 {
		return n, io.EOF
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

func splitZIMSource(path string) ([]string, error) {
	if dict.MainExt(path) != ".zimaa" {
		return []string{path}, nil
	}
	parts, missing, err := dict.ZIMPartSet(path)
	if err != nil {
		return nil, err
	}
	if missing != "" {
		return nil, fmt.Errorf("split ZIM is incomplete: missing %s", missing)
	}
	return parts, nil
}

func splitSourceRevision(path string) (string, error) {
	parts, missing, err := dict.ZIMPartSet(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, p := range parts {
		st, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\n", strings.ToLower(filepath.Base(p)), st.Size(), st.ModTime().UnixNano())
	}
	if missing != "" {
		_, _ = io.WriteString(h, "gap\x00"+strings.ToLower(filepath.Base(missing)))
	}
	return fmt.Sprintf("zim-parts:%x", h.Sum(nil)), nil
}
