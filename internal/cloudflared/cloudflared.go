// Package cloudflared opens Cloudflare quick tunnels by running cloudflared.
package cloudflared

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os/exec"
	"time"
)

// ClientIPHeader is the request header in which Cloudflare reports the visitor's address.
const ClientIPHeader = "CF-Connecting-IP"

const (
	defaultExecutable = "cloudflared"
	loopbackAnyPort   = "127.0.0.1:0"
	startTimeout      = 30 * time.Second
	pollInterval      = 250 * time.Millisecond
	pollTimeout       = 2 * time.Second
)

// Backend opens quick tunnels. The zero value runs the cloudflared found on PATH.
type Backend struct {
	// executable replaces the cloudflared executable.
	// Tests set it to run a fake.
	executable string
}

// Tunnel is a running quick tunnel.
type Tunnel struct {
	url     *url.URL
	exited  chan struct{}
	exitErr error
}

// Open starts cloudflared in front of local and returns once the tunnel accepts traffic.
// The tunnel runs until ctx is done. The error wraps [exec.ErrNotFound] when cloudflared is not installed.
func (b Backend) Open(ctx context.Context, local netip.AddrPort) (*Tunnel, error) {
	path, err := exec.LookPath(cmp.Or(b.executable, defaultExecutable))
	if err != nil {
		return nil, fmt.Errorf("find cloudflared: %w", err)
	}
	metrics, err := freeLoopbackAddr(ctx)
	if err != nil {
		return nil, err
	}

	runCtx, stop := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, path, "tunnel", "--no-autoupdate", "--url", "http://"+local.String(), "--metrics", metrics.String())
	stderr := &tail{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		stop()
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}

	t := &Tunnel{exited: make(chan struct{})}
	go func() {
		defer close(t.exited)
		defer stop()
		if err := cmd.Wait(); runCtx.Err() == nil {
			t.exitErr = exitError(err, stderr)
		}
	}()

	hostname, err := awaitReady(runCtx, metrics, t.exited)
	if err != nil {
		stop()
		return nil, cmp.Or(t.Wait(), err)
	}
	t.url = &url.URL{Scheme: "https", Host: hostname}
	return t, nil
}

// URL returns the public address of the tunnel.
func (t *Tunnel) URL() *url.URL {
	return t.url
}

// Wait blocks until cloudflared has exited.
// It returns nil when the context ended the tunnel and an error when cloudflared stopped by itself.
func (t *Tunnel) Wait() error {
	<-t.exited
	return t.exitErr
}

func freeLoopbackAddr(ctx context.Context) (netip.AddrPort, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", loopbackAnyPort)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("find a free port for cloudflared metrics: %w", err)
	}
	addr, err := netip.ParseAddrPort(ln.Addr().String())
	return addr, errors.Join(err, ln.Close())
}

func awaitReady(ctx context.Context, metrics netip.AddrPort, exited <-chan struct{}) (hostname string, err error) {
	ctx, cancel := context.WithTimeout(ctx, startTimeout)
	defer cancel()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	client := &http.Client{Timeout: pollTimeout, Transport: &http.Transport{DisableKeepAlives: true}}
	metricsURL := "http://" + metrics.String()

	for {
		if hostname, ok := quickTunnelHostname(ctx, client, metricsURL); ok && isReady(ctx, client, metricsURL) {
			return hostname, nil
		}
		select {
		case <-exited:
			return "", errors.New("cloudflared exited before the tunnel was ready")
		case <-ctx.Done():
			return "", fmt.Errorf("wait for cloudflared to connect: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func quickTunnelHostname(ctx context.Context, client *http.Client, metricsURL string) (hostname string, ok bool) {
	resp, err := get(ctx, client, metricsURL+"/quicktunnel")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()

	var body struct {
		Hostname string `json:"hostname"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", false
	}
	return body.Hostname, body.Hostname != ""
}

func isReady(ctx context.Context, client *http.Client, metricsURL string) bool {
	resp, err := get(ctx, client, metricsURL+"/ready")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func get(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func exitError(waitErr error, stderr *tail) error {
	if last := stderr.lastLine(); last != "" {
		return fmt.Errorf("cloudflared exited: %s", last)
	}
	if waitErr != nil {
		return fmt.Errorf("cloudflared exited: %w", waitErr)
	}
	return errors.New("cloudflared exited")
}
