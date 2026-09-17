package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

const (
	tagline        = "share localhost with only the people you allow"
	scanningStatus = "scanning…"
	headerGap      = 4
	keyRows        = 2
)

var logo = []string{
	`▀█▀ █▀▀ ▄▀▀`,
	` █  ▀▀▄  ▀▄`,
	` ▀  ▄▄▀ ▀▀ `,
}

func (m Model) headerLines(width int) []string {
	left := make([]string, len(logo))
	for i, row := range logo {
		left[i] = " " + lipgloss.NewStyle().Foreground(logoGradient[i]).Bold(true).Render(row)
	}
	right := []string{statusStyle.Render(m.status()), detailStyle.Render("refresh " + refreshInterval.String()), ""}

	withTagline := left[0] + pad("", headerGap) + taglineStyle.Render(tagline)
	if lipgloss.Width(withTagline)+headerGap+lipgloss.Width(right[0]) < width {
		left[0] = withTagline
	}
	for i, row := range keyGrid(m.visibleKeys()) {
		left[len(logo)-keyRows+i] += pad("", headerGap) + row
	}

	lines := make([]string, len(logo))
	for i := range lines {
		if lipgloss.Width(left[i])+headerGap+lipgloss.Width(right[i]) >= width {
			right[i] = ""
		}
		lines[i] = pad(left[i], width-lipgloss.Width(right[i])-1) + right[i]
	}
	return lines
}

func keyGrid(bindings []key.Binding) []string {
	rows := make([]string, keyRows)
	for column := range slices.Chunk(bindings, keyRows) {
		keyWidth, descWidth := 0, 0
		for _, b := range column {
			keyWidth = max(keyWidth, lipgloss.Width(b.Help().Key))
			descWidth = max(descWidth, lipgloss.Width(b.Help().Desc))
		}
		for i := range rows {
			cell := ""
			if i < len(column) {
				cell = pad(keyStyle.Render(column[i].Help().Key), keyWidth+1) + descStyle.Render(column[i].Help().Desc)
			}
			rows[i] += pad(cell, keyWidth+1+descWidth+columnGap)
		}
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

func (m Model) status() string {
	if !m.scanned {
		return scanningStatus
	}
	status := plural(len(m.rows()), "service")
	if len(m.shares) > 0 {
		status += " · " + plural(len(m.shares), "share")
	}
	return status
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
