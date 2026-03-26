# CMDR: Agent Behavior Governance

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Next.js](https://img.shields.io/badge/Next.js-16-000000?logo=next.js&logoColor=white)](https://nextjs.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![MCP_HACK//26](https://img.shields.io/badge/MCP__HACK%2F%2F26-Secure_%26_Govern-blueviolet)](https://aihackathon.dev/)

**Same model. Same tools. Different instructions. CMDR caught it.**

CMDR is a governance system for MCP-enabled AI agents. It captures agent runs via OpenTelemetry, detects behavioral drift against approved baselines, and gates deployments by replaying scenarios with frozen tool responses.

> Built for [MCP_HACK//26](https://aihackathon.dev/) in the **Secure & Govern MCP** track using [agentgateway](https://github.com/solo-io/agentgateway) and [freeze-mcp](https://github.com/lethaltrifecta/freeze-mcp).

---

## The Problem

Teams change prompts, role files, and tool configurations far more often than they change foundation models. These changes are invisible to model evaluation tools — the agent uses the same model, same API keys, but its behavior changes silently. CMDR governs **any change that alters agent behavior**: model swaps, prompt changes, instruction files, and tool configuration.

## See It In Action

Someone updates `role.md` from conservative to aggressive instructions. Same model, same tools — only the instructions changed. CMDR catches the behavioral divergence:

![Shadow Replay showing step-by-step divergence at step 3](docs/screenshots/shadow-replay.png)

<details>
<summary><strong>More screenshots</strong></summary>

**Divergence Engine** — Verdict-first review with "What Changed" context:
![Divergence detail](docs/screenshots/divergence-detail.png)

**The Gauntlet** — Answers all four operator questions:
![Gauntlet report](docs/screenshots/gauntlet-report.png)

</details>

## Key Features

| Feature | What it does |
|---------|-------------|
| **Drift Detection** | Compares live agent traces against approved baselines using behavioral fingerprinting |
| **Deployment Gates** | Replays baseline scenarios with different configs; blocks deploys that diverge |
| **Deterministic Replay** | [freeze-mcp](https://github.com/lethaltrifecta/freeze-mcp) serves frozen tool responses so the only variable is the agent's behavior |
| **Change Context Tracking** | Tags runs with what changed (instruction file, model, config) and surfaces it in the UI |
| **Mission Control UI** | Four review surfaces: Launchpad, Divergence Engine, Shadow Replay, The Gauntlet |
| **CI-Friendly** | `exit 0` for pass, `exit 1` for fail — drop into any pipeline |

## Quick Start

```bash
# Start services and run the demo
make setup-dev
make dev-up
make build
./bin/cmdr demo seed
./bin/cmdr serve &

# Start the UI (requires REPLAY_API_ORIGIN for API proxy)
cd ui && REPLAY_API_ORIGIN=http://localhost:8080 npx next dev
# Open http://localhost:3000
```

Or run the CLI-only demo:

```bash
make demo    # seeds data + runs drift check + gate checks
```

## Architecture

```text
agentgateway (OTLP emitter)
        |
        | OTLP (gRPC / HTTP)
        v
+----------------------------------+
|  CMDR                            |
|  - OTLP receiver & span parser  |
|  - Behavioral fingerprinting    |
|  - Replay engine                |
|  - HTTP API + Mission Control UI |
+----------------------------------+
        |                    |
        v                    v
  PostgreSQL          agentgateway
  (12 tables)       (replay via LLM proxy)
                         |
                         v
                    freeze-mcp
                 (frozen tool responses)
```

## Core Workflows

### Drift Detection

```bash
cmdr drift check <baseline-trace-id> <candidate-trace-id>
```

Compares tool call patterns, risk distributions, token usage, and response content. Returns a composite drift score (0.0-1.0) with a `pass`/`warn`/`fail` verdict.

### Deployment Gates

```bash
cmdr gate check --baseline <trace-id> --model gpt-4o --threshold 0.8
```

Replays baseline prompts through agentgateway with a candidate config, compares the result, and exits `0` (pass) or `1` (fail).

### Governance UI

Four review surfaces at `http://localhost:3000`:

- **Launchpad** (`/launchpad`) — Browse traces, approve baselines, launch comparisons
- **Divergence Engine** (`/divergence`) — Verdict-first triage queue (FAIL / WARN / PASS / PENDING)
- **Shadow Replay** (`/shadow-replay`) — Raw side-by-side step evidence without verdicts
- **The Gauntlet** (`/gauntlet`) — Canonical trial reports answering four operator questions

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.26, PostgreSQL 15, OpenTelemetry |
| CLI | spf13/cobra |
| API | OpenAPI 3.0, oapi-codegen |
| Frontend | Next.js 16, React 19, TypeScript, Radix UI, Tailwind CSS |
| Telemetry | OTLP gRPC + HTTP receivers |
| Container | Multi-stage Docker build with distroless runtime |
| CI | GitHub Actions (test, lint, build) |

## Development

```bash
make dev-up       # Start PostgreSQL + Jaeger
make build        # Build binary
make test         # Unit tests (no DB required)
make test-storage # Integration tests (requires PostgreSQL)
make lint         # golangci-lint
```

## Project Layout

```text
cmd/cmdr/commands/    CLI: serve, drift, gate, demo
pkg/api/              HTTP API server + OpenAPI handlers
pkg/drift/            Behavioral fingerprinting + scoring
pkg/replay/           Prompt replay + agent loop engine
pkg/diff/             Structural + semantic comparison
pkg/otelreceiver/     OTLP gRPC/HTTP receiver + parser
pkg/storage/          PostgreSQL models, queries, migrations
ui/                   Mission Control UI (Next.js 16 + React 19)
docs/                 Architecture, demo runbooks, blog draft
```

## Documentation

| Doc | Purpose |
|-----|---------|
| [QUICKSTART](docs/QUICKSTART.md) | Local setup and first commands |
| [DEMO](docs/DEMO.md) | 2-3 minute deterministic demo |
| [MIGRATION_DEMO](docs/MIGRATION_DEMO.md) | Full gateway-driven scenario |
| [BLOG_DRAFT](docs/BLOG_DRAFT.md) | "Same Model, Different Instructions, CMDR Caught It" |
| [Architecture index](docs/README.md) | Full documentation guide |
| [SUBMISSION_NOTES](docs/SUBMISSION_NOTES.md) | Hackathon framing and scoring alignment |

## Open Source Integrations

- **[agentgateway](https://github.com/solo-io/agentgateway)** — LLM + MCP proxy that emits OTEL traces. CMDR ingests these for capture and uses agentgateway as the replay client.
- **[freeze-mcp](https://github.com/lethaltrifecta/freeze-mcp)** — MCP server that serves frozen tool responses from CMDR's `tool_captures` table. Enables deterministic replay.

## License

MIT — see [LICENSE](LICENSE).
