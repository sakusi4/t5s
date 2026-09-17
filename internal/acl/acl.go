// Package acl decides which client addresses may reach a shared service.
package acl

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

// List is a set of allowed address ranges. The zero value allows nothing.
type List struct {
	prefixes []netip.Prefix
}

// Parse builds a List from IP addresses and CIDR ranges.
// It returns an error for an empty list, an unparsable entry,
// or a range that allows every address.
func Parse(entries []string) (List, error) {
	if len(entries) == 0 {
		return List{}, errors.New("allowlist is empty")
	}
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, entry := range entries {
		prefix, err := parsePrefix(strings.TrimSpace(entry))
		if err != nil {
			return List{}, err
		}
		prefixes = append(prefixes, prefix)
	}
	return List{prefixes: prefixes}, nil
}

// Allows reports whether addr is inside one of the ranges.
func (l List) Allows(addr netip.Addr) bool {
	addr = addr.Unmap().WithZone("")
	for _, prefix := range l.prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parsePrefix(entry string) (netip.Prefix, error) {
	if !strings.Contains(entry, "/") {
		addr, err := netip.ParseAddr(entry)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("parse allowlist entry %q: %w", entry, err)
		}
		addr = addr.Unmap().WithZone("")
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(entry)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse allowlist entry %q: %w", entry, err)
	}
	if prefix.Bits() == 0 {
		return netip.Prefix{}, fmt.Errorf("allowlist entry %q allows every address", entry)
	}
	return prefix.Masked(), nil
}
