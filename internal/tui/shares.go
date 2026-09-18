package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/proxy"
	"github.com/sakusi4/t5s/internal/share"
)

const (
	flashDuration       = 4 * time.Second
	copiedText          = "URL copied"
	stoppingText        = "stopping shares…"
	alreadySharedText   = "%s is already shared: x stops it, c copies the URL"
	notSharedText       = "%s is not shared: s shares it"
	stoppedText         = "%s share stopped"
	expiredText         = "%s share expired"
	installCloudflared  = "cloudflared is not installed: brew install cloudflared"
	shareFailedTemplate = "%s: %v"
)

type activeShare struct {
	service   discovery.Service
	share     *share.Share
	accesses  []proxy.Access
	untracked int
}

type row struct {
	service    discovery.Service
	share      *share.Share
	opening    bool
	responding bool
}

type shareStartedMsg struct {
	service discovery.Service
	share   *share.Share
}

type shareFailedMsg struct {
	service discovery.Service
	err     error
}

type shareEndedMsg struct {
	port uint16
	err  error
}

type copiedMsg struct {
	text string
	err  error
}

type flashExpiredMsg struct {
	id int
}

func (m Model) rows() []row {
	rows := make([]row, 0, len(m.services)+len(m.shares)+len(m.opening))
	for _, s := range m.services {
		rows = append(rows, row{service: s, responding: true})
	}
	pin := func(service discovery.Service) int {
		i := slices.IndexFunc(rows, func(r row) bool { return r.service.Addr.Port() == service.Addr.Port() })
		if i < 0 {
			rows = append(rows, row{service: service})
			return len(rows) - 1
		}
		return i
	}
	for _, a := range m.shares {
		rows[pin(a.service)].share = a.share
	}
	for _, service := range m.opening {
		rows[pin(service)].opening = true
	}
	slices.SortStableFunc(rows, func(a, b row) int {
		return cmp.Compare(a.service.Addr.Port(), b.service.Addr.Port())
	})
	return rows
}

func (m Model) openDialog(selected row) (Model, tea.Cmd) {
	if selected.share != nil || selected.opening {
		return m.withFlash(fmt.Sprintf(alreadySharedText, displayName(selected.service)))
	}
	dialog, cmd := newShareDialog(selected.service, m.lastAllow, m.allowWidth(), m.own)
	m.dialog = &dialog
	return m, cmd
}

func (m Model) handleDialogKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	dialog, cmd, outcome := m.dialog.update(msg, m.keys)
	switch outcome {
	case dialogOpen:
		m.dialog = &dialog
		return m, cmd
	case dialogCanceled:
		m.dialog = nil
		return m, nil
	case dialogSubmitted:
		m.dialog = nil
		m.lastAllow = dialog.allow.Value()
		m.opening = append(slices.Clone(m.opening), dialog.service)
		return m, m.startShareCmd(dialog.service, dialog.request)
	}
	return m, nil
}

func (m Model) startShareCmd(service discovery.Service, req share.Request) tea.Cmd {
	return func() tea.Msg {
		s, err := share.Start(context.Background(), m.cfg.Backend, req)
		if err != nil {
			return shareFailedMsg{service: service, err: err}
		}
		return shareStartedMsg{service: service, share: s}
	}
}

func (m Model) handleShareMsg(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case shareStartedMsg:
		return m.shareStarted(msg)
	case shareFailedMsg:
		m.opening = withoutPort(m.opening, msg.service.Addr.Port())
		return m.afterShareGone(shareFailure(msg))
	case shareEndedMsg:
		return m.shareEnded(msg)
	case copiedMsg:
		next, flashCmd := m.withFlash(copiedText)
		if msg.err != nil {
			return next, tea.Batch(flashCmd, tea.SetClipboard(msg.text))
		}
		return next, flashCmd
	case flashExpiredMsg:
		if msg.id == m.flashID {
			m.flash = ""
		}
	}
	return m, nil
}

func (m Model) shareStarted(msg shareStartedMsg) (Model, tea.Cmd) {
	m.opening = withoutPort(m.opening, msg.service.Addr.Port())
	m.shares = append(slices.Clone(m.shares), activeShare{service: msg.service, share: msg.share})
	wait := func() tea.Msg {
		return shareEndedMsg{port: msg.service.Addr.Port(), err: msg.share.Wait()}
	}
	if m.quitting {
		msg.share.Stop()
		return m, wait
	}
	return m, tea.Batch(wait, m.copyCmd(msg.share.URL().String()))
}

func (m Model) shareEnded(msg shareEndedMsg) (Model, tea.Cmd) {
	i := slices.IndexFunc(m.shares, func(a activeShare) bool { return a.service.Addr.Port() == msg.port })
	if i < 0 {
		return m, nil
	}
	name := displayName(m.shares[i].service)
	m.shares = slices.Delete(slices.Clone(m.shares), i, i+1)
	if m.screen == screenAccess && m.access.port == msg.port {
		m = m.closeAccess()
	}

	text := fmt.Sprintf(stoppedText, name)
	switch {
	case errors.Is(msg.err, share.ErrExpired):
		text = fmt.Sprintf(expiredText, name)
	case msg.err != nil:
		text = fmt.Sprintf(shareFailedTemplate, name, msg.err)
	}
	return m.afterShareGone(text)
}

func (m Model) afterShareGone(text string) (Model, tea.Cmd) {
	selected, hadSelection := m.selectedRow()
	m = m.selectPort(selected.service.Addr.Port(), hadSelection)
	if m.quitting && len(m.shares) == 0 && len(m.opening) == 0 {
		return m, tea.Quit
	}
	return m.withFlash(text)
}

func shareFailure(msg shareFailedMsg) string {
	if errors.Is(msg.err, exec.ErrNotFound) {
		return installCloudflared
	}
	return fmt.Sprintf(shareFailedTemplate, displayName(msg.service), msg.err)
}

func (m Model) stopShare(selected row) (Model, tea.Cmd) {
	if selected.share == nil {
		return m.withFlash(fmt.Sprintf(notSharedText, displayName(selected.service)))
	}
	selected.share.Stop()
	return m, nil
}

func (m Model) copyURL(selected row) (Model, tea.Cmd) {
	if selected.share == nil {
		return m.withFlash(fmt.Sprintf(notSharedText, displayName(selected.service)))
	}
	return m, m.copyCmd(selected.share.URL().String())
}

func (m Model) copyCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return copiedMsg{text: text, err: m.cfg.Copy(text)}
	}
}

func (m Model) quit() (Model, tea.Cmd) {
	if len(m.shares) == 0 && len(m.opening) == 0 {
		return m, tea.Quit
	}
	m.quitting = true
	for _, a := range m.shares {
		a.share.Stop()
	}
	return m.withFlash(stoppingText)
}

func (m Model) withFlash(text string) (Model, tea.Cmd) {
	m.flash = text
	m.flashID++
	id := m.flashID
	return m, tea.Tick(flashDuration, func(time.Time) tea.Msg {
		return flashExpiredMsg{id: id}
	})
}

func (m Model) applyClock(now time.Time) Model {
	m.now = now
	return m.refreshAccesses()
}

func withoutPort(services []discovery.Service, port uint16) []discovery.Service {
	return slices.DeleteFunc(slices.Clone(services), func(s discovery.Service) bool {
		return s.Addr.Port() == port
	})
}
