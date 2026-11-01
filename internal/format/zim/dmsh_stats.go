// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package zim

import "strings"

// A read-through build's size is decided before a single cluster is
// decompressed: how many articles the archive holds, how much of it is media,
// and how many compressed bytes sit in clusters that carry article text and
// nothing else. All of that is in the directory, so measuring it costs one
// sequential walk of the dirent region - 1.3 s on a 48 GB archive - and never
// touches a cluster payload. The reader records what it measures in the
// prepared database, and DirectoryStats offers the same measurement for a
// library that was prepared before the measurement existed.
type DirectoryStats struct {
	Entries    int // content-namespace entries, redirects included
	Articles   int // text/html entries
	Redirects  int
	Media      int    // image, audio, video, font, application/* other than javascript
	Other      int    // css, javascript, text/plain and the like
	Clusters   int    // clusters in the file, whatever they carry
	TextBytes  uint64 // compressed bytes of clusters holding articles only
	MixedBytes uint64 // ... of clusters holding articles and other entries together
	OtherBytes uint64 // ... of clusters with no article at all (media, Kiwix's Xapian index, ...)
}

// MeasureDirectory opens path read-only and measures its directory. The
// archive is not decompressed: the numbers describe the container, which is
// what an estimate about a future build can know beforehand - and what a
// library prepared before this measurement existed can be measured with,
// because it needs nothing but the file that is already on disk.
func MeasureDirectory(path string) (DirectoryStats, error) {
	d, err := Open(path)
	if err != nil {
		return DirectoryStats{}, err
	}
	defer d.Close()
	return d.contentStats()
}

// contentStats walks the content namespace once and prices every cluster from
// the pointer table the container already parsed.
//
// Only content-namespace entries are classified: the M and W namespaces hold
// metadata and layout pages, and the X namespace holds Kiwix's own full-text
// index - none of it is text this program would ever index, and a cluster is
// priced by what its CONTENT entries are, so a metadata blob sharing a cluster
// with an article does not make that cluster mixed.
func (d *Dict) contentStats() (DirectoryStats, error) {
	c := d.c
	st := DirectoryStats{Clusters: int(c.clusterCount)}
	hasArticle := make([]bool, int(c.clusterCount))
	hasOther := make([]bool, int(c.clusterCount))
	w := c.walk()
	for i := d.lo; i < d.hi; i++ {
		de, err := w.entry(i)
		if err != nil {
			return st, err
		}
		st.Entries++
		switch {
		case de.isRedirect():
			st.Redirects++
			continue
		case c.htmlMIME[de.mimetype]:
			st.Articles++
			if n := int(de.cluster); n < len(hasArticle) {
				hasArticle[n] = true
			}
		default:
			if isMediaMIME(c.mimeOf(de)) {
				st.Media++
			} else {
				st.Other++
			}
			if n := int(de.cluster); n < len(hasOther) {
				hasOther[n] = true
			}
		}
	}
	for i := 0; i < int(c.clusterCount); i++ {
		start, end := c.extent(uint32(i))
		if end <= start {
			continue
		}
		switch size := end - start; {
		case hasArticle[i] && hasOther[i]:
			st.MixedBytes += size
		case hasArticle[i]:
			st.TextBytes += size
		default:
			st.OtherBytes += size
		}
	}
	return st, nil
}

// isMediaMIME answers whether an entry is a resource rather than text. Kiwix
// files keep images and audio in the same content namespace as articles, and
// telling them apart from the mime type is the whole trick: the archive's own
// size says nothing, because it is compressed and half of it may be pictures.
func isMediaMIME(m string) bool {
	base := mimeBase(m)
	switch {
	case strings.HasPrefix(base, "image/"), strings.HasPrefix(base, "audio/"),
		strings.HasPrefix(base, "video/"), strings.HasPrefix(base, "font/"):
		return true
	case strings.HasPrefix(base, "application/"):
		return base != "application/javascript"
	}
	return false
}
