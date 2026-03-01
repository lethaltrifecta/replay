package commands

import (
	"github.com/spf13/cobra"
)

var groundTruthCmd = &cobra.Command{
	Use:   "ground-truth",
	Short: "Manage ground truth data",
	Long:  `Add, list, update, and delete ground truth reference answers`,
}

var groundTruthAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new ground truth entry",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement ground truth add
		cmd.Println("Ground truth add not yet implemented")
		return nil
	},
}

var groundTruthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List ground truth entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement ground truth list
		cmd.Println("Ground truth list not yet implemented")
		return nil
	},
}

var groundTruthUpdateCmd = &cobra.Command{
	Use:   "update [task-id]",
	Short: "Update a ground truth entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement ground truth update
		cmd.Printf("Ground truth update for %s not yet implemented\n", args[0])
		return nil
	},
}

var groundTruthDeleteCmd = &cobra.Command{
	Use:   "delete [task-id]",
	Short: "Delete a ground truth entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement ground truth delete
		cmd.Printf("Ground truth delete for %s not yet implemented\n", args[0])
		return nil
	},
}

func init() {
	// Add subcommands to ground-truth command
	groundTruthCmd.AddCommand(groundTruthAddCmd)
	groundTruthCmd.AddCommand(groundTruthListCmd)
	groundTruthCmd.AddCommand(groundTruthUpdateCmd)
	groundTruthCmd.AddCommand(groundTruthDeleteCmd)

	// Flags for add
	groundTruthAddCmd.Flags().String("task-id", "", "Task ID")
	groundTruthAddCmd.Flags().String("task-type", "", "Task type")
	groundTruthAddCmd.Flags().String("input", "", "Path to input JSON file")
	groundTruthAddCmd.Flags().String("expected-output", "", "Path to expected output file")
	_ = groundTruthAddCmd.MarkFlagRequired("task-id")
	_ = groundTruthAddCmd.MarkFlagRequired("expected-output")

	// Flags for list
	groundTruthListCmd.Flags().String("task-type", "", "Filter by task type")

	// Flags for update
	groundTruthUpdateCmd.Flags().String("expected-output", "", "Path to new expected output file")
	_ = groundTruthUpdateCmd.MarkFlagRequired("expected-output")
}
