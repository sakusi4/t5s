package discovery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

const rememberedHeader = "X-Remembered"

func probedAt(s *Scanner, addr netip.AddrPort) (listener, bool) {
	for l := range s.probed {
		if dialAddr(l.addr) == addr {
			return l, true
		}
	}
	return listener{}, false
}

func TestScan(t *testing.T) {
	t.Run("finds a running http server with its process", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Powered-By", "Express")
		}))
		t.Cleanup(srv.Close)
		want := serverAddr(t, srv)

		scanner := Scanner{includeOwnListeners: true}
		services, err := scanner.Scan(t.Context())
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		for _, s := range services {
			if s.Addr != want {
				continue
			}
			if s.Framework != "Express" || s.Process == "" || s.PID == 0 {
				t.Errorf("Scan() found %+v, want framework Express with process details", s)
			}
			return
		}
		t.Errorf("Scan() = %+v, want a service at %s", services, want)
	})

	t.Run("a listener is probed only once across scans", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)
		own := serverAddr(t, srv)

		scanner := Scanner{includeOwnListeners: true}
		if _, err := scanner.Scan(t.Context()); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		l, ok := probedAt(&scanner, own)
		if !ok {
			t.Fatalf("first Scan() did not probe %s", own)
		}
		scanner.probed[l] = probeResult{web: true, header: http.Header{rememberedHeader: {"yes"}}}

		if _, err := scanner.Scan(t.Context()); err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if scanner.probed[l].header.Get(rememberedHeader) == "" {
			t.Errorf("second Scan() probed %s again, want the remembered result kept", own)
		}
	})

	t.Run("listeners of this process are neither listed nor probed", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)
		own := serverAddr(t, srv)

		var scanner Scanner
		services, err := scanner.Scan(t.Context())
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		for _, s := range services {
			if s.Addr == own {
				t.Errorf("Scan() lists %+v, want listeners of this process skipped", s)
			}
		}
		if _, ok := probedAt(&scanner, own); ok {
			t.Errorf("Scan() probed %s, want listeners of this process skipped", own)
		}
	})

	t.Run("canceled context is an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		var scanner Scanner
		if _, err := scanner.Scan(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("Scan() error = %v, want context.Canceled", err)
		}
	})
}

func TestNewService(t *testing.T) {
	c := candidate{process: processInfo{name: "com.docker.backend"}}

	t.Run("container name replaces the inferred name", func(t *testing.T) {
		got := newService(c, http.Header{}, "shop-web", "/Users/dev")
		if got.Name != "shop-web" || got.Framework != "Docker" {
			t.Errorf("newService() = %+v, want name shop-web and framework Docker", got)
		}
	})

	t.Run("detected framework is kept for a container", func(t *testing.T) {
		got := newService(c, http.Header{"Server": {"nginx"}}, "shop-web", "/Users/dev")
		if got.Framework != "nginx" {
			t.Errorf("newService() framework = %q, want nginx", got.Framework)
		}
	})
}
