// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

import "golang.org/x/text/unicode/norm"

// ArticleOptions separates optional cleanup and presentation from DSL repair.
type ArticleOptions struct {
	Enhance bool
	Styles  bool
}

// transformArticleBody uses the upstream range balancer, not the GD reference
// tree. NFC normalization keeps our generated titles, links and text consistent.
func transformArticleBody(text, key string, ab *abbrevMap) (string, []string, error) {
	return transformBodyAbbrev(norm.NFC.String(text), norm.NFC.String(key), ab)
}

func transformArticleWithOptions(text, key string, ab *abbrevMap, options ArticleOptions) (string, []string, error) {
	body, media, err := transformArticleBody(text, key, ab)
	if err != nil || (!options.Enhance && !options.Styles) {
		return body, media, err
	}
	body, err = prepareGDHTML(body, options)
	return body, media, err
}
