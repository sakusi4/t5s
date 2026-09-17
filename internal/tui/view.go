package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

const (
	servicesTitle    = "Local services"
	sharesTitle      = "Active shares"
	accessTitle      = "Access"
	dialogTitle      = "Share %s (%s)"
	nameTitle        = "NAME"
	frameworkTitle   = "FRAMEWORK"
	addrTitle        = "ADDRESS"
	allowLabel       = "Allow   "
	expireLabel      = "Expire  "
	noServicesText   = "No local web services found."
	noSharesText     = "No active shares. Press s to share the selected service."
	noAccessText     = "Requests to the selected share appear here."
	untrackedText    = "+%d requests from other addresses"
	sharedNote       = "shared"
	openingNote      = "opening tunnel…"
	notRespondingTag = "not responding"
	allowedTag       = "allowed"
	blockedTag       = "blocked"
	placeholder      = "-"
	runningMark      = "●"
	sharedMark       = "◉"
	hintSeparator    = " · "
	rowIndent        = 3
	columnGap        = 2
	columnShare      = 5
	borderWidth      = 1
	maxAccessRows    = 6
	dialogRows       = 4
	defaultWidth     = 80
	defaultHeight    = 24
)

// View renders the model.
func (m Model) View() tea.View {
	inner := m.width - 2*borderWidth
	header := m.headerLines(m.width)
	footer := m.footerLines()
	lower := m.lowerPanels(inner)
	table, selected := m.tableLines(inner)
	filler := make([]string, max(m.height-len(header)-len(footer)-len(lower)-len(table)-2*borderWidth, 0))

	lines := slices.Concat(header, panel(servicesTitle, slices.Concat(table, filler), inner), lower, footer)
	for i, line := range lines {
		lines[i] = fit(line, m.width)
	}

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.BackgroundColor = black
	v.ForegroundColor = white
	switch {
	case m.dialog != nil:
		dialogTop := len(header) + len(table) + len(filler) + 2*borderWidth
		v.Cursor = m.dialog.cursor(borderWidth+1, dialogTop+borderWidth)
	case len(m.rows()) > 0:
		v.Cursor = tea.NewCursor(borderWidth, len(header)+borderWidth+selected)
		v.Cursor.Shape = tea.CursorBar
		v.Cursor.Blink = false
	}
	return v
}

func (m Model) lowerPanels(inner int) []string {
	if m.dialog != nil {
		return m.dialog.lines(inner, m.keys)
	}
	return slices.Concat(panel(sharesTitle, m.shareLines(), inner), panel(m.accessPanelTitle(), m.accessLines(), inner))
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
	rows := m.rows()
	if m.scanned && len(rows) == 0 {
		return []string{"", emptyStyle.Render(indent + noServicesText)}, 0
	}

	nameWidth, frameworkWidth, addrWidth := lipgloss.Width(nameTitle), lipgloss.Width(frameworkTitle), lipgloss.Width(addrTitle)
	for _, r := range rows {
		nameWidth = max(nameWidth, lipgloss.Width(displayName(r.service)))
		frameworkWidth = max(frameworkWidth, lipgloss.Width(displayFramework(r.service)))
		addrWidth = max(addrWidth, lipgloss.Width(displayAddr(r.service)))
	}
	nameWidth = max(nameWidth+columnGap, inner/columnShare)
	frameworkWidth = max(frameworkWidth+columnGap, inner/columnShare)
	addrWidth += columnGap

	lines = []string{columnStyle.Render(indent + pad(nameTitle, nameWidth) + pad(frameworkTitle, frameworkWidth) + addrTitle)}
	selected = len(lines) + m.cursor
	for i, r := range rows {
		name, framework, addr := pad(displayName(r.service), nameWidth), pad(displayFramework(r.service), frameworkWidth), pad(displayAddr(r.service), addrWidth)
		if i == m.cursor {
			lines = append(lines, selectedStyle.Render(fit(" "+r.mark()+" "+name+framework+addr+r.note(), inner)))
			continue
		}
		lines = append(lines, " "+r.markStyle().Render(r.mark())+" "+nameStyle.Render(name)+frameworkStyle.Render(framework)+addrStyle.Render(addr)+noteStyle.Render(r.note()))
	}
	return lines, selected
}

func (r row) mark() string {
	if r.share != nil {
		return sharedMark
	}
	return runningMark
}

func (r row) markStyle() lipgloss.Style {
	if !r.responding {
		return downStyle
	}
	return runningStyle
}

func (r row) note() string {
	var notes []string
	switch {
	case r.share != nil:
		notes = append(notes, sharedNote)
	case r.opening:
		notes = append(notes, openingNote)
	}
	if !r.responding {
		notes = append(notes, notRespondingTag)
	}
	return strings.Join(notes, hintSeparator)
}

