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
	accessTitle      = "Access"
	dialogTitle      = "Share %s (%s)"
	nameTitle        = "NAME"
	frameworkTitle   = "FRAMEWORK"
	addrTitle        = "ADDRESS"
	expiresTitle     = "EXPIRES"
	urlTitle         = "URL"
	clientTitle      = "ADDRESS"
	verdictTitle     = "VERDICT"
	requestsTitle    = "REQUESTS"
	lastSeenTitle    = "LAST SEEN"
	allowLabel       = "Allow   "
	expireLabel      = "Expire  "
	scanningStatus   = "scanning…"
	refreshText      = "refresh %s"
	noServicesText   = "No local web services found."
	noAccessText     = "No requests yet."
	rangeText        = "%d–%d of %d"
	addressesText    = "%d addresses"
	untrackedText    = "+%d untracked requests"
	openingNote      = "opening tunnel…"
	notRespondingTag = "not responding"
	allowedTag       = "allowed"
	blockedTag       = "blocked"
	placeholder      = "-"
	runningMark      = "●"
	sharedMark       = "◉"
	hintSeparator    = " · "
	ellipsis         = "…"
	rowIndent        = 3
	columnGap        = 3
	borderWidth      = 1
	cursorWidth      = 1
	dialogIndent     = " "
	statusHeight     = 1
	dialogHeight     = 5
	defaultWidth     = 80
	defaultHeight    = 24
)

// View renders the model.
func (m Model) View() tea.View {
	inner := m.width - 2*borderWidth
	header := m.headerLines(m.width)
	body, cursorLine := m.bodyLines(inner)
	var dialog []string
	if m.dialog != nil {
		dialog = m.dialog.lines(inner)
	}

	lines := slices.Concat(header, body, dialog, []string{m.statusLine()})
	for i, line := range lines {
		lines[i] = fit(line, m.width)
	}
	overflow := max(len(lines)-m.height, 0)
	lines = append(lines[overflow:], make([]string, max(m.height-len(lines), 0))...)

	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.BackgroundColor = black
	v.ForegroundColor = white
	switch {
	case m.dialog != nil:
		v.Cursor = m.dialog.cursor(borderWidth+1, len(header)+len(body)+borderWidth-overflow)
	case cursorLine >= 0:
		v.Cursor = tea.NewCursor(borderWidth, len(header)+cursorLine-overflow)
		v.Cursor.Shape = tea.CursorBar
		v.Cursor.Blink = false
	}
	return v
}

func (m Model) bodyHeight() int {
	height := m.height - len(logo) - statusHeight
	if m.dialog != nil {
		height -= dialogHeight
	}
	return max(height, 2*borderWidth+1)
}

func (m Model) allowWidth() int {
	return max(m.width-2*borderWidth-len(dialogIndent)-lipgloss.Width(allowLabel)-cursorWidth, 1)
}

func (m Model) visibleRows() int {
	return max(m.bodyHeight()-2*borderWidth-1, 1)
}

func (m Model) visibleAccesses() int {
	return max(m.bodyHeight()-2*borderWidth-2, 1)
}

func (m Model) bodyLines(inner int) (lines []string, cursorLine int) {
	if m.screen == screenAccess {
		if current, ok := m.accessShare(); ok {
			return m.accessPanel(current, inner)
		}
	}
	return m.servicesPanel(inner)
}

func (m Model) servicesPanel(inner int) (lines []string, cursorLine int) {
	rows := m.rows()
	content := make([]string, 0, m.bodyHeight())
	cursorLine = -1
	switch {
	case m.scanned && len(rows) == 0:
		content = append(content, "", emptyStyle.Render(strings.Repeat(" ", rowIndent)+noServicesText))
	default:
		table := m.serviceTable(rows, inner)
		content = append(content, table...)
		if len(rows) > 0 {
			cursorLine = borderWidth + 1 + m.cursor - m.offset
		}
	}
	return panel(servicesTitle, m.servicesLabel(), pad2(content, m.bodyHeight()-2*borderWidth), inner), cursorLine
}

