# Implementation Status

Last updated: 2026-02-27

## Completed

### Foundation
- [x] Project structure and Go module
- [x] Configuration system (`pkg/config/`) with envconfig + validation
- [x] Structured logging (`pkg/utils/logger/`) with zap
- [x] CLI framework (Cobra) with all commands scaffolded
- [x] Docker multi-stage build (distroless runtime)
- [x] docker-compose (PostgreSQL 15 + Jaeger)
- [x] GitHub Actions CI (test, lint, build)
- [x] Makefile with dev workflow commands
- [x] Local dev setup scripts

### Database Layer (`pkg/storage/`)
- [x] Storage interface — 34 methods
- [x] PostgreSQL implementation — full CRUD for all 12 tables
- [x] Data models — 11 structs with proper Go tags, JSONB support
- [x] Migrations — embedded SQL, 12 tables with indexes and constraints
- [x] Integration tests — 5 test functions (traces, tools, experiments, evaluators)

### OTLP Receiver (`pkg/otelreceiver/`)
- [x] gRPC receiver (port 4317)
- [x] HTTP receiver (port 4318) with JSON + protobuf support
- [x] Span parser — extracts `gen_ai.*` attributes
- [x] LLM data extraction — model, provider, prompts, completions, tokens, parameters
- [x] Tool call extraction from span events
- [x] Risk class determination (read/write/destructive by tool name)
- [x] Args hash calculation (SHA-256, recursive normalization, int/float coercion)
- [x] Integration with storage layer
- [x] Integrated into `serve` command
- [x] Unit tests with mock storage + args hash determinism tests

### freeze-mcp (Separate Repo)
- [x] Python MCP server (FastAPI + MCP SDK + SSE transport)
- [x] Reads from CMDR's `tool_captures` table (read-only)
- [x] Lookup by `(tool_name, args_hash, trace_id)`
- [x] Args hash normalization matching CMDR's Go implementation
- [x] Per-session trace scoping via `X-Freeze-Trace-ID` header
- [x] LRU cache, async PostgreSQL pool
- [x] Fully tested with pinned cross-language hash vectors

## In Progress

### Drift Detection (`pkg/drift/` — TODO)
- [ ] Baseline management (storage methods + migration)
- [ ] Behavior fingerprinting (tool patterns, risk distribution, token usage)
- [ ] Drift comparison (sequence similarity, risk escalation, composite score)
- [ ] Drift CLI commands (`cmdr drift baseline/check/status/watch`)
- [ ] Drift results storage

### Deployment Gate (`pkg/replay/`, `pkg/agwclient/`, `pkg/diff/` — TODO)
- [ ] agentgateway HTTP client
- [ ] Prompt replay engine (load baseline, replay each step with variant model)
- [ ] Behavior diff (tool call comparison, risk changes, token delta)
- [ ] Gate CLI commands (`cmdr gate check/report`)

## Not Started

- [ ] Demo scenario scripting
- [ ] Report generation (Markdown)
- [ ] Demo video recording
- [ ] Blog post
- [ ] Submission materials

## Descoped for Hackathon

- REST API (CLI-only for now)
- 5 evaluator types (replaced by drift + gate verdicts)
- Human review queue
- LLM-as-judge
- kagent integration
- agentregistry integration
- Production hardening (JWT, rate limiting, Prometheus)

## Known Issues

- `serve.go` uses `time.Sleep(500ms)` for startup check — should use proper readiness signal
- Shutdown is incomplete (`shutdownCtx` created but unused)
- No request body size limit on HTTP OTLP endpoint
- Dockerfile uses Go 1.24, matches go.mod (fixed from 1.23)
- Some notes/ docs reference stale paths (`/Users/jaden.lee/...`)
