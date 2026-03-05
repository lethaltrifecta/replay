package commands

import "github.com/spf13/cobra"

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run demo scenarios (seeds data, uses mock LLM)",
	Long:  "Self-contained demo for hackathon presentation. Seeds realistic agent traces and replays with canned model responses. No external LLM or agentgateway needed.",
}

func init() {
	demoCmd.AddCommand(demoSeedCmd)
	demoCmd.AddCommand(demoGateCmd)
}
