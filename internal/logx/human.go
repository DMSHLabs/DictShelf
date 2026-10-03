// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package logx

import "fmt"

// Size is a byte count as people read it: decimal units, as the app's pages
// (index.html mb, lemmas.html, setup.html) and the platforms' file managers
// show them, so one file never reads as two sizes. Under 10 MB keeps one
// decimal, where a whole number would hide most of the difference.
func Size(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d B", n)
	case n < 1_000_000:
		return fmt.Sprintf("%d kB", (n+500)/1_000)
	case n < 10_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n < 1_000_000_000:
		return fmt.Sprintf("%d MB", (n+500_000)/1_000_000)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// Plural is n with the word that agrees with it: "1 entry", "2 entries".
func Plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
