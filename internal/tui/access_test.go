package tui

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/proxy"
)

func accesses(n int) []proxy.Access {
	list := make([]proxy.Access, 0, n)
	addr := netip.MustParseAddr("198.18.0.0")
	for range n {
		addr = addr.Next()
		list = append(list, proxy.Access{Addr: addr, Requests: 1})
	}
	return list
}

func manyServices(n int) []discovery.Service {
	services := make([]discovery.Service, 0, n)
	for i := range n {
		services = append(services, service(uint16(3000+i), fmt.Sprintf("svc-%02d", i)))
	}
	return services
}

func TestModel_Access(t *testing.T) {
	front, api := service(5173, "front"), service(8080, "api")

	t.Run("enter opens the access log of a shared service", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front, api), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		if m.screen != screenAccess || m.access.port != front.Addr.Port() {
			t.Errorf("screen = %d, access port = %d, want the access log of %d", m.screen, m.access.port, front.Addr.Port())
		}
	})

	t.Run("enter on a service that is not shared explains", func(t *testing.T) {
		m, _ := update(t, scanned(t, &fakeDeps{}, front), pressKey(tea.KeyEnter))
		if m.screen != screenServices || !strings.Contains(m.flash, "not shared") {
			t.Errorf("screen = %d, flash = %q, want the list and an explanation", m.screen, m.flash)
		}
	})

	t.Run("esc goes back and the list keeps its selection", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, api, front)
		m, _ = update(t, m, press('j'))
		m = shared(t, m, allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m, _ = update(t, m, pressKey(tea.KeyEscape))
		if m.screen != screenServices || m.cursor != 1 {
			t.Errorf("screen = %d, cursor = %d, want the list with the cursor still on row 1", m.screen, m.cursor)
		}
	})

	t.Run("up and down move inside the log and stop at both ends", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front, api), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m.shares[0].accesses = accesses(3)
		m, _ = update(t, m, press('k'))
		for range 5 {
			m, _ = update(t, m, press('j'))
		}
		if m.access.cursor != 2 || m.cursor != 0 {
			t.Errorf("access cursor = %d, list cursor = %d, want 2 and the list untouched", m.access.cursor, m.cursor)
		}
	})

	t.Run("cursor stays on its address when newer requests arrive", func(t *testing.T) {
		deps := &fakeDeps{}
		m := shared(t, scanned(t, deps, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		watched := netip.MustParseAddr("198.51.100.1")
		deps.visit(t, watched.String())
		m, _ = update(t, m, clockMsg(time.Now()))
		m.access.cursor = slices.IndexFunc(m.shares[0].accesses, func(a proxy.Access) bool { return a.Addr == watched })

		deps.visit(t, "198.51.100.2")
		m, _ = update(t, m, clockMsg(time.Now()))
		if got, _ := m.selectedAccessAddr(); got != watched || m.access.cursor == 0 {
			t.Errorf("cursor = %d on %s, want it to follow %s below the newer address", m.access.cursor, got, watched)
		}
	})

	t.Run("c copies the url and x stops the share from the log", func(t *testing.T) {
		deps := &fakeDeps{}
		m := shared(t, scanned(t, deps, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		_, cmd := update(t, m, press('c'))
		await[copiedMsg](t, cmd)
		if got := deps.clipboard(); len(got) != 2 || got[1] != fakeURL {
			t.Errorf("clipboard = %q, want the url copied again", got)
		}

		running := m.shares[0].share
		update(t, m, press('x'))
		waitStopped(t, running)
	})

	t.Run("s does not open the dialog from the log", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m, _ = update(t, m, press('s'))
		if m.dialog != nil {
			t.Errorf("dialog opened from the access log")
		}
	})

	t.Run("the end of the share returns to the list and says why", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m, _ = update(t, m, shareEndedMsg{port: front.Addr.Port()})
		if m.screen != screenServices || m.flash != "front share stopped" {
			t.Errorf("screen = %d, flash = %q, want the list and %q", m.screen, m.flash, "front share stopped")
		}
	})

	t.Run("q from the log waits for the shares like the list does", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m, _ = update(t, m, press('q'))
		if !m.quitting {
			t.Errorf("quitting = false after q in the access log")
		}
	})

	t.Run("window follows the cursor through a long log", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, front), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m.shares[0].accesses = accesses(200)
		for range 150 {
			m, _ = update(t, m, press('j'))
		}
		visible := m.visibleAccesses()
		if m.access.cursor != 150 || m.access.offset != 150-visible+1 {
			t.Errorf("cursor = %d, offset = %d, want 150 and %d", m.access.cursor, m.access.offset, 150-visible+1)
		}
		lines := plainLines(m.View())
		view := strings.Join(lines, "\n")
		if !strings.Contains(view, "198.18.0.151 ") || strings.Contains(view, "198.18.0.1 ") {
			t.Errorf("View() = %q, want the 151st address and not the first", view)
		}
		if got := m.View().Cursor; got == nil || got.Y != lineContaining(t, lines, "198.18.0.151 ") {
			t.Errorf("View().Cursor = %+v, want it on the selected address", got)
		}
	})
}

