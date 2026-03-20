# CMDR - Agent Behavior Governance

CMDR (Comparative Model Deterministic Replay) is a governance-oriented trace analysis service for LLM agents.
It ingests OpenTelemetry spans, stores normalized replay/tool data in PostgreSQL, and compares live traces against approved baselines to detect behavioral drift.

## Features

- **OTLP ingestion** — gRPC + HTTP receivers for OpenTelemetry spans
- **Span parsing** — `gen_ai.*` attribute extraction into replay-friendly storage models
- **Tool capture** — deterministic args hashing + risk classification
- **Drift detection** — behavioral fingerprinting + comparison scoring against baselines
- **Deployment gate** — replay baseline prompts with a variant model via agentgateway, structural + semantic diff, CI/CD pass/fail verdict
- **Deterministic demo** — no-external-LLM demo showing drift detection and gate verdicts

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

Run the full database migration demo with saved artifacts:

```bash
cmdr demo migration run
```

This writes a self-contained artifact bundle with logs, structured results, a markdown report, a judge highlight, and a demo script.

Show the newest saved migration demo bundle:

```bash
cmdr demo migration latest
```

## Commands

| Command | Description |
|---------|-------------|
| `cmdr serve` | Start OTLP receiver + HTTP API server |
| `cmdr drift baseline {set,list,remove}` | Manage known-good baselines |
| `cmdr drift check` | Compare a trace against its baseline |
| `cmdr drift status` | Show drift scores for recent traces |
| `cmdr drift watch` | Continuous drift monitoring (poll mode) |
| `cmdr gate check` | Replay baseline with variant model, produce pass/fail verdict |
| `cmdr gate report` | Show saved experiment results |
| `cmdr demo seed` | Seed database with deterministic demo data |
| `cmdr demo gate` | Run gate check with demo models |
| `cmdr demo migration run` | Full migration demo with saved artifacts |
| `cmdr demo migration latest` | Show newest migration demo bundle |
| `cmdr demo migration verdict` | Native verdict for migration traces |

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
- Migration demo full-loop harness:
  - Uses Docker PostgreSQL, CMDR, agentgateway, a mock migration MCP server, and freeze-mcp.
  - Preferred entrypoint: `cmdr demo migration run`
  - Latest artifact summary: `cmdr demo migration latest`
  - Run: `make test-migration-demo-full-loop`
  - Native verdict: `cmdr demo migration verdict --baseline <trace> --candidate <trace>`
  - Full runbook: [docs/MIGRATION_DEMO.md](docs/MIGRATION_DEMO.md)

## Project Layout

```
cmd/cmdr/
  commands/
    serve.go              # OTLP receiver + API server startup
    drift.go              # baseline + drift check commands
    gate.go               # deployment gate check + report
    demo.go               # deterministic hackathon demo

pkg/
  api/                    # HTTP API server, handlers, middleware
  agwclient/              # agentgateway HTTP client (OpenAI-compatible, retry)
  config/                 # env-based config loading/validation
  diff/                   # structural + semantic comparison engine
  drift/                  # fingerprint extraction + comparison scoring
  otelreceiver/           # OTLP receiver + span parsing
  replay/                 # prompt replay orchestration engine
  storage/                # PostgreSQL models, queries, migrations
  utils/logger/           # zap logger wrapper

test/e2e/                 # end-to-end tests (freeze contract, replay)
scripts/                  # dev setup, OTLP testing, demo utilities
docs/                     # architecture, setup, and demo documentation
```

## Documentation

- [docs/QUICKSTART.md](docs/QUICKSTART.md) — Setup and first commands
- [docs/LOCAL_DEV_SETUP.md](docs/LOCAL_DEV_SETUP.md) — Local environment guide
- [docs/DEMO.md](docs/DEMO.md) — Presenter script and expected outputs
- [docs/MIGRATION_DEMO.md](docs/MIGRATION_DEMO.md) — Full migration demo runbook
- [docs/GATE_REPLAY_ARCHITECTURE.md](docs/GATE_REPLAY_ARCHITECTURE.md) — Gate replay design
- [docs/DATABASE_LAYER.md](docs/DATABASE_LAYER.md) — Schema overview
- [docs/AGENTGATEWAY_CAPTURE.md](docs/AGENTGATEWAY_CAPTURE.md) — agentgateway trace capture
- [docs/FREEZE_AGENT_LOOP.md](docs/FREEZE_AGENT_LOOP.md) — freeze-mcp integration
- [docs/E2E_DEMO_PLAN.md](docs/E2E_DEMO_PLAN.md) — End-to-end demo walkthrough
- [docs/OTLP_RECEIVER.md](docs/OTLP_RECEIVER.md) — OTLP receiver implementation details
- [docs/SUBMISSION_NOTES.md](docs/SUBMISSION_NOTES.md) — Hackathon submission summary

## Hackathon

Built for [MCP_HACK//26](https://aihackathon.dev/) — Secure & Govern MCP.

CMDR is the missing deployment safety layer for MCP agents. It captures real agent behavior from agentgateway, freezes the tool environment with freeze-mcp, replays the same scenario with a candidate model or prompt, and blocks unsafe rollout when tool behavior or risk profile changes.

- **agentgateway** — telemetry source (OTLP traces) and replay proxy (LLM + MCP routing)
- **freeze-mcp** — serves frozen tool responses at the MCP boundary for deterministic replay
- **Drift detection** — runtime governance via continuous behavioral fingerprinting
- **Deployment gates** — pre-deployment governance via replay-based verification with CI/CD exit codes

Full submission details: [docs/SUBMISSION_NOTES.md](docs/SUBMISSION_NOTES.md)

## License

MIT — see [LICENSE](LICENSE).
