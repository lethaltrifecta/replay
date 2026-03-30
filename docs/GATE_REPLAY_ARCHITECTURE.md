# Gate Replay Architecture

This document describes how `cmdr gate check` works — both prompt-only replay and agent-loop replay with freeze-mcp.

## The Problem

A deployment gate needs to answer: **"If I swap model A for model B, does the agent still behave safely?"**

This requires replaying a captured agent run with a different model and comparing the resulting behavior. The replay must be deterministic — the tool environment should be frozen so that only the model's behavior varies, not the external world.

## Prompt-Only Replay

The prompt-only replay engine (`pkg/replay/engine.go`) sends the **baseline's prompt** at each step to the variant model and records the response. This is the fallback mode when freeze-mcp is not available.

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

The agent-loop replay (`pkg/replay/agent_loop.go`) runs a real execution loop backed by freeze-mcp for deterministic tool responses. This is the primary mode when `CMDR_MCP_URL` is configured.

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
     agentgateway         freeze-mcp        PostgreSQL
     (LLM proxy)         (frozen tools)   (tool_captures)
              │                │
              ▼                │
         upstream LLM         │
         (e.g. OpenAI)        │
```

**Step-by-step replay loop:**

1. Construct the initial prompt from the baseline's step 0 (system message + user message + tools).
2. Send to the variant model via agentgateway with `X-Freeze-Trace-ID` header.
3. Variant model responds — may include `tool_calls`.
4. For each tool call: execute via freeze-mcp (MCP session).
   - **Match**: freeze-mcp finds a baseline capture with matching `(tool_name, args_hash, trace_id)` → returns the frozen result.
   - **Locator match**: Per-call `X-Freeze-Span-Id` + `X-Freeze-Step-Index` headers select the exact baseline capture for duplicate tool calls.
   - **No match**: freeze-mcp returns `tool_not_captured` → this is a **divergence**.
   - **Exhausted**: All baseline captures for a tool+args pair consumed → `baseline_exhausted` divergence.
5. Append assistant message + tool results to the conversation.
6. Send updated conversation to variant model for the next turn.
7. Repeat until the model produces a final response (no tool calls), a divergence is detected, or max turns is reached.
8. Store the variant's full trace (steps + tool captures) in the database.
9. Diff the baseline and variant trajectories. Produce verdict.

### Mode Selection

`cmdr gate check` automatically selects the replay mode:

- If `CMDR_MCP_URL` is configured and freeze-mcp is reachable → **agent-loop replay**
- If MCP connection fails → falls back to **prompt-only replay** with a warning
- If `CMDR_MCP_URL` is not set → **prompt-only replay** only

### What This Achieves

- **Real agent trajectory.** The variant model sees its own tool results (from freeze-mcp), not the baseline's. Decisions cascade naturally.
- **Deterministic tool environment.** freeze-mcp serves the exact same tool results the baseline saw. The only variable is the model's behavior.
- **Safety boundary.** Any tool call not in the baseline captures is rejected by freeze-mcp.
- **Divergence detection at the tool boundary.** You see that the model *tried to do* something different, not just that it *said* something different.

### Comparison

| | Prompt-Only Replay | Agent-Loop Replay |
|---|---|---|
| Tool calls executed | No | Yes, via freeze-mcp |
| Variant sees own tool results | No (sees baseline's) | Yes (frozen, deterministic) |
| Cascading divergence | Not detected | Detected naturally |
| Uncaptured tool blocked | No | Yes (freeze-mcp rejects) |
| Tests | "Same decisions given same context?" | "Same behavior when actually running?" |
| Requires freeze-mcp | No | Yes |

## Key Components

### freeze-mcp (separate repo)

Python MCP server serving frozen tool responses from `tool_captures` table.

- Lookup: `(tool_name, args_hash, trace_id)` with optional `(span_id, step_index)` locator targeting
- Scoped per-session via `X-Freeze-Trace-ID` header
- Returns `tool_not_captured` error for uncaptured tool calls
- Contract tests: `test/e2e/freeze_contract_test.go` (11 scenarios)

### agentgateway (separate repo)

LLM proxy that emits OTEL traces. Used for:
- Proxying chat completion requests to the upstream model
- CORS configured to allow freeze headers

### Replay Engine (`pkg/replay/`)

- `engine.go` — prompt-only replay (step-by-step baseline prompt → variant model)
- `agent_loop.go` — agent-loop replay (real conversation with tool execution via freeze-mcp)
- `tool_executor.go` — MCP client with `ToolLocator` interface for per-call capture targeting

### Request Headers

The `X-Freeze-Trace-ID` header flows through:

```
gate check request
  → FreezeHeaders() defaults to baseline trace ID if not provided
    → NewMCPToolExecutor(ctx, mcpURL, freezeHeaders)
      → freezeRoundTripper base headers (on every MCP request)
        → freeze-mcp
```

Per-call locator headers (`X-Freeze-Span-Id`, `X-Freeze-Step-Index`) are set/cleared dynamically around each `CallTool` via the `ToolLocator` interface.

## Data Flow

### Capture

```
Live agent run
  → agentgateway (with OTLP tracing)
    → CMDR OTLP receiver
      → otel_traces + replay_traces + tool_captures (PostgreSQL)
```

### Replay

```
cmdr gate check --baseline <trace-id> --model <variant>
  │
  ├─ Load baseline from replay_traces + tool_captures
  ├─ Create Experiment + ExperimentRuns
  │
  ├─ Agent loop (if MCP available):
  │    ├─ Send prompt to variant model (via agentgateway)
  │    ├─ If tool_calls in response:
  │    │    ├─ Match baseline capture → set locator headers
  │    │    ├─ Execute via freeze-mcp
  │    │    ├─ freeze-mcp returns frozen result OR error (divergence)
  │    │    └─ Append tool results to conversation
  │    ├─ Store variant ReplayTrace + ToolCaptures
  │    └─ Repeat until no tool_calls, divergence, or max turns
  │
  ├─ Diff baseline vs variant trajectories
  │    ├─ Tool call comparison (sequence, frequency)
  │    ├─ Risk profile comparison (escalation detection)
  │    ├─ Response comparison (semantic similarity)
  │    └─ First divergence identification
  │
  └─ Verdict: pass / fail
```

## Prior Art

- [docs/FREEZE_AGENT_LOOP.md](FREEZE_AGENT_LOOP.md) — First proof that freeze-mcp can drive a real agent loop
- [docs/MIGRATION_DEMO.md](MIGRATION_DEMO.md) — Full capture → freeze replay → verdict demo
- `test/e2e/freeze_contract_test.go` — Contract tests for freeze-mcp behavior (11 scenarios)
