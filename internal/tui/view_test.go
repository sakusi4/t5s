package tui

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plainLines(v tea.View) []string {
	return strings.Split(sgrPattern.ReplaceAllString(v.Content, ""), "\n")
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

func TestModel_View(t *testing.T) {
	three := []discovery.Service{service(80, "nginx"), service(5173, "front"), service(9030, "app")}

	tests := []struct {
		name    string
		msgs    []tea.Msg
		want    []string
		notWant []string
	}{
		{
			name: "before the first scan",
			want: []string{"scanning…", "refresh 3s", "╭─ Local services ─", "╰─", "↑/k up · ↓/j down · q quit"},
		},
		{
			name: "header shows the logo and the tagline",
			want: []string{"▀█▀ █▀▀ ▄▀▀", "share localhost with only the people you allow"},
		},
		{
			name: "no services",
			msgs: []tea.Msg{scannedMsg{}},
			want: []string{"0 services", "No local web services found."},
		},
		{
			name:    "one service",
			msgs:    []tea.Msg{scannedMsg{services: three[:1]}},
			want:    []string{"1 service"},
			notWant: []string{"1 services"},
		},
		{
			name: "services",
			msgs: []tea.Msg{scannedMsg{services: three}},
			want: []string{"3 services", "NAME", "FRAMEWORK", "ADDRESS", "● nginx", "localhost:5173"},
		},
		{
			name: "missing framework falls back to process then dash",
			msgs: []tea.Msg{scannedMsg{services: []discovery.Service{
				{Addr: netip.MustParseAddrPort("127.0.0.1:80"), Name: "nginx", Process: "nginx-worker"},
				{Addr: netip.MustParseAddrPort("192.168.0.5:8080")},
			}}},
			want: []string{"nginx-worker", "● - ", "192.168.0.5:8080"},
		},
		{
			name: "scan error",
			msgs: []tea.Msg{scannedMsg{err: errors.New("lsof failed")}},
			want: []string{"lsof failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(scanReturning(nil, nil))
			for _, msg := range tt.msgs {
				m, _ = update(t, m, msg)
			}
			got := strings.Join(plainLines(m.View()), "\n")
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("View() = %q, want it to contain %q", got, want)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(got, notWant) {
					t.Errorf("View() = %q, want it not to contain %q", got, notWant)
				}
			}
		})
	}

	t.Run("screen is full and black", func(t *testing.T) {
		got := New(scanReturning(nil, nil)).View()
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
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: three})
		m, _ = update(t, m, press('j'))

		got := m.View()
		selected := lineContaining(t, plainLines(got), "front")
		if got.Cursor == nil || got.Cursor.Y != selected {
			t.Errorf("View().Cursor = %+v, want it on line %d", got.Cursor, selected)
		}
	})

	t.Run("only the selected row has a background", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{services: three})
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
		m, _ := update(t, New(scanReturning(nil, nil)), scannedMsg{})
		if got := m.View(); got.Cursor != nil {
			t.Errorf("View().Cursor = %+v, want nil", got.Cursor)
		}
	})

	t.Run("every line is as wide as the window and long rows are cut", func(t *testing.T) {
		const width, height = 50, 16
		long := []discovery.Service{service(3000, strings.Repeat("long-name-", 8))}
		m, _ := update(t, New(scanReturning(nil, nil)), tea.WindowSizeMsg{Width: width, Height: height})
		m, _ = update(t, m, scannedMsg{services: long})

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

	t.Run("tagline is hidden when the window is too narrow for it", func(t *testing.T) {
		m, _ := update(t, New(scanReturning(nil, nil)), tea.WindowSizeMsg{Width: 50, Height: 16})
		got := strings.Join(plainLines(m.View()), "\n")
		if strings.Contains(got, "share localhost") || !strings.Contains(got, "▀█▀ █▀▀ ▄▀▀") {
			t.Errorf("View() = %q, want the logo without the tagline at width 50", got)
		}
	})
}
