# Gate Replay Architecture

This document describes how `cmdr gate check` works today and the target architecture that integrates freeze-mcp for deterministic agent-loop replay.

## The Problem

A deployment gate needs to answer: **"If I swap model A for model B, does the agent still behave safely?"**

This requires replaying a captured agent run with a different model and comparing the resulting behavior. The replay must be deterministic — the tool environment should be frozen so that only the model's behavior varies, not the external world.

## Prompt-Only Replay

The prompt-only replay engine (`pkg/replay/engine.go`) sends the **baseline's prompt** at each step to the variant model and records the response. This is the default mode when freeze-mcp is not available.

```
Baseline trace (5 steps):
  step 0: prompt=[system, user]                    → completion + tool_call=read_file
  step 1: prompt=[..., asst, tool_result_from_s0]  → completion + tool_call=edit_file
  step 2: prompt=[..., tool_result_from_s1]         → completion + tool_call=run_tests
  ...

Prompt-only replay:
  step 0: send baseline step 0 prompt to variant model → record variant response
  step 1: send baseline step 1 prompt to variant model → record variant response
  step 2: send baseline step 2 prompt to variant model → record variant response
  ...
```

### Tradeoff

This answers: *"Given identical context at each step, does the variant model make similar decisions?"*

It does **not** answer: *"If the variant model actually ran its tools, would the agent complete the task safely?"* — because divergence at step 0 doesn't propagate to step 1 (each step replays the baseline's context).

This mode is fast and requires no external infrastructure beyond agentgateway.

## Agent-Loop Replay with freeze-mcp

The agent-loop replay mode replaces prompt-only replay with a real execution loop backed by freeze-mcp for deterministic tool responses. This is the primary mode when the full stack is available.

### How It Works

```
                    ┌──────────────────────────────────────────────────────┐
                    │                  CMDR Gate Check                     │
                    │                                                      │
                    │  1. Load baseline trace steps + tool captures        │
                    │  2. Start agent loop with variant model              │
                    │  3. Diff baseline vs variant trajectory              │
                    │  4. Produce verdict                                  │
                    └──────────┬───────────────────────────────────────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
              ▼                ▼                ▼
     agentgateway         agentgateway      freeze-mcp
     (LLM proxy)         (MCP proxy)       (frozen tools)
         :3001              :3003             :9090
              │                │                │
              ▼                └───────────────►│
         upstream LLM                    PostgreSQL
         (or mock)                    (tool_captures)
```

**Step-by-step replay loop:**

1. Construct the initial prompt from the baseline's step 0 (system message + user message).
2. Send to the variant model via agentgateway (LLM port) with `X-Freeze-Trace-ID` header.
3. Variant model responds — may include `tool_calls`.
4. For each tool call: execute via agentgateway (MCP port) → freeze-mcp.
   - **Match**: freeze-mcp finds a baseline capture with matching `(tool_name, args_hash, trace_id)` → returns the frozen result.
   - **No match**: freeze-mcp returns an error → this is a **divergence**. The tool was never part of the approved baseline.
5. Append assistant message + tool results to the conversation.
6. Send updated conversation to variant model for the next turn.
7. Repeat until the model produces a final response (no tool calls) or a divergence is detected.
8. Store the variant's full trace (steps + tool captures) in the database.
9. Diff the baseline and variant trajectories. Produce verdict.

### What This Achieves

- **Real agent trajectory.** The variant model sees its own tool results (from freeze-mcp), not the baseline's. Decisions cascade naturally — a different tool call at step 0 changes what the model sees at step 1.
- **Deterministic tool environment.** freeze-mcp serves the exact same tool results the baseline saw. The only variable is the model's behavior.
- **Safety boundary.** Any tool call not in the baseline captures is rejected by freeze-mcp. An unsafe model that tries `drop_table` when the baseline only did `inspect_schema` is blocked immediately.
- **Divergence detection at the tool boundary.** You don't just see that the model *said* something different — you see that it *tried to do* something different, and what happened when it did.

### Comparison

