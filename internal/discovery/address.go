package discovery

import (
	"cmp"
	"net/netip"
	"slices"
)

type listener struct {
	addr netip.AddrPort
	pid  int32
}

func dialAddr(listen netip.AddrPort) netip.AddrPort {
	if !listen.Addr().IsUnspecified() {
		return listen
	}
	if listen.Addr().Is4() {
		return netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 1}), listen.Port())
	}
	return netip.AddrPortFrom(netip.IPv6Loopback(), listen.Port())
}

func dedupeByPort(listeners []listener) []listener {
	byPort := make(map[uint16]listener)
	for _, l := range listeners {
		kept, seen := byPort[l.addr.Port()]
		if !seen || (kept.addr.Addr().Is6() && l.addr.Addr().Is4()) {
			byPort[l.addr.Port()] = l
		}
	}
	unique := make([]listener, 0, len(byPort))
	for _, l := range byPort {
		unique = append(unique, l)
	}
	slices.SortFunc(unique, func(a, b listener) int {
		return cmp.Compare(a.addr.Port(), b.addr.Port())
	})
	return unique
}
