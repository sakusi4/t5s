package discovery

import (
	"syscall"
	"testing"
)

func TestParseListenIP(t *testing.T) {
	tests := []struct {
		name   string
		ip     string
		family uint32
		want   string
		wantOK bool
	}{
		{"macos ipv4 wildcard", "*", syscall.AF_INET, "0.0.0.0", true},
		{"macos ipv6 wildcard", "*", syscall.AF_INET6, "::", true},
		{"ipv4 literal", "127.0.0.1", syscall.AF_INET, "127.0.0.1", true},
		{"ipv4-mapped ipv6 is unmapped", "::ffff:127.0.0.1", syscall.AF_INET6, "127.0.0.1", true},
		{"garbage is rejected", "not-an-ip", syscall.AF_INET, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseListenIP(tt.ip, tt.family)
			if ok != tt.wantOK || (ok && got.String() != tt.want) {
				t.Errorf("parseListenIP(%q, %d) = %v, %v, want %s, %v", tt.ip, tt.family, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
