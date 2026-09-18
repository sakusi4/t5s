package tui

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func verdicts(t *testing.T, m Model) map[netip.Addr]bool {
	t.Helper()
	m, _ = update(t, m, clockMsg(time.Now()))
	got := make(map[netip.Addr]bool)
	for _, a := range m.shares[0].accesses {
		got[a.Addr] = a.Allowed
	}
	return got
}

func TestModel_OwnAddress(t *testing.T) {
	dead, front := service(9, "dead"), service(5173, "front")

	t.Run("Init looks the own address up", func(t *testing.T) {
		got := await[ownAddrsMsg](t, New((&fakeDeps{}).config()).Init())
		if got.err != nil || len(got.addrs) != len(fakeOwn) {
			t.Errorf("Init() delivered %+v, want %v", got, fakeOwn)
		}
	})

	t.Run("lookup failure arrives as an error", func(t *testing.T) {
		got := await[ownAddrsMsg](t, New((&fakeDeps{ownErr: errors.New("offline")}).config()).Init())
		if got.err == nil || got.addrs != nil {
			t.Errorf("Init() delivered %+v, want the error", got)
		}
	})

	t.Run("status line ends with the own address once it is known", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, front)
		expectText(t, m, []string{"finding your address…"}, []string{"you 192.0.2.10"})
		m = known(t, m)
		lines := plainLines(m.View())
		if last := strings.TrimRight(lines[len(lines)-1], " "); !strings.HasSuffix(last, "you 192.0.2.10 · 2001:db8::10") {
			t.Errorf("last line = %q, want it to end with the own address", last)
		}
		m, _ = update(t, m, ownAddrsMsg{err: errors.New("offline")})
		expectText(t, m, []string{"your address unknown"}, []string{"192.0.2.10"})
	})

	t.Run("a notice shares the status line with the own address", func(t *testing.T) {
		m, _ := update(t, known(t, scanned(t, &fakeDeps{}, front)), press('x'))
		lines := plainLines(m.View())
		last := lines[len(lines)-1]
		if !strings.HasPrefix(last, " front is not shared") || !strings.HasSuffix(strings.TrimRight(last, " "), "2001:db8::10") {
			t.Errorf("last line = %q, want the notice on the left and the own address on the right", last)
		}
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 50, Height: 24})
		expectText(t, m, []string{"front is not shared"}, []string{"you 192.0.2.10"})
	})

	t.Run("dialog says the own address is always allowed", func(t *testing.T) {
		m := known(t, scanned(t, &fakeDeps{}, front))
		m, _ = update(t, m, press('s'))
		expectText(t, m, []string{"Your own address is always allowed."}, nil)

		got := m.View()
		styled, plain := strings.Split(got.Content, "\n"), plainLines(got)
		note, allow := lineContaining(t, plain, "Your own address"), lineContaining(t, plain, "Allow   ")
		if note != allow-1 {
			t.Errorf("note on line %d, want it right above the allow line %d", note, allow)
		}
		if want := sgrPattern.FindString(noteStyle.Render("x")); !strings.Contains(styled[note], want) {
			t.Errorf("note line = %q, want it in the note colour %q", styled[note], want)
		}
		if got.Cursor == nil || got.Cursor.Y != allow {
			t.Errorf("View().Cursor = %+v, want it on the allow line %d", got.Cursor, allow)
		}

		m = typeText(t, m, "0.0.0.0/0")
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		expectText(t, m, []string{"allows every address", "Your own address is always allowed."}, nil)

		m = typeText(t, m, "1")
		expectText(t, m, []string{"Your own address is always allowed."}, []string{"allows every address"})
	})

	t.Run("dialog asks for an address while the own one is unknown", func(t *testing.T) {
		m, _ := update(t, scanned(t, &fakeDeps{}, front), press('s'))
		expectText(t, m, []string{"Your own address is unknown; allow at least one."}, nil)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		if m.dialog == nil || !strings.Contains(plainText(m), "allowlist is empty") {
			t.Errorf("dialog = %v, view = %q, want it open with the empty list refused", m.dialog, plainText(m))
		}
	})

	t.Run("own address learned while the dialog is open is allowed too", func(t *testing.T) {
		deps := &fakeDeps{}
		m, _ := update(t, scanned(t, deps, dead), press('s'))
		m = known(t, m)
		m, cmd := update(t, m, pressKey(tea.KeyEnter))
		m, _ = update(t, m, await[shareStartedMsg](t, cmd))
		deps.visit(t, "192.0.2.10")
		if got := verdicts(t, m); !got[netip.MustParseAddr("192.0.2.10")] {
			t.Errorf("verdicts = %v, want the own address allowed", got)
		}
	})

	t.Run("nothing typed shares with the own address only", func(t *testing.T) {
		deps := &fakeDeps{}
		m := shared(t, known(t, scanned(t, deps, dead)), "")
		deps.visit(t, "192.0.2.10")
		deps.visit(t, "198.51.100.7")
		got := verdicts(t, m)
		if !got[netip.MustParseAddr("192.0.2.10")] || got[netip.MustParseAddr("198.51.100.7")] {
			t.Errorf("verdicts = %v, want the own address allowed and the other blocked", got)
		}
	})

	t.Run("typed addresses are allowed next to the own one", func(t *testing.T) {
		deps := &fakeDeps{}
		m := shared(t, known(t, scanned(t, deps, dead)), allowedEntry)
		deps.visit(t, allowedEntry)
		deps.visit(t, "192.0.2.10")
		got := verdicts(t, m)
		if !got[netip.MustParseAddr(allowedEntry)] || !got[netip.MustParseAddr("192.0.2.10")] {
			t.Errorf("verdicts = %v, want both allowed", got)
		}
	})

	t.Run("the next dialog remembers only what was typed", func(t *testing.T) {
		m := shared(t, known(t, scanned(t, &fakeDeps{}, dead, front)), "")
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, press('s'))
		if got := m.dialog.allow.Value(); got != "" {
			t.Errorf("allow field = %q, want it empty", got)
		}
	})
}
