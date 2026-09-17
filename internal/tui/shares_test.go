package tui

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/share"
)

const allowedEntry = "203.0.113.42"

func TestModel_Share(t *testing.T) {
	front, api := service(5173, "front"), service(8080, "api")

	t.Run("s opens the dialog for the selected service", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, front, api)
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, press('s'))
		if m.dialog == nil || m.dialog.service.Addr != api.Addr {
			t.Errorf("dialog = %+v, want one for %s", m.dialog, api.Addr)
		}
	})

	t.Run("esc closes the dialog without sharing", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, front)
		m, _ = update(t, m, press('s'))
		m, cmd := update(t, m, pressKey(tea.KeyEscape))
		if m.dialog != nil || cmd != nil || len(m.opening) != 0 {
			t.Errorf("dialog = %v, cmd nil = %v, opening = %d, want closed and nothing started", m.dialog, cmd == nil, len(m.opening))
		}
	})

	t.Run("keys typed in the dialog do not reach the list", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, front, api)
		m, _ = update(t, m, press('s'))
		m, cmd := update(t, m, press('q'))
		m, _ = update(t, m, press('j'))
		if m.dialog == nil || m.cursor != 0 || m.dialog.allow.Value() != "qj" {
			t.Errorf("dialog = %v, cursor = %d, want q and j typed into the allow field", m.dialog, m.cursor)
		}
		if cmd != nil {
			if _, quit := cmd().(tea.QuitMsg); quit {
				t.Errorf("q in the dialog quit the program")
			}
		}
	})

	t.Run("submitting starts the share, copies the url and says so", func(t *testing.T) {
		deps := &fakeDeps{}
		m := scanned(t, deps, front)
		m, _ = update(t, m, press('s'))
		m = typeText(t, m, allowedEntry)
		m, cmd := update(t, m, pressKey(tea.KeyEnter))
		if m.dialog != nil || len(m.opening) != 1 {
			t.Fatalf("dialog = %v, opening = %d, want closed and one share opening", m.dialog, len(m.opening))
		}

		m, cmd = update(t, m, await[shareStartedMsg](t, cmd))
		t.Cleanup(func() { m.shares[0].share.Stop() })
		if len(m.shares) != 1 || len(m.opening) != 0 || m.shares[0].share.URL().String() != fakeURL {
			t.Fatalf("shares = %+v, opening = %d, want one share at %s", m.shares, len(m.opening), fakeURL)
		}

		m, _ = update(t, m, await[copiedMsg](t, cmd))
		if got := deps.clipboard(); len(got) != 1 || got[0] != fakeURL {
			t.Errorf("clipboard = %q, want %q", got, fakeURL)
		}
		if m.flash != copiedText {
			t.Errorf("flash = %q, want %q", m.flash, copiedText)
		}
	})

	t.Run("the next dialog remembers the last allow list", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front, api), allowedEntry)
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, press('s'))
		if m.dialog == nil || m.dialog.allow.Value() != allowedEntry {
			t.Errorf("dialog = %+v, want the allow field prefilled with %q", m.dialog, allowedEntry)
		}
	})

	t.Run("s on a shared service explains instead of opening the dialog", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, press('s'))
		if m.dialog != nil || !strings.Contains(m.flash, "already shared") {
			t.Errorf("dialog = %v, flash = %q, want no dialog and an explanation", m.dialog, m.flash)
		}
	})

	t.Run("c copies the url again", func(t *testing.T) {
		deps := &fakeDeps{}
		m := shared(t, scanned(t, deps, front), allowedEntry)
		_, cmd := update(t, m, press('c'))
		await[copiedMsg](t, cmd)
		if got := deps.clipboard(); len(got) != 2 || got[1] != fakeURL {
			t.Errorf("clipboard = %q, want the url copied twice", got)
		}
	})

	t.Run("failed clipboard falls back to the terminal", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, front)
		m, cmd := update(t, m, copiedMsg{text: fakeURL, err: errors.New("no clipboard utility")})
		if m.flash != copiedText || cmd == nil {
			t.Errorf("flash = %q, cmd nil = %v, want %q and a terminal clipboard command", m.flash, cmd == nil, copiedText)
		}
	})

	t.Run("x stops the share and says so", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		stopped := m.shares[0].share
		m, _ = update(t, m, press('x'))
		if err := stopped.Wait(); err != nil {
			t.Fatalf("share.Wait() error = %v, want nil after x", err)
		}
		m, _ = update(t, m, shareEndedMsg{port: front.Addr.Port()})
		if len(m.shares) != 0 || m.flash != "front share stopped" {
			t.Errorf("shares = %d, flash = %q, want none and %q", len(m.shares), m.flash, "front share stopped")
		}
	})

	t.Run("x and c on a service that is not shared explain", func(t *testing.T) {
		for _, k := range []rune{'x', 'c'} {
			m, _ := update(t, scanned(t, &fakeDeps{}, front), press(k))
			if !strings.Contains(m.flash, "not shared") {
				t.Errorf("flash after %c = %q, want an explanation", k, m.flash)
			}
		}
	})

	endings := []struct {
		name string
		err  error
		want string
	}{
		{"expiration", share.ErrExpired, "front share expired"},
		{"lost tunnel", errors.New("tunnel lost: cloudflared exited: connection lost"), "front: tunnel lost: cloudflared exited: connection lost"},
	}
	for _, tt := range endings {
		t.Run(tt.name+" removes the share and says why", func(t *testing.T) {
			m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
			m, _ = update(t, m, shareEndedMsg{port: front.Addr.Port(), err: tt.err})
			if len(m.shares) != 0 || m.flash != tt.want {
				t.Errorf("shares = %d, flash = %q, want none and %q", len(m.shares), m.flash, tt.want)
			}
		})
	}

	failures := []struct {
		name string
		err  error
		want string
	}{
		{"missing cloudflared", fmt.Errorf("find cloudflared: %w", exec.ErrNotFound), installCloudflared},
		{"tunnel failure", errors.New("cloudflared exited: 429 Too Many Requests"), "front: open tunnel: cloudflared exited: 429 Too Many Requests"},
	}
	for _, tt := range failures {
		t.Run(tt.name+" is explained", func(t *testing.T) {
			deps := &fakeDeps{openErr: tt.err}
			m := scanned(t, deps, front)
			m, _ = update(t, m, press('s'))
			m = typeText(t, m, allowedEntry)
			m, cmd := update(t, m, pressKey(tea.KeyEnter))
			m, _ = update(t, m, await[shareFailedMsg](t, cmd))
			if len(m.opening) != 0 || len(m.shares) != 0 || m.flash != tt.want {
				t.Errorf("opening = %d, shares = %d, flash = %q, want none and %q", len(m.opening), len(m.shares), m.flash, tt.want)
			}
		})
	}

	t.Run("a shared service stays listed when it stops responding", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front, api), allowedEntry)
		m, _ = update(t, m, scannedMsg{services: []discovery.Service{api}})

		rows := m.rows()
		if len(rows) != 2 || rows[0].service.Addr != front.Addr || rows[0].responding || rows[0].share == nil || !rows[1].responding {
			t.Errorf("rows = %+v, want front pinned as not responding before api", rows)
		}
		if m.cursor != 0 {
			t.Errorf("cursor = %d, want it to stay on the shared service", m.cursor)
		}
	})

	t.Run("clock refreshes the access log of each share", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, clockMsg(time.Now()))
		if m.shares[0].accesses == nil {
			t.Errorf("accesses = nil, want a snapshot (possibly empty) after the clock")
		}
	})

	t.Run("only the latest notice is cleared by its timer", func(t *testing.T) {
		m, _ := update(t, scanned(t, &fakeDeps{}, front), press('x'))
		first := m.flashID
		m, _ = update(t, m, press('c'))
		m, _ = update(t, m, flashExpiredMsg{id: first})
		if m.flash == "" {
			t.Errorf("an old timer cleared the newer notice")
		}
		m, _ = update(t, m, flashExpiredMsg{id: m.flashID})
		if m.flash != "" {
			t.Errorf("flash = %q, want it cleared by its own timer", m.flash)
		}
	})
}

