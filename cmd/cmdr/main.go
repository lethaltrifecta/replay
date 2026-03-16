package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/lethaltrifecta/replay/cmd/cmdr/commands"
)

var (
	// Version is set by build flags
	Version = "dev"
)

func main() {
	if err := commands.Execute(Version); err != nil {
		if !errors.Is(err, commands.ErrGateFailed) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}
