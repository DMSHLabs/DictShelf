// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/DMSHLabs/DictShelf/internal/dict"
)

// Icon returns the first standard OpenZIM illustration available in the
// metadata namespace, preferring the higher-resolution variants.
func (d *Dict) Icon() (io.ReadCloser, string, error) {
	for _, name := range []string{
		"Illustration_96x96@2", "Illustration_96x96",
		"Illustration_48x48@2", "Illustration_48x48",
		"Illustration_32x32@2", "Illustration_32x32",
		"Illustration_16x16@2", "Illustration_16x16",
	} {
		_, entry, ok := d.c.find('M', name)
		if !ok || entry.isRedirect() || int(entry.mimetype) >= len(d.c.mimes) {
			continue
		}
		mime := strings.ToLower(strings.TrimSpace(strings.SplitN(d.c.mimes[entry.mimetype], ";", 2)[0]))
		if !strings.HasPrefix(mime, "image/") {
			continue
		}
		body, err := d.c.content(entry)
		if err != nil {
			return nil, "", err
		}
		if len(body) == 0 || len(body) > 2<<20 {
			return nil, "", fmt.Errorf("ZIM illustration %s has invalid size %d", name, len(body))
		}
		return io.NopCloser(bytes.NewReader(body)), mime, nil
	}
	return nil, "", dict.ErrNotFound
}
