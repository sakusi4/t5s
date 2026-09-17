package discovery

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

const probeTimeout = time.Second

type probeResult struct {
	header http.Header
	web    bool
	retry  bool
}

func newProbeClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func probe(ctx context.Context, client *http.Client, addr netip.AddrPort) probeResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr.String()+"/", nil)
	if err != nil {
		return probeResult{}
	}
	resp, err := client.Do(req)
	if err != nil {
		var urlErr *url.Error
		return probeResult{retry: errors.As(err, &urlErr) && urlErr.Timeout()}
	}
	defer resp.Body.Close()
	return probeResult{header: resp.Header, web: true}
}

func probeAll(ctx context.Context, addrs []netip.AddrPort) []probeResult {
	client := newProbeClient(probeTimeout)
	results := make([]probeResult, len(addrs))
	var wg sync.WaitGroup
	for i, addr := range addrs {
		wg.Go(func() {
			results[i] = probe(ctx, client, addr)
		})
	}
	wg.Wait()
	return results
}
