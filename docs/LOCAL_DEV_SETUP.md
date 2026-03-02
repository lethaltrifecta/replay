# Local Dev Setup

This document is a practical checklist for day-to-day CMDR development.
For first-run instructions, start with [QUICKSTART.md](QUICKSTART.md).

## Local stack

`make dev-up` starts:
- PostgreSQL (`localhost:5432`)
- Jaeger UI (`http://localhost:16686`)

`make run` starts:
- CMDR OTLP gRPC receiver (`0.0.0.0:4317`)
- CMDR OTLP HTTP receiver + health (`0.0.0.0:4318`, `/v1/traces`, `/health`)

## Boot sequence

```bash
make setup-dev
make run
```

Then validate:

```bash
curl -i http://localhost:4318/health
```

## Environment variables

Primary:

```bash
CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable
CMDR_AGENTGATEWAY_URL=http://localhost:8080
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318
```

Notes:
- `CMDR_AGENTGATEWAY_URL` is currently required by config validation.
- Copy `.env.example` to `.env` to customize settings.

## Common workflows

### Run tests

```bash
make test
make test-storage
```

- `make test` runs package tests under `./pkg/...`.
- `make test-storage` expects local PostgreSQL to be available.

### Rebuild from clean state

```bash
make dev-reset
make build
make run
```

### Check recent traces

```bash
psql "$CMDR_POSTGRES_URL" -c "SELECT trace_id, model, provider, created_at FROM replay_traces ORDER BY created_at DESC LIMIT 10;"
```

### Run drift checks

```bash
cmdr drift baseline set <trace-id>
cmdr drift check <candidate-trace-id>
```

## Troubleshooting

### `CMDR_AGENTGATEWAY_URL is required`
Set `CMDR_AGENTGATEWAY_URL` in `.env` before running `make run`.

### PostgreSQL connection failures
- run `make dev-up`
- verify container health: `docker compose ps`
- verify DSN in `.env`

### OTLP endpoint not reachable
- confirm `cmdr serve` is running
- check port usage: `lsof -i :4318`
- health check: `curl -i http://localhost:4318/health`

## Status reminder

As of this phase:
- Drift baseline/check is implemented.
- Experiment/eval/ground-truth commands are still scaffolds.
- Replay gate flow is planned, not yet implemented.
