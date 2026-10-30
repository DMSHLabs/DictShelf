// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"github.com/DMSHLabs/DictShelf/internal/dict"
	"io"
	"strings"
)

// ExportHeadwords streams Browse-visible titles, including aliases and repeated titles.
func (s *Store) ExportHeadwords(ctx context.Context, out io.Writer, progress func(int64)) error {
	rows, err := s.db.QueryContext(ctx, "SELECT w FROM entry WHERE "+browseWhere+browseOrder)
	if err != nil {
		return err
	}
	defer rows.Close()
	var count int64
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var word string
		if err := rows.Scan(&word); err != nil {
			return err
		}
		if _, err := io.WriteString(out, clean.Replace(dict.DisplayText(word))+"\n"); err != nil {
			return err
		}
		count++
		if count%4096 == 0 {
			progress(count)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	progress(count)
	return ctx.Err()
}
