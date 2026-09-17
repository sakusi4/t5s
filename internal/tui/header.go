package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

const (
	tagline   = "share localhost with only the people you allow"
	headerGap = 4
	keyRows   = 2
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
	room := width - lipgloss.Width(left[0]) - headerGap - 1
	keysWidth := min(lipgloss.Width(keyGrid(m.keys.sharedBindings())[0]), room)
	right := make([]string, len(logo))
	for i, row := range keyGrid(m.visibleKeys()) {
		right[len(logo)-keyRows+i] = pad(row, keysWidth)
	}

	withTagline := left[0] + pad("", headerGap) + taglineStyle.Render(tagline)
	if lipgloss.Width(withTagline) < width {
		left[0] = withTagline
	}

	lines := make([]string, len(logo))
	for i := range lines {
		lines[i] = pad(left[i], max(width-lipgloss.Width(right[i])-1, lipgloss.Width(left[i])+headerGap)) + right[i]
	}
	return lines
}

func keyGrid(bindings []key.Binding) []string {
	rows := make([]string, keyRows)
	for start := 0; start < len(bindings); start += keyRows {
		column := bindings[start:min(start+keyRows, len(bindings))]
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
			if start > 0 {
				rows[i] += pad("", columnGap)
			}
			rows[i] += pad(cell, keyWidth+1+descWidth)
		}
	}
	return rows
}
