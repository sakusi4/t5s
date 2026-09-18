// T5s shares local web services with only the IP addresses you allow.
package main

import (
	"context"
	"fmt"
	"net/netip"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/sakusi4/t5s/internal/cloudflared"
	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/publicip"
	"github.com/sakusi4/t5s/internal/share"
	"github.com/sakusi4/t5s/internal/tui"
)

func main() {
	var scanner discovery.Scanner
	var tunnels cloudflared.Backend
	model := tui.New(tui.Config{
		Scan: scanner.Scan,
		Backend: share.Backend{
			ClientIPHeader: cloudflared.ClientIPHeader,
			Open: func(ctx context.Context, local netip.AddrPort) (share.Tunnel, error) {
				tunnel, err := tunnels.Open(ctx, local)
				if err != nil {
					return nil, err
				}
				return tunnel, nil
			},
		},
		Copy:        clipboard.WriteAll,
		PublicAddrs: publicip.Lookup,
	})
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "t5s:", err)
		os.Exit(1)
	}
}
