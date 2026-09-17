package tui

import (
	"fmt"
	"net/netip"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/proxy"
)

type accessView struct {
	port   uint16
	cursor int
	offset int
}

func (m Model) openAccess(selected row) (Model, tea.Cmd) {
	if selected.share == nil {
		return m.withFlash(fmt.Sprintf(notSharedText, displayName(selected.service)))
	}
	m.screen = screenAccess
	m.access = accessView{port: selected.service.Addr.Port()}
	return m.refreshAccesses(), nil
}

func (m Model) closeAccess() Model {
	m.screen = screenServices
	m.access = accessView{}
	return m
}

func (m Model) accessShare() (activeShare, bool) {
	i := slices.IndexFunc(m.shares, func(a activeShare) bool { return a.service.Addr.Port() == m.access.port })
	if i < 0 {
		return activeShare{}, false
	}
	return m.shares[i], true
}

func (m Model) handleAccessKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	current, ok := m.accessShare()
	if !ok {
		return m.closeAccess(), nil
	}
	switch {
	case key.Matches(msg, m.keys.quit):
		return m.quit()
	case key.Matches(msg, m.keys.back):
		return m.closeAccess(), nil
	case key.Matches(msg, m.keys.up):
		m.access.cursor = max(m.access.cursor-1, 0)
	case key.Matches(msg, m.keys.down):
		m.access.cursor = min(m.access.cursor+1, max(len(current.accesses)-1, 0))
	case key.Matches(msg, m.keys.stop):
		current.share.Stop()
	case key.Matches(msg, m.keys.copy):
		return m, m.copyCmd(current.share.URL().String())
	}
	return m, nil
}

func (m Model) refreshAccesses() Model {
	selected, hadSelection := m.selectedAccessAddr()
	shares := slices.Clone(m.shares)
	for i := range shares {
		shares[i].accesses, shares[i].untracked = shares[i].share.Accesses()
	}
	m.shares = shares

	current, ok := m.accessShare()
	if !ok || !hadSelection {
		return m
	}
	if i := slices.IndexFunc(current.accesses, func(a proxy.Access) bool { return a.Addr == selected }); i >= 0 {
		m.access.cursor = i
	}
	return m
}

func (m Model) selectedAccessAddr() (netip.Addr, bool) {
	current, ok := m.accessShare()
	if !ok || m.access.cursor >= len(current.accesses) {
		return netip.Addr{}, false
	}
	return current.accesses[m.access.cursor].Addr, true
}
