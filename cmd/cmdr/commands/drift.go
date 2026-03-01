package commands

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/storage"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Drift detection commands",
	Long:  `Manage baselines and view drift detection results`,
}

var driftBaselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "Manage baselines",
	Long:  `Set, list, and remove baseline traces for drift comparison`,
}

var driftBaselineSetCmd = &cobra.Command{
	Use:   "set <trace-id>",
	Short: "Mark a trace as a baseline",
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftBaselineSet,
}

var driftBaselineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all baselines",
	RunE:  runDriftBaselineList,
}

var driftBaselineRemoveCmd = &cobra.Command{
	Use:   "remove <trace-id>",
	Short: "Remove a baseline",
	Args:  cobra.ExactArgs(1),
	RunE:  runDriftBaselineRemove,
}

func init() {
	driftCmd.AddCommand(driftBaselineCmd)
	driftBaselineCmd.AddCommand(driftBaselineSetCmd)
	driftBaselineCmd.AddCommand(driftBaselineListCmd)
	driftBaselineCmd.AddCommand(driftBaselineRemoveCmd)

	driftBaselineSetCmd.Flags().String("name", "", "Human-readable name for the baseline")
	driftBaselineSetCmd.Flags().String("description", "", "Description of the baseline")
}

func connectDB() (*storage.PostgresStorage, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	store, err := storage.NewPostgresStorage(cfg.PostgresURL, cfg.PostgresMaxConn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

func runDriftBaselineSet(cmd *cobra.Command, args []string) error {
	traceID := args[0]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	name, _ := cmd.Flags().GetString("name")
	description, _ := cmd.Flags().GetString("description")

	baseline := &storage.Baseline{
		TraceID: traceID,
	}
	if name != "" {
		baseline.Name = &name
	}
	if description != "" {
		baseline.Description = &description
	}

	ctx := context.Background()
	if err := store.MarkTraceAsBaseline(ctx, baseline); err != nil {
		return fmt.Errorf("failed to set baseline: %w", err)
	}

	cmd.Printf("Baseline set for trace %s\n", traceID)
	if name != "" {
		cmd.Printf("  Name: %s\n", name)
	}
	if description != "" {
		cmd.Printf("  Description: %s\n", description)
	}

	return nil
}

func runDriftBaselineList(cmd *cobra.Command, args []string) error {
	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	baselines, err := store.ListBaselines(ctx)
	if err != nil {
		return fmt.Errorf("failed to list baselines: %w", err)
	}

	if len(baselines) == 0 {
		cmd.Println("No baselines found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TRACE ID\tNAME\tDESCRIPTION\tCREATED AT")
	for _, b := range baselines {
		name := ""
		if b.Name != nil {
			name = *b.Name
		}
		desc := ""
		if b.Description != nil {
			desc = *b.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			b.TraceID, name, desc, b.CreatedAt.Format("2006-01-02 15:04:05"),
		)
	}
	w.Flush()

	return nil
}

func runDriftBaselineRemove(cmd *cobra.Command, args []string) error {
	traceID := args[0]

	store, err := connectDB()
	if err != nil {
		return err
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.UnmarkBaseline(ctx, traceID); err != nil {
		return fmt.Errorf("failed to remove baseline: %w", err)
	}

	cmd.Printf("Baseline removed for trace %s\n", traceID)
	return nil
}
