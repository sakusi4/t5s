// Package publicip finds the public addresses of this machine as Cloudflare sees them.
package publicip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const (
	traceURL       = "https://www.cloudflare.com/cdn-cgi/trace"
	requestTimeout = 5 * time.Second
	maxTraceSize   = 4 << 10
	addrPrefix     = "ip="
)

var networks = []string{"tcp4", "tcp6"}

// Lookup returns the public IPv4 and IPv6 addresses of this machine, in that order,
// leaving out the one that cannot be found. It returns an error only when neither can.
func Lookup(ctx context.Context) ([]netip.Addr, error) {
	return lookup(ctx, traceURL)
}

func lookup(ctx context.Context, url string) ([]netip.Addr, error) {
	var addrs []netip.Addr
	var errs []error
	for _, network := range networks {
		addr, err := fetch(ctx, newClient(network), url)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", network, err))
			continue
		}
		addrs = append(addrs, addr)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("find public address: %w", errors.Join(errs...))
	}
	return addrs, nil
}

func newClient(network string) *http.Client {
	var dialer net.Dialer
	return &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}

func fetch(ctx context.Context, client *http.Client, url string) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("build trace request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("fetch trace: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return netip.Addr{}, fmt.Errorf("fetch trace: %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTraceSize))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("read trace: %w", err)
	}
	return parseTrace(string(body))
}

func parseTrace(body string) (netip.Addr, error) {
	for _, line := range strings.Split(body, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), addrPrefix)
		if !ok {
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return netip.Addr{}, fmt.Errorf("parse trace address: %w", err)
		}
		return addr, nil
	}
	return netip.Addr{}, errors.New("trace has no ip line")
}
