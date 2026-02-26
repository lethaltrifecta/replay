# Quick Start Guide - Local Development

This guide will help you set up and run CMDR for local development in under 5 minutes.

## Prerequisites

- **Go 1.24+** - [Install Go](https://golang.org/doc/install)
- **Docker & Docker Compose** - [Install Docker](https://docs.docker.com/get-docker/)
  - Supports both Docker Compose V2 (`docker compose`) and V1 (`docker-compose`)
- **Make** - Usually pre-installed on macOS/Linux

## One-Command Setup

```bash
make setup-dev
```

This will:
1. Create `.env` from `.env.example`
2. Start PostgreSQL and Jaeger with docker-compose
3. Download Go dependencies
4. Build the CMDR binary

## Manual Setup (Step-by-Step)

### 1. Clone and Navigate

```bash
cd /Users/jaden.lee/code/lethal/replay
```

### 2. Create Environment Config

```bash
cp .env.example .env
```

**Important**: Edit `.env` and set `CMDR_AGENTGATEWAY_URL` to your actual agentgateway instance:

```bash
# .env
CMDR_AGENTGATEWAY_URL=http://your-agentgateway-host:port
```

### 3. Start Development Services

```bash
make dev-up
```

This starts:
- **PostgreSQL** on `localhost:5432`
- **Jaeger** UI at `http://localhost:16686`

### 4. Download Dependencies

```bash
make deps
```

### 5. Build CMDR

```bash
make build
```

## Running CMDR

### Start the Service

```bash
make run
```

The service will start on the ports defined in `.env`:
- **HTTP API**: `http://localhost:8080`
- **OTLP gRPC**: `localhost:4317`
- **OTLP HTTP**: `localhost:4318`
- **Freeze-Tools MCP**: `localhost:9090`

### Run Tests

```bash
# All tests
make test

# Storage tests with real PostgreSQL
make test-storage

# Generate coverage report
make coverage
```

## Development Workflow

### Day-to-Day Commands

```bash
# Start services (PostgreSQL, Jaeger)
make dev-up

# Run CMDR
make run

# In another terminal: run tests
make test

# Format code
make fmt

# Run linter
make lint

# Stop services when done
make dev-down
```

### Reset Database

If you need to start fresh:

```bash
make dev-reset
```

This will:
1. Stop all services
2. Delete all data volumes
3. Start PostgreSQL fresh

## Verify Setup

### 1. Check Services are Running

```bash
# Docker Compose V2 (newer)
docker compose ps

# OR Docker Compose V1 (older)
docker-compose ps
```

You should see:
- `cmdr-postgres` - healthy
- `cmdr-jaeger` - running

### 2. Test Database Connection

```bash
# Run storage tests
make test-storage
```

If successful, you'll see all tests passing.

### 3. Test CMDR Binary

```bash
./bin/cmdr --version
```

## Configuration Overview

All configuration is in `.env`. Key settings:

```bash
# Service
CMDR_API_PORT=8080
CMDR_LOG_LEVEL=debug

# Database (for docker-compose)
CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable

# Agentgateway (CHANGE THIS!)
CMDR_AGENTGATEWAY_URL=http://localhost:8080

# OTLP Receiver
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318

# Freeze-Tools
CMDR_FREEZETOOLS_PORT=9090
```

## Available Make Commands

Run `make help` to see all available commands:

```
Development:
  setup-dev            Set up local development environment
  dev-up               Start development services
  dev-down             Stop development services
  dev-reset            Reset development database

Build:
  build                Build the binary
  run                  Build and run the service

Testing:
  test                 Run unit tests
  test-storage         Run storage tests
  test-integration     Run integration tests
  coverage             Generate coverage report

Code Quality:
  lint                 Run linter
  fmt                  Format code

Maintenance:
  clean                Clean build artifacts
  deps                 Download dependencies

Docker:
  docker-build         Build Docker image
  docker-run           Run Docker container
```

## Troubleshooting

### PostgreSQL Connection Fails

```bash
# Check if PostgreSQL is running (supports both docker compose and docker-compose)
docker compose ps postgres || docker-compose ps postgres

# Check logs
docker compose logs postgres || docker-compose logs postgres

# Restart PostgreSQL
make dev-reset
```

### Docker Compose Not Found

If you see "docker-compose not found":

```bash
# Check if you have Docker Compose V2
docker compose version

# If that works, the scripts will auto-detect and use it
# If not, install Docker Compose:
# https://docs.docker.com/compose/install/
```

### Port Already in Use

If port 8080, 4317, 4318, or 5432 is already in use:

1. Edit `.env` and change the ports
2. Edit `docker-compose.yml` for database ports
3. Restart services: `make dev-down && make dev-up`

### Go Dependencies Issues

```bash
# Clean and re-download
go clean -modcache
make deps
```

### Build Fails

```bash
# Clean and rebuild
make clean
make build
```

## Next Steps

Once your local environment is running:

1. **Explore the codebase**:
   - `cmd/cmdr/` - CLI entry point
   - `pkg/storage/` - Database layer (complete)
   - `pkg/config/` - Configuration
   - `DRAFT_PLAN2.md` - Architecture documentation

2. **Run the CLI**:
   ```bash
   ./bin/cmdr --help
   ./bin/cmdr serve --help
   ./bin/cmdr experiment --help
   ```

3. **Start developing**:
   - Next: Implement OTLP receiver (`pkg/otelreceiver/`)
   - Then: Implement Freeze-Tools (`pkg/freezetools/`)
   - See: `IMPLEMENTATION_STATUS.md` for roadmap

## Environment Variables Reference

| Variable | Default | Description |
|----------|---------|-------------|
| `CMDR_API_PORT` | 8080 | HTTP API server port |
| `CMDR_LOG_LEVEL` | debug | Log level (debug/info/warn/error) |
| `CMDR_POSTGRES_URL` | (see .env.example) | PostgreSQL connection string |
| `CMDR_AGENTGATEWAY_URL` | http://localhost:8080 | Agentgateway HTTP URL |
| `CMDR_OTLP_GRPC_ENDPOINT` | 0.0.0.0:4317 | OTLP gRPC receiver endpoint |
| `CMDR_OTLP_HTTP_ENDPOINT` | 0.0.0.0:4318 | OTLP HTTP receiver endpoint |
| `CMDR_FREEZETOOLS_PORT` | 9090 | Freeze-Tools MCP server port |

For the complete list, see `.env.example`.

## Getting Help

- **Documentation**: Check `DRAFT_PLAN2.md` for architecture
- **Database**: See `docs/DATABASE_LAYER.md`
- **Issues**: Create an issue in the repository
- **Code Examples**: Check test files (`*_test.go`)

---

**Ready to start?** Run:

```bash
make setup-dev && make run
```

🎉 You're all set!
