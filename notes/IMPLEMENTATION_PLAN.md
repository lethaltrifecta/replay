# LLM Replay Service - Implementation Plan

## Context

This plan covers the creation of a new standalone Go service called `llm-replay-service` that will enable cross-model differential replay of LLM requests. The service will:

1. Fetch historical LLM traces from OTLP collectors (Jaeger/Tempo)
2. Extract prompts, completions, parameters, and metrics from trace spans
3. Replay those prompts across multiple models via agentgateway
4. Compare outputs, tokens, costs, and latency across models
5. Provide a REST API for job submission and results querying

This service is motivated by the need to evaluate different LLM models on real production workloads, enabling data-driven model selection and cost optimization.

The service will be built in Go, following patterns established in the agentgateway controller codebase, and deployed as a separate microservice at `/Users/jaden.lee/code/lethal/replay`.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                  LLM Replay Service                      │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  ┌──────────────┐      ┌──────────────┐               │
│  │ Trace Fetcher│─────▶│ Storage      │               │
│  │ (Jaeger/     │      │ (Memory/     │               │
│  │  Tempo)      │      │  Postgres)   │               │
│  └──────────────┘      └──────────────┘               │
│         │                      │                        │
│         ▼                      ▼                        │
│  ┌────────────────────────────────────┐               │
│  │     Replay Engine                  │               │
│  │  - Job Management                  │               │
│  │  - Worker Pool (concurrent)        │               │
│  │  - Rate Limiting                   │               │
│  └────────────────────────────────────┘               │
│         │                                               │
│         ▼                                               │
│  ┌──────────────────┐                                 │
│  │ Agentgateway     │                                 │
│  │ Client           │                                 │
│  └──────────────────┘                                 │
│         │                                               │
│         ▼                                               │
│  ┌──────────────────┐      ┌──────────────┐          │
│  │ Comparator       │─────▶│ REST API     │          │
│  │ - Similarity     │      │ - Job CRUD   │          │
│  │ - Diffs          │      │ - Results    │          │
│  │ - Metrics        │      └──────────────┘          │
│  └──────────────────┘                                 │
└─────────────────────────────────────────────────────────┘
         ▲                              │
         │                              ▼
   ┌─────────────┐           ┌──────────────────┐
   │ Jaeger/Tempo│           │ Agentgateway     │
   │   (OTLP)    │           │   (LLM Proxy)    │
   └─────────────┘           └──────────────────┘
