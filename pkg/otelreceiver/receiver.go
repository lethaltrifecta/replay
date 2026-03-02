package otelreceiver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"google.golang.org/grpc"

	"github.com/lethaltrifecta/replay/pkg/storage"
	"github.com/lethaltrifecta/replay/pkg/utils/logger"
)

var (
	errUnsupportedContentType      = errors.New("unsupported content type")
	errUnsupportedResponseEncoding = errors.New("unsupported response encoding")
)

type httpPayloadEncoding int

const (
	httpPayloadEncodingUnknown httpPayloadEncoding = iota
	httpPayloadEncodingJSON
	httpPayloadEncodingProtobuf
)

// Receiver handles OTLP trace ingestion
type Receiver struct {
	ptraceotlp.UnimplementedGRPCServer
	storage    storage.Storage
	logger     *logger.Logger
	grpcServer *grpc.Server
	httpServer *http.Server
	parser     *Parser
}

// Config holds receiver configuration
type Config struct {
	GRPCEndpoint string
	HTTPEndpoint string
}

// NewReceiver creates a new OTLP receiver
func NewReceiver(cfg Config, storage storage.Storage, log *logger.Logger) (*Receiver, error) {
	if storage == nil {
		return nil, fmt.Errorf("storage cannot be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	parser := NewParser(log)

	return &Receiver{
		storage: storage,
		logger:  log,
		parser:  parser,
	}, nil
}

// Start starts both gRPC and HTTP receivers
func (r *Receiver) Start(ctx context.Context, cfg Config) error {
	errChan := make(chan error, 2)

	// Start gRPC server
	go func() {
		if err := r.startGRPC(cfg.GRPCEndpoint); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Start HTTP server
	go func() {
		if err := r.startHTTP(cfg.HTTPEndpoint); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	r.logger.Info("OTLP receiver started",
		"grpc_endpoint", cfg.GRPCEndpoint,
		"http_endpoint", cfg.HTTPEndpoint,
	)

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		return r.Stop()
	case err := <-errChan:
		return err
	}
}

// startGRPC starts the gRPC OTLP receiver
func (r *Receiver) startGRPC(endpoint string) error {
	r.logger.Info("Starting OTLP gRPC receiver...", "endpoint", endpoint)

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		r.logger.Error("Failed to create gRPC listener", "endpoint", endpoint, "error", err)
		return fmt.Errorf("failed to listen on %s: %w", endpoint, err)
	}

	r.grpcServer = grpc.NewServer()
	ptraceotlp.RegisterGRPCServer(r.grpcServer, r)

	r.logger.Info("OTLP gRPC receiver listening", "endpoint", endpoint)
	return r.grpcServer.Serve(listener)
}

// startHTTP starts the HTTP OTLP receiver
func (r *Receiver) startHTTP(endpoint string) error {
	r.logger.Info("Starting OTLP HTTP receiver...", "endpoint", endpoint)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleHTTPTraces)
	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	r.httpServer = &http.Server{
		Addr:    endpoint,
		Handler: mux,
	}

	r.logger.Info("OTLP HTTP receiver listening", "endpoint", endpoint)
	return r.httpServer.ListenAndServe()
}

// Export implements the ptraceotlp.GRPCServer interface for gRPC traces
func (r *Receiver) Export(ctx context.Context, req ptraceotlp.ExportRequest) (ptraceotlp.ExportResponse, error) {
	traces := req.Traces()

	if err := r.processTraces(ctx, traces); err != nil {
		r.logger.Error("Failed to process gRPC traces", "error", err)
		return ptraceotlp.NewExportResponse(), err
	}

	return ptraceotlp.NewExportResponse(), nil
}

// handleHTTPTraces handles HTTP OTLP trace requests
func (r *Receiver) handleHTTPTraces(w http.ResponseWriter, req *http.Request) {
	r.logger.Debug("Received HTTP OTLP request", "method", req.Method, "path", req.URL.Path)

	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := req.Context()

	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	r.logger.Debug("Received trace data", "size_bytes", len(body))

	traces, responseEncoding, err := unmarshalHTTPTraces(req.Header.Get("Content-Type"), body)
	if err != nil {
		if errors.Is(err, errUnsupportedContentType) {
			r.logger.Error("Unsupported OTLP HTTP content type", "content_type", req.Header.Get("Content-Type"))
			http.Error(w, "Unsupported Content-Type", http.StatusUnsupportedMediaType)
			return
		}

		r.logger.Error("Failed to unmarshal HTTP traces", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	r.logger.Debug("Unmarshaled traces", "resource_spans", traces.ResourceSpans().Len())

	if err := r.processTraces(ctx, traces); err != nil {
		r.logger.Error("Failed to process HTTP traces", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	r.logger.Debug("HTTP trace processed successfully")

	if err := writeHTTPExportResponse(w, responseEncoding); err != nil {
		r.logger.Error("Failed to marshal OTLP HTTP response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// unmarshalHTTPTraces unmarshals OTLP traces from HTTP request payloads.
// Supports OTLP JSON and OTLP protobuf based on Content-Type.
func unmarshalHTTPTraces(contentType string, body []byte) (ptrace.Traces, httpPayloadEncoding, error) {
	mediaType := baseContentType(contentType)

	switch mediaType {
	case "application/json":
		unmarshaler := &ptrace.JSONUnmarshaler{}
		traces, err := unmarshaler.UnmarshalTraces(body)
		return traces, httpPayloadEncodingJSON, err
	case "application/x-protobuf", "application/protobuf", "application/octet-stream":
		unmarshaler := &ptrace.ProtoUnmarshaler{}
		traces, err := unmarshaler.UnmarshalTraces(body)
		return traces, httpPayloadEncodingProtobuf, err
	case "":
		// Fallback for clients that omit Content-Type.
		protoUnmarshaler := &ptrace.ProtoUnmarshaler{}
		if traces, err := protoUnmarshaler.UnmarshalTraces(body); err == nil {
			return traces, httpPayloadEncodingProtobuf, nil
		}

		jsonUnmarshaler := &ptrace.JSONUnmarshaler{}
		traces, err := jsonUnmarshaler.UnmarshalTraces(body)
		return traces, httpPayloadEncodingJSON, err
	default:
		return ptrace.Traces{}, httpPayloadEncodingUnknown, fmt.Errorf("%w: %s", errUnsupportedContentType, mediaType)
	}
}

func writeHTTPExportResponse(w http.ResponseWriter, encoding httpPayloadEncoding) error {
	resp := ptraceotlp.NewExportResponse()

	var (
		contentType string
		body        []byte
		err         error
	)

	switch encoding {
	case httpPayloadEncodingJSON:
		contentType = "application/json"
		body, err = resp.MarshalJSON()
	case httpPayloadEncodingProtobuf:
		contentType = "application/x-protobuf"
		body, err = resp.MarshalProto()
	default:
		return fmt.Errorf("%w: %d", errUnsupportedResponseEncoding, encoding)
	}
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	return err
}

func baseContentType(contentType string) string {
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	if contentType == "" {
		return ""
	}

	if idx := strings.Index(contentType, ";"); idx >= 0 {
		return strings.TrimSpace(contentType[:idx])
	}

	return contentType
}

// processTraces processes incoming traces and stores them
func (r *Receiver) processTraces(ctx context.Context, traces ptrace.Traces) error {

	var (
		otelTraces   []*storage.OTELTrace
		replayTraces []*storage.ReplayTrace
		toolCaptures []*storage.ToolCapture
	)

	resourceSpans := traces.ResourceSpans()
	r.logger.Debug("Processing traces", "resource_spans_count", resourceSpans.Len())

	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		scopeSpans := rs.ScopeSpans()
		r.logger.Debug("Processing resource span", "index", i, "scope_spans_count", scopeSpans.Len())

		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			r.logger.Debug("Processing scope span", "index", j, "spans_count", spans.Len())

			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				traceID := span.TraceID().String()
				r.logger.Debug("Processing span", "index", k, "trace_id", traceID, "name", span.Name())

				// Store raw OTEL trace
				otelTrace := r.parser.ParseOTELSpan(span, rs.Resource())
				otelTraces = append(otelTraces, otelTrace)
				r.logger.Debug("Stored raw OTEL trace", "trace_id", traceID)

				// Check if this is an LLM span (has gen_ai.* attributes)
				isLLM := r.parser.IsLLMSpan(span)
				r.logger.Debug("Checked if LLM span", "trace_id", traceID, "is_llm", isLLM)

				if isLLM {
					replayTrace := r.parser.ParseLLMSpan(span, rs.Resource())
					if replayTrace == nil {
						r.logger.Warn("ParseLLMSpan returned nil", "trace_id", traceID)
						continue
					}

					replayTraces = append(replayTraces, replayTrace)
					r.logger.Debug("Stored LLM trace",
						"trace_id", replayTrace.TraceID,
						"model", replayTrace.Model,
						"tokens", replayTrace.TotalTokens,
					)

					// Parse and store tool calls
					parsedToolCaptures := r.parser.ParseToolCalls(span, replayTrace.TraceID, replayTrace.SpanID)
					r.logger.Debug("Parsed tool calls", "trace_id", traceID, "count", len(parsedToolCaptures))

					for idx, capture := range parsedToolCaptures {
						toolCaptures = append(toolCaptures, capture)
						r.logger.Debug("Stored tool capture",
							"trace_id", capture.TraceID,
							"tool_name", capture.ToolName,
							"step_index", capture.StepIndex,
							"index", idx,
						)
					}
				}
			}
		}
	}

	err := r.insertTraces(ctx, otelTraces, replayTraces, toolCaptures)
	if err != nil {
		return err
	}

	r.logger.Debug("Finished processing traces")
	return nil
}

func (r *Receiver) insertTraces(ctx context.Context, otels []*storage.OTELTrace, replays []*storage.ReplayTrace, tools []*storage.ToolCapture) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	counts, err := r.storage.CreateIngestionBatch(ctx, otels, replays, tools)
	if err != nil {
		return fmt.Errorf("ingestion batch: %w", err)
	}

	r.logger.Debug("Stored ingestion batch",
		"otel_traces", counts.OTELTraces,
		"replay_traces", counts.ReplayTraces,
		"tool_captures", counts.ToolCaptures,
	)
	return nil
}

// Stop gracefully stops the receiver
func (r *Receiver) Stop() error {
	r.logger.Info("Stopping OTLP receiver...")

	if r.grpcServer != nil {
		r.grpcServer.GracefulStop()
	}

	if r.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.httpServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown HTTP server: %w", err)
		}
	}

	r.logger.Info("OTLP receiver stopped")
	return nil
}
