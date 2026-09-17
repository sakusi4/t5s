package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	black  = lipgloss.Color("#000000")
	white  = lipgloss.Color("#FFFFFF")
	gray   = lipgloss.Color("#8A8A8A")
	violet = lipgloss.Color("#A78BFA")
	orchid = lipgloss.Color("#C084FC")
	pink   = lipgloss.Color("#F472B6")
	green  = lipgloss.Color("#4ADE80")
	red    = lipgloss.Color("#F87171")
)

var logoGradient = []color.Color{violet, orchid, pink}

var (
	taglineStyle   = lipgloss.NewStyle().Foreground(gray)
	statusStyle    = lipgloss.NewStyle().Foreground(white).Bold(true)
	detailStyle    = lipgloss.NewStyle().Foreground(gray)
	borderStyle    = lipgloss.NewStyle().Foreground(violet)
	panelStyle     = lipgloss.NewStyle().Foreground(white).Bold(true)
	columnStyle    = lipgloss.NewStyle().Foreground(gray).Bold(true)
	runningStyle   = lipgloss.NewStyle().Foreground(green)
	downStyle      = lipgloss.NewStyle().Foreground(red)
	nameStyle      = lipgloss.NewStyle().Foreground(white)
	frameworkStyle = lipgloss.NewStyle().Foreground(violet)
	addrStyle      = lipgloss.NewStyle().Foreground(gray)
	noteStyle      = lipgloss.NewStyle().Foreground(pink)
	urlStyle       = lipgloss.NewStyle().Foreground(white).Bold(true)
	allowedStyle   = lipgloss.NewStyle().Foreground(green)
	blockedStyle   = lipgloss.NewStyle().Foreground(red)
	selectedStyle  = lipgloss.NewStyle().Background(violet).Foreground(black).Bold(true)
	chosenStyle    = lipgloss.NewStyle().Foreground(violet).Bold(true)
	emptyStyle     = lipgloss.NewStyle().Foreground(gray)
	errorStyle     = lipgloss.NewStyle().Foreground(red)
	flashStyle     = lipgloss.NewStyle().Foreground(pink).Bold(true)
	keyStyle       = lipgloss.NewStyle().Foreground(violet).Bold(true)
	descStyle      = lipgloss.NewStyle().Foreground(gray)
)
