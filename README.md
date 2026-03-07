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
- Deployment gate: replay baseline prompts with a variant model via agentgateway, diff results with structural + semantic scoring, produce CI/CD pass/fail verdict
- Semantic diff: tool-call sequence/frequency comparison, risk escalation detection, response divergence (Jaccard + length)
- Gate CLI commands for replay check and experiment reporting

Planned but not implemented yet:
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
+-------------------------------+       +-------------------------------+
| PostgreSQL                    |       | agentgateway                  |
| - otel_traces                 |       | (LLM + MCP proxy)             |
| - replay_traces               |       +-------------------------------+
| - tool_captures               |                   ^
| - baselines / drift_results   |                   |
| - experiments / runs          |       cmdr gate check (replay prompts
| - analysis_results            |        with variant model config)
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

## Gate Workflow

Replay a captured baseline trace with a different model and verify the behavior is comparable:

```bash
cmdr gate check --baseline <trace-id> --model gpt-4o --threshold 0.8
```

This will:
1. Load all baseline replay steps from the database
2. Send each prompt to the variant model via agentgateway
3. Store variant responses as a new experiment with runs
4. Compute similarity across 6 dimensions (step count, tokens, latency, tool calls, risk, response) when tool data is available, or 4 dimensions (structural + response) as fallback
5. Print a verdict and exit with code 0 (pass) or 1 (fail)

View a saved experiment report:

```bash
cmdr gate report <experiment-id>
```

## Judge Demo

Run a deterministic, no-external-LLM demo that shows:
- drift detection catching risky behavior
- deployment gate failing a dangerous model with exit code `1`
- deployment gate passing a safe model with exit code `0`

```bash
make dev-up
make demo
```

Manual demo commands:

```bash
cmdr demo seed
cmdr drift check demo-drifted-002 --baseline demo-baseline-001
cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger   # exits 1
cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet  # exits 0
```

Full presenter script and expected outputs: [docs/DEMO.md](docs/DEMO.md)

## Command Status

- `cmdr serve`: implemented
- `cmdr drift baseline {set,list,remove}`: implemented
- `cmdr drift check`: implemented
- `cmdr drift status`: implemented
- `cmdr drift watch`: implemented
- `cmdr gate check`: implemented (structural + semantic diff)
- `cmdr gate report`: implemented
- `cmdr demo {seed,gate}`: implemented (deterministic hackathon demo commands)
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

- Pure unit packages (`pkg/config`, `pkg/drift`, `pkg/otelreceiver`, `pkg/agwclient`, `pkg/diff`, `pkg/replay`) run without PostgreSQL.
- Storage tests (`pkg/storage`) require a reachable PostgreSQL instance at `CMDR_POSTGRES_URL`.
- Freeze contract e2e test (`test/e2e/freeze_contract_test.go`) is opt-in:
  - Start freeze-mcp (`python -m freeze_mcp.server`) against the same PostgreSQL.
  - Run: `make test-e2e-freeze-contract`
  - Optional endpoint overrides: `E2E_FREEZE_HEALTH_URL`, `E2E_FREEZE_MCP_URL`.
- Replay + freeze e2e test (`test/e2e/replay_freeze_test.go`) is opt-in:
  - Start CMDR (`cmdr serve`) and freeze-mcp (`python -m freeze_mcp.server`) against the same PostgreSQL.
  - Run: `make test-e2e-replay-freeze`
  - Optional endpoint overrides: `E2E_OTLP_HEALTH_URL`, `E2E_OTLP_INGEST_URL`, `E2E_FREEZE_HEALTH_URL`, `E2E_FREEZE_MCP_URL`.

## Project Layout

```
cmd/cmdr/
  commands/
    serve.go              # OTLP receiver startup
    drift.go              # baseline + drift check commands
    gate.go               # deployment gate check + report
    experiment.go         # scaffold
    eval.go               # scaffold
    ground_truth.go       # scaffold

pkg/
  agwclient/              # agentgateway HTTP client (OpenAI-compatible)
  config/                 # env-based config loading/validation
  diff/                   # structural + semantic comparison engine for gate verdicts
  drift/                  # fingerprint extraction + comparison scoring
  otelreceiver/           # OTLP receiver + span parsing
  replay/                 # prompt replay orchestration engine
  storage/                # PostgreSQL models, queries, migrations
  utils/logger/           # zap logger wrapper

docs/                     # setup/testing/receiver docs
notes/                    # implementation notes + planning
```

## Documentation

- [docs/QUICKSTART.md](docs/QUICKSTART.md)
- [docs/LOCAL_DEV_SETUP.md](docs/LOCAL_DEV_SETUP.md)
- [docs/E2E_DEMO_PLAN.md](docs/E2E_DEMO_PLAN.md)
- [docs/AGENTGATEWAY_CAPTURE.md](docs/AGENTGATEWAY_CAPTURE.md)
- [docs/OTLP_RECEIVER.md](docs/OTLP_RECEIVER.md)
- [docs/TESTING_OTLP.md](docs/TESTING_OTLP.md)
- [docs/DEBUGGING_OTLP.md](docs/DEBUGGING_OTLP.md)
- [docs/DEMO.md](docs/DEMO.md)
- [notes/IMPLEMENTATION_STATUS.md](notes/IMPLEMENTATION_STATUS.md)
- [docs/REFACTORING.md](docs/REFACTORING.md)

## Hackathon

Built for MCP_HACK//26 (Secure & Govern MCP), with a governance-first focus:
- detect production drift from known-good behavior
- gate model/prompt rollouts with replay-driven behavior comparison
- make `agentgateway` the primary capture and replay integration
