# Implementation Status

Last updated: 2026-03-02

## Completed

### Core platform
- [x] Project structure, build tooling, CI, Docker assets
- [x] Environment config loading and validation (`pkg/config`)
- [x] Structured logging (`pkg/utils/logger`)

### Storage layer
- [x] PostgreSQL storage implementation (`pkg/storage`)
- [x] Initial schema migration (`001_initial_schema.sql`)
- [x] Baseline + drift migration (`002_baselines_and_drift.sql`)
- [x] Baseline CRUD methods (`MarkTraceAsBaseline`, `ListBaselines`, `GetBaseline`, `UnmarkBaseline`)
- [x] Drift result persistence methods (`CreateDriftResult`, `GetDriftResults`, `GetDriftResultsByBaseline`, `GetLatestDriftResult`, `ListDriftResults`, `HasDriftResultForBaseline`)

### OTLP ingestion
- [x] gRPC + HTTP OTLP receiver (`pkg/otelreceiver/receiver.go`)
- [x] LLM span parsing from `gen_ai.*` attributes (`pkg/otelreceiver/parser.go`)
- [x] Tool capture extraction with risk classification + args hash
- [x] Health endpoint on OTLP HTTP server (`/health`)

### Drift detection
- [x] Fingerprint extraction (`pkg/drift/fingerprint.go`)
- [x] Comparison/scoring engine (`pkg/drift/compare.go`)
- [x] Default weighting + pass/warn/fail thresholds (`pkg/drift/config.go`)
- [x] Drift CLI:
  - `cmdr drift baseline set`
  - `cmdr drift baseline list`
  - `cmdr drift baseline remove`
  - `cmdr drift check`
  - `cmdr drift status`
  - `cmdr drift watch`

## Partially implemented / scaffolded

- `cmdr experiment *`: command scaffolds only
- `cmdr eval *`: command scaffolds only
- `cmdr ground-truth *`: command scaffolds only
- Deployment gate / replay flow: planned, not implemented

## Testing snapshot

Verified in this workspace:
- `go test ./pkg/config ./pkg/drift ./pkg/otelreceiver ./cmd/cmdr/commands` -> passing

Not fully runnable in this sandbox:
- `go test ./pkg/storage` requires reachable PostgreSQL (`CMDR_POSTGRES_URL`), but sandbox-local DB access is restricted here.

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
