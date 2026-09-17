package discovery

import (
	"net/netip"
	"slices"
	"testing"
)

func TestDialAddr(t *testing.T) {
	tests := []struct {
		name   string
		listen string
		want   string
	}{
		{"ipv4 wildcard becomes ipv4 loopback", "0.0.0.0:3000", "127.0.0.1:3000"},
		{"ipv6 wildcard becomes ipv6 loopback", "[::]:5173", "[::1]:5173"},
		{"ipv4 loopback is kept", "127.0.0.1:8080", "127.0.0.1:8080"},
		{"ipv6 loopback is kept", "[::1]:5173", "[::1]:5173"},
		{"specific address is kept", "192.168.0.5:8080", "192.168.0.5:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dialAddr(netip.MustParseAddrPort(tt.listen))
			if got.String() != tt.want {
				t.Errorf("dialAddr(%s) = %s, want %s", tt.listen, got, tt.want)
			}
		})
	}
}

func TestDedupeByPort(t *testing.T) {
	tests := []struct {
		name string
		in   []listener
		want []listener
	}{
		{
			name: "ipv4 replaces ipv6 on the same port",
			in: []listener{
				{netip.MustParseAddrPort("[::1]:3000"), 10},
				{netip.MustParseAddrPort("127.0.0.1:3000"), 10},
			},
			want: []listener{{netip.MustParseAddrPort("127.0.0.1:3000"), 10}},
		},
		{
			name: "first process wins among workers sharing a port",
			in: []listener{
				{netip.MustParseAddrPort("127.0.0.1:80"), 885},
				{netip.MustParseAddrPort("127.0.0.1:80"), 886},
			},
			want: []listener{{netip.MustParseAddrPort("127.0.0.1:80"), 885}},
		},
		{
			name: "result is ordered by port",
			in: []listener{
				{netip.MustParseAddrPort("127.0.0.1:8080"), 2},
				{netip.MustParseAddrPort("[::1]:5173"), 1},
			},
			want: []listener{
				{netip.MustParseAddrPort("[::1]:5173"), 1},
				{netip.MustParseAddrPort("127.0.0.1:8080"), 2},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeByPort(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Errorf("dedupeByPort() = %v, want %v", got, tt.want)
			}
		})
	}
}
