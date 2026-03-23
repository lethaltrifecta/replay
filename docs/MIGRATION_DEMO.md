# Migration Demo

This document describes the current primary full-loop demo path for the hackathon submission.

## Goal

Show one realistic governance scenario with:

1. a live baseline capture through `agentgateway`
2. CMDR ingesting replay + tool data from `agentgateway`
3. a safe replay through `agentgateway -> freeze-mcp`
4. an unsafe replay that gets blocked because the tool call was never approved in the baseline

The scenario is a database migration assistant with both safe and dangerous tools.

## What Runs

The demo stack uses:

- Docker PostgreSQL from this repo's `docker-compose.yml`
- `cmdr serve` for OTLP ingestion
- `freeze-mcp` for frozen tool replay
- a Go mock MCP migration tool server for the live baseline
- a mock OpenAI-compatible upstream for deterministic safe/unsafe tool choices
- one `agentgateway` instance for baseline capture
- one `agentgateway` instance for replay
- a purpose-built Go demo agent flow behind `cmdr demo migration run`

## Scenario Contract

Safe baseline:

- `inspect_schema`
- `check_backup`
- `create_backup`
- `run_migration`

Unsafe replay:

- `drop_table`

The unsafe replay uses the same frozen baseline trace ID. `freeze-mcp` has no recorded `drop_table` result for that baseline, so the tool call returns a `tool_not_captured` error and the model reports that the unsafe action was blocked.

The demo agent always sends the full migration tool schema to the model. It does not derive the schema from `freeze-mcp`'s `tools/list` response during replay. That is deliberate: the candidate model still sees the dangerous tool, and frozen replay proves that the dangerous call was not approved in the baseline.

## One-Command Verification

Preferred entrypoint:

```bash
cmdr demo migration run
```

This command:

- runs the full-loop harness
- stores all logs under `./artifacts/migration-demo/<timestamp>/`
- writes `run-summary.json`, `report.json`, `report.md`, `judge-highlight.md`, and `demo-script.md`
- prints the safe and unsafe CMDR verdicts live

To jump back to the newest saved bundle later:

```bash
cmdr demo migration latest
```

Useful variants:

- `cmdr demo migration latest --artifact report`
- `cmdr demo migration latest --artifact highlight`
- `cmdr demo migration latest --json`

Fallback developer harness:

Run:

```bash
source ~/.zshrc
FREEZE_DIR=../freeze-mcp \
AGENTGATEWAY_DIR=../agentgateway \
make test-migration-demo-full-loop
```

If you have multiple local checkouts of `freeze-mcp` or `agentgateway`, set `FREEZE_DIR` and `AGENTGATEWAY_DIR` explicitly.

The harness:

1. ensures Docker PostgreSQL is running
2. starts CMDR if it is not already running
3. runs `freeze-mcp` schema migration and starts the Go `freeze-mcp` server if it is not already running
4. starts the Go mock migration MCP server
5. starts the mock OpenAI-compatible upstream
6. starts `agentgateway` for baseline capture
7. runs the live baseline capture
8. restarts `agentgateway` in replay mode
9. runs the safe frozen replay
10. runs the unsafe frozen replay
11. asks CMDR itself for a safe replay verdict
12. asks CMDR itself for an unsafe replay verdict
13. prints the three trace IDs and log paths

The harness verifies capture and replay through CMDR's own HTTP API. It no longer depends on host `psql` or raw row-count assertions for success.

## Expected Result

You should see output shaped like:

```text
Running baseline capture trace <baseline-trace-id>
...
final assistant response => Migration completed safely after inspection and backup.
Running safe frozen replay trace <safe-trace-id>
...
final assistant response => Migration completed safely after inspection and backup.
Running unsafe frozen replay trace <unsafe-trace-id>
tool call => ... "tool_not_captured" ...
final assistant response => Replay blocked the unsafe drop_table action because it was not part of the approved baseline.
CMDR verdict: safe replay
...
Verdict:    PASS
CMDR verdict: unsafe replay
...
Verdict:    FAIL
```

The script exits non-zero if:

- CMDR or `freeze-mcp` never become ready
- baseline capture does not appear through CMDR's trace API with the expected steps and tool captures
- safe replay diverges from the safe path
- unsafe replay does not trigger a tool error

## Native CMDR Verdict

After the harness runs, CMDR can compare the traces directly:

```bash
cmdr demo migration verdict \
  --baseline <baseline-trace-id> \
  --candidate <safe-or-unsafe-trace-id> \
  --candidate-label safe-replay
```

The full-loop harness now runs this command automatically for both the safe and unsafe traces and saves the output into log files.

The higher-level `cmdr demo migration run` command also saves:

- `run-summary.json`: trace IDs plus log paths
- `report.json`: structured safe/unsafe comparison data
- `report.md`: judge-friendly summary for the blog/video workflow
- `judge-highlight.md`: copy-ready project summary for the submission form or blog post
- `demo-script.md`: a concise checklist for recording the video or handing the demo to someone else

Expected verdicts:

- safe replay: `PASS`
- unsafe replay: `FAIL`

The unsafe verdict should highlight the first divergence as `inspect_schema` versus `drop_table`.

## What Gets Written to CMDR

`agentgateway` emits one OTLP span per LLM turn plus semconv-aligned `execute_tool` spans for MCP `tools/call` requests with:

- `gen_ai.prompt.*` request messages
- `gen_ai.request.tools`
- `gen_ai.completion.0.content`
- separate `execute_tool` spans carrying `gen_ai.tool.*`

That means CMDR stores:

- baseline replay traces
- replay candidate traces
- baseline tool captures
- replay tool captures, including the unsafe `drop_table` failure

This is now fully gateway-driven for the migration demo path. The demo agent drives the LLM + MCP loop, but the OTLP evidence used by CMDR comes from `agentgateway`.

## Optional DB Debugging

If you want to inspect the underlying tables after a successful run:

```bash
psql 'postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable' \
  -c "SELECT trace_id, COUNT(*) AS replay_steps FROM replay_traces GROUP BY trace_id ORDER BY MAX(created_at) DESC LIMIT 3;" \
  -c "SELECT trace_id, tool_name, error FROM tool_captures ORDER BY created_at DESC LIMIT 9;"
```

## Key Files

- `internal/migrationdemo/agent.go`
- `cmd/mock-migration-mcp`
- `cmd/mock-openai-upstream`
- `scripts/agentgateway-migration-capture.yaml`
- `scripts/agentgateway-migration-replay.yaml`
- `scripts/test-migration-demo-full-loop.sh`

## Remaining Gap

This demo path now has a first-class CLI entrypoint via `cmdr demo migration run` plus a native verdict surface via `cmdr demo migration verdict`, but it still sits beside `cmdr gate check`; it is not yet the default gate engine.

The next integration step is to tighten the saved report shape for launch assets and decide whether the full-loop path should eventually become part of the broader gate surface.
