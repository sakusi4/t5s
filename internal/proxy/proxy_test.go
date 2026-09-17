package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
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
	markerHeader   = "X-T5s-Test"
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

func newRequest(t *testing.T, p *Proxy, path, clientIP string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+p.Addr().String()+path, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = publicHost
	req.Header.Set(markerHeader, t.Name())
	if clientIP != "" {
		req.Header.Set(clientIPHeader, clientIP)
	}
	return req
}

func fromThisTest(r *http.Request) bool {
	return r.Header.Get(markerHeader) != ""
}

func findAccess(accesses []Access, addr string) (access Access, position int, ok bool) {
	for i, a := range accesses {
		if a.Addr.String() == addr {
			return a, i, true
		}
	}
	return Access{}, 0, false
}

func stray(t *testing.T, addr netip.AddrPort) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr.String()+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("stray GET %s error = %v", addr, err)
	}
	resp.Body.Close()
}

func get(t *testing.T, p *Proxy, clientIP string) (status int, body string) {
	t.Helper()
	req := newRequest(t, p, "/", clientIP)
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
	target := startUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fromThisTest(r) {
			return
		}
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
			stray(t, target)
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
		if fromThisTest(r) {
			requests <- r
		}
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
			stray(t, target)
			req := newRequest(t, p, "/", allowedIP)
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

func upgradeHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok || r.Header.Get("Upgrade") != "websocket" {
			http.Error(w, "expected an upgrade request", http.StatusBadRequest)
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		defer conn.Close()
		fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		if err := rw.Flush(); err != nil {
			return
		}
		buf := make([]byte, 64)
		for {
			n, err := rw.Read(buf)
			if err != nil {
				return
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return
			}
		}
	})
}

func dialUpgrade(t *testing.T, p *Proxy) (net.Conn, *bufio.Reader) {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", p.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy error = %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n%s: %s\r\n%s: %s\r\n\r\n", publicHost, clientIPHeader, allowedIP, markerHeader, t.Name())

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read upgrade response error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d, want 101", resp.StatusCode)
	}
	return conn, reader
}

func TestProxy_Upgrade(t *testing.T) {
	target := startUpstream(t, upgradeHandler(t))

	t.Run("bytes flow both ways after the upgrade", func(t *testing.T) {
		p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
		conn, reader := dialUpgrade(t, p)

		fmt.Fprint(conn, "ping")
		echo := make([]byte, 4)
		if _, err := io.ReadFull(reader, echo); err != nil || string(echo) != "ping" {
			t.Errorf("echo = %q, %v, want ping", echo, err)
		}
	})

	t.Run("stopping the proxy cuts an open connection", func(t *testing.T) {
		p, stop := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
		_, reader := dialUpgrade(t, p)

		stop()
		if _, err := reader.ReadByte(); err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
			t.Errorf("read after stop error = %v, want the connection closed", err)
		}
	})
}

func TestProxy_ServerSentEvents(t *testing.T) {
	sendSecond := make(chan struct{})
	target := startUpstream(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: first\n\n")
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush() error = %v", err)
		}
		<-sendSecond
		fmt.Fprint(w, "data: second\n\n")
	}))
	p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})

	req := newRequest(t, p, "/events", allowedIP)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /events error = %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil || first != "data: first\n" {
		t.Fatalf("first event = %q, %v, want it before the second is sent", first, err)
	}
	close(sendSecond)
	rest, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(rest), "data: second") {
		t.Errorf("rest = %q, %v, want the second event", rest, err)
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
		stray(t, p.Addr())
		get(t, p, allowedIP)
		get(t, p, blockedIP)

		accesses, _ := p.Accesses()
		blocked, blockedAt, ok := findAccess(accesses, blockedIP)
		if !ok || blocked.Allowed || blocked.Requests != 1 {
			t.Errorf("access of %s = %+v, want blocked with 1 request", blockedIP, blocked)
		}
		allowed, allowedAt, ok := findAccess(accesses, allowedIP)
		if !ok || !allowed.Allowed || allowed.Requests != 2 || allowed.LastSeen.IsZero() {
			t.Errorf("access of %s = %+v, want allowed with 2 requests", allowedIP, allowed)
		}
		if blockedAt > allowedAt {
			t.Errorf("Accesses() = %+v, want the later %s before %s", accesses, blockedIP, allowedIP)
		}
	})

	t.Run("tracking stops growing at the limit", func(t *testing.T) {
		p, _ := startProxy(t, Config{Target: target, Allow: allow(t, allowedIP), ClientIPHeader: clientIPHeader})
		stray(t, p.Addr())
		base := netip.MustParseAddr("198.18.0.0")
		addr := base
		for range maxTrackedAddrs + 5 {
			addr = addr.Next()
			get(t, p, addr.String())
		}

		accesses, untracked := p.Accesses()
		if len(accesses) != maxTrackedAddrs || untracked < 5 {
			t.Errorf("Accesses() = %d addresses, %d untracked, want %d and at least 5", len(accesses), untracked, maxTrackedAddrs)
		}
	})
}
