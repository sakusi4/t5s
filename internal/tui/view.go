package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

const (
	panelTitle     = "Local services"
	nameTitle      = "NAME"
	frameworkTitle = "FRAMEWORK"
	addrTitle      = "ADDRESS"
	noServicesText = "No local web services found."
	placeholder    = "-"
	runningMark    = "●"
	hintSeparator  = " · "
	rowIndent      = 3
	columnGap      = 2
	columnShare    = 4
	borderWidth    = 1
	defaultWidth   = 80
	defaultHeight  = 24
)

// View renders the model.
func (m Model) View() tea.View {
	header := m.headerLines(m.width)
	footer := m.footerLines()
	inner := m.width - 2*borderWidth
	rows, selected := m.tableLines(inner)
	filler := make([]string, max(m.height-len(header)-len(footer)-len(rows)-2*borderWidth, 0))

	lines := slices.Concat(header, panel(panelTitle, slices.Concat(rows, filler), inner), footer)
	for i, line := range lines {
		lines[i] = fit(line, m.width)
	}

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.BackgroundColor = black
	v.ForegroundColor = white
	if len(m.services) > 0 {
		v.Cursor = tea.NewCursor(borderWidth, len(header)+borderWidth+selected)
		v.Cursor.Shape = tea.CursorBar
		v.Cursor.Blink = false
	}
	return v
}

func panel(title string, content []string, inner int) []string {
	rule := strings.Repeat("─", max(inner-lipgloss.Width("─ "+title+" "), 0))
	top := borderStyle.Render("╭─ ") + panelStyle.Render(title) + " " + borderStyle.Render(rule+"╮")

	side := borderStyle.Render("│")
	lines := []string{top}
	for _, line := range content {
		lines = append(lines, side+fit(line, inner)+side)
	}
	return append(lines, borderStyle.Render("╰"+strings.Repeat("─", max(inner, 0))+"╯"))
}

func (m Model) tableLines(inner int) (lines []string, selected int) {
	indent := strings.Repeat(" ", rowIndent)
	if m.scanned && len(m.services) == 0 {
		return []string{"", emptyStyle.Render(indent + noServicesText)}, 0
	}

	nameWidth, frameworkWidth := lipgloss.Width(nameTitle), lipgloss.Width(frameworkTitle)
	for _, s := range m.services {
		nameWidth = max(nameWidth, lipgloss.Width(displayName(s)))
		frameworkWidth = max(frameworkWidth, lipgloss.Width(displayFramework(s)))
	}
	nameWidth = max(nameWidth+columnGap, inner/columnShare)
	frameworkWidth = max(frameworkWidth+columnGap, inner/columnShare)

	lines = []string{columnStyle.Render(indent + pad(nameTitle, nameWidth) + pad(frameworkTitle, frameworkWidth) + addrTitle)}
	selected = len(lines) + m.cursor
	for i, s := range m.services {
		name, framework, addr := pad(displayName(s), nameWidth), pad(displayFramework(s), frameworkWidth), displayAddr(s)
		if i == m.cursor {
			lines = append(lines, selectedStyle.Render(fit(" "+runningMark+" "+name+framework+addr, inner)))
			continue
		}
		lines = append(lines, " "+runningStyle.Render(runningMark)+" "+nameStyle.Render(name)+frameworkStyle.Render(framework)+addrStyle.Render(addr))
	}
	return lines, selected
}

func (m Model) footerLines() []string {
	hints := make([]string, 0, len(m.keys.bindings()))
	for _, b := range m.keys.bindings() {
		hints = append(hints, keyStyle.Render(b.Help().Key)+" "+descStyle.Render(b.Help().Desc))
	}
	help := " " + strings.Join(hints, descStyle.Render(hintSeparator))
	if m.err == nil {
		return []string{help}
	}
	return []string{" " + errorStyle.Render(m.err.Error()), help}
}

func pad(s string, width int) string {
	return lipgloss.PlaceHorizontal(width, lipgloss.Left, s)
}

func fit(s string, width int) string {
	return pad(lipgloss.NewStyle().MaxWidth(max(width, 0)).Render(s), width)
}

func displayName(s discovery.Service) string {
	if s.Name == "" {
		return placeholder
	}
	return s.Name
}

func displayFramework(s discovery.Service) string {
	if s.Framework != "" {
		return s.Framework
	}
	if s.Process != "" {
		return s.Process
	}
	return placeholder
}

func displayAddr(s discovery.Service) string {
	if s.Addr.Addr().IsLoopback() {
		return fmt.Sprintf("localhost:%d", s.Addr.Port())
	}
	return s.Addr.String()
}