func TestModel_ListScroll(t *testing.T) {
	m := scanned(t, &fakeDeps{}, manyServices(30)...)
	for range 20 {
		m, _ = update(t, m, press('j'))
	}
	visible := m.visibleRows()
	if m.cursor != 20 || m.offset != 20-visible+1 {
		t.Fatalf("cursor = %d, offset = %d, want 20 and %d", m.cursor, m.offset, 20-visible+1)
	}
	lines := plainLines(m.View())
	view := strings.Join(lines, "\n")
	first, last := m.offset+1, m.offset+visible
	if !strings.Contains(view, fmt.Sprintf("%d–%d of 30", first, last)) || !strings.Contains(view, "svc-20") || strings.Contains(view, "svc-00") {
		t.Errorf("View() = %q, want rows %d–%d of 30 with svc-20 and without svc-00", view, first, last)
	}
	if got := m.View().Cursor; got == nil || got.Y != lineContaining(t, lines, "svc-20") {
		t.Errorf("View().Cursor = %+v, want it on svc-20", got)
	}
}

func TestModel_ViewFitsTheWindow(t *testing.T) {
	const width = 80
	for _, height := range []int{40, 24, 16, 12, 8} {
		for _, screenName := range []string{"list", "dialog", "access"} {
			t.Run(fmt.Sprintf("%s at %d lines", screenName, height), func(t *testing.T) {
				m := scanned(t, &fakeDeps{}, manyServices(30)...)
				m = shared(t, m, allowedEntry)
				m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: height})
				wantCursorOn := "svc-00"
				switch screenName {
				case "dialog":
					m, _ = update(t, m, press('j'))
					m, _ = update(t, m, press('s'))
					wantCursorOn = "Allow"
				case "access":
					m, _ = update(t, m, pressKey(tea.KeyEnter))
					m.shares[0].accesses = accesses(1000)
					m, _ = update(t, m, press('j'))
					wantCursorOn = "198.18.0.2 "
				}

				lines := plainLines(m.View())
				if len(lines) != height {
					t.Fatalf("View() has %d lines, want %d", len(lines), height)
				}
				if last := lines[height-1]; !strings.Contains(last, "URL copied") {
					t.Errorf("last line = %q, want the notice", last)
				}
				if screenName != "dialog" && lineContaining(t, lines, "q quit") != lineContaining(t, lines, "╭─")-1 {
					t.Errorf("View() = %q, want the keys on the line above the panel", lines)
				}
				if got := m.View().Cursor; got == nil || got.Y != lineContaining(t, lines, wantCursorOn) {
					t.Errorf("View().Cursor = %+v, want it on the line with %q", got, wantCursorOn)
				}
				if screenName == "dialog" && !strings.Contains(strings.Join(lines, "\n"), "enter share · tab switch · esc cancel") {
					t.Errorf("dialog keys are not visible at %d lines", height)
				}
			})
		}
	}
}
