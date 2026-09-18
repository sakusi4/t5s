package tui

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func pressKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func typeInto(d shareDialog, text string) shareDialog {
	keys := newKeyMap()
	for _, r := range text {
		d, _, _ = d.update(press(r), keys)
	}
	return d
}

func TestShareDialog(t *testing.T) {
	const allowWidth = 40
	keys := newKeyMap()
	target := service(5173, "front")

	t.Run("starts on the allow field with the last list and one hour", func(t *testing.T) {
		d, _ := newShareDialog(target, "10.0.0.0/8", allowWidth, nil)
		if d.field != fieldAllow || d.allow.Value() != "10.0.0.0/8" || expiryChoices[d.expiry] != time.Hour {
			t.Errorf("dialog = field %d, allow %q, expiry %s, want allow field, 10.0.0.0/8, 1h", d.field, d.allow.Value(), expiryChoices[d.expiry])
		}
	})

	t.Run("enter with a valid list submits the request", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d = typeInto(d, "203.0.113.42, 10.0.0.0/8")
		d, _, _ = d.update(pressKey(tea.KeyTab), keys)
		d, _, _ = d.update(pressKey(tea.KeyRight), keys)
		d, _, outcome := d.update(pressKey(tea.KeyEnter), keys)

		if outcome != dialogSubmitted {
			t.Fatalf("outcome = %d, err = %v, want submitted", outcome, d.err)
		}
		req := d.request
		if req.Target != target.Addr || req.Expires != 4*time.Hour {
			t.Errorf("request = %+v, want target %s for 4h", req, target.Addr)
		}
		if !req.Allow.Allows(netip.MustParseAddr("203.0.113.42")) || !req.Allow.Allows(netip.MustParseAddr("10.1.2.3")) || req.Allow.Allows(netip.MustParseAddr("198.51.100.7")) {
			t.Errorf("request allowlist does not match the typed entries")
		}
	})

	t.Run("enter with a bad list keeps the dialog open and shows why", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d = typeInto(d, "0.0.0.0/0")
		d, _, outcome := d.update(pressKey(tea.KeyEnter), keys)
		if outcome != dialogOpen || d.err == nil {
			t.Errorf("outcome = %d, err = %v, want open with an error", outcome, d.err)
		}

		d = typeInto(d, "1")
		if d.err != nil {
			t.Errorf("err = %v after editing, want it cleared", d.err)
		}
	})

	t.Run("submit allows the own addresses next to the typed ones", func(t *testing.T) {
		d, _ := newShareDialog(target, "203.0.113.42", allowWidth, fakeOwn)
		d, _, outcome := d.update(pressKey(tea.KeyEnter), keys)
		if outcome != dialogSubmitted {
			t.Fatalf("outcome = %d, err = %v, want submitted", outcome, d.err)
		}
		for _, addr := range []string{"203.0.113.42", "192.0.2.10", "2001:db8::10"} {
			if !d.request.Allow.Allows(netip.MustParseAddr(addr)) {
				t.Errorf("request does not allow %s", addr)
			}
		}
		if d.request.Allow.Allows(netip.MustParseAddr("198.51.100.7")) {
			t.Errorf("request allows an address nobody listed")
		}
	})

	t.Run("nothing typed shares with the own addresses only", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, fakeOwn)
		d, _, outcome := d.update(pressKey(tea.KeyEnter), keys)
		if outcome != dialogSubmitted || !d.request.Allow.Allows(fakeOwn[0]) {
			t.Errorf("outcome = %d, err = %v, want submitted with the own address allowed", outcome, d.err)
		}
	})

	t.Run("nothing typed is refused while the own address is unknown", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d, _, outcome := d.update(pressKey(tea.KeyEnter), keys)
		if outcome != dialogOpen || d.err == nil {
			t.Errorf("outcome = %d, err = %v, want open with an error", outcome, d.err)
		}
	})

	t.Run("expiry stays within the choices", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d, _, _ = d.update(pressKey(tea.KeyTab), keys)
		for range len(expiryChoices) + 2 {
			d, _, _ = d.update(pressKey(tea.KeyLeft), keys)
		}
		if d.expiry != 0 {
			t.Errorf("expiry after many lefts = %d, want 0", d.expiry)
		}
		for range len(expiryChoices) + 2 {
			d, _, _ = d.update(pressKey(tea.KeyRight), keys)
		}
		if d.expiry != len(expiryChoices)-1 {
			t.Errorf("expiry after many rights = %d, want %d", d.expiry, len(expiryChoices)-1)
		}
	})

	t.Run("arrow keys edit the text while the allow field is active", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d, _, _ = d.update(pressKey(tea.KeyRight), keys)
		if d.expiry != defaultExpiryIndex {
			t.Errorf("expiry = %d, want it unchanged at %d", d.expiry, defaultExpiryIndex)
		}
	})

	t.Run("tab switches between the fields", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		d, _, _ = d.update(pressKey(tea.KeyTab), keys)
		if d.field != fieldExpire {
			t.Errorf("field after tab = %d, want expire", d.field)
		}
		d, _, _ = d.update(pressKey(tea.KeyTab), keys)
		if d.field != fieldAllow {
			t.Errorf("field after second tab = %d, want allow", d.field)
		}
	})

	t.Run("esc cancels", func(t *testing.T) {
		d, _ := newShareDialog(target, "", allowWidth, nil)
		if _, _, outcome := d.update(pressKey(tea.KeyEscape), keys); outcome != dialogCanceled {
			t.Errorf("outcome = %d, want canceled", outcome)
		}
	})
}

func TestSplitEntries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"commas and spaces", "203.0.113.42, 10.0.0.0/8  2001:db8::/32", []string{"203.0.113.42", "10.0.0.0/8", "2001:db8::/32"}},
		{"trailing comma", "203.0.113.42,", []string{"203.0.113.42"}},
		{"empty", "  ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitEntries(tt.value); !slices.Equal(got, tt.want) {
				t.Errorf("splitEntries(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestFormatExpiry(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{15 * time.Minute, "15m"},
		{time.Hour, "1h"},
		{24 * time.Hour, "24h"},
	}
	for _, tt := range tests {
		if got := formatExpiry(tt.in); got != tt.want {
			t.Errorf("formatExpiry(%s) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
