package discovery

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestScan(t *testing.T) {
	t.Run("finds a running http server with its process", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Powered-By", "Express")
		}))
		t.Cleanup(srv.Close)
		want := serverAddr(t, srv)

		var scanner Scanner
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
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			requests.Add(1)
		}))
		t.Cleanup(srv.Close)

		var scanner Scanner
		for range 2 {
			if _, err := scanner.Scan(t.Context()); err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
		}
		if got := requests.Load(); got != 1 {
			t.Errorf("server received %d requests, want 1", got)
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
