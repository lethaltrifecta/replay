package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/lethaltrifecta/replay/pkg/replay"
	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

// ServerConfig holds configuration for the HTTP API server.
type ServerConfig struct {
	Port                 int
	MaxConcurrentReplays int
	MCPURL               string
	AgentLoopMaxTurns    int
}

// Server is the HTTP API server for CMDR gate operations.
type Server struct {
	httpServer        *http.Server
	store             storage.Storage
	completer         replay.Completer
	log               *logger.Logger
	sem               chan struct{} // concurrency limiter
	ctx               context.Context
	cancel            context.CancelFunc
	mcpURL            string
	agentLoopMaxTurns int
}

func isNilCompleter(completer replay.Completer) bool {
	if completer == nil {
		return true
	}

	value := reflect.ValueOf(completer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// NewServer creates a new API server.
func NewServer(cfg ServerConfig, store storage.Storage, completer replay.Completer, log *logger.Logger) *Server {
	// Normalize typed nil to untyped nil so s.completer == nil works reliably.
	// In Go, a nil *ConcreteType stored in an interface is non-nil.
	if isNilCompleter(completer) {
		completer = nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{
		store:             store,
		completer:         completer,
		log:               log,
		sem:               make(chan struct{}, cfg.MaxConcurrentReplays),
		ctx:               ctx,
		cancel:            cancel,
		mcpURL:            cfg.MCPURL,
		agentLoopMaxTurns: cfg.AgentLoopMaxTurns,
	}

	h := HandlerWithOptions(s, StdHTTPServerOptions{
		BaseURL: "/api/v1",
	})

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
	err := s.httpServer.Shutdown(ctx)
	s.cancel()
	return err
}