func (m Model) shareLines() []string {
	if len(m.shares) == 0 {
		return []string{emptyStyle.Render(" " + noSharesText)}
	}
	nameWidth, urlWidth := 0, 0
	for _, a := range m.shares {
		nameWidth = max(nameWidth, lipgloss.Width(displayName(a.service)))
		urlWidth = max(urlWidth, lipgloss.Width(a.share.URL().String()))
	}
	lines := make([]string, len(m.shares))
	for i, a := range m.shares {
		left := formatSpan(a.share.ExpiresAt().Sub(m.now)) + " left"
		lines[i] = " " + nameStyle.Render(pad(displayName(a.service), nameWidth+columnGap)) +
			urlStyle.Render(pad(a.share.URL().String(), urlWidth+columnGap)) + detailStyle.Render(left)
	}
	return lines
}

func (m Model) accessPanelTitle() string {
	if selected, ok := m.selectedShare(); ok {
		return accessTitle + hintSeparator + displayName(selected.service)
	}
	return accessTitle
}

func (m Model) selectedShare() (activeShare, bool) {
	selected, ok := m.selectedRow()
	if !ok {
		return activeShare{}, false
	}
	i := slices.IndexFunc(m.shares, func(a activeShare) bool { return a.service.Addr.Port() == selected.service.Addr.Port() })
	if i < 0 {
		return activeShare{}, false
	}
	return m.shares[i], true
}

func (m Model) accessLines() []string {
	selected, ok := m.selectedShare()
	if !ok || len(selected.accesses) == 0 {
		return []string{emptyStyle.Render(" " + noAccessText)}
	}
	shown := selected.accesses[:min(len(selected.accesses), maxAccessRows)]
	addrWidth := 0
	for _, a := range shown {
		addrWidth = max(addrWidth, lipgloss.Width(a.Addr.String()))
	}
	lines := make([]string, 0, len(shown)+1)
	for _, a := range shown {
		verdict := blockedStyle.Render(pad(blockedTag, len(blockedTag)+columnGap))
		if a.Allowed {
			verdict = allowedStyle.Render(pad(allowedTag, len(allowedTag)+columnGap))
		}
		lines = append(lines, " "+nameStyle.Render(pad(a.Addr.String(), addrWidth+columnGap))+verdict+
			detailStyle.Render(pad(fmt.Sprintf("%d req", a.Requests), 10)+formatSpan(m.now.Sub(a.LastSeen))+" ago"))
	}
	hidden := selected.untracked
	for _, a := range selected.accesses[len(shown):] {
		hidden += a.Requests
	}
	if hidden > 0 {
		lines = append(lines, emptyStyle.Render(" "+fmt.Sprintf(untrackedText, hidden)))
	}
	return lines
}

func (d shareDialog) lines(inner int, keys keyMap) []string {
	choices := make([]string, len(expiryChoices))
	for i, choice := range expiryChoices {
		choices[i] = descStyle.Render(" " + formatExpiry(choice) + " ")
		if i == d.expiry {
			choices[i] = chosenStyle.Render("‹" + formatExpiry(choice) + "›")
		}
	}
	allowLabelStyle, expireLabelStyle := chosenStyle, descStyle
	if d.field == fieldExpire {
		allowLabelStyle, expireLabelStyle = descStyle, chosenStyle
	}
	problem := ""
	if d.err != nil {
		problem = " " + errorStyle.Render(d.err.Error())
	}
	content := []string{
		" " + allowLabelStyle.Render(allowLabel) + d.allow.View(),
		" " + expireLabelStyle.Render(expireLabel) + strings.Join(choices, " "),
		problem,
		" " + hints(keys.dialogBindings()),
	}
	return panel(fmt.Sprintf(dialogTitle, displayName(d.service), displayAddr(d.service)), content, inner)
}

func (d shareDialog) cursor(x, y int) *tea.Cursor {
	if d.field == fieldExpire {
		cursor := tea.NewCursor(x, y+1)
		cursor.Shape = tea.CursorBar
		cursor.Blink = false
		return cursor
	}
	cursor := d.allow.Cursor()
	if cursor == nil {
		return nil
	}
	cursor.X += x + lipgloss.Width(allowLabel)
	cursor.Y += y
	return cursor
}

func (m Model) footerLines() []string {
	status := ""
	switch {
	case m.flash != "":
		status = " " + flashStyle.Render(m.flash)
	case m.err != nil:
		status = " " + errorStyle.Render(m.err.Error())
	}
	if m.dialog != nil {
		return []string{status, ""}
	}
	return []string{status, " " + hints(m.visibleKeys())}
}

func (m Model) visibleKeys() []key.Binding {
	bindings := []key.Binding{m.keys.up, m.keys.down}
	if selected, ok := m.selectedRow(); ok {
		switch {
		case selected.share != nil:
			bindings = append(bindings, m.keys.stop, m.keys.copy)
		case !selected.opening:
			bindings = append(bindings, m.keys.share)
		}
	}
	return append(bindings, m.keys.quit)
}

func hints(bindings []key.Binding) string {
	parts := make([]string, len(bindings))
	for i, b := range bindings {
		parts[i] = keyStyle.Render(b.Help().Key) + " " + descStyle.Render(b.Help().Desc)
	}
	return strings.Join(parts, descStyle.Render(hintSeparator))
}

func formatSpan(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(int(d.Seconds()), 0))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
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