```

## Directory Structure

```
/Users/jaden.lee/code/lethal/replay/
├── cmd/
│   └── replay/
│       └── main.go                    # CLI entry point with cobra
├── pkg/
│   ├── config/
│   │   ├── config.go                  # Configuration structs & loading
│   │   ├── config_test.go
│   │   └── validation.go
│   ├── tracefetcher/
│   │   ├── interface.go               # TraceFetcher interface
│   │   ├── jaeger_client.go           # Jaeger HTTP API client
│   │   ├── tempo_client.go            # Tempo implementation
│   │   ├── otlp_parser.go             # Parse OTLP spans
│   │   ├── trace_filter.go            # Filter by LLM attributes
│   │   └── tracefetcher_test.go
│   ├── agwclient/
│   │   ├── client.go                  # HTTP client for agentgateway
│   │   ├── request_builder.go         # Build LLM requests
│   │   ├── retry.go                   # Retry with backoff
│   │   ├── options.go                 # Functional options pattern
│   │   └── agwclient_test.go
│   ├── replayengine/
│   │   ├── engine.go                  # Orchestration logic
│   │   ├── workerpool.go              # Worker pool for concurrency
│   │   ├── job.go                     # Job types & status
│   │   ├── scheduler.go               # Task scheduling
│   │   └── replayengine_test.go
│   ├── storage/
│   │   ├── interface.go               # Storage interface
│   │   ├── memory.go                  # In-memory implementation
│   │   ├── postgres.go                # PostgreSQL implementation
│   │   ├── models.go                  # Data models
│   │   └── storage_test.go
│   ├── comparator/
│   │   ├── comparator.go              # Result comparison
│   │   ├── similarity.go              # Text similarity algorithms
│   │   ├── metrics.go                 # Token/cost calculations
│   │   ├── diff.go                    # Output diffing
│   │   └── comparator_test.go
│   ├── api/
│   │   ├── server.go                  # HTTP server setup
│   │   ├── handlers.go                # REST handlers
│   │   ├── middleware.go              # Logging, auth
│   │   ├── routes.go                  # Route definitions
│   │   └── api_test.go
│   ├── version/
│   │   └── version.go                 # Build version info
│   └── utils/
│       ├── errors.go                  # Error wrapping
│       ├── logger.go                  # Structured logging
│       └── time.go                    # Time utilities
├── test/
│   ├── integration/
│   │   ├── replay_test.go             # E2E tests
│   │   └── testdata/
│   │       └── sample_traces.json
│   ├── mocks/
│   │   └── mock_interfaces.go         # Generated mocks
│   └── helpers/
│       └── test_helpers.go
├── deployments/
│   ├── docker/
│   │   └── Dockerfile
│   └── kubernetes/
│       ├── deployment.yaml
│       ├── service.yaml
│       └── configmap.yaml
├── scripts/
│   ├── build.sh
│   ├── test.sh
│   └── lint.sh
├── docs/
│   ├── architecture.md
│   ├── api.md
│   └── configuration.md
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── .golangci.yml
```

## Core Components

### 1. Configuration Package (`pkg/config/`)

**Purpose**: Load and validate configuration from environment variables and optional YAML files.

**Pattern**: Use `github.com/kelseyhightower/envconfig` (same as agentgateway controller at `controller/api/settings/settings.go`)

**Key Types**:
```go
type Config struct {
    TraceBackend        string `envconfig:"TRACE_BACKEND" default:"jaeger"`
    JaegerURL           string `envconfig:"JAEGER_URL" default:"http://localhost:16686"`
    TempoURL            string `envconfig:"TEMPO_URL"`
    AgentgatewayURL     string `envconfig:"AGENTGATEWAY_URL" required:"true"`
    AgentgatewayTimeout int    `envconfig:"AGENTGATEWAY_TIMEOUT" default:"60"`
    WorkerPoolSize      int    `envconfig:"WORKER_POOL_SIZE" default:"10"`
    MaxConcurrency      int    `envconfig:"MAX_CONCURRENCY" default:"5"`
    StorageBackend      string `envconfig:"STORAGE_BACKEND" default:"memory"`
    PostgresURL         string `envconfig:"POSTGRES_URL"`
    APIPort             int    `envconfig:"API_PORT" default:"8080"`
    LogLevel            string `envconfig:"LOG_LEVEL" default:"info"`
}
```

**Reference**: `/Users/jaden.lee/code/lethal/agentgateway/controller/api/settings/settings.go` (lines 114-202)

### 2. Trace Fetcher Package (`pkg/tracefetcher/`)

**Purpose**: Fetch traces from OTLP backends and extract LLM-specific data from span attributes.

**Pattern**: Interface-based design with multiple implementations

**Key Interface**:
```go
type TraceFetcher interface {
    FetchTraces(ctx context.Context, query TraceQuery) ([]Trace, error)
    FetchTrace(ctx context.Context, traceID string) (*Trace, error)
}
```

**Implementations**:
- `jaeger_client.go`: HTTP API client for Jaeger (`GET /api/traces`)
- `tempo_client.go`: Tempo implementation

**OTLP Parsing** (`otlp_parser.go`):
- Extract span attributes from flattened format (e.g., `gen_ai.prompt.0.role`, `gen_ai.prompt.0.content`)
- Parse token counts from `gen_ai.usage.*` attributes
- Extract parameters from `gen_ai.request.*` attributes
- Based on agentgateway's OTEL tracing format documented in research

**Key Types**:
```go
type Trace struct {
    TraceID     string
    SpanID      string
    Timestamp   time.Time
    Prompt      []Message
    Completion  []string
    Model       string
    Provider    string
    Parameters  LLMParameters
    Metrics     LLMMetrics
}

