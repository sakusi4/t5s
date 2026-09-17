package tui

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

const (
	tagline        = "share localhost with only the people you allow"
	scanningStatus = "scanning…"
	headerGap      = 4
)

var logo = []string{
	`▀█▀ █▀▀ ▄▀▀`,
	` █  ▀▀▄  ▀▄`,
	` ▀  ▄▄▀ ▀▀ `,
}

func (m Model) headerLines(width int) []string {
	if !m.showsHeader() {
		return nil
	}
	left := make([]string, len(logo))
	for i, row := range logo {
		left[i] = " " + lipgloss.NewStyle().Foreground(logoGradient[i]).Bold(true).Render(row)
	}
	right := []string{statusStyle.Render(m.status()), detailStyle.Render("refresh " + refreshInterval.String()), ""}

	withTagline := left[0] + pad("", headerGap) + taglineStyle.Render(tagline)
	if lipgloss.Width(withTagline)+headerGap+lipgloss.Width(right[0]) < width {
		left[0] = withTagline
	}

	lines := make([]string, len(logo))
	for i := range lines {
		lines[i] = pad(left[i], width-lipgloss.Width(right[i])-1) + right[i]
	}
	return lines
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
