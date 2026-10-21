// Copyright (C) 2026 glowinthedark
//
// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"net"
	"net/netip"
	"slices"
	"strings"
)

// isWildcard reports whether a listen host accepts on every interface: an
// unspecified address in either spelling, or no host at all (":6888").
func isWildcard(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

// serveURLs is what the banner, the tray tooltip and a second launch call the
// server. A concrete bind is its own address. A wildcard bind is listed by the
// addresses other devices reach it by, the default route's first - 0.0.0.0
// names no machine, and Windows cannot even connect to it - and is loopback
// when the machine has none.
func serveURLs(ip, port string) (main string, more []string) {
	addr := net.JoinHostPort(ip, port)
	if !isWildcard(ip) {
		return "http://" + addr + "/", nil
	}
	lan := lanIPv4(routeIPv4(), interfaceAddrs())
	if len(lan) == 0 {
		return "http://" + probeHost(addr) + "/", nil
	}
	urls := make([]string, len(lan))
	for i, a := range lan {
		urls[i] = "http://" + net.JoinHostPort(a.String(), port) + "/"
	}
	return urls[0], urls[1:]
}

// lanIPv4 orders the addresses worth printing: primary first, then the
// interfaces' in the order the OS lists them, each once. IPv4 only - the
// address a person types on a phone - and never loopback or link-local
// (169.254/16 is what a machine gives itself when DHCP failed).
func lanIPv4(primary netip.Addr, all []netip.Addr) []netip.Addr {
	var out []netip.Addr
	for _, a := range append([]netip.Addr{primary}, all...) {
		a = a.Unmap()
		if !a.Is4() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() ||
			slices.Contains(out, a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

// routeIPv4 is the source address of the default route. Connecting a UDP
// socket only consults the routing table - nothing is sent - and 192.0.2.1
// (TEST-NET-1) is an address no host owns, so the answer is the route to
// "elsewhere". Invalid when there is no such route.
func routeIPv4() netip.Addr {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return netip.Addr{}
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		if ip, ok := netip.AddrFromSlice(a.IP); ok {
			return ip
		}
	}
	return netip.Addr{}
}

// interfaceAddrs lists the addresses of the interfaces that are up and
// running. A failure is an empty list: the banner then names loopback.
func interfaceAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	const live = net.FlagUp | net.FlagRunning
	var out []netip.Addr
	for _, ifc := range ifs {
		if ifc.Flags&live != live || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				if ip, ok := netip.AddrFromSlice(n.IP); ok {
					out = append(out, ip)
				}
			}
		}
	}
	return out
}

// trustedHosts is TRUSTED_HOSTS plus SERVER_IP when that is a name (D104
// Am. 1): the name the server was told to listen on is one it answers to. A
// rebinding page sends its OWN name as Host, never the one in this config, so
// trusting it opens nothing. An address needs no entry - every IP literal is
// accepted - and neither does "localhost".
func trustedHosts(listed []string, ip string) []string {
	if ip == "" || strings.EqualFold(ip, "localhost") {
		return listed
	}
	if _, err := netip.ParseAddr(ip); err == nil {
		return listed
	}
	return append(slices.Clip(listed), ip)
}
