// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

package dsl

// NormalizeArticle converts one DSL article body to the HTML used by the
// prepared index. Binary formats carrying DSL markup use this same converter
// instead of introducing another body-normalization path.
func NormalizeArticle(text, key string) (string, error) {
	body, _, err := transformArticleBody(text, key, nil)
	return body, err
}
