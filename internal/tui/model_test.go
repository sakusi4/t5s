package tui

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

func service(port uint16, name string) discovery.Service {
	return discovery.Service{
		Addr: netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), port),
		Name: name,
	}
}

func scanReturning(services []discovery.Service, err error) ScanFunc {
	return func(context.Context) ([]discovery.Service, error) {
		return services, err
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

func TestModel_Init(t *testing.T) {
	want := []discovery.Service{service(3000, "dashboard")}
	m := New(scanReturning(want, nil))

	msg, ok := m.Init()().(scannedMsg)
	if !ok || len(msg.services) != 1 || msg.services[0].Name != "dashboard" {
		t.Errorf("Init()() = %+v, want scannedMsg with dashboard", msg)
	}
}

func TestModel_Update(t *testing.T) {
	two := []discovery.Service{service(3000, "dashboard"), service(8080, "api")}

	t.Run("scan result replaces services and schedules the next tick", func(t *testing.T) {
		m, cmd := update(t, New(scanReturning(nil, nil)), scannedMsg{services: two})
		if len(m.services) != 2 || !m.scanned || cmd == nil {
			t.Errorf("services = %d, scanned = %v, cmd nil = %v, want 2, true, false", len(m.services), m.scanned, cmd == nil)
		}
	})

	t.Run("tick starts a scan", func(t *testing.T) {
		_, cmd := update(t, New(scanReturning(two, nil)), tickMsg{})
		if msg, ok := cmd().(scannedMsg); !ok || len(msg.services) != 2 {
			t.Errorf("tick cmd returned %+v, want scannedMsg with 2 services", msg)
		}
	})

	t.Run("selection follows its port when the list changes", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: two})
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, scannedMsg{services: []discovery.Service{service(80, "nginx"), service(3000, "dashboard"), service(8080, "api")}})
		if m.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m.cursor)
		}
	})

	t.Run("selection moves into range when its port disappears", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: two})
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, scannedMsg{services: two[:1]})
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("scan error keeps the previous services", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: two})
		m, cmd := update(t, m, scannedMsg{err: errors.New("lsof failed")})
		if len(m.services) != 2 || m.err == nil || cmd == nil {
			t.Errorf("services = %d, err = %v, cmd nil = %v, want 2, error, false", len(m.services), m.err, cmd == nil)
		}
	})

	t.Run("successful scan clears the error", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{err: errors.New("lsof failed")})
		m, _ = update(t, m, scannedMsg{services: two})
		if m.err != nil {
			t.Errorf("err = %v, want nil", m.err)
		}
	})

	t.Run("cursor stops at both ends", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: two})
		m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
		if m.cursor != 0 {
			t.Errorf("cursor after up = %d, want 0", m.cursor)
		}
		m, _ = update(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
		m, _ = update(t, m, press('j'))
		if m.cursor != 1 {
			t.Errorf("cursor after down twice = %d, want 1", m.cursor)
		}
		m, _ = update(t, m, press('k'))
		if m.cursor != 0 {
			t.Errorf("cursor after k = %d, want 0", m.cursor)
		}
	})

	t.Run("q and ctrl+c quit", func(t *testing.T) {
		for _, msg := range []tea.KeyPressMsg{press('q'), {Code: 'c', Mod: tea.ModCtrl}} {
			_, cmd := update(t, New(scanReturning(nil, nil)), msg)
			if cmd == nil {
				t.Fatalf("Update(%s) cmd = nil, want tea.Quit", msg)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("Update(%s) cmd did not return tea.QuitMsg", msg)
			}
		}
	})
}
