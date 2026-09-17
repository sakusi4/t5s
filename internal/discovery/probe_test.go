package discovery

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

const testProbeTimeout = 200 * time.Millisecond

func serverAddr(t *testing.T, srv *httptest.Server) netip.AddrPort {
	t.Helper()
	return netip.MustParseAddrPort(srv.Listener.Addr().String())
}

func TestProbe(t *testing.T) {
	t.Run("any status counts as a web service", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)

		got := probe(t.Context(), newProbeClient(testProbeTimeout), serverAddr(t, srv))
		if !got.web {
			t.Errorf("probe() web = false, want true")
		}
	})

	t.Run("response headers are returned", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-Powered-By", "Express")
		}))
		t.Cleanup(srv.Close)

		got := probe(t.Context(), newProbeClient(testProbeTimeout), serverAddr(t, srv))
		if h := got.header.Get("X-Powered-By"); h != "Express" {
			t.Errorf("probe() X-Powered-By = %q, want %q", h, "Express")
		}
	})

	t.Run("redirect is not followed", func(t *testing.T) {
		followed := false
		mux := http.NewServeMux()
		mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		})
		mux.HandleFunc("/login", func(http.ResponseWriter, *http.Request) {
			followed = true
		})
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		got := probe(t.Context(), newProbeClient(testProbeTimeout), serverAddr(t, srv))
		if !got.web || followed {
			t.Errorf("probe() web = %v, followed = %v, want true, false", got.web, followed)
		}
	})

	t.Run("closed port is not a web service", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		addr := serverAddr(t, srv)
		srv.Close()

		if got := probe(t.Context(), newProbeClient(testProbeTimeout), addr); got.web || got.retry {
			t.Errorf("probe() = %+v, want not web and no retry", got)
		}
	})

	t.Run("listener that answers without http is not probed again", func(t *testing.T) {
		var lc net.ListenConfig
		ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Listen() error = %v", err)
		}
		t.Cleanup(func() { ln.Close() })
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}()

		addr := netip.MustParseAddrPort(ln.Addr().String())
		if got := probe(t.Context(), newProbeClient(testProbeTimeout), addr); got.web || got.retry {
			t.Errorf("probe() = %+v, want not web and no retry", got)
		}
	})

	t.Run("listener that stays silent is probed again", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)

		got := probe(t.Context(), newProbeClient(testProbeTimeout), serverAddr(t, srv))
		if got.web || !got.retry {
			t.Errorf("probe() = %+v, want not web and retry", got)
		}
	})
}

func TestProbeAll(t *testing.T) {
	web := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(web.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closedAddr := serverAddr(t, closed)
	closed.Close()

	got := probeAll(t.Context(), []netip.AddrPort{serverAddr(t, web), closedAddr})
	if len(got) != 2 || !got[0].web || got[1].web {
		t.Errorf("probeAll() = %+v, want [web, not web]", got)
	}
}
