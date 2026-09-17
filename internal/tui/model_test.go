package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

func TestModel_Init(t *testing.T) {
	deps := &fakeDeps{}
	m := New(deps.config(service(3000, "dashboard")))

	msg := await[scannedMsg](t, m.Init())
	if len(msg.services) != 1 || msg.services[0].Name != "dashboard" {
		t.Errorf("Init() scanned %+v, want dashboard", msg)
	}
}

func TestModel_Update(t *testing.T) {
	two := []discovery.Service{service(3000, "dashboard"), service(8080, "api")}

	t.Run("scan result replaces services and schedules the next tick", func(t *testing.T) {
		m, cmd := update(t, New((&fakeDeps{}).config()), scannedMsg{services: two})
		if len(m.services) != 2 || !m.scanned || cmd == nil {
			t.Errorf("services = %d, scanned = %v, cmd nil = %v, want 2, true, false", len(m.services), m.scanned, cmd == nil)
		}
	})

	t.Run("tick starts a scan", func(t *testing.T) {
		_, cmd := update(t, New((&fakeDeps{}).config(two...)), tickMsg{})
		if msg := await[scannedMsg](t, cmd); len(msg.services) != 2 {
			t.Errorf("tick cmd returned %+v, want 2 services", msg)
		}
	})

	t.Run("clock moves the time forward", func(t *testing.T) {
		now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
		m, cmd := update(t, New((&fakeDeps{}).config()), clockMsg(now))
		if !m.now.Equal(now) || cmd == nil {
			t.Errorf("now = %s, cmd nil = %v, want %s and the next clock tick", m.now, cmd == nil, now)
		}
	})

	t.Run("selection follows its port when the list changes", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, two...)
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, scannedMsg{services: []discovery.Service{service(80, "nginx"), service(3000, "dashboard"), service(8080, "api")}})
		if m.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m.cursor)
		}
	})

	t.Run("selection moves into range when its port disappears", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, two...)
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, scannedMsg{services: two[:1]})
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m.cursor)
		}
	})

	t.Run("scan error keeps the previous services", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, two...)
		m, cmd := update(t, m, scannedMsg{err: errors.New("lsof failed")})
		if len(m.services) != 2 || m.err == nil || cmd == nil {
			t.Errorf("services = %d, err = %v, cmd nil = %v, want 2, error, false", len(m.services), m.err, cmd == nil)
		}
	})

	t.Run("cursor stops at both ends", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, two...)
		m, _ = update(t, m, pressKey(tea.KeyUp))
		if m.cursor != 0 {
			t.Errorf("cursor after up = %d, want 0", m.cursor)
		}
		m, _ = update(t, m, pressKey(tea.KeyDown))
		m, _ = update(t, m, press('j'))
		if m.cursor != 1 {
			t.Errorf("cursor after down twice = %d, want 1", m.cursor)
		}
		m, _ = update(t, m, press('k'))
		if m.cursor != 0 {
			t.Errorf("cursor after k = %d, want 0", m.cursor)
		}
	})

	t.Run("q and ctrl+c quit at once when nothing is shared", func(t *testing.T) {
		for _, msg := range []tea.KeyPressMsg{press('q'), {Code: 'c', Mod: tea.ModCtrl}} {
			_, cmd := update(t, scanned(t, &fakeDeps{}, two...), msg)
			if cmd == nil {
				t.Fatalf("Update(%s) cmd = nil, want tea.Quit", msg)
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Errorf("Update(%s) cmd did not return tea.QuitMsg", msg)
			}
		}
	})
}
