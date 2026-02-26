# CMDR - Replay-Backed Behavior Analysis Lab

CMDR is a deterministic replay and evaluation system for comparing LLM agent behavior across models, prompts, policies, and tool configurations.

## Features

- **Deterministic Replay**: Freeze-Tools ensures identical tool execution results across replays
- **4D Analysis**: Behavior, safety, quality, and efficiency analysis
- **Comprehensive Evaluation**: Rule-based, LLM-judge, rubric, human-loop, and ground-truth evaluators
- **Winner Determination**: Automated ranking and recommendation based on configurable criteria

## Quick Start

### One-Command Setup

```bash
make setup-dev
```

This sets up your entire local development environment in one command.

For detailed instructions, see **[Quick Start Guide](docs/QUICKSTART.md)**.

### Manual Setup

```bash
# 1. Create environment config
cp .env.example .env

# 2. Edit .env and set CMDR_AGENTGATEWAY_URL

# 3. Start development services
make dev-up

# 4. Download dependencies
make deps

# 5. Build and run
make run
```

## Documentation

- **[Quick Start Guide](docs/QUICKSTART.md)** - Get started in 5 minutes
- **[DRAFT_PLAN2.md](DRAFT_PLAN2.md)** - Complete architecture specification
- **[Database Layer](docs/DATABASE_LAYER.md)** - Database implementation details
- **[Implementation Status](IMPLEMENTATION_STATUS.md)** - Current progress and roadmap

## Architecture

CMDR consists of several key components:

1. **OTLP Receiver** - Ingests OTEL traces from agentgateway
2. **Freeze-Tools** - Deterministic MCP server for replay
3. **Replay Engine** - Orchestrates experiment execution
4. **Analyzer** - 4D comparative analysis (behavior, safety, quality, efficiency)
5. **Evaluator** - Multi-strategy evaluation framework
6. **API/CLI** - REST API and command-line interface

See [DRAFT_PLAN2.md](DRAFT_PLAN2.md) for complete architecture documentation.

## Usage

### Create an Experiment

```bash
# Using CLI
cmdr experiment run \
  --baseline-trace abc123 \
  --variants variants.yaml

# Using API
curl -X POST http://localhost:8080/api/v1/experiments \
  -H "Content-Type: application/json" \
  -d '{
    "baseline_trace_id": "abc123",
    "variants": [
      {"name": "claude", "model": "claude-3-5-sonnet-20241022"},
      {"name": "gpt4", "model": "gpt-4"}
    ]
  }'
```

### Get Results

```bash
# Get experiment status
cmdr experiment status exp-789

# Get evaluation summary with winner
cmdr eval summary exp-789

# Get detailed report
cmdr experiment report exp-789 --output report.md
```

## Development

### Available Commands

```bash
make help                 # Show all available commands

# Development
make setup-dev           # Set up local development environment
make dev-up              # Start PostgreSQL and Jaeger
make dev-down            # Stop services
make dev-reset           # Reset database

# Build & Run
make build               # Build binary
make run                 # Build and run service

# Testing
make test                # Run unit tests
make test-storage        # Run storage tests with real PostgreSQL
make coverage            # Generate coverage report

# Code Quality
make lint                # Run linter
make fmt                 # Format code
```

### Running Tests

```bash
# All tests
make test

# Storage tests (requires PostgreSQL)
make dev-up
make test-storage

# Generate coverage report
make coverage
open coverage.html
```

### Project Structure

```
.
├── cmd/
│   └── cmdr/              # CLI entry point
├── pkg/
│   ├── config/            # Configuration ✅
│   ├── storage/           # Database layer ✅
│   ├── otelreceiver/      # OTLP trace ingestion (TODO)
│   ├── freezetools/       # Deterministic replay (TODO)
│   ├── replayengine/      # Experiment orchestration (TODO)
│   ├── analyzer/          # 4D analysis (TODO)
│   ├── evaluator/         # Evaluation framework (TODO)
│   └── api/               # REST API (TODO)
├── test/
│   ├── integration/       # Integration tests
│   └── e2e/               # End-to-end tests
├── docs/                  # Documentation
├── scripts/               # Utility scripts
└── deployments/           # Docker and K8s configs
```

## Configuration

Configuration is managed via environment variables. Copy `.env.example` to `.env` and adjust:

```bash
# Key settings
CMDR_API_PORT=8080
CMDR_POSTGRES_URL=postgres://user:pass@localhost:5432/cmdr
CMDR_AGENTGATEWAY_URL=http://your-agentgateway:port
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318
```

See `.env.example` for all configuration options.

## Current Status

✅ **Phase 1 Foundation - COMPLETE**
- [x] Project structure and build system
- [x] Configuration management
- [x] Structured logging
- [x] CLI framework (Cobra)
- [x] Database layer (PostgreSQL with 12 tables)
- [x] Migration system
- [x] Comprehensive test suite

🚧 **Phase 1 Core Components - IN PROGRESS**
- [ ] OTLP receiver
- [ ] Freeze-Tools MCP server
- [ ] Component integration

See [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) for detailed progress.

## Contributing

Contributions welcome! Please read CONTRIBUTING.md first.

## License

MIT License - See LICENSE file for details

## Links

- **Repository**: https://github.com/lethaltrifecta/replay
- **Documentation**: [docs/](docs/)
- **Issues**: https://github.com/lethaltrifecta/replay/issues
