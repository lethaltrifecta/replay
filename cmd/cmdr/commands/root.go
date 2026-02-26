package commands

import (
	"github.com/spf13/cobra"
)

var (
	version string
)

// Execute is the entry point for the CLI
func Execute(v string) error {
	version = v
	return rootCmd.Execute()
}

var rootCmd = &cobra.Command{
	Use:   "cmdr",
	Short: "CMDR - Replay-Backed Behavior Analysis Lab",
	Long: `CMDR is a deterministic replay and evaluation system for comparing
LLM agent behavior across models, prompts, policies, and tool configurations.`,
	Version: version,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(experimentCmd)
	rootCmd.AddCommand(evalCmd)
	rootCmd.AddCommand(groundTruthCmd)
}
