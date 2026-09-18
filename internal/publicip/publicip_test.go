package publicip

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

const trace = "fl=962f16\nh=www.cloudflare.com\nip=203.0.113.42\nts=1789712054.000\nvisit_scheme=https\n"

func traceServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write trace: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestParseTrace(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    netip.Addr
		wantErr bool
	}{
		{"ipv4", trace, netip.MustParseAddr("203.0.113.42"), false},
		{"ipv6 with windows line ends", "h=www.cloudflare.com\r\nip=2001:db8::1a2b\r\n", netip.MustParseAddr("2001:db8::1a2b"), false},
		{"no ip line", "h=www.cloudflare.com\nts=1789712054.000\n", netip.Addr{}, true},
		{"unparsable ip", "ip=not-an-address\n", netip.Addr{}, true},
		{"empty body", "", netip.Addr{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrace(tt.body)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("parseTrace(%q) = %v, %v, want %v, error %v", tt.body, got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestFetch(t *testing.T) {
	t.Run("reads the address from the trace", func(t *testing.T) {
		srv := traceServer(t, trace, http.StatusOK)
		got, err := fetch(t.Context(), newClient("tcp4"), srv.URL)
		if err != nil || got != netip.MustParseAddr("203.0.113.42") {
			t.Errorf("fetch() = %v, %v, want 203.0.113.42", got, err)
		}
	})

	t.Run("a failed status is an error", func(t *testing.T) {
		srv := traceServer(t, trace, http.StatusServiceUnavailable)
		if _, err := fetch(t.Context(), newClient("tcp4"), srv.URL); err == nil || !strings.Contains(err.Error(), "503") {
			t.Errorf("fetch() error = %v, want the status in it", err)
		}
	})

	t.Run("an oversized body is cut before parsing", func(t *testing.T) {
		srv := traceServer(t, strings.Repeat("x=y\n", 2000)+trace, http.StatusOK)
		if _, err := fetch(t.Context(), newClient("tcp4"), srv.URL); err == nil {
			t.Errorf("fetch() error = nil, want the ip line beyond the size limit to be missed")
		}
	})

	t.Run("the client speaks only the network it was made for", func(t *testing.T) {
		srv := traceServer(t, trace, http.StatusOK)
		if _, err := fetch(t.Context(), newClient("tcp6"), srv.URL); err == nil {
			t.Errorf("fetch() over tcp6 reached an IPv4 server, want an error")
		}
	})
}

func TestLookup(t *testing.T) {
	t.Run("returns the addresses that could be found", func(t *testing.T) {
		srv := traceServer(t, trace, http.StatusOK)
		got, err := lookup(t.Context(), srv.URL)
		if err != nil || len(got) != 1 || got[0] != netip.MustParseAddr("203.0.113.42") {
			t.Errorf("lookup() = %v, %v, want just the IPv4 address of an IPv4-only server", got, err)
		}
	})

	t.Run("fails only when no address could be found", func(t *testing.T) {
		srv := traceServer(t, "h=www.cloudflare.com\n", http.StatusOK)
		if got, err := lookup(t.Context(), srv.URL); err == nil || got != nil {
			t.Errorf("lookup() = %v, %v, want nil and an error", got, err)
		}
	})
}
