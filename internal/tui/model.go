// Package tui renders the t5s terminal interface.
package tui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/share"
)

const (
	refreshInterval = 3 * time.Second
	scanTimeout     = 5 * time.Second
	clockInterval   = time.Second
)

// ScanFunc lists the local web services to display.
type ScanFunc func(context.Context) ([]discovery.Service, error)

// Config holds what the screen needs from the outside.
type Config struct {
	Scan    ScanFunc
	Backend share.Backend
	Copy    func(text string) error
}

type screen int

const (
	screenServices screen = iota
	screenAccess
)

// Model is the Bubble Tea model of the t5s screen.
type Model struct {
	cfg       Config
	services  []discovery.Service
	shares    []activeShare
	opening   []discovery.Service
	screen    screen
	cursor    int
	offset    int
	access    accessView
	scanned   bool
	err       error
	dialog    *shareDialog
	lastAllow string
	flash     string
	flashID   int
	quitting  bool
	now       time.Time
	width     int
	height    int
	keys      keyMap
}

type scannedMsg struct {
	services []discovery.Service
	err      error
}

type tickMsg struct{}

type clockMsg time.Time

// New returns a Model that lists the services found by cfg.Scan and shares them through cfg.Backend.
func New(cfg Config) Model {
	return Model{cfg: cfg, keys: newKeyMap(), width: defaultWidth, height: defaultHeight}
}

// Init starts the first scan and the clock.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), clockCmd())
}

// Update applies msg to the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	return next.scrolled(), cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scannedMsg:
		return m.applyScan(msg), tickCmd()
	case tickMsg:
		return m, m.scanCmd()
	case clockMsg:
		return m.applyClock(time.Time(msg)), clockCmd()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.dialog != nil {
			dialog := m.dialog.resized(m.allowWidth())
			m.dialog = &dialog
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case shareStartedMsg, shareFailedMsg, shareEndedMsg, copiedMsg, flashExpiredMsg:
		return m.handleShareMsg(msg)
	}
	return m, nil
}

func (m Model) scanCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		services, err := m.cfg.Scan(ctx)
		return scannedMsg{services: services, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

func clockCmd() tea.Cmd {
	return tea.Tick(clockInterval, func(now time.Time) tea.Msg {
		return clockMsg(now)
	})
}

func (m Model) applyScan(msg scannedMsg) Model {
	m.scanned = true
	m.err = msg.err
	if msg.err != nil {
		return m
	}
	selected, hadSelection := m.selectedRow()
	m.services = msg.services
	return m.selectPort(selected.service.Addr.Port(), hadSelection)
}

func (m Model) selectPort(port uint16, hadSelection bool) Model {
	rows := m.rows()
	m.cursor = min(m.cursor, max(len(rows)-1, 0))
	if !hadSelection {
		return m
	}
	for i, r := range rows {
		if r.service.Addr.Port() == port {
			m.cursor = i
		}
	}
	return m
}

func (m Model) selectedRow() (row, bool) {
	rows := m.rows()
	if m.cursor >= len(rows) {
		return row{}, false
	}
	return rows[m.cursor], true
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.dialog != nil {
		return m.handleDialogKey(msg)
	}
	if m.screen == screenAccess {
		return m.handleAccessKey(msg)
	}
	selected, hasSelection := m.selectedRow()
	switch {
	case key.Matches(msg, m.keys.quit):
		return m.quit()
	case key.Matches(msg, m.keys.up):
		m.cursor = max(m.cursor-1, 0)
	case key.Matches(msg, m.keys.down):
		m.cursor = min(m.cursor+1, max(len(m.rows())-1, 0))
	case hasSelection && key.Matches(msg, m.keys.open):
		return m.openAccess(selected)
	case hasSelection && key.Matches(msg, m.keys.share):
		return m.openDialog(selected)
	case hasSelection && key.Matches(msg, m.keys.stop):
		return m.stopShare(selected)
	case hasSelection && key.Matches(msg, m.keys.copy):
		return m.copyURL(selected)
	}
	return m, nil
}
