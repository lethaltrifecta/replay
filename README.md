# CMDR - Agent Behavior Governance

CMDR (Comparative Model Deterministic Replay) is a governance-oriented trace analysis service for LLM agents.
It ingests OpenTelemetry spans, stores normalized replay/tool data in PostgreSQL, and compares live traces against approved baselines to detect behavioral drift.

## Current Scope

Implemented in this repo:
- OTLP ingestion (gRPC + HTTP)
- `gen_ai.*` span parsing into replay-friendly storage models
- Tool capture extraction with deterministic args hashing + risk classification
- Baseline management + drift scoring (fingerprint + comparison engine)
- Drift CLI commands for baseline set/list/remove and trace drift checks

Planned but not implemented yet:
- Deployment gate / prompt replay engine
- agentgateway replay client
- Behavior diff engine for replay comparisons
- Full experiment/eval/ground-truth CLI workflows (currently scaffolded)

## Architecture

```
agentgateway (or any OTLP emitter)
        |
        | OTLP (gRPC/HTTP)
        v
+-------------------------------+
| CMDR service (`cmdr serve`)   |
| - OTLP receiver               |
| - parser (`gen_ai.*`)         |
| - drift baseline/check logic  |
+-------------------------------+
        |
        v
+-------------------------------+
| PostgreSQL                    |
| - otel_traces                 |
| - replay_traces               |
| - tool_captures               |
| - baselines                   |
| - drift_results               |
| - (experiment/eval tables)    |
+-------------------------------+

freeze-mcp (separate repo/service)
reads `tool_captures` for deterministic tool replay.
```

## Quick Start

```bash
# 1) one-time local setup
make setup-dev

# 2) adjust .env (at minimum set CMDR_AGENTGATEWAY_URL)

# 3) run CMDR
make run
```

CMDR starts OTLP listeners on:
- `CMDR_OTLP_GRPC_ENDPOINT` (default `0.0.0.0:4317`)
- `CMDR_OTLP_HTTP_ENDPOINT` (default `0.0.0.0:4318`)

Health check:
```bash
curl -i http://localhost:4318/health
```

More detailed setup and validation: [docs/QUICKSTART.md](docs/QUICKSTART.md)

## Drift Workflow

Mark an existing trace as baseline:

```bash
cmdr drift baseline set <trace-id> --name "prod-baseline"
```

List baselines:

```bash
cmdr drift baseline list
```

Compare a candidate trace against the most recent baseline:

```bash
cmdr drift check <candidate-trace-id>
```

Compare against a specific baseline:

```bash
cmdr drift check <candidate-trace-id> --baseline <baseline-trace-id>
```

Remove baseline:

```bash
cmdr drift baseline remove <trace-id>
```

Each drift check stores a row in `drift_results` with:
- composite drift score (`0.0` to `1.0`)
- verdict (`pass` / `warn` / `fail`)
- detailed per-dimension breakdown

## Command Status

- `cmdr serve`: implemented
- `cmdr drift baseline {set,list,remove}`: implemented
- `cmdr drift check`: implemented
- `cmdr drift status`: implemented
- `cmdr drift watch`: implemented
- `cmdr experiment *`: scaffold only (prints not implemented)
- `cmdr eval *`: scaffold only (prints not implemented)
- `cmdr ground-truth *`: scaffold only (prints not implemented)

## Development

```bash
make dev-up          # start postgres + jaeger
make run             # build + run cmdr serve
make dev-down        # stop services
make dev-reset       # wipe and restart local DB

make test            # unit tests under ./pkg/... (storage tests require DB)
make test-storage    # storage tests against local postgres
make lint
make fmt
```

## Testing Notes

- Pure unit packages (`pkg/config`, `pkg/drift`, `pkg/otelreceiver`) run without PostgreSQL.
- Storage tests (`pkg/storage`) require a reachable PostgreSQL instance at `CMDR_POSTGRES_URL`.

## Project Layout

```
cmd/cmdr/
  commands/
    serve.go              # OTLP receiver startup
    drift.go              # baseline + drift check commands
    experiment.go         # scaffold
    eval.go               # scaffold
    ground_truth.go       # scaffold

pkg/
  config/                 # env-based config loading/validation
  otelreceiver/           # OTLP receiver + span parsing
  storage/                # PostgreSQL models, queries, migrations
  drift/                  # fingerprint extraction + comparison scoring
  utils/logger/           # zap logger wrapper

docs/                     # setup/testing/receiver docs
notes/                    # implementation notes + planning
```

## Documentation

- [docs/QUICKSTART.md](docs/QUICKSTART.md)
- [docs/LOCAL_DEV_SETUP.md](docs/LOCAL_DEV_SETUP.md)
- [docs/OTLP_RECEIVER.md](docs/OTLP_RECEIVER.md)
- [docs/TESTING_OTLP.md](docs/TESTING_OTLP.md)
- [docs/DEBUGGING_OTLP.md](docs/DEBUGGING_OTLP.md)
- [notes/IMPLEMENTATION_STATUS.md](notes/IMPLEMENTATION_STATUS.md)
- [docs/REFACTORING.md](docs/REFACTORING.md)

## Hackathon

Built for MCP_HACK//26 (Secure & Govern MCP), with a governance-first focus:
- detect production drift from known-good behavior
- eventually gate model/prompt rollouts with deterministic replay
