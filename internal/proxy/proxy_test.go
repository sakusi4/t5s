package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sakusi4/t5s/internal/acl"
)

const (
	clientIPHeader = "CF-Connecting-IP"
	allowedIP      = "203.0.113.42"
	blockedIP      = "198.51.100.7"
	publicHost     = "abc-def.trycloudflare.com"
)

func allow(t *testing.T, entries ...string) acl.List {
	t.Helper()
	list, err := acl.Parse(entries)
	if err != nil {
		t.Fatalf("acl.Parse(%q) error = %v", entries, err)
	}
	return list
}

func startUpstream(t *testing.T, handler http.Handler) netip.AddrPort {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return netip.MustParseAddrPort(srv.Listener.Addr().String())
}

func startProxy(t *testing.T, cfg Config) (p *Proxy, stop context.CancelFunc) {
	t.Helper()
	ctx, stop := context.WithCancel(t.Context())
	p, err := Start(ctx, cfg)
	if err != nil {
		stop()
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		stop()
		if err := p.Wait(); err != nil {
			t.Errorf("Wait() error = %v", err)
		}
	})
	return p, stop
}

func get(t *testing.T, p *Proxy, clientIP string) (status int, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+p.Addr().String()+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = publicHost
	if clientIP != "" {
		req.Header.Set(clientIPHeader, clientIP)
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET through proxy error = %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	return resp.StatusCode, string(raw)
}

func TestProxy_AccessControl(t *testing.T) {
	var upstreamCalls atomic.Int32
	target := startUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		fmt.Fprint(w, "hello from upstream")
	}))
	p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})

	t.Run("listens on loopback only", func(t *testing.T) {
		if !p.Addr().Addr().IsLoopback() {
			t.Errorf("Addr() = %s, want a loopback address", p.Addr())
		}
	})

	t.Run("allowed address reaches the service", func(t *testing.T) {
		status, body := get(t, p, allowedIP)
		if status != http.StatusOK || body != "hello from upstream" {
			t.Errorf("GET = %d %q, want 200 from upstream", status, body)
		}
	})

	tests := []struct {
		name     string
		clientIP string
		wantIP   string
	}{
		{"other address is blocked and shown its own address", blockedIP, blockedIP},
		{"request without the header counts as the connecting address", "", "127.0.0.1"},
		{"unparsable header counts as the connecting address", "not-an-ip", "127.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := upstreamCalls.Load()
			status, body := get(t, p, tt.clientIP)
			if status != http.StatusForbidden || !strings.Contains(body, tt.wantIP) {
				t.Errorf("GET = %d %q, want 403 mentioning %s", status, body, tt.wantIP)
			}
			if upstreamCalls.Load() != before {
				t.Errorf("blocked request reached the service")
			}
		})
	}

	t.Run("address zone cannot inject markup into the blocked page", func(t *testing.T) {
		_, body := get(t, p, "fe80::1%<script>alert(1)</script>")
		if strings.Contains(body, "<script>") {
			t.Errorf("blocked page = %q, want no script tag", body)
		}
	})
}

func TestProxy_ForwardedRequest(t *testing.T) {
	requests := make(chan *http.Request, 1)
	target := startUpstream(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests <- r
	}))
	localHost := fmt.Sprintf("localhost:%d", target.Port())

	tests := []struct {
		name         string
		keepHost     bool
		inboundProto string
		wantHost     string
		wantProto    string
	}{
		{"host is rewritten to localhost by default", false, "", localHost, "http"},
		{"host is kept when asked", true, "", publicHost, "http"},
		{"scheme reported by the tunnel is passed on", false, "https", localHost, "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader, KeepHost: tt.keepHost})
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+p.Addr().String()+"/", nil)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			req.Host = publicHost
			req.Header.Set(clientIPHeader, allowedIP)
			req.Header.Set("X-Forwarded-For", "10.9.9.9")
			if tt.inboundProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.inboundProto)
			}
			resp, err := http.DefaultTransport.RoundTrip(req)
			if err != nil {
				t.Fatalf("RoundTrip() error = %v", err)
			}
			resp.Body.Close()
			seen := <-requests

			if seen.Host != tt.wantHost {
				t.Errorf("upstream Host = %q, want %q", seen.Host, tt.wantHost)
			}
			if got := seen.Header.Get("X-Forwarded-For"); got != allowedIP {
				t.Errorf("upstream X-Forwarded-For = %q, want %q", got, allowedIP)
			}
			if got := seen.Header.Get("X-Forwarded-Host"); got != publicHost {
				t.Errorf("upstream X-Forwarded-Host = %q, want %q", got, publicHost)
			}
			if got := seen.Header.Get("X-Forwarded-Proto"); got != tt.wantProto {
				t.Errorf("upstream X-Forwarded-Proto = %q, want %q", got, tt.wantProto)
			}
		})
	}
}

func TestProxy_UnreachableService(t *testing.T) {
	gone := httptest.NewServer(http.NotFoundHandler())
	goneAddr := netip.MustParseAddrPort(gone.Listener.Addr().String())
	gone.Close()

	p, _ := startProxy(t, Config{Target: goneAddr, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
	if status, _ := get(t, p, allowedIP); status != http.StatusBadGateway {
		t.Errorf("GET = %d, want 502", status)
	}
}

func TestProxy_Expiration(t *testing.T) {
	target := startUpstream(t, http.NotFoundHandler())
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	p, err := Start(ctx, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := p.Wait(); err != nil {
		t.Errorf("Wait() error = %v, want nil after the deadline", err)
	}

	var d net.Dialer
	if conn, err := d.DialContext(t.Context(), "tcp", p.Addr().String()); err == nil {
		conn.Close()
		t.Errorf("dial after expiration succeeded, want the listener closed")
	}
}

func TestProxy_Accesses(t *testing.T) {
	target := startUpstream(t, http.NotFoundHandler())

	t.Run("requests are counted per address, most recent first", func(t *testing.T) {
		p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
		get(t, p, allowedIP)
		get(t, p, allowedIP)
		get(t, p, blockedIP)

		accesses, untracked := p.Accesses()
		if len(accesses) != 2 || untracked != 0 {
			t.Fatalf("Accesses() = %+v, %d, want 2 addresses and 0 untracked", accesses, untracked)
		}
		latest, earlier := accesses[0], accesses[1]
		if latest.Addr.String() != blockedIP || latest.Allowed || latest.Requests != 1 {
			t.Errorf("latest = %+v, want %s blocked with 1 request", latest, blockedIP)
		}
		if earlier.Addr.String() != allowedIP || !earlier.Allowed || earlier.Requests != 2 || earlier.LastSeen.IsZero() {
			t.Errorf("earlier = %+v, want %s allowed with 2 requests", earlier, allowedIP)
		}
	})

	t.Run("tracking stops growing at the limit", func(t *testing.T) {
		p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
		base := netip.MustParseAddr("198.18.0.0")
		addr := base
		for range maxTrackedAddrs + 5 {
			addr = addr.Next()
			get(t, p, addr.String())
		}

		accesses, untracked := p.Accesses()
		if len(accesses) != maxTrackedAddrs || untracked != 5 {
			t.Errorf("Accesses() = %d addresses, %d untracked, want %d and 5", len(accesses), untracked, maxTrackedAddrs)
		}
	})
}
