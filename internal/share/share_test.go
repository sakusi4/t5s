package share

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sakusi4/t5s/internal/acl"
)

const (
	clientIPHeader = "X-Fake-Client-IP"
	markerHeader   = "X-T5s-Test"
	allowedIP      = "203.0.113.42"
	blockedIP      = "198.51.100.7"
	publicURL      = "https://fake-share.example.test"
	hour           = time.Hour
)

type fakeTunnel struct {
	exited chan struct{}
	err    error
}

func (f *fakeTunnel) URL() *url.URL {
	return &url.URL{Scheme: "https", Host: strings.TrimPrefix(publicURL, "https://")}
}

func (f *fakeTunnel) Wait() error {
	<-f.exited
	return f.err
}

type fakeBackend struct {
	openErr error
	opened  chan netip.AddrPort
	lose    chan error
	tunnel  *fakeTunnel
	hold    chan struct{}
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{opened: make(chan netip.AddrPort, 1), lose: make(chan error, 1)}
}

func (f *fakeBackend) backend() Backend {
	return Backend{ClientIPHeader: clientIPHeader, Open: f.open}
}

func (f *fakeBackend) open(ctx context.Context, local netip.AddrPort) (Tunnel, error) {
	f.opened <- local
	if f.openErr != nil {
		return nil, f.openErr
	}
	tunnel := &fakeTunnel{exited: make(chan struct{})}
	f.tunnel = tunnel
	go func() {
		defer close(tunnel.exited)
		select {
		case <-ctx.Done():
		case tunnel.err = <-f.lose:
		}
		if f.hold != nil {
			<-f.hold
		}
	}()
	return tunnel, nil
}

func startTarget(t *testing.T) netip.AddrPort {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(markerHeader) != "" {
			fmt.Fprint(w, "hello from the target")
		}
	}))
	t.Cleanup(srv.Close)
	return netip.MustParseAddrPort(srv.Listener.Addr().String())
}

func request(t *testing.T, target netip.AddrPort, expires time.Duration) Request {
	t.Helper()
	allow, err := acl.Parse([]string{allowedIP})
	if err != nil {
		t.Fatalf("acl.Parse() error = %v", err)
	}
	return Request{Target: target, Allow: allow, Expires: expires}
}

func start(ctx context.Context, t *testing.T, fake *fakeBackend, req Request) *Share {
	t.Helper()
	s, err := Start(ctx, fake.backend(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		s.Stop()
		<-s.ended
	})
	return s
}

func get(t *testing.T, addr netip.AddrPort, clientIP string) (status int, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr.String()+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set(markerHeader, t.Name())
	req.Header.Set(clientIPHeader, clientIP)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s error = %v", addr, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	return resp.StatusCode, string(raw)
}

func tunnelIsClosed(f *fakeBackend) bool {
	select {
	case <-f.tunnel.exited:
		return true
	default:
		return false
	}
}