type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type LLMMetrics struct {
    PromptTokens     int64
    CompletionTokens int64
    TotalTokens      int64
    Latency          time.Duration
}
```

### 3. Agentgateway Client Package (`pkg/agwclient/`)

**Purpose**: HTTP client to send replay requests to agentgateway

**Pattern**: Follow agentgateway's curl options pattern at `controller/pkg/utils/requestutils/curl/option.go`

**Key Features**:
- Connection pooling
- Retry with exponential backoff (using `github.com/avast/retry-go/v4`)
- Timeout configuration
- Request/response tracing

**Functional Options**:
```go
type Option func(*Client)

func WithAuth(apiKey string) Option
func WithTimeout(duration time.Duration) Option
func WithRetry(attempts int) Option
func WithCustomHeaders(headers map[string]string) Option
```

**Reference**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/utils/requestutils/curl/option.go`

### 4. Replay Engine Package (`pkg/replayengine/`)

**Purpose**: Orchestrate replay jobs across multiple models with concurrent execution

**Pattern**: Worker pool pattern from agentgateway at `controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go`

**Key Components**:

**`engine.go`**: Main orchestration
- Create jobs from trace queries
- Dispatch to worker pool
- Track job progress
- Handle cancellation via context

**`workerpool.go`**: Concurrent execution
- Based on `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go`
- Fixed pool size with dynamic task queue
- Rate limiting per model
- Graceful shutdown

**`job.go`**: Job state management
```go
type Job struct {
    ID           string
    Name         string
    TraceIDs     []string
    Models       []string
    Status       JobStatus
    Progress     float64
    CreatedAt    time.Time
    CompletedAt  *time.Time
    Results      []ReplayResult
    Comparisons  []Comparison
}

type JobStatus string
const (
    JobStatusPending   JobStatus = "pending"
    JobStatusRunning   JobStatus = "running"
    JobStatusCompleted JobStatus = "completed"
    JobStatusFailed    JobStatus = "failed"
    JobStatusCancelled JobStatus = "cancelled"
)
```

**Reference**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go` (lines 10-206)

### 5. Storage Package (`pkg/storage/`)

**Purpose**: Persist jobs and results

**Pattern**: Interface with multiple backends

**Interface**:
```go
type Storage interface {
    CreateJob(ctx context.Context, job *Job) error
    GetJob(ctx context.Context, id string) (*Job, error)
    UpdateJob(ctx context.Context, job *Job) error
    ListJobs(ctx context.Context, filters ListFilters) ([]*Job, error)
    SaveResults(ctx context.Context, jobID string, results []ReplayResult) error
}
```

**Implementations**:
- `memory.go`: In-memory using `sync.Map` (initial development)
- `postgres.go`: PostgreSQL with prepared statements (production)

**Schema** (for Postgres):
```sql
CREATE TABLE replay_jobs (
    id UUID PRIMARY KEY,
    name VARCHAR(255),
    trace_ids TEXT[],
    models TEXT[],
    status VARCHAR(50),
    progress FLOAT,
    created_at TIMESTAMP,
    completed_at TIMESTAMP,
    results JSONB
);

