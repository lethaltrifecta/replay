# CMDR — Agent Behavior Governance

**agentgateway gives you observability. CMDR gives you governance.**

CMDR (**C**omparative **M**odel **D**eterministic **R**eplay) is a governance system for LLM agents. It captures agent runs via OpenTelemetry, detects behavioral drift in production, and gates deployments by replaying scenarios with frozen tool responses.

## The Problem

When you change an agent's model, prompt, or policy, how do you know it still behaves correctly? Traditional eval frameworks score outputs after the fact. CMDR catches problems **before deployment** and **during production**:

- **Drift Detection** — Continuously compare live agent behavior against a known-good baseline. Alert when tool call patterns, risk levels, or token usage shift unexpectedly.
- **Deployment Gate** — Before rolling out a model change, replay captured scenarios with deterministic tool responses. Block the deploy if behavior regresses.

## Architecture

```
                    ┌─────────────────┐
                    │  agentgateway   │  ← LLM proxy, emits OTEL traces
                    └────────┬────────┘
                             │ OTLP (gRPC/HTTP)
                             ▼
┌──────────────────────────────────────────────┐
│                    CMDR                       │
│                                              │
│  ┌──────────┐  ┌───────────┐  ┌───────────┐ │
│  │  OTLP    │  │  Drift    │  │ Deployment│ │
│  │ Receiver │  │ Detection │  │   Gate    │ │
│  └────┬─────┘  └─────┬─────┘  └─────┬─────┘ │
│       │              │              │        │
│       ▼              ▼              ▼        │
│  ┌──────────────────────────────────────┐    │
│  │          PostgreSQL Storage          │    │
│  └──────────────────────────────────────┘    │
│                      ▲                       │
└──────────────────────┼───────────────────────┘
                       │ (read-only)
              ┌────────┴────────┐
              │   freeze-mcp   │  ← MCP server returning frozen tool responses
              └────────────────┘
```

**freeze-mcp** is a separate service (Python) that reads from CMDR's `tool_captures` table and serves frozen tool responses via the MCP protocol during replay. It runs as a sidecar alongside CMDR.

## Quick Start

```bash
# Set up local dev environment
make setup-dev

# Start services (PostgreSQL + Jaeger)
make dev-up

# Build and run CMDR
make run
```

CMDR starts listening for OTLP traces on ports 4317 (gRPC) and 4318 (HTTP). Point agentgateway's OTLP exporter at CMDR to start capturing agent runs.

For detailed setup, see [docs/QUICKSTART.md](docs/QUICKSTART.md).

## Usage

### Capture Agent Runs

CMDR automatically captures and parses OTEL traces from agentgateway. Every LLM call and tool call is stored with model, prompts, completions, tokens, tool args, and risk classification.

### Detect Drift

```bash
# Mark a known-good trace as baseline
cmdr drift baseline set <trace_id>

# Check if a new trace has drifted from baseline
cmdr drift check --trace-id <trace_id>

# Watch for drift continuously
cmdr drift watch
```

### Gate Deployments

```bash
# Replay a baseline trace with a different model
cmdr gate check \
  --baseline <trace_id> \
  --model gpt-4o-mini \
  --threshold 0.8

# Exit code 0 = pass, 1 = fail (CI/CD friendly)
```

## Development

```bash
make setup-dev       # One-command dev setup
make dev-up          # Start PostgreSQL + Jaeger
make dev-down        # Stop services
make dev-reset       # Wipe and restart database
make build           # Build binary to bin/cmdr
make run             # Build + run with .env
make test            # Unit tests with race detection
make test-storage    # Integration tests (needs PostgreSQL)
make lint            # golangci-lint
make fmt             # gofmt
```

## Project Structure

```
cmd/cmdr/                  # CLI entry point
  commands/
    root.go                # Cobra root, registers subcommands
    serve.go               # OTLP receiver + server startup
    drift.go               # Drift detection commands (TODO)
    gate.go                # Deployment gate commands (TODO)

pkg/
  config/                  # Environment-based config
  storage/                 # PostgreSQL storage layer (12 tables)
  otelreceiver/            # OTLP gRPC + HTTP receiver, span parser
  drift/                   # Drift detection engine (TODO)
  replay/                  # Prompt replay engine (TODO)
  agwclient/               # agentgateway HTTP client (TODO)
  diff/                    # Behavior diff engine (TODO)
  utils/logger/            # Zap logger wrapper
```

## Configuration

All config via environment variables with `CMDR_` prefix. See `.env.example`.

| Variable | Required | Description |
|---|---|---|
| `CMDR_POSTGRES_URL` | Yes | PostgreSQL connection string |
| `CMDR_AGENTGATEWAY_URL` | Yes | agentgateway endpoint |
| `CMDR_OTLP_GRPC_ENDPOINT` | No | OTLP gRPC listen address (default: `0.0.0.0:4317`) |
| `CMDR_OTLP_HTTP_ENDPOINT` | No | OTLP HTTP listen address (default: `0.0.0.0:4318`) |

## Ports

| Port | Service |
|------|---------|
| 4317 | OTLP gRPC receiver |
| 4318 | OTLP HTTP receiver |
| 8080 | HTTP API (planned) |
| 9090 | freeze-mcp (separate service) |

## Current Status

| Component | Status |
|---|---|
| Config, logging, CLI scaffolding | Done |
| PostgreSQL storage (12 tables, migrations, full CRUD) | Done |
| OTLP receiver (gRPC + HTTP, parser, tool extraction) | Done |
| freeze-mcp (MCP server, frozen tool responses) | Done (separate repo) |
| Drift detection | Not started |
| Deployment gate (prompt replay) | Not started |
| agentgateway client | Not started |
| Behavior diff engine | Not started |

## Hackathon

Built for [MCP_HACK//26](https://aihackathon.dev) in the **Secure & Govern MCP** category, using [agentgateway](https://github.com/agentgateway/agentgateway) as the core integration point.

## Documentation

- [Quick Start Guide](docs/QUICKSTART.md)
- [Database Layer](docs/DATABASE_LAYER.md)
- [OTLP Receiver](docs/OTLP_RECEIVER.md)
- [Testing OTLP](docs/TESTING_OTLP.md)
- [Architecture Spec](notes/DRAFT_PLAN2.md)

## License

MIT License - See LICENSE file for details
