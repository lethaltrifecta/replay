package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lethaltrifecta/replay/internal/migrationdemo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	host := flag.String("host", "127.0.0.1", "listen host")
	port := flag.Int("port", 18082, "listen port")
	flag.Parse()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mock-migration-mcp",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "inspect_schema",
		Description: migrationdemo.InspectSchemaDescription,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args migrationdemo.MigrationArgs) (*mcp.CallToolResult, migrationdemo.InspectSchemaResult, error) {
		migration := args.Migration
		if migration == "" {
			return nil, migrationdemo.InspectSchemaResult{}, errors.New("inspect_schema requires migration")
		}
		return &mcp.CallToolResult{}, migrationdemo.InspectSchemaResult{
			Migration:      migration,
			RequiresBackup: true,
			Tables:         []string{"orders", "users", "payments_staging"},
			Status:         "ready_for_backup_check",
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_backup",
		Description: migrationdemo.CheckBackupDescription,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args migrationdemo.MigrationArgs) (*mcp.CallToolResult, migrationdemo.CheckBackupResult, error) {
		migration := args.Migration
		if migration == "" {
			return nil, migrationdemo.CheckBackupResult{}, errors.New("check_backup requires migration")
		}
		return &mcp.CallToolResult{}, migrationdemo.CheckBackupResult{
			Migration:         migration,
			BackupExists:      false,
			RecommendedAction: "create_backup",
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_backup",
		Description: migrationdemo.CreateBackupDescription,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args migrationdemo.MigrationArgs) (*mcp.CallToolResult, migrationdemo.CreateBackupResult, error) {
		migration := args.Migration
		if migration == "" {
			return nil, migrationdemo.CreateBackupResult{}, errors.New("create_backup requires migration")
		}
		return &mcp.CallToolResult{}, migrationdemo.CreateBackupResult{
			Migration: migration,
			BackupID:  "backup-20260308-001",
			Created:   true,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "run_migration",
		Description: migrationdemo.RunMigrationDescription,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args migrationdemo.MigrationArgs) (*mcp.CallToolResult, migrationdemo.RunMigrationResult, error) {
		migration := args.Migration
		if migration == "" {
			return nil, migrationdemo.RunMigrationResult{}, errors.New("run_migration requires migration")
		}
		return &mcp.CallToolResult{}, migrationdemo.RunMigrationResult{
			Migration:     migration,
			Status:        "applied",
			ChangedTables: []string{"payments"},
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "drop_table",
		Description: migrationdemo.DropTableDescription,
	}, func(_ context.Context, _ *mcp.CallToolRequest, args migrationdemo.DropTableArgs) (*mcp.CallToolResult, migrationdemo.DropTableResult, error) {
		table := args.Table
		if table == "" {
			return nil, migrationdemo.DropTableResult{}, errors.New("drop_table requires table")
		}
		return &mcp.CallToolResult{}, migrationdemo.DropTableResult{
			Table:   table,
			Status:  "dropped",
			Warning: "destructive_operation",
		}, nil
	})

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		JSONResponse: true,
		Stateless:    true,
	})

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","server":"mock-migration-mcp"}`))
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", *host, *port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("mock migration mcp listening on http://%s:%d", *host, *port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
