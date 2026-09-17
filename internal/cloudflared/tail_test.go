package cloudflared

import (
	"fmt"
	"strings"
	"testing"
)

func TestTail(t *testing.T) {
	t.Run("last line skips trailing blank lines", func(t *testing.T) {
		var out tail
		fmt.Fprint(&out, "INF starting\nERR connection lost\n\n")
		if got := out.lastLine(); got != "ERR connection lost" {
			t.Errorf("lastLine() = %q, want %q", got, "ERR connection lost")
		}
	})

	t.Run("nothing written gives an empty last line", func(t *testing.T) {
		var out tail
		if got := out.lastLine(); got != "" {
			t.Errorf("lastLine() = %q, want empty", got)
		}
	})

	t.Run("memory stays bounded while the end is kept", func(t *testing.T) {
		var out tail
		for range 100 {
			fmt.Fprintln(&out, strings.Repeat("x", 100))
		}
		fmt.Fprintln(&out, "ERR the end")

		if len(out.buf) > tailSize {
			t.Errorf("tail holds %d bytes, want at most %d", len(out.buf), tailSize)
		}
		if got := out.lastLine(); got != "ERR the end" {
			t.Errorf("lastLine() = %q, want %q", got, "ERR the end")
		}
	})
}
