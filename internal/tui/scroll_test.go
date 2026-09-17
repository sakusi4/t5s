package tui

import "testing"

func TestKeepVisible(t *testing.T) {
	tests := []struct {
		name                           string
		offset, cursor, visible, total int
		want                           int
	}{
		{"everything fits", 0, 2, 10, 5, 0},
		{"cursor inside the window leaves it alone", 3, 5, 5, 30, 3},
		{"cursor below the window pulls it down", 0, 7, 5, 30, 3},
		{"cursor above the window pulls it up", 10, 4, 5, 30, 4},
		{"shrunk list pulls the window back", 20, 9, 5, 10, 5},
		{"no room still shows the cursor", 0, 3, 0, 10, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keepVisible(tt.offset, tt.cursor, tt.visible, tt.total); got != tt.want {
				t.Errorf("keepVisible(%d, %d, %d, %d) = %d, want %d", tt.offset, tt.cursor, tt.visible, tt.total, got, tt.want)
			}
		})
	}
}
