// T5s shares local web services with only the IP addresses you allow.
package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/sakusi4/t5s/internal/discovery"
	"github.com/sakusi4/t5s/internal/tui"
)

func main() {
	var scanner discovery.Scanner
	if _, err := tea.NewProgram(tui.New(scanner.Scan)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "t5s:", err)
		os.Exit(1)
	}
}