CREATE TABLE replay_results (
    id UUID PRIMARY KEY,
    job_id UUID REFERENCES replay_jobs(id),
    trace_id VARCHAR(255),
    model VARCHAR(255),
    response TEXT,
    metrics JSONB,
    error TEXT,
    created_at TIMESTAMP
);
```

### 6. Comparator Package (`pkg/comparator/`)

**Purpose**: Compare outputs across models and generate metrics

**Key Features**:
- Text similarity (Levenshtein distance, cosine similarity)
- Token count deltas
- Cost estimation (configurable pricing per model)
- Latency comparison
- Diff generation (text, JSON)

**Key Types**:
```go
type Comparison struct {
    TraceID       string
    BaselineModel string
    TargetModel   string
    Similarity    float64  // 0-1 score
    TokenDiff     int64    // vs baseline
    CostDiff      float64  // USD
    LatencyDiff   time.Duration
    Diff          string   // Human-readable diff
}

type ComparisonSummary struct {
    TotalTraces     int
    AvgSimilarity   float64
    TotalTokenDelta int64
    TotalCostDelta  float64
}
```

### 7. API Package (`pkg/api/`)

**Purpose**: REST API for job management and results querying

**Framework**: `net/http` with `gorilla/mux` router

**Endpoints**:
- `POST /api/v1/replay` - Submit replay job
- `GET /api/v1/replay/:id` - Get job status
- `GET /api/v1/replay/:id/results` - Get comparison results
- `GET /api/v1/replay` - List jobs (with filters)
- `DELETE /api/v1/replay/:id` - Cancel job
- `GET /health` - Health check
- `GET /ready` - Readiness check

**Middleware**:
- Request logging
- Request ID injection
- Error handling
- CORS (if needed)
- Authentication (optional)

## Implementation Sequence

### Phase 1: Foundation (Days 1-3)
1. Initialize Go module and repository structure
2. Implement config package with envconfig
3. Set up logging with zap
4. Create Makefile with build/test/lint targets
5. Set up CI workflow (GitHub Actions)

**Files to create**:
- `go.mod`, `go.sum`
- `pkg/config/config.go`
- `pkg/utils/logger.go`
- `Makefile`
- `.github/workflows/ci.yml`

### Phase 2: Trace Fetcher (Days 4-6)
1. Define TraceFetcher interface
2. Implement Jaeger HTTP client
3. Implement OTLP span parser
4. Write unit tests with mock traces
5. Write integration test with local Jaeger

**Files to create**:
- `pkg/tracefetcher/interface.go`
- `pkg/tracefetcher/jaeger_client.go`
- `pkg/tracefetcher/otlp_parser.go`
- `pkg/tracefetcher/tracefetcher_test.go`
- `test/testdata/sample_traces.json`

**Reference**: Agentgateway trace format at `crates/agentgateway/src/telemetry/trc.rs`

### Phase 3: Agentgateway Client (Days 7-9)
1. Implement HTTP client with connection pooling
2. Add functional options pattern
3. Implement retry logic with backoff
4. Build request mapper from Trace to agentgateway format
5. Write unit tests and integration tests

**Files to create**:
- `pkg/agwclient/client.go`
- `pkg/agwclient/options.go`
- `pkg/agwclient/retry.go`
- `pkg/agwclient/request_builder.go`
- `pkg/agwclient/agwclient_test.go`

**Reference**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/utils/requestutils/curl/option.go`

### Phase 4: Storage Layer (Days 10-12)
1. Define Storage interface
2. Implement in-memory storage with sync.Map
3. Design PostgreSQL schema
4. Implement PostgreSQL storage with connection pooling
5. Write storage tests

**Files to create**:
- `pkg/storage/interface.go`
- `pkg/storage/memory.go`
- `pkg/storage/postgres.go`
- `pkg/storage/models.go`
- `pkg/storage/storage_test.go`
- `deployments/migrations/001_initial.sql`

### Phase 5: Replay Engine (Days 13-16)
1. Define Job types and state machine
2. Implement worker pool (based on agentgateway pattern)
3. Implement engine orchestration logic
4. Add rate limiting and cancellation support
5. Write engine tests

