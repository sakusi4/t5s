// Package share exposes one local web service through a tunnel to allowed addresses only.
package share

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"time"

	"github.com/sakusi4/t5s/internal/acl"
	"github.com/sakusi4/t5s/internal/proxy"
)

// ErrExpired is returned by [Share.Wait] when the share ended because its time ran out.
var ErrExpired = errors.New("share expired")

// Tunnel is an open public entry point.
type Tunnel interface {
	URL() *url.URL
	Wait() error
}

// Backend opens tunnels and names the header in which it reports the visitor's address.
type Backend struct {
	Open           func(ctx context.Context, local netip.AddrPort) (Tunnel, error)
	ClientIPHeader string
}

// Request describes what to share, with whom, and for how long.
type Request struct {
	Target  netip.AddrPort
	Allow   acl.List
	Expires time.Duration
}

// Share is a running share.
type Share struct {
	target    netip.AddrPort
	url       *url.URL
	expiresAt time.Time
	proxy     *proxy.Proxy
	stop      context.CancelFunc
	ended     chan struct{}
	endErr    error
}

// Start puts a guarded proxy in front of the target and opens a tunnel to the proxy.
// The share runs until it expires, Stop is called, ctx is done, or the tunnel is lost.
func Start(ctx context.Context, backend Backend, req Request) (*Share, error) {
	expiresAt := time.Now().Add(req.Expires)
	ctx, stop := context.WithDeadline(ctx, expiresAt)

	p, err := proxy.Start(ctx, proxy.Config{Target: req.Target, Allow: req.Allow, ClientIPHeader: backend.ClientIPHeader})
	if err != nil {
		stop()
		return nil, fmt.Errorf("start proxy: %w", err)
	}
	tunnel, err := backend.Open(ctx, p.Addr())
	if err != nil {
		stop()
		return nil, errors.Join(fmt.Errorf("open tunnel: %w", err), p.Wait())
	}

	s := &Share{target: req.Target, url: tunnel.URL(), expiresAt: expiresAt, proxy: p, stop: stop, ended: make(chan struct{})}
	go s.supervise(ctx, tunnel)
	return s, nil
}

// Target returns the local service being shared.
func (s *Share) Target() netip.AddrPort {
	return s.target
}

// URL returns the public address of the share.
func (s *Share) URL() *url.URL {
	return s.url
}

// ExpiresAt returns when the share ends by itself.
func (s *Share) ExpiresAt() time.Time {
	return s.expiresAt
}

// Accesses returns the client addresses seen so far, most recent first,
// and the number of requests from addresses that were not tracked.
func (s *Share) Accesses() (accesses []proxy.Access, untracked int) {
	return s.proxy.Accesses()
}

// Stop ends the share.
func (s *Share) Stop() {
	s.stop()
}

// Wait blocks until the proxy and the tunnel have stopped. It returns nil after Stop
// or the end of the context, [ErrExpired] after the expiration, and the reason otherwise.
func (s *Share) Wait() error {
	<-s.ended
	return s.endErr
}

func (s *Share) supervise(ctx context.Context, tunnel Tunnel) {
	defer close(s.ended)

	tunnelDone := make(chan error, 1)
	go func() {
		err := tunnel.Wait()
		s.stop()
		tunnelDone <- err
	}()

	proxyErr := s.proxy.Wait()
	s.stop()
	tunnelErr := <-tunnelDone

	switch {
	case tunnelErr != nil:
		s.endErr = fmt.Errorf("tunnel lost: %w", tunnelErr)
	case proxyErr != nil:
		s.endErr = fmt.Errorf("proxy stopped: %w", proxyErr)
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		s.endErr = ErrExpired
	}
}