| | Prompt-Only Replay | Agent-Loop Replay |
|---|---|---|
| Tool calls executed | No | Yes, via freeze-mcp |
| Variant sees own tool results | No (sees baseline's) | Yes (frozen, deterministic) |
| Cascading divergence | Not detected | Detected naturally |
| Uncaptured tool blocked | No | Yes (freeze-mcp rejects) |
| Tests | "Same decisions given same context?" | "Same behavior when actually running?" |
| Requires freeze-mcp | No | Yes |
| Requires agentgateway MCP proxy | No | Yes |

## Key Components

### freeze-mcp (existing, separate repo)

Python MCP server serving frozen tool responses from `tool_captures` table.

- Lookup: `(tool_name, args_hash, trace_id)` with optional `(span_id, step_index)` targeting
- Scoped per-session via `X-Freeze-Trace-ID` header
- Returns `tool_not_captured` error for uncaptured tool calls
- Already proven in the migration demo (`test-migration-demo-full-loop.sh`)

### agentgateway (existing, separate repo)

LLM + MCP proxy. Two listeners:

- **LLM port**: Proxies chat completion requests to the upstream model. Forwards custom headers (including `X-Freeze-Trace-ID`).
- **MCP port**: Proxies MCP tool calls to freeze-mcp. CORS configured to allow `X-Freeze-Trace-ID`.

### Replay Engine

`pkg/replay/engine.go` implements prompt-only replay and is the foundation for agent-loop mode. The agent-loop extension:

1. Maintains its own conversation history (not the baseline's).
2. Executes tool calls via an MCP client connected through agentgateway to freeze-mcp.
3. Detects divergence when freeze-mcp rejects a tool call.
4. Emits OTLP spans for each turn (LLM call + tool executions) so the variant trace is fully captured.
5. Stores variant `replay_traces` and `tool_captures` for post-hoc diffing.

### Request Headers

The `X-Freeze-Trace-ID` header flows through:

```
gate check request
  → VariantConfig.RequestHeaders
    → CompletionRequest.Headers (LLM calls)
    → MCP session headers (tool calls)
      → agentgateway → freeze-mcp
```

The API (`POST /api/v1/gate/check`) accepts `request_headers` in the request body, which are forwarded to both the LLM and MCP clients.

## Data Flow

### Capture (already implemented)

```
Live agent run
  → agentgateway (with OTLP tracing)
    → CMDR OTLP receiver
      → otel_traces + replay_traces + tool_captures (PostgreSQL)
```

### Replay (target)

```
cmdr gate check --baseline <trace-id> --model <variant> --freeze-trace-id <trace-id>
  │
  ├─ Load baseline from replay_traces + tool_captures
  ├─ Create Experiment + ExperimentRuns
  │
  ├─ Agent loop:
  │    ├─ Send prompt to variant model (via agentgateway LLM port)
  │    ├─ If tool_calls in response:
  │    │    ├─ Execute each via agentgateway MCP port → freeze-mcp
  │    │    ├─ freeze-mcp returns frozen result OR error (divergence)
  │    │    └─ Append tool results to conversation
  │    ├─ Store variant ReplayTrace + ToolCaptures
  │    └─ Repeat until no tool_calls or divergence
  │
  ├─ Diff baseline vs variant trajectories
  │    ├─ Tool call comparison (sequence, frequency)
  │    ├─ Risk profile comparison (escalation detection)
  │    ├─ Response comparison (semantic similarity)
  │    └─ First divergence identification
  │
  └─ Verdict: pass / fail
```

### Verdict (already implemented)

```
cmdr demo migration verdict --baseline <id> --candidate <id>
  │
  ├─ Load both traces from DB
  ├─ diff.CompareAll()
  └─ Print verdict + dimension breakdown
```

## Migration Path

The prompt-only replay engine remains useful as a fast, infrastructure-free sanity check. The agent-loop replay is the primary gate check mode when freeze-mcp and agentgateway are available.

Suggested approach:

1. The replay engine detects whether MCP connectivity is available (agentgateway MCP port + freeze-mcp health check).
2. If available: run agent-loop replay with freeze-mcp.
3. If not available: fall back to prompt-only replay with a warning that tool execution is not verified.
4. The verdict always reports which mode was used.

## Prior Art

- [docs/FREEZE_AGENT_LOOP.md](FREEZE_AGENT_LOOP.md) — First proof that freeze-mcp can drive a real agent loop
- [docs/MIGRATION_DEMO.md](MIGRATION_DEMO.md) — Full capture → freeze replay → verdict demo (Python agent loop)
- `scripts/test-migration-demo-full-loop.sh` — End-to-end orchestration proving the complete flow
- `test/e2e/freeze_contract_test.go` — Contract tests for freeze-mcp behavior (11 scenarios)
