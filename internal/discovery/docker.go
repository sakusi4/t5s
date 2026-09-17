package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

const dockerTimeout = time.Second

type container struct {
	Names []string
	Ports []containerPort
}

type containerPort struct {
	PublicPort uint16
	Type       string
}

func dockerSockets(home string) []string {
	return []string{
		"/var/run/docker.sock",
		filepath.Join(home, ".docker", "run", "docker.sock"),
	}
}

func containerNames(ctx context.Context, sockets []string) map[uint16]string {
	for _, socket := range sockets {
		containers, err := fetchContainers(ctx, newUnixClient(socket), "http://docker")
		if err == nil {
			return namesByPort(containers)
		}
	}
	return map[uint16]string{}
}

func newUnixClient(socket string) *http.Client {
	return &http.Client{
		Timeout: dockerTimeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
}

func fetchContainers(ctx context.Context, client *http.Client, baseURL string) ([]container, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/containers/json", nil)
	if err != nil {
		return nil, fmt.Errorf("build docker request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list docker containers: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list docker containers: status %d", resp.StatusCode)
	}
	var containers []container
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("decode docker containers: %w", err)
	}
	return containers, nil
}

func namesByPort(containers []container) map[uint16]string {
	names := make(map[uint16]string)
	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		for _, p := range c.Ports {
			if p.Type == "tcp" && p.PublicPort != 0 {
				names[p.PublicPort] = strings.TrimPrefix(c.Names[0], "/")
			}
		}
	}
	return names
}
