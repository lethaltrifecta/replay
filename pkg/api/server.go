package api

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// ServerConfig holds configuration for the HTTP API server.
type ServerConfig struct {
	Port                 int
	MaxConcurrentReplays int
}

// Server is the HTTP API server for CMDR gate operations.
type Server struct {
	httpServer *http.Server
	store      storage.Storage
	completer  replay.Completer
	log        *logger.Logger
	sem        chan struct{} // concurrency limiter
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewServer creates a new API server.
func NewServer(cfg ServerConfig, store storage.Storage, completer replay.Completer, log *logger.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		store:     store,
		completer: completer,
		log:       log,
		sem:       make(chan struct{}, cfg.MaxConcurrentReplays),
		ctx:       ctx,
		cancel:    cancel,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/gate/check", s.handleGateCheck)
	mux.HandleFunc("GET /api/v1/gate/status/{id}", s.handleGateStatus)
	mux.HandleFunc("GET /api/v1/gate/report/{id}", s.handleGateReport)
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	return s
}

// Start begins serving HTTP requests. Blocks until the server is shut down.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.httpServer.Addr, err)
	}
	s.log.Infow("API server listening", "addr", s.httpServer.Addr)
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the server and cancels any in-flight pipelines.
func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	return s.httpServer.Shutdown(ctx)
}
