package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/acl"
	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/share"
)

const (
	allowPlaceholder   = "203.0.113.42, 10.0.0.0/8"
	defaultExpiryIndex = 1
)

var expiryChoices = []time.Duration{15 * time.Minute, time.Hour, 4 * time.Hour, 24 * time.Hour}

type dialogField int

const (
	fieldAllow dialogField = iota
	fieldExpire
)

type dialogOutcome int

const (
	dialogOpen dialogOutcome = iota
	dialogCanceled
	dialogSubmitted
)

type shareDialog struct {
	service discovery.Service
	allow   textinput.Model
	expiry  int
	field   dialogField
	err     error
	request share.Request
}

func newShareDialog(service discovery.Service, lastAllow string, allowWidth int) (shareDialog, tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = allowPlaceholder
	input.SetWidth(allowWidth)
	input.SetVirtualCursor(false)
	input.SetValue(lastAllow)
	focus := input.Focus()
	return shareDialog{service: service, allow: input, expiry: defaultExpiryIndex}, focus
}

// resized returns the dialog with its allow input scrolled to fit allowWidth.
func (d shareDialog) resized(allowWidth int) shareDialog {
	d.allow.SetWidth(allowWidth)
	d.allow.SetCursor(d.allow.Position())
	return d
}

func (d shareDialog) update(msg tea.KeyPressMsg, keys keyMap) (shareDialog, tea.Cmd, dialogOutcome) {
	switch {
	case key.Matches(msg, keys.cancel):
		return d, nil, dialogCanceled
	case key.Matches(msg, keys.confirm):
		return d.submit()
	case key.Matches(msg, keys.next):
		return d.switchField()
	case d.field == fieldExpire && key.Matches(msg, keys.left):
		d.expiry = max(d.expiry-1, 0)
		return d, nil, dialogOpen
	case d.field == fieldExpire && key.Matches(msg, keys.right):
		d.expiry = min(d.expiry+1, len(expiryChoices)-1)
		return d, nil, dialogOpen
	case d.field == fieldAllow:
		var cmd tea.Cmd
		d.allow, cmd = d.allow.Update(msg)
		d.err = nil
		return d, cmd, dialogOpen
	}
	return d, nil, dialogOpen
}

func (d shareDialog) submit() (shareDialog, tea.Cmd, dialogOutcome) {
	allow, err := acl.Parse(splitEntries(d.allow.Value()))
	if err != nil {
		d.err = err
		return d, nil, dialogOpen
	}
	d.request = share.Request{Target: d.service.Addr, Allow: allow, Expires: expiryChoices[d.expiry]}
	return d, nil, dialogSubmitted
}

func (d shareDialog) switchField() (shareDialog, tea.Cmd, dialogOutcome) {
	if d.field == fieldAllow {
		d.field = fieldExpire
		d.allow.Blur()
		return d, nil, dialogOpen
	}
	d.field = fieldAllow
	return d, d.allow.Focus(), dialogOpen
}

func splitEntries(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
}

func formatExpiry(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
