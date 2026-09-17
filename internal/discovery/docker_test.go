package discovery

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
)

const containersJSON = `[
	{"Names": ["/shop-web"], "Ports": [
		{"IP": "0.0.0.0", "PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"},
		{"PrivatePort": 9000, "Type": "tcp"},
		{"IP": "0.0.0.0", "PrivatePort": 53, "PublicPort": 5353, "Type": "udp"}
	]},
	{"Names": [], "Ports": [{"PrivatePort": 80, "PublicPort": 8081, "Type": "tcp"}]}
]`

func TestFetchContainers(t *testing.T) {
	t.Run("published tcp ports map to container names", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/containers/json" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, containersJSON)
		}))
		t.Cleanup(srv.Close)

		containers, err := fetchContainers(t.Context(), srv.Client(), srv.URL)
		if err != nil {
			t.Fatalf("fetchContainers() error = %v", err)
		}
		got := namesByPort(containers)
		want := map[uint16]string{8080: "shop-web"}
		if !maps.Equal(got, want) {
			t.Errorf("namesByPort() = %v, want %v", got, want)
		}
	})

	t.Run("non-200 response is an error", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)

		if _, err := fetchContainers(t.Context(), srv.Client(), srv.URL); err == nil {
			t.Errorf("fetchContainers() error = nil, want error")
		}
	})
}

func TestDockerSockets(t *testing.T) {
	got := dockerSockets("/Users/dev")
	want := []string{"/var/run/docker.sock", "/Users/dev/.docker/run/docker.sock"}
	if !slices.Equal(got, want) {
		t.Errorf("dockerSockets() = %v, want %v", got, want)
	}
}

func TestContainerNames(t *testing.T) {
	t.Run("missing socket gives an empty table", func(t *testing.T) {
		sockets := []string{filepath.Join(t.TempDir(), "missing.sock")}
		if got := containerNames(t.Context(), sockets); len(got) != 0 {
			t.Errorf("containerNames() = %v, want empty", got)
		}
	})
}
