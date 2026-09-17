package cloudflared

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var local = netip.MustParseAddrPort("127.0.0.1:3000")

func fake(t *testing.T, mode string) (backend Backend, argsFile string) {
	t.Helper()
	argsFile = filepath.Join(t.TempDir(), "args")
	t.Setenv(fakeModeEnv, mode)
	t.Setenv(fakeArgsEnv, argsFile)
	return Backend{executable: os.Args[0]}, argsFile
}

func recordedArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	return strings.Split(string(raw), "\n")
}

func open(t *testing.T, backend Backend) (tunnel *Tunnel, stop context.CancelFunc) {
	t.Helper()
	ctx, stop := context.WithCancel(t.Context())
	tunnel, err := backend.Open(ctx, local)
	if err != nil {
		stop()
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		stop()
		<-tunnel.exited
	})
	return tunnel, stop
}

func fakeGet(t *testing.T, metrics, endpoint string) string {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+metrics+endpoint, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}

func refusesConnections(t *testing.T, addr string) bool {
	t.Helper()
	var d net.Dialer
	conn, err := d.DialContext(t.Context(), "tcp", addr)
	if err != nil {
		return true
	}
	conn.Close()
	return false
}

func TestBackend_Open(t *testing.T) {
	t.Run("returns the public url once the tunnel is connected", func(t *testing.T) {
		backend, argsFile := fake(t, modeReady)
		tunnel, _ := open(t, backend)

		if got := tunnel.URL().String(); got != "https://"+fakeHostname {
			t.Errorf("URL() = %q, want %q", got, "https://"+fakeHostname)
		}
		args := recordedArgs(t, argsFile)
		want := []string{"tunnel", "--no-autoupdate", "--url", "http://127.0.0.1:3000", "--metrics"}
		if len(args) != len(want)+1 || !slices.Equal(args[:len(want)], want) {
			t.Fatalf("cloudflared args = %q, want %q followed by the metrics address", args, want)
		}
		if metrics, err := netip.ParseAddrPort(args[len(want)]); err != nil || !metrics.Addr().IsLoopback() {
			t.Errorf("metrics address = %q, want a loopback address", args[len(want)])
		}
	})

	t.Run("waits until cloudflared reports a connection", func(t *testing.T) {
		backend, argsFile := fake(t, modeSlow)
		tunnel, _ := open(t, backend)

		if got := tunnel.URL().Host; got != fakeHostname {
			t.Errorf("URL().Host = %q, want %q", got, fakeHostname)
		}
		polls, err := strconv.Atoi(fakeGet(t, flagValue(recordedArgs(t, argsFile), "--metrics"), "/polls"))
		if err != nil || polls < slowPolls {
			t.Errorf("Open() returned after %d readiness polls (%v), want at least %d", polls, err, slowPolls)
		}
	})

	t.Run("reports why cloudflared exited during startup", func(t *testing.T) {
		backend, _ := fake(t, modeCrash)

		_, err := backend.Open(t.Context(), local)
		if err == nil || !strings.Contains(err.Error(), fakeCrashLog) {
			t.Errorf("Open() error = %v, want it to contain %q", err, fakeCrashLog)
		}
	})

	t.Run("missing executable is reported as not found", func(t *testing.T) {
		backend := Backend{executable: "t5s-no-such-cloudflared"}

		if _, err := backend.Open(t.Context(), local); !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("Open() error = %v, want exec.ErrNotFound", err)
		}
	})

	t.Run("deadline during startup stops cloudflared", func(t *testing.T) {
		backend, argsFile := fake(t, modeNever)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		_, err := backend.Open(ctx, local)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Open() error = %v, want context.DeadlineExceeded", err)
		}
		metrics := flagValue(recordedArgs(t, argsFile), "--metrics")
		if !refusesConnections(t, metrics) {
			t.Errorf("fake cloudflared still listens on %s, want it stopped", metrics)
		}
	})
}

func TestTunnel_Wait(t *testing.T) {
	t.Run("returns nil after the context stops the tunnel", func(t *testing.T) {
		backend, argsFile := fake(t, modeReady)
		tunnel, stop := open(t, backend)
		metrics := flagValue(recordedArgs(t, argsFile), "--metrics")

		stop()
		if err := tunnel.Wait(); err != nil {
			t.Errorf("Wait() error = %v, want nil", err)
		}
		if !refusesConnections(t, metrics) {
			t.Errorf("fake cloudflared still listens on %s, want it stopped", metrics)
		}
	})

	t.Run("reports why cloudflared stopped by itself", func(t *testing.T) {
		backend, argsFile := fake(t, modeReady)
		tunnel, _ := open(t, backend)

		fakeGet(t, flagValue(recordedArgs(t, argsFile), "--metrics"), "/die")

		if err := tunnel.Wait(); err == nil || !strings.Contains(err.Error(), fakeLostLog) {
			t.Errorf("Wait() error = %v, want it to contain %q", err, fakeLostLog)
		}
	})
}
