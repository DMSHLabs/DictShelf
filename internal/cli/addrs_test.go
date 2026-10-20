// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestIsWildcard(t *testing.T) {
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"", true}, {"0.0.0.0", true}, {"::", true},
		{"127.0.0.1", false}, {"::1", false}, {"192.168.1.5", false},
		{"localhost", false}, {"wudict.lan", false}, {"[::]", false}, // brackets belong to host:port
	} {
		if got := isWildcard(tc.host); got != tc.want {
			t.Errorf("isWildcard(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// The default route's address leads, each address appears once, and nothing a
// person on another device could not use is listed.
func TestLanIPv4(t *testing.T) {
	a := netip.MustParseAddr
	got := lanIPv4(a("192.168.1.5"), []netip.Addr{
		a("10.8.0.2"),
		a("192.168.1.5"),       // the primary again
		a("::ffff:172.17.0.1"), // v4-mapped is the v4 address
		a("169.254.10.1"),      // link-local: DHCP failed
		a("127.0.0.1"),         // loopback
		a("fe80::1"),           // IPv6 is not listed
		a("2001:db8::5"),
		a("0.0.0.0"),
	})
	want := []netip.Addr{a("192.168.1.5"), a("10.8.0.2"), a("172.17.0.1")}
	if !slices.Equal(got, want) {
		t.Errorf("lanIPv4 = %v, want %v", got, want)
	}
	// No route: the interfaces alone, in their order.
	if got := lanIPv4(netip.Addr{}, []netip.Addr{a("10.0.0.3")}); !slices.Equal(got, []netip.Addr{a("10.0.0.3")}) {
		t.Errorf("lanIPv4 without a route = %v", got)
	}
}

// A concrete bind is printed as given; a wildcard never prints an
// unspecified address, whatever this machine's interfaces are.
func TestServeURLs(t *testing.T) {
	for _, tc := range []struct{ ip, want string }{
		{"127.0.0.1", "http://127.0.0.1:6888/"},
		{"192.168.1.5", "http://192.168.1.5:6888/"},
		{"::1", "http://[::1]:6888/"},
		{"wudict.lan", "http://wudict.lan:6888/"},
	} {
		if main, more := serveURLs(tc.ip, "6888"); main != tc.want || more != nil {
			t.Errorf("serveURLs(%q) = %q, %v; want %q", tc.ip, main, more, tc.want)
		}
	}
	for _, ip := range []string{"0.0.0.0", "::", ""} {
		main, more := serveURLs(ip, "6888")
		for _, u := range append([]string{main}, more...) {
			if strings.Contains(u, "0.0.0.0") || strings.Contains(u, "[::]") || strings.HasPrefix(u, "http://:") {
				t.Errorf("serveURLs(%q) printed %q", ip, u)
			}
		}
	}
}

// D104 Am. 1: SERVER_IP as a name is trusted; addresses and localhost add
// nothing, and the configured list is never written through.
func TestTrustedHosts(t *testing.T) {
	listed := make([]string, 1, 4) // spare capacity: an append must not reach it
	listed[0] = "proxy.lan"
	for _, tc := range []struct {
		ip   string
		want []string
	}{
		{"127.0.0.1", []string{"proxy.lan"}},
		{"0.0.0.0", []string{"proxy.lan"}},
		{"::", []string{"proxy.lan"}},
		{"fe80::1%eth0", []string{"proxy.lan"}},
		{"localhost", []string{"proxy.lan"}},
		{"LocalHost", []string{"proxy.lan"}},
		{"", []string{"proxy.lan"}},
		{"wudict.lan", []string{"proxy.lan", "wudict.lan"}},
	} {
		if got := trustedHosts(listed, tc.ip); !slices.Equal(got, tc.want) {
			t.Errorf("trustedHosts(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
	if spare := listed[:2][1]; spare != "" {
		t.Errorf("trustedHosts wrote %q into the caller's spare capacity", spare)
	}
	if got := trustedHosts(nil, "wudict.lan"); !slices.Equal(got, []string{"wudict.lan"}) {
		t.Errorf("trustedHosts(nil) = %v", got)
	}
}