func (m Model) servicesLabel() string {
	rows := m.rows()
	label := plural(len(rows), "service")
	if span := rangeLabel(m.offset, m.visibleRows(), len(rows)); span != "" {
		label = span + " services"
	}
	if !m.scanned {
		label = scanningStatus
	}
	if len(m.shares) > 0 {
		label += hintSeparator + plural(len(m.shares), "share")
	}
	return label + hintSeparator + fmt.Sprintf(refreshText, refreshInterval)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (m Model) serviceTable(rows []row, inner int) []string {
	widths := []int{lipgloss.Width(nameTitle), lipgloss.Width(frameworkTitle), lipgloss.Width(addrTitle), lipgloss.Width(expiresTitle)}
	for _, r := range rows {
		cells := m.cells(r)
		for i := range widths {
			widths[i] = max(widths[i], lipgloss.Width(cells[i]))
		}
	}
	indent := strings.Repeat(" ", rowIndent)
	titles := []string{nameTitle, frameworkTitle, addrTitle, expiresTitle, urlTitle}
	lines := []string{columnStyle.Render(indent + strings.Join(alignCells(widths, titles), ""))}

	end := min(m.offset+m.visibleRows(), len(rows))
	for i := m.offset; i < end; i++ {
		r := rows[i]
		cells := alignCells(widths, m.cells(r))
		if i == m.cursor {
			lines = append(lines, selectedStyle.Render(fit(" "+r.mark()+" "+strings.Join(cells, ""), inner)))
			continue
		}
		styles := []lipgloss.Style{nameStyle, frameworkStyle, r.addrStyle(), detailStyle, r.urlStyle()}
		line := " " + r.markStyle().Render(r.mark()) + " "
		for j, cell := range cells {
			line += styles[j].Render(cell)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) cells(r row) []string {
	cells := []string{displayName(r.service), displayFramework(r.service), displayAddr(r.service), "", ""}
	switch {
	case r.share != nil:
		cells[3] = formatSpan(r.share.ExpiresAt().Sub(m.now))
		cells[4] = r.share.URL().String()
	case r.opening:
		cells[4] = openingNote
	}
	if !r.responding {
		cells[4] = strings.TrimSpace(cells[4] + hintSeparator + notRespondingTag)
	}
	return cells
}

func alignCells(widths []int, cells []string) []string {
	last := len(cells) - 1
	for last > 0 && cells[last] == "" {
		last--
	}
	aligned := make([]string, last+1)
	for i := range aligned {
		aligned[i] = cells[i]
		if i < last {
			aligned[i] = pad(cells[i], widths[i]+columnGap)
		}
	}
	return aligned
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

func (r row) addrStyle() lipgloss.Style {
	if !r.responding {
		return downStyle
	}
	return addrStyle
}

func (r row) urlStyle() lipgloss.Style {
	if r.share == nil {
		return noteStyle
	}
	return urlStyle
}

func (m Model) accessPanel(current activeShare, inner int) (lines []string, cursorLine int) {
	indent := strings.Repeat(" ", rowIndent)
	info := " " + urlStyle.Render(current.share.URL().String()) + detailStyle.Render(hintSeparator+formatSpan(current.share.ExpiresAt().Sub(m.now))+" left")
	content := []string{info}
	cursorLine = -1

	if len(current.accesses) == 0 {
		content = append(content, emptyStyle.Render(indent+noAccessText))
	} else {
		widths := []int{lipgloss.Width(clientTitle), lipgloss.Width(verdictTitle), lipgloss.Width(requestsTitle)}
		for _, a := range current.accesses {
			widths[0] = max(widths[0], lipgloss.Width(a.Addr.String()))
		}
		titles := []string{clientTitle, verdictTitle, requestsTitle, lastSeenTitle}
		content = append(content, columnStyle.Render(indent+strings.Join(alignCells(widths, titles), "")))
		end := min(m.access.offset+m.visibleAccesses(), len(current.accesses))
		for i := m.access.offset; i < end; i++ {
			a := current.accesses[i]
			verdict, verdictStyle := blockedTag, blockedStyle
			if a.Allowed {
				verdict, verdictStyle = allowedTag, allowedStyle
			}
			seen := formatSpan(m.now.Sub(a.LastSeen)) + " ago"
			cells := alignCells(widths, []string{a.Addr.String(), verdict, fmt.Sprintf("%d", a.Requests), seen})
			if i == m.access.cursor {
				content = append(content, selectedStyle.Render(fit(indent+strings.Join(cells, ""), inner)))
				continue
			}
			content = append(content, indent+nameStyle.Render(cells[0])+verdictStyle.Render(cells[1])+detailStyle.Render(cells[2]+cells[3]))
		}
		cursorLine = borderWidth + 2 + m.access.cursor - m.access.offset
	}

	label := fmt.Sprintf(addressesText, len(current.accesses))
	if span := rangeLabel(m.access.offset, m.visibleAccesses(), len(current.accesses)); span != "" {
		label = span + " addresses"
	}
	if current.untracked > 0 {
		label += hintSeparator + fmt.Sprintf(untrackedText, current.untracked)
	}
	title := accessTitle + hintSeparator + displayName(current.service)
	return panel(title, label, pad2(content, m.bodyHeight()-2*borderWidth), inner), cursorLine
}

func rangeLabel(offset, visible, total int) string {
	if total <= visible {
		return ""
	}
	return fmt.Sprintf(rangeText, offset+1, min(offset+visible, total), total)
}

func pad2(lines []string, height int) []string {
	if len(lines) >= height {
		return lines[:max(height, 0)]
	}
	return append(lines, make([]string, height-len(lines))...)
}

func panel(title, right string, content []string, inner int) []string {
	if lipgloss.Width("─ "+title+"  "+right+" ─") > inner {
		right = ""
	}
	left := "─ " + title + " "
	tail := ""
	if right != "" {
		tail = " " + right + " ─"
	}
	rule := strings.Repeat("─", max(inner-lipgloss.Width(left)-lipgloss.Width(tail), 0))
	top := borderStyle.Render("╭─ ") + panelStyle.Render(title) + " " + borderStyle.Render(rule)
	if right != "" {
		top += " " + detailStyle.Render(right) + borderStyle.Render(" ─")
	}
	top += borderStyle.Render("╮")

	side := borderStyle.Render("│")
	lines := []string{top}
	for _, line := range content {
		lines = append(lines, side+fit(line, inner)+side)
	}
	return append(lines, borderStyle.Render("╰"+strings.Repeat("─", max(inner, 0))+"╯"))
}

func (d shareDialog) lines(inner int) []string {
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
		dialogIndent + allowLabelStyle.Render(allowLabel) + d.allow.View(),
		dialogIndent + expireLabelStyle.Render(expireLabel) + strings.Join(choices, " "),
		problem,
	}
	return panel(fmt.Sprintf(dialogTitle, displayName(d.service), displayAddr(d.service)), "", content, inner)
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

func (m Model) statusLine() string {
	switch {
	case m.flash != "":
		return " " + flashStyle.Render(m.flash)
	case m.err != nil:
		return " " + errorStyle.Render(m.err.Error())
	default:
		return ""
	}
}

func (m Model) visibleKeys() []key.Binding {
	if m.dialog != nil {
		return m.keys.dialogBindings()
	}
	bindings := []key.Binding{m.keys.up, m.keys.down}
	if m.screen == screenAccess {
		return append(bindings, m.keys.back, m.keys.copy, m.keys.stop, m.keys.quit)
	}
	if selected, ok := m.selectedRow(); ok {
		switch {
		case selected.share != nil:
			return m.keys.sharedBindings()
		case !selected.opening:
			bindings = append(bindings, m.keys.share)
		}
	}
	return append(bindings, m.keys.quit)
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
	if lipgloss.Width(s) <= width {
		return pad(s, width)
	}
	return pad(lipgloss.NewStyle().MaxWidth(max(width-lipgloss.Width(ellipsis), 0)).Render(s)+ellipsis, width)
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
