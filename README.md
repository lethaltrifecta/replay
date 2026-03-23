# CMDR: Agent Behavior Governance

CMDR (Comparative Model Deterministic Replay) is a governance layer for MCP-enabled LLM agents.
It captures behavior from OpenTelemetry traces, stores replay and tool evidence in PostgreSQL, compares runs against approved baselines, and blocks unsafe model or prompt changes with deterministic replay.

> Status: hackathon-ready and demoable today. The strongest end-to-end path is the migration demo and replay workflow; the project is not yet hardened as a multi-tenant production service.

Built for [MCP_HACK//26](https://aihackathon.dev/) in the `Secure & Govern MCP` track.

## Why CMDR

Agent teams need answers to two operational questions:

1. Did our agent's behavior drift from the last known-good state?
2. If we change the model, prompt, or policy, is it still safe to deploy?

CMDR answers both:

- **Drift detection** compares live traces against approved baselines
- **Replay-based gates** rerun baseline scenarios with a candidate configuration
- **Frozen MCP replay** isolates model behavior from live tool-side noise
- **CI-friendly verdicts** return `exit 0` for pass and `exit 1` for fail

## What You Can Run Right Away

- Ingest OTLP traces over gRPC or HTTP
- Store normalized replay steps and tool captures in PostgreSQL
- Compare a candidate trace against a baseline with `cmdr drift check`
- Run `cmdr gate check` for replay-based pass/fail verdicts
- Run a deterministic demo with no external LLM dependency
- Run a full migration scenario that produces judge-friendly artifacts

## Quick Paths

- Want the fastest local setup: [docs/QUICKSTART.md](docs/QUICKSTART.md)
- Want the 2-3 minute deterministic demo: [docs/DEMO.md](docs/DEMO.md)
- Want the full gateway-driven migration demo: [docs/MIGRATION_DEMO.md](docs/MIGRATION_DEMO.md)
- Want the architecture overview: [docs/README.md](docs/README.md)
- Want the hackathon framing: [docs/SUBMISSION_NOTES.md](docs/SUBMISSION_NOTES.md)

## Architecture

```text
agentgateway (or any OTLP emitter)
        |
        | OTLP (gRPC/HTTP)
        v
+--------------------------------+
| CMDR service (`cmdr serve`)    |
| - OTLP receiver                |
| - parser (`gen_ai.*`)          |
| - pair-based drift + gate logic|
+--------------------------------+
        |
        v
+--------------------------------+       +-------------------------------+
| PostgreSQL                     |       | agentgateway                  |
| - otel_traces                  |       | (LLM + MCP proxy)             |
| - replay_traces                |       +-------------------------------+
| - tool_captures                |                   ^
| - baselines / drift_results    |                   |
| - experiments / runs           |       cmdr gate check (replay prompts
| - analysis_results             |        with variant model config)
+--------------------------------+

freeze-mcp (separate repo/service)
reads `tool_captures` for deterministic tool replay.
```

## Quick Start

```bash
# 1) one-time local setup
make setup-dev

# 2) configure environment
cp .env.example .env

# 3) run CMDR
make run
```

Health check:

```bash
curl -i http://localhost:4318/health
```

CMDR starts OTLP listeners on:

- `CMDR_OTLP_GRPC_ENDPOINT` (default `0.0.0.0:4317`)
- `CMDR_OTLP_HTTP_ENDPOINT` (default `0.0.0.0:4318`)

For the full local setup, validation steps, and sample OTLP payloads, use [docs/QUICKSTART.md](docs/QUICKSTART.md).

## Core Workflows

### Drift Detection

Compare a candidate trace against an explicit baseline:

```bash
cmdr drift check <baseline-trace-id> <candidate-trace-id>
cmdr drift list --limit 20
cmdr drift list --baseline <baseline-trace-id> --limit 20
```

Each drift result stores:

- a composite drift score (`0.0` to `1.0`)
- a verdict (`pass`, `warn`, or `fail`)
- a typed breakdown of the contributing dimensions

Current scope notes:

- Drift is pair-based on this branch. A result is always `candidate` compared against `baseline`.
- Baselines are currently created through the HTTP API or the seeded demo flow, not a dedicated CLI command.

### Deployment Gates

Replay a captured baseline trace with a different model and verify whether the behavior stays within the threshold:

```bash
cmdr gate check --baseline <trace-id> --model gpt-4o --threshold 0.8
cmdr gate report <experiment-id>
```

`cmdr gate check`:

1. loads the baseline replay steps
2. replays the prompts through agentgateway
3. stores the variant run as a new experiment
4. compares structure, response behavior, and tool behavior when available
5. exits `0` on pass and `1` on fail

When `MCPURL` is configured, the replay path can execute a multi-turn tool loop against `freeze-mcp`; otherwise it falls back to prompt-only replay.

### Demos

Deterministic demo:

```bash
make dev-up
make demo
```

Manual deterministic demo:

```bash
cmdr demo seed
cmdr drift check demo-baseline-001 demo-drifted-002
cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger
cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet
```

Full migration demo with saved artifacts:

```bash
cmdr demo migration run
cmdr demo migration latest
```

The migration demo writes a self-contained artifact bundle with logs, a structured report, a markdown report, a judge highlight, and a demo script.

## Project Status

CMDR is in a good shape for demos, evaluation, and iteration. It is intentionally narrower than a fully productized governance platform.

- The best end-to-end proof is the migration demo in [docs/MIGRATION_DEMO.md](docs/MIGRATION_DEMO.md).
- The replay system supports both prompt-only and MCP-backed agent-loop execution.
- The full frozen replay path depends on companion services such as `agentgateway` and `freeze-mcp`.
- Production hardening areas still outside current scope include authentication, multi-tenancy, secret management, and deployment packaging.

## Development

```bash
make dev-up
make run
make dev-down
make dev-reset

make test
make test-storage
make lint
make fmt
```

Testing notes:

- Pure unit packages (`pkg/config`, `pkg/drift`, `pkg/otelreceiver`, `pkg/agwclient`, `pkg/diff`, `pkg/replay`) run without PostgreSQL.
- Storage tests (`pkg/storage`) require a reachable PostgreSQL instance at `CMDR_POSTGRES_URL`.
- End-to-end freeze and replay tests are opt-in and require companion services.
- The full migration demo harness is available via `cmdr demo migration run` or `make test-migration-demo-full-loop`.

## Project Layout

```text
cmd/cmdr/
  commands/              # CLI entrypoints for serve, drift, gate, and demo flows

pkg/
  api/                   # HTTP API server, handlers, generated OpenAPI surface
  agwclient/             # agentgateway HTTP client
  config/                # env-based config loading and validation
  diff/                  # structural and semantic comparison engine
  drift/                 # behavioral fingerprint extraction and scoring
  otelreceiver/          # OTLP receiver and span parsing
  replay/                # replay engine, agent loop, MCP tool executor
  storage/               # PostgreSQL models, queries, and migrations

internal/                # focused internal helpers for demo and replay flows
scripts/                 # setup scripts, demo harnesses, local utilities
test/                    # integration and e2e suites
docs/                    # setup, architecture, demo, and submission docs
ui/                      # generated API package and UI work in progress
```

## Documentation

Start with [docs/README.md](docs/README.md) for a curated index.

- [docs/QUICKSTART.md](docs/QUICKSTART.md) — local setup and first commands
- [docs/LOCAL_DEV_SETUP.md](docs/LOCAL_DEV_SETUP.md) — development environment details
- [docs/DEMO.md](docs/DEMO.md) — deterministic demo runbook
- [docs/MIGRATION_DEMO.md](docs/MIGRATION_DEMO.md) — full gateway-driven migration scenario
- [docs/GATE_REPLAY_ARCHITECTURE.md](docs/GATE_REPLAY_ARCHITECTURE.md) — replay and gate design
- [docs/AGENTGATEWAY_CAPTURE.md](docs/AGENTGATEWAY_CAPTURE.md) — agentgateway capture details
- [docs/FREEZE_AGENT_LOOP.md](docs/FREEZE_AGENT_LOOP.md) — frozen MCP loop proof
- [docs/DATABASE_LAYER.md](docs/DATABASE_LAYER.md) — schema overview
- [docs/E2E_DEMO_PLAN.md](docs/E2E_DEMO_PLAN.md) — end-to-end hackathon alignment
- [docs/OTLP_RECEIVER.md](docs/OTLP_RECEIVER.md) — OTLP receiver details
- [docs/SUBMISSION_NOTES.md](docs/SUBMISSION_NOTES.md) — hackathon framing and submission copy

## Contributing

Contribution guidance lives in [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Security reporting and scope notes live in [SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE).
