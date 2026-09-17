package cloudflared

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	fakeModeEnv  = "T5S_FAKE_CLOUDFLARED"
	fakeArgsEnv  = "T5S_FAKE_CLOUDFLARED_ARGS"
	fakeHostname = "fake-quick-tunnel.trycloudflare.com"
	fakeCrashLog = "ERR failed to request quick Tunnel: 429 Too Many Requests"
	fakeLostLog  = "ERR connection to the edge was lost"

	modeReady = "ready"
	modeSlow  = "slow"
	modeNever = "never"
	modeCrash = "crash"

	slowPolls = 3
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeModeEnv); mode != "" {
		os.Exit(runFakeCloudflared(mode, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func runFakeCloudflared(mode string, args []string) (exitCode int) {
	if file := os.Getenv(fakeArgsEnv); file != "" {
		if err := os.WriteFile(file, []byte(strings.Join(args, "\n")), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	if mode == modeCrash {
		fmt.Fprintln(os.Stderr, "INF Requesting new quick Tunnel on trycloudflare.com...")
		fmt.Fprintln(os.Stderr, fakeCrashLog)
		return 1
	}

	var readyPolls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/quicktunnel", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"hostname":%q}`, fakeHostname)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		polls := readyPolls.Add(1)
		connected := mode == modeReady || (mode == modeSlow && polls >= slowPolls)
		if !connected {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"status":503,"readyConnections":0}`)
			return
		}
		fmt.Fprint(w, `{"status":200,"readyConnections":1}`)
	})
	mux.HandleFunc("/polls", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, readyPolls.Load())
	})
	mux.HandleFunc("/die", func(http.ResponseWriter, *http.Request) {
		fmt.Fprintln(os.Stderr, fakeLostLog)
		os.Exit(1)
	})

	srv := &http.Server{Addr: flagValue(args, "--metrics"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintln(os.Stderr, srv.ListenAndServe())
	return 2
}

func flagValue(args []string, name string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
