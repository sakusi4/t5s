package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	up   key.Binding
	down key.Binding
	quit key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		up:   key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		down: key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		quit: key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) bindings() []key.Binding {
	return []key.Binding{k.up, k.down, k.quit}
}
