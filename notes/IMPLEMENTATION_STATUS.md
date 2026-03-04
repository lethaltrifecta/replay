# Implementation Status

Last updated: 2026-03-04

## Completed

### Core platform
- [x] Project structure, build tooling, CI, Docker assets
- [x] Environment config loading and validation (`pkg/config`)
- [x] Structured logging (`pkg/utils/logger`)

### Storage layer
- [x] PostgreSQL storage implementation with pgx/v5 + batch COPY inserts (`pkg/storage`)
- [x] Initial schema migration (`001_initial_schema.sql`)
- [x] Baseline + drift migration (`002_baselines_and_drift.sql`)
- [x] Drift unique constraint migration (`003_drift_unique_constraint.sql`)
- [x] Baseline CRUD methods (`MarkTraceAsBaseline`, `ListBaselines`, `GetBaseline`, `UnmarkBaseline`)
- [x] Drift result persistence methods — idempotent `CreateDriftResult` with `ON CONFLICT`, `GetDriftResults`, `GetDriftResultsByBaseline(limit)`, `GetLatestDriftResult`, `ListDriftResults` (conditional LIMIT), `HasDriftResultForBaseline`
- [x] `SortAsc` option on `TraceFilters` for ASC poll queries

### OTLP ingestion
- [x] gRPC + HTTP OTLP receiver (`pkg/otelreceiver/receiver.go`)
- [x] LLM span parsing from `gen_ai.*` attributes (`pkg/otelreceiver/parser.go`)
- [x] Tool capture extraction with risk classification + args hash
- [x] Health endpoint on OTLP HTTP server (`/health`)
- [x] HTTP handler: JSON + Protobuf content negotiation with fallback

### Drift detection (Phase 1 — COMPLETE)
- [x] Fingerprint extraction (`pkg/drift/fingerprint.go`)
- [x] Comparison/scoring engine (`pkg/drift/compare.go`)
- [x] Default weighting + pass/warn/fail thresholds (`pkg/drift/config.go`)
- [x] Drift CLI:
  - `cmdr drift baseline set/list/remove`
  - `cmdr drift check` — compare trace against baseline
  - `cmdr drift status` — show recent drift results (unified `--limit`)
  - `cmdr drift watch` — continuous poll with ASC cursor, DB-seeded start, retry + overflow handling
- [x] Hardened poll logic: ASC sort prevents cursor starvation on overflow, DB-seeded cursor prevents clock-skew gaps, unique constraint prevents concurrent dedup races

### Deployment gate (Phase 2 — COMPLETE, structural + semantic scoring)
- [x] agentgateway HTTP client (`pkg/agwclient/client.go`) — OpenAI-compatible, retry with backoff, non-retryable 4xx detection, tool call response parsing
- [x] Replay orchestration engine (`pkg/replay/engine.go`) — load baseline, replay each step via agentgateway, persist variant traces + experiment lifecycle, capture variant tool calls in metadata
- [x] Structural diff engine (`pkg/diff/diff.go`) — step count / token / latency similarity scoring with configurable threshold
- [x] Semantic diff engine (`pkg/diff/semantic.go`) — tool-call sequence/frequency comparison, risk escalation detection, response divergence (Jaccard + length similarity)
- [x] 6-dimension `CompareAll` scoring (structural + semantic) with 4-dimension fallback when no tool data available
- [x] Gate CLI:
  - `cmdr gate check --baseline <trace_id> --model <model> [--threshold]` — replay + diff + pass/fail exit code
  - `cmdr gate report <experiment_id>` — show saved experiment results
- [x] Unit tests for all packages (`agwclient`, `diff`, `replay`)
- **Descoped:** Tool argument comparison (args values deliberately excluded from diff — too noisy for structural/semantic comparison; tool name + risk class is sufficient for behavioral equivalence).

## Not started

### Phase 3: Demo Polish
- Color-coded CLI output
- Report generation
- Demo scenario + recording

## Partially implemented / scaffolded

- `cmdr experiment *`: command scaffolds only
- `cmdr eval *`: command scaffolds only
- `cmdr ground-truth *`: command scaffolds only

## Testing snapshot

Unit tests (no DB required):
- `go test ./pkg/config ./pkg/drift ./pkg/otelreceiver ./pkg/agwclient ./pkg/diff ./pkg/replay ./cmd/cmdr/commands -race` → passing

Integration tests (requires `make dev-up`):
- `make test-storage` → passing (includes idempotency test for drift dedup)

## Known technical debt

- `serve` startup relies on `time.Sleep(500ms)` readiness check
- graceful shutdown logic is incomplete in `serve.go`
- OTLP HTTP handler lacks request body size limits
- drift report details are untyped (`map[string]interface{}`)
- storage interface is broad, causing heavy mocks in tests

## Related docs

- `README.md` for current product-level status
- `TODO.md` for execution backlog
- `docs/REFACTORING.md` for prioritized refactor opportunities
