// Copyright (C) 2026 DmShAl (Shepeta Dmitry)
//
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package cli

import (
	"context"
	"errors"
	"testing"
)

// The tray behaviors must resolve from the saved settings before the window
// exists, with the same defaults the settings page shows.
func TestDesktopReadTraySettings(t *testing.T) {
	tests := []struct {
		name            string
		request         bool
		values          map[string]string
		err             error
		close, minimize bool
	}{
		{name: "no request keeps the defaults", close: true, minimize: false},
		{name: "missing keys keep the defaults", request: true, values: map[string]string{}, close: true, minimize: false},
		{name: "saved values are read", request: true, values: map[string]string{"wudict_close_to_tray": "0", "wudict_minimize_to_tray": "1"}, close: false, minimize: true},
		{name: "explicit close keeps it on", request: true, values: map[string]string{"wudict_close_to_tray": "1"}, close: true, minimize: false},
		{name: "unexpected value type keeps the defaults", request: true, close: true, minimize: false},
		{name: "read failure keeps the defaults", request: true, err: errors.New("storage unavailable"), close: true, minimize: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var request func(context.Context, string, string) (any, error)
			if tc.request {
				request = func(context.Context, string, string) (any, error) {
					if tc.values == nil && tc.err == nil {
						return map[string]any{"wudict_close_to_tray": 0}, nil
					}
					return tc.values, tc.err
				}
			}
			closeToTray, minimizeToTray := desktopReadTraySettings(request)
			if closeToTray != tc.close || minimizeToTray != tc.minimize {
				t.Errorf("desktopReadTraySettings() = close=%t minimize=%t; want close=%t minimize=%t",
					closeToTray, minimizeToTray, tc.close, tc.minimize)
			}
		})
	}
}
