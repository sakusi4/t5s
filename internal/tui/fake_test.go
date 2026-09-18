package tui

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/share"
)

const (
	fakeURL            = "https://fake-abc-def.trycloudflare.com"
	fakeClientIPHeader = "X-Fake-Client-IP"
	awaitTimeout       = 5 * time.Second
)

type fakeTunnel struct {
	done <-chan struct{}
}

func (f fakeTunnel) URL() *url.URL {
	return &url.URL{Scheme: "https", Host: "fake-abc-def.trycloudflare.com"}
}

func (f fakeTunnel) Wait() error {
	<-f.done
	return nil
}

var fakeOwn = []netip.Addr{netip.MustParseAddr("192.0.2.10"), netip.MustParseAddr("2001:db8::10")}

type fakeDeps struct {
	mu      sync.Mutex
	copied  []string
	local   netip.AddrPort
	openErr error
	copyErr error
	ownErr  error
}

func (f *fakeDeps) config(services ...discovery.Service) Config {
	return Config{
		Scan: func(context.Context) ([]discovery.Service, error) { return services, nil },
		Backend: share.Backend{
			ClientIPHeader: fakeClientIPHeader,
			Open: func(ctx context.Context, local netip.AddrPort) (share.Tunnel, error) {
				if f.openErr != nil {
					return nil, f.openErr
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				f.local = local
				return fakeTunnel{done: ctx.Done()}, nil
			},
		},
		Copy: func(text string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.copied = append(f.copied, text)
			return f.copyErr
		},
		PublicAddrs: func(context.Context) ([]netip.Addr, error) {
			if f.ownErr != nil {
				return nil, f.ownErr
			}
			return fakeOwn, nil
		},
	}
}

func (f *fakeDeps) clipboard() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.copied...)
}

func known(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = update(t, m, ownAddrsMsg{addrs: fakeOwn})
	return m
}

func (f *fakeDeps) visit(t *testing.T, client string) {
	t.Helper()
	f.mu.Lock()
	local := f.local
	f.mu.Unlock()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+local.String()+"/", nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext() error = %v", err)
	}
	req.Header.Set(fakeClientIPHeader, client)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request to the proxy as %s: error = %v", client, err)
	}
	resp.Body.Close()
}

func waitStopped(t *testing.T, running *share.Share) {
	t.Helper()
	stopped := make(chan error, 1)
	go func() { stopped <- running.Wait() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("share.Wait() error = %v, want nil", err)
		}
	case <-time.After(awaitTimeout):
		t.Fatalf("share is still running after %s", awaitTimeout)
	}
}

func service(port uint16, name string) discovery.Service {
	return discovery.Service{
		Addr: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port),
		Name: name,
	}
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want Model", next)
	}
	return model, cmd
}

func press(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: string(code)}
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = update(t, m, press(r))
	}
	return m
}

func await[T tea.Msg](t *testing.T, cmd tea.Cmd) T {
	t.Helper()
	found := make(chan T, 8)
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		go func() {
			switch msg := cmd().(type) {
			case tea.BatchMsg:
				for _, c := range msg {
					run(c)
				}
			case T:
				found <- msg
			}
		}()
	}
	run(cmd)
	select {
	case msg := <-found:
		return msg
	case <-time.After(awaitTimeout):
		var zero T
		t.Fatalf("no %T arrived within %s", zero, awaitTimeout)
		return zero
	}
}

func scanned(t *testing.T, deps *fakeDeps, services ...discovery.Service) Model {
	t.Helper()
	m, _ := update(t, New(deps.config(services...)), scannedMsg{services: services})
	return m
}

func shared(t *testing.T, m Model, allow string) Model {
	t.Helper()
	m, _ = update(t, m, press('s'))
	m = typeText(t, m, allow)
	m, cmd := update(t, m, pressKey(tea.KeyEnter))
	m, cmd = update(t, m, await[shareStartedMsg](t, cmd))
	m, _ = update(t, m, await[copiedMsg](t, cmd))
	t.Cleanup(func() {
		for _, a := range m.shares {
			a.share.Stop()
			if err := a.share.Wait(); err != nil && !errors.Is(err, share.ErrExpired) {
				t.Errorf("share.Wait() error = %v", err)
			}
		}
	})
	return m
}