**Files to create**:
- `pkg/replayengine/job.go`
- `pkg/replayengine/workerpool.go`
- `pkg/replayengine/engine.go`
- `pkg/replayengine/scheduler.go`
- `pkg/replayengine/replayengine_test.go`

**Reference**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go`

### Phase 6: Comparator (Days 17-18)
1. Implement text similarity algorithms
2. Implement token/cost delta calculations
3. Implement diff generation
4. Write comparator tests

**Files to create**:
- `pkg/comparator/comparator.go`
- `pkg/comparator/similarity.go`
- `pkg/comparator/metrics.go`
- `pkg/comparator/diff.go`
- `pkg/comparator/comparator_test.go`

### Phase 7: REST API (Days 19-21)
1. Set up HTTP server with graceful shutdown
2. Implement API handlers (CRUD)
3. Add middleware (logging, errors)
4. Write API tests
5. Document API endpoints

**Files to create**:
- `pkg/api/server.go`
- `pkg/api/handlers.go`
- `pkg/api/middleware.go`
- `pkg/api/routes.go`
- `pkg/api/api_test.go`
- `docs/api.md`

### Phase 8: Main Entry Point (Days 22-23)
1. Implement CLI with cobra
2. Wire up all components
3. Add graceful shutdown
4. Write end-to-end test

**Files to create**:
- `cmd/replay/main.go`
- `test/integration/replay_test.go`

### Phase 9: Docker & Deployment (Days 24-25)
1. Create multi-stage Dockerfile
2. Create docker-compose for local development
3. Create Kubernetes manifests
4. Write deployment documentation

**Files to create**:
- `deployments/docker/Dockerfile`
- `deployments/docker-compose.yml`
- `deployments/kubernetes/deployment.yaml`
- `deployments/kubernetes/service.yaml`
- `deployments/kubernetes/configmap.yaml`
- `docs/deployment.md`

### Phase 10: Documentation & Polish (Days 26-28)
1. Write comprehensive README
2. Add architecture documentation
3. Add configuration guide
4. Polish code and add comments
5. Run final lint and format

**Files to create**:
- `README.md`
- `docs/architecture.md`
- `docs/configuration.md`
- `.golangci.yml`

## Key Dependencies

```go
require (
    // HTTP routing
    github.com/gorilla/mux v1.8.1

    // Configuration
    github.com/kelseyhightower/envconfig v1.4.0

    // Logging
    go.uber.org/zap v1.27.1

    // Retry logic
    github.com/avast/retry-go/v4 v4.7.0

    // PostgreSQL
    github.com/lib/pq v1.10.9

    // OpenTelemetry (trace parsing)
    go.opentelemetry.io/otel v1.40.0
    go.opentelemetry.io/otel/trace v1.40.0
    go.opentelemetry.io/proto/otlp v1.9.0

    // CLI framework
    github.com/spf13/cobra v1.10.2

    // Testing
    github.com/stretchr/testify v1.11.1
    github.com/golang/mock v1.7.0-rc.1

    // Utilities
    github.com/google/uuid v1.6.0
)
```

## Configuration

**Environment Variables** (prefix: `REPLAY_`):
```bash
REPLAY_TRACE_BACKEND=jaeger
REPLAY_JAEGER_URL=http://localhost:16686
REPLAY_AGENTGATEWAY_URL=http://localhost:3000
REPLAY_AGENTGATEWAY_TIMEOUT=60
REPLAY_WORKER_POOL_SIZE=10
REPLAY_MAX_CONCURRENCY=5
REPLAY_STORAGE_BACKEND=memory
REPLAY_POSTGRES_URL=postgres://user:pass@localhost:5432/replay
REPLAY_API_PORT=8080
REPLAY_LOG_LEVEL=info
```

**Loading Pattern** (from agentgateway):
```go
var cfg Config
if err := envconfig.Process("REPLAY", &cfg); err != nil {
    return fmt.Errorf("failed to load config: %w", err)
}
```

## Testing Strategy

**Unit Tests**:
- Each package has `*_test.go` files
- Use table-driven tests
- Mock external dependencies
- Target 80%+ coverage
- Run: `make test`

**Integration Tests**:
- Test against real Jaeger (via testcontainers)
- Test against real PostgreSQL
- Test full replay flow
- Location: `test/integration/`
- Run: `make test-integration`

**E2E Tests**:
- Full system test with all components
- Use sample trace data
- Verify API responses
- Run: `make test-e2e`

## Docker & Deployment

**Dockerfile** (multi-stage):
```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/replay ./cmd/replay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/replay /bin/replay
EXPOSE 8080
ENTRYPOINT ["/bin/replay"]
CMD ["serve"]
```

**Local Development**:
```bash
docker-compose up -d  # Start all services
make run              # Run locally
```

**Kubernetes Deployment**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: replay-service
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: replay
        image: ghcr.io/lethal/replay:latest
        ports:
        - containerPort: 8080
        env:
        - name: REPLAY_AGENTGATEWAY_URL
          value: "http://agentgateway:8080"
```