func refusesConnections(t *testing.T, addr netip.AddrPort) bool {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", addr.String())
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

func TestStart(t *testing.T) {
	t.Run("opens the tunnel in front of a proxy that guards the target", func(t *testing.T) {
		fake := newFakeBackend()
		target := startTarget(t)
		s := start(t.Context(), t, fake, request(t, target, hour))
		proxyAddr := <-fake.opened

		if got := s.URL().String(); got != publicURL {
			t.Errorf("URL() = %q, want %q", got, publicURL)
		}
		if s.Target() != target {
			t.Errorf("Target() = %s, want %s", s.Target(), target)
		}
		if !proxyAddr.Addr().IsLoopback() || proxyAddr == target {
			t.Errorf("tunnel opened in front of %s, want a loopback proxy and not the target %s", proxyAddr, target)
		}
		if status, body := get(t, proxyAddr, allowedIP); status != http.StatusOK || body != "hello from the target" {
			t.Errorf("allowed GET = %d %q, want 200 from the target", status, body)
		}
		if status, _ := get(t, proxyAddr, blockedIP); status != http.StatusForbidden {
			t.Errorf("blocked GET = %d, want 403", status)
		}
	})

	t.Run("expires after the requested time", func(t *testing.T) {
		before := time.Now()
		s := start(t.Context(), t, newFakeBackend(), request(t, startTarget(t), hour))
		after := time.Now()

		if got := s.ExpiresAt(); got.Before(before.Add(hour)) || got.After(after.Add(hour)) {
			t.Errorf("ExpiresAt() = %s, want between %s and %s", got, before.Add(hour), after.Add(hour))
		}
	})

	t.Run("reports requests seen by the proxy", func(t *testing.T) {
		fake := newFakeBackend()
		s := start(t.Context(), t, fake, request(t, startTarget(t), hour))
		get(t, <-fake.opened, blockedIP)

		accesses, _ := s.Accesses()
		for _, a := range accesses {
			if a.Addr.String() == blockedIP && !a.Allowed && a.Requests == 1 {
				return
			}
		}
		t.Errorf("Accesses() = %+v, want %s blocked with 1 request", accesses, blockedIP)
	})

	t.Run("failure to open the tunnel stops the proxy", func(t *testing.T) {
		fake := newFakeBackend()
		fake.openErr = errors.New("cloudflared exited: 429 Too Many Requests")

		_, err := Start(t.Context(), fake.backend(), request(t, startTarget(t), hour))
		if err == nil || !strings.Contains(err.Error(), "429 Too Many Requests") {
			t.Errorf("Start() error = %v, want the reason the tunnel failed", err)
		}
		if proxyAddr := <-fake.opened; !refusesConnections(t, proxyAddr) {
			t.Errorf("proxy still listens on %s, want it stopped", proxyAddr)
		}
	})
}

func TestShare_Wait(t *testing.T) {
	t.Run("returns nil after Stop and everything is down", func(t *testing.T) {
		fake := newFakeBackend()
		s := start(t.Context(), t, fake, request(t, startTarget(t), hour))
		proxyAddr := <-fake.opened

		s.Stop()
		if err := s.Wait(); err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
		if !refusesConnections(t, proxyAddr) {
			t.Errorf("proxy still listens on %s, want it stopped", proxyAddr)
		}
		if !tunnelIsClosed(fake) {
			t.Errorf("tunnel is still open after Wait, want it closed")
		}
	})

	t.Run("does not return while the tunnel is still shutting down", func(t *testing.T) {
		fake := newFakeBackend()
		fake.hold = make(chan struct{})
		s := start(t.Context(), t, fake, request(t, startTarget(t), hour))

		s.Stop()
		if err := s.proxy.Wait(); err != nil {
			t.Fatalf("proxy.Wait() error = %v", err)
		}
		select {
		case <-s.ended:
			t.Errorf("Wait() returned while the tunnel was still open")
		default:
		}
		close(fake.hold)
		if err := s.Wait(); err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
	})

	t.Run("returns nil when the parent context ends", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		s := start(ctx, t, newFakeBackend(), request(t, startTarget(t), hour))

		cancel()
		if err := s.Wait(); err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
	})

	t.Run("returns ErrExpired when the time runs out", func(t *testing.T) {
		fake := newFakeBackend()
		s := start(t.Context(), t, fake, request(t, startTarget(t), 100*time.Millisecond))
		proxyAddr := <-fake.opened

		if err := s.Wait(); !errors.Is(err, ErrExpired) {
			t.Errorf("Wait() error = %v, want ErrExpired", err)
		}
		if !refusesConnections(t, proxyAddr) || !tunnelIsClosed(fake) {
			t.Errorf("proxy refuses = %v, tunnel closed = %v, want both true", refusesConnections(t, proxyAddr), tunnelIsClosed(fake))
		}
	})

	t.Run("reports a lost tunnel and stops the proxy", func(t *testing.T) {
		fake := newFakeBackend()
		s := start(t.Context(), t, fake, request(t, startTarget(t), hour))
		proxyAddr := <-fake.opened

		fake.lose <- errors.New("cloudflared exited: connection to the edge was lost")
		err := s.Wait()
		if err == nil || errors.Is(err, ErrExpired) || !strings.Contains(err.Error(), "connection to the edge was lost") {
			t.Errorf("Wait() error = %v, want the reason the tunnel was lost", err)
		}
		if !refusesConnections(t, proxyAddr) {
			t.Errorf("proxy still listens on %s, want it stopped", proxyAddr)
		}
	})
}
