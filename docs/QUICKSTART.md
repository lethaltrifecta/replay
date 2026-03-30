# Quick Start (Local Development)

This guide gets CMDR running locally for trace ingestion + drift checks.

## Prerequisites

- Go 1.26+
- Docker + Docker Compose
- Make

## 1) One-command setup

```bash
make setup-dev
```

What this does:
- creates `.env` from `.env.example` (if missing)
- starts PostgreSQL + Jaeger
- downloads Go dependencies
- builds `bin/cmdr`

## 2) Configure environment

At minimum, confirm `.env` has:

```bash
CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable
CMDR_AGENTGATEWAY_URL=http://localhost:8080
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318
```

Note:
- `CMDR_AGENTGATEWAY_URL` is required — it's the endpoint used by `cmdr gate check` to replay prompts with variant models.

## 3) Run CMDR

```bash
make run
```

You should see logs showing:
- database connected + migrated
- OTLP gRPC receiver started
- OTLP HTTP receiver started

## 4) Validate health + ingestion endpoint

```bash
curl -i http://localhost:4318/health
```

Expected: `HTTP/1.1 200 OK`.

## 5) Send a sample OTLP trace

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "quickstart"}}]},
      "scopeSpans": [{
        "spans": [{
          "traceId": "01020304050607080910111213141516",
          "spanId": "0102030405060708",
          "name": "llm.completion",
          "startTimeUnixNano": "1234567890000000000",
          "endTimeUnixNano": "1234567891000000000",
          "attributes": [
            {"key": "gen_ai.request.model", "value": {"stringValue": "claude-3-5-sonnet-20241022"}},
            {"key": "gen_ai.system", "value": {"stringValue": "anthropic"}},
            {"key": "gen_ai.prompt.0.role", "value": {"stringValue": "user"}},
            {"key": "gen_ai.prompt.0.content", "value": {"stringValue": "hello"}},
            {"key": "gen_ai.completion.0.content", "value": {"stringValue": "hi"}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": "5"}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": "3"}}
          ]
        }]
      }]
    }]
  }'
```

Expected response: `{}`

## 6) Verify stored data

```bash
psql "$CMDR_POSTGRES_URL" -c "SELECT trace_id, model, provider, total_tokens FROM replay_traces ORDER BY created_at DESC LIMIT 5;"
psql "$CMDR_POSTGRES_URL" -c "SELECT trace_id, tool_name, risk_class FROM tool_captures ORDER BY created_at DESC LIMIT 5;"
```

## 7) Run drift commands

```bash
cmdr drift check <baseline-trace-id> <candidate-trace-id>
cmdr drift list --limit 10
```

## Useful commands

```bash
make dev-up
make dev-down
make dev-reset
make test
make test-storage
make lint
make fmt
```

## What's available

- `cmdr serve` — OTLP receiver + HTTP API server
- `cmdr drift check <baseline> <candidate>` — compare traces against an explicit baseline
- `cmdr drift list` — inspect stored drift results
- `cmdr drift watch <baseline>` — continuous drift monitoring
- `cmdr gate check` — replay baseline with variant model, produce pass/fail verdict
- `cmdr demo seed` / `cmdr demo gate` — deterministic demo (no external LLM needed)
- `cmdr demo migration run` — full capture → replay → verdict demo (requires agentgateway + freeze-mcp)

## UI

The governance review UI is at `ui/`. To run locally:

```bash
cd ui
pnpm install
REPLAY_API_ORIGIN=http://localhost:8080 pnpm dev
# Open http://localhost:3000
```

Or via Docker Compose (full stack including agentgateway + freeze-mcp):

```bash
docker compose -f docker-compose.dev.yml up -d
# Open http://localhost:3000
```
