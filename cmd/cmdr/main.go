package main

import (
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
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
