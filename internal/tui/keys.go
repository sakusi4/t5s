package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	up      key.Binding
	down    key.Binding
	share   key.Binding
	stop    key.Binding
	copy    key.Binding
	quit    key.Binding
	confirm key.Binding
	cancel  key.Binding
	next    key.Binding
	left    key.Binding
	right   key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		share:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "share")),
		stop:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		copy:    key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
		quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "share")),
		cancel:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		next:    key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch")),
		left:    key.NewBinding(key.WithKeys("left")),
		right:   key.NewBinding(key.WithKeys("right")),
	}
}

func (k keyMap) bindings() []key.Binding {
	return []key.Binding{k.up, k.down, k.quit}
}
