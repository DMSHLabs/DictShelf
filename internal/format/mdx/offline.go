// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
// SPDX-License-Identifier: GPL-3.0-or-later

package mdx

import (
	"regexp"
	"strings"
)

var (
	emptyAdElement = regexp.MustCompile(`(?is)<(span|div|aside|section)\b([^>]*)>\s*</(span|div|aside|section)\s*>`)
	adElementID    = regexp.MustCompile(`(?i)(^|[[:space:]])id[[:space:]]*=[[:space:]]*("([^"]*)"|'([^']*)'|([^[:space:]>]+))`)
	adSlotID       = regexp.MustCompile(`(?i)^ad_(contentslot|extraslot|mpuslot|leftslot|rightslot|btmslot|topslot|stickyslot)(_[0-9]+)?$`)
	adMediaAttr    = regexp.MustCompile(`(?i)(^|[[:space:]])(src|srcset|poster|data|background)[[:space:]]*=`)
)

// stripEmptyAdSlots drops only empty, named advertising placeholders. An ID
// is part of the dictionary's ad-slot vocabulary, and the empty-element match
// ensures any text, image or other child content remains untouched.
func stripEmptyAdSlots(body string) string {
	return emptyAdElement.ReplaceAllStringFunc(body, func(element string) string {
		parts := emptyAdElement.FindStringSubmatch(element)
		if len(parts) != 4 || !strings.EqualFold(parts[1], parts[3]) {
			return element
		}
		attr := adElementID.FindStringSubmatch(parts[2])
		if len(attr) == 0 {
			return element
		}
		id := attr[3]
		if id == "" {
			id = attr[4]
		}
		if id == "" {
			id = attr[5]
		}
		if !adSlotID.MatchString(id) {
			return element
		}
		if adMediaAttr.MatchString(parts[2]) || strings.Contains(strings.ToLower(parts[2]), "url(") {
			return element
		}
		return ""
	})
}
