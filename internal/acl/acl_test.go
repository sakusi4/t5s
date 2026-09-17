package acl

import (
	"net/netip"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		wantErr string
	}{
		{"single ipv4", []string{"203.0.113.42"}, ""},
		{"single ipv6", []string{"2001:db8::1a2b"}, ""},
		{"cidr ranges", []string{"10.0.0.0/8", "2001:db8::/32"}, ""},
		{"surrounding spaces are ignored", []string{" 203.0.113.42 "}, ""},
		{"empty list", nil, "allowlist is empty"},
		{"blank entry", []string{""}, `parse allowlist entry ""`},
		{"not an address", []string{"example.com"}, `parse allowlist entry "example.com"`},
		{"bad prefix length", []string{"10.0.0.0/33"}, `parse allowlist entry "10.0.0.0/33"`},
		{"ipv4 range that allows everyone", []string{"0.0.0.0/0"}, `"0.0.0.0/0" allows every address`},
		{"ipv6 range that allows everyone", []string{"::/0"}, `"::/0" allows every address`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.entries)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Parse(%q) error = %v, want nil", tt.entries, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse(%q) error = %v, want it to contain %q", tt.entries, err, tt.wantErr)
			}
		})
	}
}

func TestList_Allows(t *testing.T) {
	list, err := Parse([]string{"203.0.113.42", "10.1.2.3/8", "2001:db8::/32"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"exact address", "203.0.113.42", true},
		{"neighbor of an exact address", "203.0.113.43", false},
		{"inside a range written with host bits", "10.200.0.1", true},
		{"outside the range", "11.0.0.1", false},
		{"inside an ipv6 range", "2001:db8::1a2b", true},
		{"outside the ipv6 range", "2001:db9::1", false},
		{"ipv4-mapped ipv6 form of an allowed address", "::ffff:203.0.113.42", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := list.Allows(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Errorf("Allows(%s) = %v, want %v", tt.addr, got, tt.want)
			}
		})
	}

	t.Run("zero value allows nothing", func(t *testing.T) {
		var zero List
		if zero.Allows(netip.MustParseAddr("127.0.0.1")) {
			t.Errorf("zero List allows 127.0.0.1, want it to allow nothing")
		}
	})
}
