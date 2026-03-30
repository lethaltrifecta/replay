# OTLP Receiver Implementation - Complete ✅

## Summary

The OTLP receiver has been implemented to ingest OpenTelemetry traces from agentgateway and parse them into CMDR's replay schema.

## Files Created

1. **`pkg/otelreceiver/receiver.go`** - Main OTLP receiver with gRPC and HTTP endpoints
2. **`pkg/otelreceiver/parser.go`** - Parser for extracting gen_ai.* attributes
3. **`pkg/otelreceiver/receiver_test.go`** - Test suite with mock storage
4. **Updated `cmd/cmdr/commands/serve.go`** - Integrated OTLP receiver into service

## Architecture

### Data Flow

```
Agentgateway → OTEL Traces → OTLP Receiver → Parser → Storage
                (gen_ai.*)       (gRPC/HTTP)    ↓
                                                 ├─→ otel_traces (raw)
                                                 ├─→ replay_traces (parsed)
                                                 └─→ tool_captures (Freeze-Tools)
```

### Components

#### 1. Receiver (`receiver.go`)

**Dual Protocol Support**:
- **gRPC OTLP** on port 4317 (default)
- **HTTP OTLP** on port 4318 (default)

**Features**:
- Implements OpenTelemetry Collector's `ptraceotlp.GRPCServer` interface
- HTTP endpoint at `/v1/traces` (POST)
- Graceful shutdown support
- Concurrent processing of resource spans
- Automatic storage of raw OTEL traces

**Key Methods**:
- `NewReceiver()` - Creates new receiver with storage and logger
- `Start()` - Starts both gRPC and HTTP servers
- `Export()` - Handles gRPC trace exports
- `handleHTTPTraces()` - Handles HTTP trace POST requests
- `processTraces()` - Processes and stores traces
- `Stop()` - Graceful shutdown

#### 2. Parser (`parser.go`)

**Parses OTEL Semantic Conventions for LLM**:
- `gen_ai.request.model` → Model name
- `gen_ai.system` → Provider (anthropic, openai, etc.)
- `gen_ai.prompt.*.role` → Message roles (user, assistant, system)
- `gen_ai.prompt.*.content` → Message content
- `gen_ai.completion.*.content` → LLM response
- `gen_ai.usage.input_tokens` → Prompt tokens
- `gen_ai.usage.output_tokens` → Completion tokens
- `gen_ai.request.temperature` → Temperature parameter
- `execute_tool` spans → Tool captures with args/results

**Key Methods**:
- `ParseOTELSpan()` - Converts OTEL span to storage model
- `IsLLMSpan()` - Checks if span contains gen_ai.* attributes
- `ParseLLMSpan()` - Extracts LLM-specific data
- `ParseToolCalls()` - Extracts tool captures from `execute_tool` spans
- `extractPrompt()` - Builds message array from attributes
- `extractCompletion()` - Gets completion text
- `extractParameters()` - Gets LLM parameters
- `calculateArgsHash()` - SHA256 hash for Freeze-Tools lookup
- `determineRiskClass()` - Classifies tools as read/write/destructive

## OTEL Semantic Conventions Support

### LLM Attributes

**Request**:
```
gen_ai.system = "anthropic"
gen_ai.request.model = "claude-3-5-sonnet-20241022"
gen_ai.request.temperature = 0.7
gen_ai.request.top_p = 0.9
gen_ai.request.max_tokens = 1024
```

**Prompt** (indexed messages):
```
gen_ai.prompt.0.role = "user"
gen_ai.prompt.0.content = "Hello, how are you?"
gen_ai.prompt.1.role = "assistant"
gen_ai.prompt.1.content = "I'm doing great!"
gen_ai.prompt.2.role = "user"
gen_ai.prompt.2.content = "Can you help me?"
```

**Completion**:
```
gen_ai.completion.0.content = "Of course! How can I help?"
```

**Usage**:
```
gen_ai.usage.input_tokens = 42
gen_ai.usage.output_tokens = 18
```

### Tool Execution Spans

Tool calls are captured from semconv-aligned `execute_tool` spans:

```
Span: execute_tool search_docs
  gen_ai.operation.name = "execute_tool"
  gen_ai.tool.name = "search_docs"
  gen_ai.tool.call.arguments = {"query": "authentication", "limit": 10}
  gen_ai.tool.call.result = {"results": [...]}
  error.message = null
```

## Risk Classification

Tools are automatically classified based on their names:

**Destructive** (`risk_class = "destructive"`):
- delete*, remove*, drop*, terminate*

**Write** (`risk_class = "write"`):
- write*, create*, update*, insert*, modify*, edit*

**Read** (`risk_class = "read"`):
- search*, read*, get*, list*, find*, query* (default)

## Storage Integration

### What Gets Stored

**1. Raw OTEL Trace** (`otel_traces` table):
```go
storage.CreateOTELTrace(ctx, &OTELTrace{
    TraceID:      "abc123...",
    SpanID:       "span-456...",
    ServiceName:  "agentgateway",
    Attributes:   {...},  // All OTEL attributes preserved
    Events:       {...},
    Status:       {...},
})
```

**2. Parsed Replay Trace** (`replay_traces` table):
```go
storage.CreateReplayTrace(ctx, &ReplayTrace{
    TraceID:          "abc123...",
    RunID:            "abc123...",
    Provider:         "anthropic",
    Model:            "claude-3-5-sonnet-20241022",
    Prompt:           {"messages": [...]},
    Completion:       "I'm doing great!",
    PromptTokens:     10,
    CompletionTokens: 5,
    TotalTokens:      15,
    LatencyMS:        1000,
})
```

