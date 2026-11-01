// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package glass

import (
	"strings"
)

// Metadata answers what the database says about itself. Xapian keeps these in
// the postlist table under a key that starts with two bytes, 0x00 0xc0, right
// after the last term - which is why they sort away from every term and from
// the document data keys (glass_database.cc, GlassDatabase::get_metadata).
//
// The values a libzim index carries, and what they are for:
//
//	language   the archive's language, an ICU locale name or a code such as
//	           "rus" or "ru"; libzim resolves it to Xapian's two-letter
//	           stemmer name and stems every query with it (src/tools.cpp,
//	           getXapianStemmer)
//	stopwords  one per line; these were NOT indexed (libzim indexes with
//	           STOP_ALL), so a query must drop them too or it will find
//	           nothing instead of everything
//	valuesmap  which Xapian value slot holds what index data (title, word
//	           count ...), as libzim's own text
//	data       the same thing in libzim's newer form
func (db *DB) Metadata(key string) (string, bool, error) {
	k := make([]byte, 0, 2+len(key))
	k = append(k, 0x00, 0xc0)
	k = append(k, key...)
	tag, found, err := db.keyTag(TablePostlist, k)
	if err != nil || !found {
		return "", false, err
	}
	return string(tag), true, nil
}

// Language returns what the index records as the archive's language, "" when it
// records none (an index built before 2017/03 has none - libzim falls back to
// the archive's own Language metadata then).
func (db *DB) Language() (string, error) {
	v, _, err := db.Metadata("language")
	return v, err
}

// Stopwords returns the words the index deliberately did not index, one per
// line, folded the way the index folds terms. A query containing one of these
// must not require it: the term is absent from every document by construction,
// so asking for it can only produce an empty answer (that is exactly what
// libzim does with the same list - src/search.cpp, new_stopper).
func (db *DB) Stopwords() ([]string, error) {
	v, _, err := db.Metadata("stopwords")
	if err != nil || v == "" {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(v, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}
