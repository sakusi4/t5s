// Package tui renders the t5s terminal interface.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
)

const (
	refreshInterval = 3 * time.Second
	scanTimeout     = 5 * time.Second
)

// ScanFunc lists the local web services to display.
type ScanFunc func(context.Context) ([]discovery.Service, error)

// Model is the Bubble Tea model of the t5s screen.
type Model struct {
	scan     ScanFunc
	services []discovery.Service
	cursor   int
	scanned  bool
	err      error
	width    int
	height   int
	keys     keyMap
}

type scannedMsg struct {
	services []discovery.Service
	err      error
}

type tickMsg struct{}

// New returns a Model that lists the services found by scan.
func New(scan ScanFunc) Model {
	return Model{scan: scan, keys: newKeyMap(), width: defaultWidth, height: defaultHeight}
}

// Init starts the first scan.
func (m Model) Init() tea.Cmd {
	return m.scanCmd()
}

// Update applies msg to the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scannedMsg:
		return m.applyScan(msg), tickCmd()
	case tickMsg:
		return m, m.scanCmd()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		services, err := m.scan(ctx)
		return scannedMsg{services: services, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m Model) applyScan(msg scannedMsg) Model {
	m.scanned = true
	m.err = msg.err
	if msg.err != nil {
		return m
	}
	selected, hadSelection := m.selectedPort()
	m.services = msg.services
	m.cursor = min(m.cursor, max(len(m.services)-1, 0))
	if !hadSelection {
		return m
	}
	for i, s := range m.services {
		if s.Addr.Port() == selected {
			m.cursor = i
		}
	}
	return m
}

func (m Model) selectedPort() (uint16, bool) {
	if m.cursor >= len(m.services) {
		return 0, false
	}
	return m.services[m.cursor].Addr.Port(), true
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.up):
		m.cursor = max(m.cursor-1, 0)
	case key.Matches(msg, m.keys.down):
		m.cursor = min(m.cursor+1, max(len(m.services)-1, 0))
	}
	return m, nil
}
