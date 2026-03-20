# End-to-End Demo Architecture

## What the demo proves

CMDR provides a complete governance loop for MCP-enabled agents:

1. A real agent run goes through `agentgateway`
2. CMDR captures the run from OTLP and stores replay + tool data
3. CMDR replays the same scenario with frozen MCP tool responses via `freeze-mcp`
4. CMDR blocks an unsafe deployment and explains the divergence

## Scoring alignment

Built for MCP_HACK//26 — "Secure & Govern MCP":

| Bucket | Points | What CMDR demonstrates |
|--------|--------|------------------------|
| Open Source Projects | 40 | `agentgateway` in the primary capture + replay path |
| Usefulness | 20 | Practical governance: drift detection + deployment gates |
| Product Readiness | 20 | One-command setup, deterministic demo, CI/CD exit codes |
| Launch | 20 | Demo video, blog post, submission assets |

Reference: <https://aihackathon.dev/submissions/>

## Demo scenario: Database migration governance

Safe baseline:
- `inspect_schema` → `check_backup` → `create_backup` → `run_migration`

Unsafe candidate:
- Skips backup, attempts `drop_table`
- Blocked by frozen replay because the dangerous call was never in the approved baseline

Why this scenario works:
- Clearly fits "secure, monitor, manage AI agent deployments"
- Makes risk classes intuitive to judges
- Demonstrates why frozen tools matter

## Architecture

```
                    +---------------------------+
                    | Sample Agent Driver       |
                    | - OpenAI-compatible API   |
                    | - MCP tool use            |
                    +-------------+-------------+
                                  |
                                  v
                    +---------------------------+
                    | agentgateway              |
                    | - provider routing        |
                    | - MCP/tool routing        |
                    | - OTLP export             |
                    +------+------+-------------+
                           |      |
                           v      v
                +----------------+  +-------------------+
                | LLM provider   |  | MCP tools         |
                |                |  | (live or freeze)   |
                +----------------+  +-------------------+

                           OTLP
                            |
                            v
                    +---------------------------+
                    | CMDR                      |
                    | - OTLP receiver           |
                    | - replay trace store      |
                    | - tool capture store      |
                    | - drift + gate diff       |
                    | - divergence report       |
                    +-------------+-------------+
                                  |
                                  v
                         +----------------+
                         | PostgreSQL     |
                         +----------------+
```

## Demo entry points

### Deterministic demo (no external LLM)

```bash
make dev-up
make demo
```

This seeds pre-built traces and runs drift + gate checks with deterministic data.

### Migration demo (full stack)

```bash
cmdr demo migration run
cmdr demo migration latest
cmdr demo migration verdict --baseline <trace> --candidate <trace>
```

Writes a self-contained artifact bundle with logs, results, report, and demo script.

### Manual commands

```bash
cmdr demo seed
cmdr drift check demo-drifted-002 --baseline demo-baseline-001
cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger   # exits 1
cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet  # exits 0
```

## What's implemented

- Real `agentgateway` capture into CMDR
- Full-loop replay proof with `freeze-mcp`
- CMDR-backed baseline capture for frozen replay
- MCP replay routed through `agentgateway`
- Database migration demo agent with safe and unsafe replay paths
- Runnable verification harness (`make test-migration-demo-full-loop`)
- Native CMDR verdict command for migration demo traces
- `cmdr demo migration run` with saved report artifacts
- `cmdr demo migration latest` for finding the newest artifact bundle

## Related docs

- [DEMO.md](DEMO.md) — Presenter script and expected outputs
- [MIGRATION_DEMO.md](MIGRATION_DEMO.md) — Full migration demo runbook
- [GATE_REPLAY_ARCHITECTURE.md](GATE_REPLAY_ARCHITECTURE.md) — Gate replay design
- [SUBMISSION_NOTES.md](SUBMISSION_NOTES.md) — Hackathon submission summary
