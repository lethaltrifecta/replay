# Freeze Agent Loop

This document describes the first full-loop replay proof for the hackathon demo.

## Goal

Prove that replay can execute a real tool-calling loop against `freeze-mcp`, instead of only replaying prompts.

The loop proven here is:

1. Agent sends messages + tools to an OpenAI-compatible LLM endpoint through `agentgateway`
2. Mock LLM returns a `tool_calls` response
3. Agent executes that tool call against `freeze-mcp`
4. `freeze-mcp` returns the frozen result from PostgreSQL
5. Agent appends the tool result and calls the LLM again
6. LLM returns a final answer

## What This Proves

- the replay runtime can preserve the MCP boundary
- frozen tool results can actually drive the next turn of the loop
- the baseline trace ID can scope tool lookup through `X-Freeze-Trace-ID`
- `agentgateway` can stay in the LLM path while `freeze-mcp` serves tool results
- CMDR can ingest the baseline trace that `freeze-mcp` later reads during replay
- the same loop works with MCP traffic proxied through `agentgateway`, not just AI traffic

## What This Does Not Prove Yet

- baseline captures coming from a live agent run instead of the minimal OTLP injector
- integration with `cmdr gate check`

Those are the next steps after this proof.

## Repo Artifacts

- `scripts/agentgateway-freeze-loop.yaml`
- `scripts/capture_freeze_baseline.py`
- `scripts/mock_toolcall_openai_upstream.py`
- `scripts/run_freeze_agent_loop.py`
- `scripts/test-agent-loop-freeze.sh`

## Running It

Prerequisites:

- PostgreSQL available at `CMDR_POSTGRES_URL`
- `cmdr serve` running at `http://127.0.0.1:4318` for the default baseline capture path
- `freeze-mcp` running at `http://127.0.0.1:9090`
- local `agentgateway` clone available at `../agentgateway`
- Python environment with `freeze_mcp`, `mcp`, `httpx`, `anyio`, and `psycopg`

If the target database is empty, the script applies CMDR's checked-in SQL migrations before seeding the tool capture.
The default PostgreSQL URL matches this repo's Docker Compose stack: `postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable`.
The default baseline source is `cmdr`; set `BASELINE_SOURCE=seed` if you want the older direct-SQL fallback.
The default MCP transport is `agentgateway`; set `MCP_TRANSPORT_MODE=direct` if you want the older direct-to-`freeze-mcp` fallback.

Run:

```bash
./scripts/test-agent-loop-freeze.sh
```

The script:

1. captures a baseline trace through CMDR OTLP ingestion, or seeds a fallback row when `BASELINE_SOURCE=seed`
2. starts a mock OpenAI-compatible tool-calling upstream
3. starts local `agentgateway`
4. runs a minimal agent loop against `freeze-mcp`
5. verifies the final assistant response includes the frozen result

## Recommended Next Step

Replace the seeded `tool_captures` row with a real CMDR-captured baseline, then decide whether MCP traffic should go:

- direct to `freeze-mcp` first, or
- through `agentgateway`'s MCP proxy for the final demo path
