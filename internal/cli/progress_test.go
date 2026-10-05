// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"slices"
	"testing"
)

func TestMeterLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    meter
		want string
	}{
		{"known total", meter{label: "entries", total: 118000, n: 41000}, "  41000/118000 entries"},
		{"unknown total", meter{label: "entries read", n: 1000}, "  1000 entries read"},
		{"count past the declared total", meter{label: "entries", total: 10, n: 11}, "  11 entries"},
		{"bytes", meter{label: "resources", total: 40000, n: 1200, bytes: true, b: 120_000_000}, "  1200/40000 resources · 120 MB"},
	} {
		if got := tc.m.line(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The line is redrawn once per every items, whatever the steps they come in.
func TestMeterTick(t *testing.T) {
	m := &meter{every: 10, next: 10}
	var at []int
	for _, n := range []int{3, 4, 2, 1, 15, 1, 6} {
		before := m.next
		m.n += n
		m.tick()
		if m.next != before {
			at = append(at, m.n)
		}
	}
	if want := []int{10, 25}; !slices.Equal(at, want) {
		t.Errorf("redrawn at %v, want %v", at, want)
	}
}

// A nil meter - stderr is not a terminal - takes every call.
func TestMeterNil(t *testing.T) {
	var m *meter
	m.Add(1)
	m.Set(2)
	m.AddBytes(3)
	m.Clear()
}
