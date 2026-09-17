// Package proxy serves a local web service to allowed client addresses only.
package proxy

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"time"

	"github.com/sakusi4/t5s/internal/acl"
)

const (
	listenAddr           = "127.0.0.1:0"
	readHeaderTimeout    = 10 * time.Second
	forwardedForHeader   = "X-Forwarded-For"
	forwardedHostHeader  = "X-Forwarded-Host"
	forwardedProtoHeader = "X-Forwarded-Proto"
	defaultProto         = "http"
	unreachableMessage   = "The local service is not responding."
)

// Config describes one shared service.
type Config struct {
	Target         netip.AddrPort
	Allow          acl.List
	ClientIPHeader string
	KeepHost       bool
}

// Proxy is a running reverse proxy in front of one local service.
type Proxy struct {
	addr     netip.AddrPort
	log      *accessLog
	served   chan struct{}
	serveErr error
}

// Start listens on a loopback port and serves until ctx is done.
// A deadline on ctx is the expiration of the share.
func Start(ctx context.Context, cfg Config) (*Proxy, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	addr, err := netip.ParseAddrPort(ln.Addr().String())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("parse listen address: %w", err), ln.Close())
	}

	p := &Proxy{addr: addr, log: &accessLog{}, served: make(chan struct{})}
	srv := &http.Server{
		Handler:           guard(cfg.Allow, cfg.ClientIPHeader, p.log)(forward(cfg)),
		ReadHeaderTimeout: readHeaderTimeout,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	stop := context.AfterFunc(ctx, func() { srv.Close() })
	go func() {
		defer close(p.served)
		defer stop()
		if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			p.serveErr = err
		}
	}()
	return p, nil
}

// Addr returns the loopback address the proxy listens on.
func (p *Proxy) Addr() netip.AddrPort {
	return p.addr
}

// Wait blocks until the proxy has stopped and returns why it stopped early, if it did.
func (p *Proxy) Wait() error {
	<-p.served
	return p.serveErr
}

// Accesses returns the client addresses seen so far, most recent first,
// and the number of requests from addresses that were not tracked.
func (p *Proxy) Accesses() (accesses []Access, untracked int) {
	return p.log.snapshot()
}

func forward(cfg Config) http.Handler {
	target := &url.URL{Scheme: "http", Host: cfg.Target.String()}
	localHost := fmt.Sprintf("localhost:%d", cfg.Target.Port())
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = localHost
			if cfg.KeepHost {
				r.Out.Host = r.In.Host
			}
			r.Out.Header.Set(forwardedForHeader, clientAddr(r.In, cfg.ClientIPHeader).String())
			r.Out.Header.Set(forwardedHostHeader, r.In.Host)
			r.Out.Header.Set(forwardedProtoHeader, cmp.Or(r.In.Header.Get(forwardedProtoHeader), defaultProto))
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, unreachableMessage, http.StatusBadGateway)
		},
	}
}
