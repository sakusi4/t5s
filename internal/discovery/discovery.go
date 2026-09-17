// Package discovery finds web services listening on local TCP ports.
package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"os"
)

const dockerFramework = "Docker"

// Service is a web service listening on a local TCP port.
type Service struct {
	Addr      netip.AddrPort
	PID       int32
	Process   string
	Name      string
	Framework string
}

// Scanner finds local web services and probes each listener only once.
type Scanner struct {
	probed map[listener]probeResult
}

type candidate struct {
	listener listener
	process  processInfo
}

// Scan returns the web services listening on this machine, ordered by port.
// It must not be called concurrently.
func (s *Scanner) Scan(ctx context.Context) ([]Service, error) {
	listeners, err := listListeners(ctx)
	if err != nil {
		return nil, fmt.Errorf("list listening sockets: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}

	var candidates []candidate
	for _, l := range dedupeByPort(listeners) {
		p := lookupProcess(ctx, l.pid)
		if !isIgnored(p.name) {
			candidates = append(candidates, candidate{listener: l, process: p})
		}
	}

	probes := s.probeOnce(ctx, candidates)
	containers := containerNames(ctx, dockerSockets(home))
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("scan local services: %w", err)
	}

	var services []Service
	for _, c := range candidates {
		if p := probes[c.listener]; p.web {
			services = append(services, newService(c, p.header, containers[c.listener.addr.Port()], home))
		}
	}
	return services, nil
}

func (s *Scanner) probeOnce(ctx context.Context, candidates []candidate) map[listener]probeResult {
	var unknown []listener
	var addrs []netip.AddrPort
	results := make(map[listener]probeResult, len(candidates))
	for _, c := range candidates {
		if known, ok := s.probed[c.listener]; ok {
			results[c.listener] = known
			continue
		}
		unknown = append(unknown, c.listener)
		addrs = append(addrs, dialAddr(c.listener.addr))
	}
	for i, r := range probeAll(ctx, addrs) {
		results[unknown[i]] = r
	}

	s.probed = make(map[listener]probeResult, len(results))
	for l, r := range results {
		if !r.retry {
			s.probed[l] = r
		}
	}
	return results
}

func newService(c candidate, header http.Header, containerName, home string) Service {
	s := Service{
		Addr:      dialAddr(c.listener.addr),
		PID:       c.listener.pid,
		Process:   c.process.name,
		Name:      inferName(c.process, home),
		Framework: inferFramework(c.process, header),
	}
	if containerName == "" {
		return s
	}
	s.Name = containerName
	if s.Framework == "" {
		s.Framework = dockerFramework
	}
	return s
}
