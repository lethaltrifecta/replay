package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the CMDR service",
	Long:  `Start the CMDR HTTP API server and OTLP receiver`,
	RunE:  runServe,
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if err := cfg.RequireAgentgateway(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Initialize logger
	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("Starting CMDR service",
		"version", version,
		"api_port", cfg.APIPort,
		"otlp_grpc", cfg.OTLPGRPCEndpoint,
		"otlp_http", cfg.OTLPHTTPEndpoint,
	)

	// Initialize database
	store, err := storage.NewPostgresStorage(cfg.PostgresURL, cfg.PostgresMaxConn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer store.Close()

	// Run database migrations
	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}
	log.Info("Database connected and migrated successfully")

	// Initialize OTLP receiver
	receiverCfg := otelreceiver.Config{
		GRPCEndpoint: cfg.OTLPGRPCEndpoint,
		HTTPEndpoint: cfg.OTLPHTTPEndpoint,
	}
	receiver, err := otelreceiver.NewReceiver(receiverCfg, store, log)
	if err != nil {
		return fmt.Errorf("failed to create OTLP receiver: %w", err)
	}

	// Start OTLP receiver in background
	receiverCtx, cancelReceiver := context.WithCancel(context.Background())
	defer cancelReceiver()

	receiverDone := make(chan error, 1)
	go func() {
		if err := receiver.Start(receiverCtx, receiverCfg); err != nil {
			log.Error("OTLP receiver failed", "error", err)
			receiverDone <- err
		}
	}()

	// Give receiver a moment to start and check for immediate errors
	time.Sleep(500 * time.Millisecond)
	select {
	case err := <-receiverDone:
		return fmt.Errorf("OTLP receiver failed to start: %w", err)
	default:
		// Receiver started successfully
		log.Info("OTLP receiver is ready")
	}

	// TODO: Start remaining components
	// - Freeze-Tools MCP server
	// - HTTP API server
	// - Replay engine worker pool

	log.Info("CMDR service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutdown signal received, gracefully stopping...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// TODO: Graceful shutdown
	// - Stop accepting new requests
	// - Wait for in-flight requests to complete
	// - Close database connections
	// - Shutdown OTLP receiver
	// - Shutdown HTTP server

	_ = shutdownCtx // Use context for shutdown operations

	log.Info("Shutdown complete")
	return nil
}
