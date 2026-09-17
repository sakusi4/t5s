package tui

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/proxy"
)

var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plainLines(v tea.View) []string {
	return strings.Split(sgrPattern.ReplaceAllString(v.Content, ""), "\n")
}

func plainText(m Model) string {
	return strings.Join(plainLines(m.View()), "\n")
}

func lineContaining(t *testing.T, lines []string, text string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, text) {
			return i
		}
	}
	t.Fatalf("no line contains %q in %q", text, lines)
	return 0
}

func expectText(t *testing.T, m Model, want, notWant []string) {
	t.Helper()
	got := plainText(m)
	for _, text := range want {
		if !strings.Contains(got, text) {
			t.Errorf("View() = %q, want it to contain %q", got, text)
		}
	}
	for _, text := range notWant {
		if strings.Contains(got, text) {
			t.Errorf("View() = %q, want it not to contain %q", got, text)
		}
	}
}

func TestModel_View(t *testing.T) {
	three := []discovery.Service{service(80, "nginx"), service(5173, "front"), service(9030, "app")}

	t.Run("before the first scan", func(t *testing.T) {
		m := New((&fakeDeps{}).config())
		expectText(t, m,
			[]string{"scanning…", "refresh 3s", "▀█▀ █▀▀ ▄▀▀", "share localhost with only the people you allow", "╭─ Local services ─", "q quit"},
			[]string{"Active shares", "s share", "x stop", "c copy", "enter access log"})
	})

	t.Run("no services", func(t *testing.T) {
		m := scanned(t, &fakeDeps{})
		expectText(t, m, []string{"0 services", "No local web services found."}, []string{"s share"})
	})

	t.Run("one service can be shared", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three[:1]...)
		expectText(t, m, []string{"1 service", "NAME", "FRAMEWORK", "ADDRESS", "EXPIRES", "URL", "● nginx", "localhost:80", "s share"}, []string{"1 services", "x stop", "c copy", "enter access log"})
	})

	t.Run("missing framework falls back to process then dash", func(t *testing.T) {
		m := scanned(t, &fakeDeps{},
			discovery.Service{Addr: netip.MustParseAddrPort("127.0.0.1:80"), Name: "nginx", Process: "nginx-worker"},
			discovery.Service{Addr: netip.MustParseAddrPort("192.168.0.5:8080")})
		expectText(t, m, []string{"2 services", "nginx-worker", "● - ", "192.168.0.5:8080"}, nil)
	})

	t.Run("scan error", func(t *testing.T) {
		m, _ := update(t, New((&fakeDeps{}).config()), scannedMsg{err: errors.New("lsof failed")})
		expectText(t, m, []string{"lsof failed"}, nil)
	})

	t.Run("notice replaces the scan error line", func(t *testing.T) {
		m, _ := update(t, scanned(t, &fakeDeps{}, three...), scannedMsg{err: errors.New("lsof failed")})
		m, _ = update(t, m, press('x'))
		expectText(t, m, []string{"nginx is not shared"}, []string{"lsof failed"})
	})

	t.Run("shared row shows its url and time left, and the keys that apply", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m, _ = update(t, m, press('j'))
		m = shared(t, m, allowedEntry)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 24})
		m.now = m.shares[0].share.ExpiresAt().Add(-58*time.Minute - 30*time.Second)

		expectText(t, m,
			[]string{"3 services · 1 share", "◉ front", fakeURL, "58m", "enter access log", "x stop", "c copy"},
			[]string{"s share", "Active shares"})
		lines := plainLines(m.View())
		if row := lines[lineContaining(t, lines, "◉ front")]; !strings.Contains(row, fakeURL) || strings.Index(row, "58m") > strings.Index(row, fakeURL) {
			t.Errorf("shared row = %q, want the time left before the url on the same row", row)
		}
	})

	t.Run("opening and not responding are marked on the row", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m.opening = []discovery.Service{three[0]}
		expectText(t, m, []string{"opening tunnel…"}, []string{"s share"})

		m = shared(t, scanned(t, &fakeDeps{}, three[1]), allowedEntry)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 24})
		m, _ = update(t, m, scannedMsg{})
		expectText(t, m, []string{"◉ front", fakeURL + " · not responding"}, nil)
	})

	t.Run("a url that does not fit ends with an ellipsis and keeps the time left", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, three[1]), allowedEntry)
		m.now = m.shares[0].share.ExpiresAt().Add(-58 * time.Minute)
		for _, tt := range []struct {
			width int
			cut   bool
		}{{70, true}, {120, false}} {
			m, _ = update(t, m, tea.WindowSizeMsg{Width: tt.width, Height: 24})
			lines := plainLines(m.View())
			shared := lines[lineContaining(t, lines, "◉ front")]
			if !strings.Contains(shared, "58m") || strings.Contains(shared, fakeURL) == tt.cut || strings.HasSuffix(shared, "…│") != tt.cut {
				t.Errorf("shared row at %d columns = %q, want cut = %v marked by an ellipsis", tt.width, shared, tt.cut)
			}
		}
	})

	t.Run("access screen lists every address of the share", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, three[1]), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		m.now = m.shares[0].share.ExpiresAt().Add(-58 * time.Minute)
		m.shares[0].accesses = []proxy.Access{
			{Addr: netip.MustParseAddr("198.51.100.7"), Allowed: false, Requests: 3, LastSeen: m.now.Add(-2 * time.Second)},
			{Addr: netip.MustParseAddr("203.0.113.42"), Allowed: true, Requests: 48, LastSeen: m.now.Add(-90 * time.Second)},
		}
		m.shares[0].untracked = 7

		expectText(t, m,
			[]string{"╭─ Access · front ─", "2 addresses · +7 untracked requests", fakeURL + " · 58m left", "ADDRESS", "VERDICT", "REQUESTS", "LAST SEEN",
				"198.51.100.7", "blocked", "2s ago", "203.0.113.42", "allowed", "48", "1m ago", "esc back", "c copy", "x stop", "q quit"},
			[]string{"Local services", "s share", "enter access log"})
	})

	t.Run("access screen without requests says so", func(t *testing.T) {
		m := shared(t, scanned(t, &fakeDeps{}, three[1]), allowedEntry)
		m, _ = update(t, m, pressKey(tea.KeyEnter))
		expectText(t, m, []string{"0 addresses", "No requests yet."}, nil)
	})

	t.Run("a row that loses only blank padding is not marked as cut", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m, _ = update(t, m, press('j'))
		m = shared(t, m, allowedEntry)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 45, Height: 24})

		lines := plainLines(m.View())
		for name, cut := range map[string]bool{"● nginx": false, "◉ front": true} {
			if line := lines[lineContaining(t, lines, name)]; strings.HasSuffix(line, "…│") != cut {
				t.Errorf("row = %q, want an ellipsis = %v", line, cut)
			}
		}
	})

	t.Run("dialog opens under the list and owns the terminal cursor", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, press('s'))
		m = typeText(t, m, "0.0.0.0/0")
		m, _ = update(t, m, pressKey(tea.KeyEnter))

		expectText(t, m,
			[]string{"╭─ Share front (localhost:5173) ─", "Allow   0.0.0.0/0", "Expire", "‹1h›", "allows every address", "enter share · tab switch · esc cancel"},
			[]string{"q quit", "s share"})
		got := m.View()
		allowLine := lineContaining(t, plainLines(got), "Allow   0.0.0.0/0")
		wantX := lipgloss.Width("│ Allow   0.0.0.0/0")
		if got.Cursor == nil || got.Cursor.Y != allowLine || got.Cursor.X != wantX {
			t.Errorf("View().Cursor = %+v, want line %d column %d (end of the typed text)", got.Cursor, allowLine, wantX)
		}

		m, _ = update(t, m, pressKey(tea.KeyTab))
		got = m.View()
		if expireLine := lineContaining(t, plainLines(got), "Expire"); got.Cursor == nil || got.Cursor.Y != expireLine {
			t.Errorf("View().Cursor = %+v, want it on the expire line %d", got.Cursor, expireLine)
		}
	})

	t.Run("empty allow input shows the whole example", func(t *testing.T) {
		m, _ := update(t, scanned(t, &fakeDeps{}, three...), press('s'))
		expectText(t, m, []string{"Allow   203.0.113.42, 10.0.0.0/8"}, nil)
	})

	for _, tt := range []struct {
		name        string
		resizeFirst bool
	}{{"opened in a narrow window", true}, {"narrowed while open", false}} {
		t.Run("allow list longer than the dialog scrolls inside it when "+tt.name, func(t *testing.T) {
			const width = 50
			m := scanned(t, &fakeDeps{}, three...)
			if tt.resizeFirst {
				m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
			}
			m, _ = update(t, m, press('s'))
			m = typeText(t, m, "10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 203.0.113.0/24")
			m, _ = update(t, m, tea.WindowSizeMsg{Width: width, Height: 24})

			got := m.View()
			lines := plainLines(got)
			allow := lines[lineContaining(t, lines, "Allow")]
			if strings.Contains(allow, "…") || !strings.Contains(allow, "203.0.113.0/24") {
				t.Errorf("allow line = %q, want the end of the list and no ellipsis", allow)
			}
			if got.Cursor == nil || got.Cursor.X >= width-borderWidth {
				t.Errorf("View().Cursor = %+v, want it inside the %d column dialog", got.Cursor, width)
			}
		})
	}

	t.Run("remembered allow list longer than the dialog opens scrolled to its end", func(t *testing.T) {
		const width = 50
		m, _ := update(t, scanned(t, &fakeDeps{}, three...), tea.WindowSizeMsg{Width: width, Height: 24})
		m = shared(t, m, "10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 203.0.113.0/24")
		m, _ = update(t, m, press('j'))
		m, _ = update(t, m, press('s'))

		got := m.View()
		lines := plainLines(got)
		allow := lines[lineContaining(t, lines, "Allow")]
		if strings.Contains(allow, "…") || !strings.Contains(allow, "203.0.113.0/24") || got.Cursor == nil || got.Cursor.X >= width-borderWidth {
			t.Errorf("allow line = %q, cursor = %+v, want the end of the list inside the dialog", allow, got.Cursor)
		}
	})

	t.Run("screen is full and black", func(t *testing.T) {
		got := New((&fakeDeps{}).config()).View()
		if !got.AltScreen {
			t.Errorf("View().AltScreen = false, want true")
		}
		if got.BackgroundColor == nil {
			t.Fatalf("View().BackgroundColor = nil, want black")
		}
		if r, g, b, _ := got.BackgroundColor.RGBA(); r != 0 || g != 0 || b != 0 {
			t.Errorf("View().BackgroundColor = (%d, %d, %d), want black", r, g, b)
		}
		if got.ForegroundColor == nil {
			t.Errorf("View().ForegroundColor = nil, want a color readable on black")
		}
	})

	t.Run("terminal cursor sits on the selected row", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m, _ = update(t, m, press('j'))

		got := m.View()
		selected := lineContaining(t, plainLines(got), "front")
		if got.Cursor == nil || got.Cursor.Y != selected {
			t.Errorf("View().Cursor = %+v, want it on line %d", got.Cursor, selected)
		}
	})

	t.Run("only the selected row has a background", func(t *testing.T) {
		m := scanned(t, &fakeDeps{}, three...)
		m, _ = update(t, m, press('j'))

		got := m.View()
		styled := strings.Split(got.Content, "\n")
		plain := plainLines(got)
		for _, name := range []string{"nginx", "front", "app"} {
			line := styled[lineContaining(t, plain, "● "+name)]
			if hasBackground := strings.Contains(line, "48;"); hasBackground != (name == "front") {
				t.Errorf("row %s has background = %v, want %v", name, hasBackground, name == "front")
			}
		}
	})

	t.Run("terminal cursor is hidden without services", func(t *testing.T) {
		if got := scanned(t, &fakeDeps{}).View(); got.Cursor != nil {
			t.Errorf("View().Cursor = %+v, want nil", got.Cursor)
		}
	})

	sizes := []struct {
		name   string
		dialog bool
	}{{"panels", false}, {"dialog", true}}
	for _, tt := range sizes {
		t.Run("every line is as wide as the window and long rows are cut with "+tt.name, func(t *testing.T) {
			const width, height = 60, 24
			long := service(3000, strings.Repeat("long-name-", 8))
			m, _ := update(t, New((&fakeDeps{}).config()), tea.WindowSizeMsg{Width: width, Height: height})
			m, _ = update(t, m, scannedMsg{services: []discovery.Service{long}})
			if tt.dialog {
				m, _ = update(t, m, press('s'))
			}

			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != height {
				t.Errorf("View() has %d lines, want %d", len(lines), height)
			}
			for i, line := range lines {
				if got := lipgloss.Width(line); got != width {
					t.Errorf("line %d is %d cells wide, want %d: %q", i, got, width, line)
				}
			}
		})
	}

	t.Run("tagline is hidden when the window is too narrow for it", func(t *testing.T) {
		m, _ := update(t, New((&fakeDeps{}).config()), tea.WindowSizeMsg{Width: 50, Height: 24})
		expectText(t, m, []string{"▀█▀ █▀▀ ▄▀▀"}, []string{"share localhost"})
	})
}

func TestFormatSpan(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{-3 * time.Second, "0s"},
		{42 * time.Second, "42s"},
		{58*time.Minute + 30*time.Second, "58m"},
		{3*time.Hour + 5*time.Minute, "3h 05m"},
	}
	for _, tt := range tests {
		if got := formatSpan(tt.in); got != tt.want {
			t.Errorf("formatSpan(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
