// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package go_mdict

import "testing"

func TestEncodingOf(t *testing.T) {
	for _, tc := range []struct {
		attr string
		want int
	}{
		{"", EncodingUtf8},
		{"UTF-8", EncodingUtf8},
		{"GBK", EncodingGb18030},
		{"gb2312", EncodingGb18030},
		{"GB18030", EncodingGb18030},
		{" GB18030 ", EncodingGb18030},
		{"Big5", EncodingBig5},
		{"UTF-16", EncodingUtf16},
		{"utf-16le", EncodingUtf16},
		{"x-unknown", EncodingUtf8},
	} {
		if got := encodingOf(tc.attr); got != tc.want {
			t.Errorf("encodingOf(%q) = %d, want %d", tc.attr, got, tc.want)
		}
	}
}