**3. Tool Captures** (`tool_captures` table):
```go
storage.CreateToolCapture(ctx, &ToolCapture{
    TraceID:   "abc123...",
    StepIndex: 0,
    ToolName:  "search_docs",
    Args:      {"query": "authentication"},
    ArgsHash:  "sha256...",  // For Freeze-Tools lookup
    Result:    {"results": [...]},
    RiskClass: "read",
})
```

## Integration with Serve Command

The OTLP receiver is now integrated into `cmdr serve`:

```go
// Initialize database
store, err := storage.NewPostgresStorage(cfg.PostgresURL, cfg.PostgresMaxConn)
// Run migrations
store.Migrate(ctx)

// Initialize OTLP receiver
receiver, err := otelreceiver.NewReceiver(receiverCfg, store, log)

// Start receiver in background
go receiver.Start(receiverCtx, receiverCfg)
```

## Testing

### Unit Tests

```bash
# Run receiver tests
go test ./pkg/otelreceiver -v
```

**Tests Included**:
- `TestParser_IsLLMSpan` - Detects LLM spans
- `TestParser_ParseLLMSpan` - Parses gen_ai.* attributes
- `TestReceiver_New` - Creates receiver successfully

### Integration Test (Manual)

```bash
# 1. Start CMDR
make dev-up
make run

# 2. Send a test OTLP trace (using grpcurl or HTTP)
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "test-service"}
        }]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "01020304050607080910111213141516",
          "spanId": "0102030405060708",
          "name": "llm.completion",
          "startTimeUnixNano": "1234567890000000000",
          "endTimeUnixNano": "1234567891000000000",
          "attributes": [{
            "key": "gen_ai.request.model",
            "value": {"stringValue": "claude-3-5-sonnet-20241022"}
          }, {
            "key": "gen_ai.system",
            "value": {"stringValue": "anthropic"}
          }, {
            "key": "gen_ai.prompt.0.role",
            "value": {"stringValue": "user"}
          }, {
            "key": "gen_ai.prompt.0.content",
            "value": {"stringValue": "Hello!"}
          }, {
            "key": "gen_ai.completion.0.content",
            "value": {"stringValue": "Hi there!"}
          }, {
            "key": "gen_ai.usage.input_tokens",
            "value": {"intValue": "10"}
          }, {
            "key": "gen_ai.usage.output_tokens",
            "value": {"intValue": "5"}
          }]
        }]
      }]
    }]
  }'

# 3. Verify in database
psql $CMDR_POSTGRES_URL -c "SELECT trace_id, model, total_tokens FROM replay_traces;"
```

## Protocol Support

### gRPC OTLP

- **Port**: 4317 (default)
- **Proto**: OpenTelemetry Collector ptrace service
- **Method**: `Export(ExportTraceServiceRequest)`

### HTTP OTLP

- **Port**: 4318 (default)
- **Endpoint**: `/v1/traces`
- **Method**: POST
- **Content-Type**: `application/json`
- **Format**: OTLP JSON (same as protobuf but in JSON)

## Error Handling

### Graceful Degradation

The receiver continues processing even if individual traces fail:

```go
if err := storage.CreateOTELTrace(ctx, otelTrace); err != nil {
    log.Warn("Failed to store OTEL trace", "error", err)
    // Continue processing other spans
    continue
}
```

### Logging

- **Info**: Server start/stop, successful processing
- **Debug**: Individual trace storage with details
- **Warn**: Failed to store individual traces (non-fatal)
- **Error**: Failed to unmarshal or critical errors

## Performance Considerations

### Concurrent Processing

- Processes resource spans concurrently (goroutine per request)
- Database connection pool handles concurrent writes
- Non-blocking storage failures (logs warning, continues)

### Memory Efficiency

- Streams spans (doesn't load all into memory)
- Immediate storage after parsing
- No buffering (real-time ingestion)

## Next Steps

With OTLP receiver complete, you can now:

1. **Test end-to-end ingestion**:
   ```bash
   make dev-up
   make run
   # Send test trace (see integration test above)
   # Check database for stored traces
   ```

2. **Integrate with agentgateway**:
   - Configure agentgateway to export OTEL traces to `localhost:4317` (gRPC)
   - See [docs/AGENTGATEWAY_CAPTURE.md](AGENTGATEWAY_CAPTURE.md) for the validated capture config

3. **Run freeze-mcp** (separate repo) for deterministic tool replay using captured `tool_captures`

## Dependencies Added

Updated `go.mod` with:
- `go.opentelemetry.io/collector/pdata v1.24.0` - OTLP data models
- `google.golang.org/grpc v1.69.4` - gRPC server

## API Endpoints

### gRPC

```protobuf
service TraceService {
  rpc Export(ExportTraceServiceRequest) returns (ExportTraceServiceResponse);
}
```

### HTTP

```
POST /v1/traces
Content-Type: application/json

{
  "resourceSpans": [...]
}
```

## Configuration

In `.env`:

```bash
# OTLP Receiver endpoints
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318
```

In `serve` command:
- Reads config from environment
- Starts both gRPC and HTTP receivers
- Registers shutdown handlers

---

**Status**: Complete and integrated into the `cmdr serve` command.