## Critical Files to Reference

1. **Worker Pool Pattern**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/kgateway/agentgatewaysyncer/status/workerpool.go`
   - Use for concurrent replay execution
   - Lines 10-206 show resource locking and queue management

2. **Options Pattern**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/utils/requestutils/curl/option.go`
   - Use for agentgateway client configuration
   - Shows functional options for flexible request building

3. **Configuration Loading**: `/Users/jaden.lee/code/lethal/agentgateway/controller/api/settings/settings.go`
   - Lines 114-202 show envconfig usage
   - Custom decoders for validation

4. **Error Handling**: `/Users/jaden.lee/code/lethal/agentgateway/controller/pkg/deployer/errors.go`
   - Lines 9-18 show sentinel errors and wrapping
   - Use for descriptive error types

5. **Testing Suite**: `/Users/jaden.lee/code/lethal/agentgateway/controller/test/e2e/tests/base/base_suite.go`
   - Shows testify/suite structure
   - Use for organized integration tests

## Verification

After implementation, verify the service works end-to-end:

1. **Start Dependencies**:
   ```bash
   docker-compose up -d jaeger agentgateway postgres
   ```

2. **Configure Service**:
   ```bash
   export REPLAY_AGENTGATEWAY_URL=http://localhost:3000
   export REPLAY_JAEGER_URL=http://localhost:16686
   export REPLAY_POSTGRES_URL=postgres://replay:replay@localhost:5432/replay
   ```

3. **Run Service**:
   ```bash
   make build
   ./bin/replay serve
   ```

4. **Submit Test Job**:
   ```bash
   curl -X POST http://localhost:8080/api/v1/replay \
     -H "Content-Type: application/json" \
     -d '{
       "trace_ids": ["abc123"],
       "models": ["gpt-4", "claude-3-5-sonnet-20241022"]
     }'
   ```

5. **Check Results**:
   ```bash
   curl http://localhost:8080/api/v1/replay/{job_id}/results
   ```

6. **Run Tests**:
   ```bash
   make test              # Unit tests
   make test-integration  # Integration tests
   make coverage          # Coverage report
   ```

7. **Verify Metrics**:
   - Check job completes successfully
   - Verify comparison results show similarity scores
   - Confirm token deltas are calculated
   - Check latency measurements

## Success Criteria

- [ ] Service successfully fetches traces from Jaeger
- [ ] Service replays prompts through agentgateway to multiple models
- [ ] Results include similarity scores, token deltas, and cost comparisons
- [ ] REST API allows job submission and results querying
- [ ] Worker pool executes replays concurrently with rate limiting
- [ ] Unit test coverage > 80%
- [ ] Integration tests pass
- [ ] Docker image builds and runs
- [ ] Documentation is complete and accurate