func TestModel_Quit(t *testing.T) {
	front := service(5173, "front")

	t.Run("waits for every share to stop before quitting", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		running := m.shares[0].share

		m, cmd := update(t, m, press('q'))
		if !m.quitting || m.flash != stoppingText {
			t.Errorf("quitting = %v, flash = %q, want true and %q", m.quitting, m.flash, stoppingText)
		}
		if cmd != nil {
			if _, quit := cmd().(tea.BatchMsg); quit {
				t.Errorf("q returned a batch that may quit before the share stopped")
			}
		}
		if err := running.Wait(); err != nil {
			t.Fatalf("share.Wait() error = %v, want nil after q", err)
		}

		_, cmd = update(t, m, shareEndedMsg{port: front.Addr.Port()})
		if cmd == nil {
			t.Fatalf("cmd = nil after the last share ended, want tea.Quit")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("cmd did not return tea.QuitMsg after the last share ended")
		}
	})

	t.Run("a share that finishes opening while quitting is stopped", func(t *testing.T) {
		deps := &fakeDeps{}
		m := scanned(t, deps, front)
		m, _ = update(t, m, press('s'))
		m = typeText(t, m, allowedEntry)
		m, cmd := update(t, m, pressKey(tea.KeyEnter))
		started := await[shareStartedMsg](t, cmd)

		m, _ = update(t, m, press('q'))
		m, cmd = update(t, m, started)
		ended := await[shareEndedMsg](t, cmd)
		if ended.err != nil {
			t.Errorf("share ended with %v, want it stopped cleanly", ended.err)
		}
		if got := deps.clipboard(); len(got) != 0 {
			t.Errorf("clipboard = %q, want nothing copied while quitting", got)
		}
		if _, cmd = update(t, m, ended); cmd == nil {
			t.Errorf("cmd = nil after the share ended, want tea.Quit")
		}
	})
}
