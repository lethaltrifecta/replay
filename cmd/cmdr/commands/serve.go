package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/lethaltrifecta/replay/pkg/agwclient"
	"github.com/lethaltrifecta/replay/pkg/api"
	"github.com/lethaltrifecta/replay/pkg/config"
	"github.com/lethaltrifecta/replay/pkg/otelreceiver"
	"github.com/lethaltrifecta/replay/pkg/replay"
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

	// Initialize logger
	log, err := logger.New(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	if cfg.AgentgatewayURL == "" {
		log.Warn("CMDR_AGENTGATEWAY_URL is not set; OTLP capture is available, but replay/gate commands still require agentgateway")
	}

	log.Info("Starting CMDR service",
		"version", version,
		"api_port", cfg.APIPort,
		"otlp_grpc", cfg.OTLPGRPCEndpoint,
		"otlp_http", cfg.OTLPHTTPEndpoint,
	)

	// Initialize database
	store, err := storage.NewPostgresStorage(ctx, cfg.PostgresURL, cfg.PostgresMaxConn)
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

	// Create agentgateway client for replay (nil when URL is not configured)
	var completer replay.Completer
	if cfg.AgentgatewayURL != "" {
		completer = agwclient.NewClient(agwclient.ClientConfig{
			BaseURL:    cfg.AgentgatewayURL,
			Timeout:    cfg.AgentgatewayTimeout,
			MaxRetries: cfg.AgentgatewayRetries,
		})
	}

	// Start HTTP API server
	apiServer := api.NewServer(api.ServerConfig{
		Port:                 cfg.APIPort,
		MaxConcurrentReplays: cfg.MaxConcurrentReplays,
		MCPURL:               cfg.MCPURL,
		AgentLoopMaxTurns:    cfg.AgentLoopMaxTurns,
	}, store, completer, log)

	apiDone := make(chan error, 1)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Error("API server failed", "error", err)
			apiDone <- err
		}
	}()

	// Give API server a moment to start
	time.Sleep(100 * time.Millisecond)
	select {
	case err := <-apiDone:
		return fmt.Errorf("API server failed to start: %w", err)
	default:
		log.Info("API server is ready", "port", cfg.APIPort)
	}

	log.Info("CMDR service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutdown signal received, gracefully stopping...")

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Graceful shutdown: stop API server, then OTLP receiver
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Error("API server shutdown error", "error", err)
	}
	cancelReceiver()

	log.Info("Shutdown complete")
	return nil
}
