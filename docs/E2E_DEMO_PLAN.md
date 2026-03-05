# Phase 4: End-to-End Demo with Real Components

## Context

Phase 3 delivered a self-contained `cmdr demo` command using mock data and a canned completer. It proves the drift detection and deployment gate logic works, but everything is synthetic. A real e2e demo demonstrates the full pipeline: a live LLM agent running through agentgateway, CMDR capturing real OTEL traces, freeze-mcp serving frozen tool responses, and the gate producing a verdict from actual model outputs.

This plan wires up all real components — agentgateway, CMDR, freeze-mcp, and live LLM providers (OpenAI + Anthropic) — into a working demo flow.

---

## Architecture

```
                   +-----------------+
                   | Agent Script    |
                   | (Python/bash)   |
                   +--------+--------+
                            |
                   POST /v1/chat/completions
                            |
                            v
                   +-----------------+
                   | agentgateway    |  :3000 (listener)
                   | - OpenAI route  |
                   | - Anthropic rt  |
                   | - OTEL tracing  +------> CMDR OTLP :4317
                   +--------+--------+
                            |
              +-------------+-------------+
              |                           |
              v                           v
      +-------+-------+          +-------+-------+
      | OpenAI API    |          | Anthropic API |
      | (gpt-4o)      |          | (claude-3.5)  |
      +---------------+          +---------------+

                   +-----------------+
                   | CMDR            |  :4317 OTLP, :4318 HTTP
                   | - OTLP receiver |
                   | - parser        |
                   | - drift/gate    |
                   +--------+--------+
                            |
                            v
                   +-----------------+
                   | PostgreSQL      |  :5432
                   | - replay_traces |
                   | - tool_captures |
                   | - baselines     |
                   +--------+--------+
                            ^
                            |  (read-only)
                   +--------+--------+
                   | freeze-mcp      |  :9090
                   | - frozen tools  |
                   | - SSE transport |
                   +-----------------+
```

5 services: PostgreSQL, CMDR, agentgateway, freeze-mcp, and a driver script.

---

## Prerequisites

| Requirement | How to get it |
|---|---|
| PostgreSQL 15 | `make dev-up` (existing docker-compose) |
| Go 1.24 | Already installed |
| Python 3.11+ | For freeze-mcp |
| agentgateway binary | `curl -sL https://agentgateway.dev/install \| bash` |
| OpenAI API key | `OPENAI_API_KEY` env var |
| Anthropic API key | `ANTHROPIC_API_KEY` env var |
| freeze-mcp repo | Clone `../freeze-mcp` (sibling dir) |

---

## Step 1: Validate agentgateway OTEL Compatibility

**Why this is first:** The entire e2e pipeline depends on agentgateway emitting OTEL spans that CMDR's parser can understand. CMDR's parser (`pkg/otelreceiver/parser.go:137-165`) expects specific attributes:

- `gen_ai.prompt.N.role` and `gen_ai.prompt.N.content` (for each message)
- `gen_ai.completion.0.content` (or `gen_ai.response.text` or `gen_ai.completion`)
- `gen_ai.request.model`, `gen_ai.system`
- `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`
- Tool calls as span events with `tool.*` attributes

**Action:** Install agentgateway, configure it with a single OpenAI backend and OTEL tracing pointed at CMDR, make one test LLM call, and inspect what CMDR receives.

```bash
# 1. Install agentgateway
curl -sL https://agentgateway.dev/install | bash

# 2. Create minimal config
cat > agw-config.yaml << 'EOF'
version: v1
config:
  tracing:
    otlpEndpoint: "http://localhost:4317"
listeners:
  - name: openai-listener
    protocol: OpenAI
    bind: 0.0.0.0:3000
    routes:
      - name: openai-route
        backends:
          - name: openai-backend
            protocol: OpenAI
            hostname: api.openai.com
            port: 443
            tls: true
            headers:
              Authorization: "Bearer ${OPENAI_API_KEY}"
EOF

# 3. Start agentgateway
agentgateway -f agw-config.yaml

# 4. Make a test call
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Say hello"}]}'

# 5. Check CMDR logs for parsed spans
```

**Critical file:** `pkg/otelreceiver/parser.go` — `extractPrompt()` at line 137, `extractCompletion()` at line 168, `ParseToolCalls()` at line 215.

**If attributes don't match:** We have two options:
- **(A) Adapt CMDR's parser** to also handle agentgateway's attribute format (low-risk, additive change to `extractPrompt`/`extractCompletion`)
- **(B) Build a thin instrumented agent script** that calls agentgateway for LLM responses but emits its own OTEL traces to CMDR with the exact `gen_ai.*` attributes the parser expects

Recommendation: Start with (A). If agentgateway's format is too different, fall back to (B).

**Deliverable:** A mapping doc of agentgateway's actual OTEL attributes vs CMDR's expected attributes, plus any parser changes needed.

---

## Step 2: agentgateway Configuration

Create `configs/agw-e2e.yaml` with two backends (OpenAI + Anthropic) and OTEL tracing:

```yaml
version: v1
config:
  tracing:
    otlpEndpoint: "http://localhost:4317"
listeners:
  - name: llm-listener
    protocol: OpenAI
    bind: 0.0.0.0:3000
    routes:
      - name: openai-route
        match:
          model: "gpt-4o*"
        backends:
          - name: openai-backend
            protocol: OpenAI
            hostname: api.openai.com
            port: 443
            tls: true
            headers:
              Authorization: "Bearer ${OPENAI_API_KEY}"
      - name: anthropic-route
        match:
          model: "claude-*"
        backends:
          - name: anthropic-backend
            protocol: Anthropic
            hostname: api.anthropic.com
            port: 443
            tls: true
            headers:
              x-api-key: "${ANTHROPIC_API_KEY}"
              anthropic-version: "2023-06-01"
```

The `match.model` routing means the agent script controls which backend is used by setting the `model` field in the request body. Both backends route through the same listener on port 3000.

**Key detail:** agentgateway translates Anthropic's native API format to/from OpenAI-compatible format automatically. The agent script always uses OpenAI-compatible JSON (`/v1/chat/completions`), and agentgateway handles the protocol translation.

**File:** New `configs/agw-e2e.yaml`

---

## Step 3: Agent Driver Script

Create `scripts/e2e-agent.py` — a simple Python script that simulates a multi-step coding agent by making sequential LLM calls through agentgateway.

**Purpose:** Produce a realistic multi-step agent trace that CMDR can capture. Not an actual agent framework — just sequential prompts that simulate the same auth-refactoring task from the Phase 3 demo.

**Flow (5 steps, matching the baseline scenario):**

```
Step 0: "Read src/auth/module.ts" → tool_call: read_file
Step 1: "Read src/auth/module.test.ts" → tool_call: read_file
Step 2: "Refactor auth to JWT" → tool_call: edit_file
Step 3: "Run tests" → tool_call: run_tests
Step 4: "Update migration docs" → tool_call: edit_file
```

**Implementation:**

```python
#!/usr/bin/env python3
"""E2E agent driver: makes real LLM calls through agentgateway."""

import requests
import json
import sys

AGW_URL = "http://localhost:3000/v1/chat/completions"

SYSTEM_PROMPT = "You are a coding assistant. ..."
USER_PROMPT = "Refactor the auth module in src/auth/module.ts to use JWT..."

# Define tools the agent can use
TOOLS = [
    {"type": "function", "function": {"name": "read_file", ...}},
    {"type": "function", "function": {"name": "edit_file", ...}},
    {"type": "function", "function": {"name": "run_tests", ...}},
]

def run_agent(model: str):
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": USER_PROMPT},
    ]

    for step in range(5):
        resp = requests.post(AGW_URL, json={
            "model": model,
            "messages": messages,
            "tools": TOOLS,
        })
        data = resp.json()
        assistant_msg = data["choices"][0]["message"]
        messages.append(assistant_msg)

        # Simulate tool execution (canned results)
        if assistant_msg.get("tool_calls"):
            for tc in assistant_msg["tool_calls"]:
                tool_result = simulate_tool(tc["function"]["name"], tc["function"]["arguments"])
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "content": json.dumps(tool_result),
                })

    print(f"Agent completed {len(messages)} messages with model {model}")

if __name__ == "__main__":
    model = sys.argv[1] if len(sys.argv) > 1 else "gpt-4o"
    run_agent(model)
```

**Key details:**
- Tool definitions use OpenAI function calling format (JSON schema)
- Tool execution is simulated with canned responses (same data as Phase 3 demo seed)
- Each LLM call generates a new OTEL span through agentgateway, which CMDR captures
- The script takes a `model` argument so we can run it with both `gpt-4o` and `claude-3-5-sonnet`

**File:** New `scripts/e2e-agent.py`

---

## Step 4: Capture Baseline + Mark It

After the agent script runs against one model (say `claude-3-5-sonnet`), CMDR has the trace in `replay_traces` and `tool_captures`.

**Challenge:** We need the trace ID. Options:
1. agentgateway may return `X-Trace-ID` or similar header — check
2. Query CMDR's database for the most recent trace: `cmdr drift status --limit 1`
3. The agent script could generate its own trace ID and pass it as a header

**Action:** After confirming how trace IDs surface, mark the trace as baseline:

```bash
# Find the trace ID from CMDR
TRACE_ID=$(psql "$CMDR_POSTGRES_URL" -t -c \
  "SELECT DISTINCT trace_id FROM replay_traces ORDER BY created_at DESC LIMIT 1")

# Mark as baseline
cmdr drift baseline set "$TRACE_ID" --name "e2e-baseline-claude"
```

**File:** Part of `scripts/e2e-demo.sh` orchestration script

---

## Step 5: Deployment Gate with Real LLM

Use the existing `cmdr gate check` command (not `cmdr demo gate`) to replay the baseline with a different real model:

```bash
# Replay baseline with gpt-4o through agentgateway
cmdr gate check \
  --baseline "$TRACE_ID" \
  --model gpt-4o \
  --threshold 0.8
```

This uses the real code path in `gate.go:53-138`:
1. `agwclient.NewClient()` with `CMDR_AGENTGATEWAY_URL=http://localhost:3000`
2. `replay.Engine.Run()` sends each baseline prompt to agentgateway, which routes to OpenAI
3. Real LLM responses come back, get stored as variant traces
4. `diff.CompareAll()` produces a real 6-dimension similarity score
5. Verdict: PASS or FAIL based on actual behavioral similarity

**Key config:** `.env` must have `CMDR_AGENTGATEWAY_URL=http://localhost:3000` (agentgateway's listener port, not the default 8080 from `.env.example`).

---

## Step 6: freeze-mcp Integration (Deterministic Replay)

**Purpose:** Show that tool responses can be frozen and replayed deterministically, which is the key to reproducible gate checks.

**Setup:**
```bash
cd ../freeze-mcp
pip install -e .
CMDR_POSTGRES_URL="postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable" \
FREEZE_TRACE_ID="$TRACE_ID" \
python -m freeze_mcp.server
```

freeze-mcp reads from CMDR's `tool_captures` table and serves frozen results via MCP SSE on port 9090. When a tool call comes in with matching `(tool_name, args_hash)`, it returns the captured result instead of executing the real tool.

**Demo point:** This shows the governance story — you can replay agent scenarios with frozen tool outputs to isolate model behavior changes from tool behavior changes.

**Note:** Full multi-turn tool replay (agent -> LLM -> freeze-mcp -> agent loop) requires agent framework integration that's out of scope for this demo. The demo shows freeze-mcp serving frozen responses to manual MCP client queries.

---

## Step 7: E2E Demo Script

Create `scripts/e2e-demo.sh` that orchestrates the full flow:

```bash
#!/bin/bash
set -euo pipefail

# Source env
source .env
export OPENAI_API_KEY ANTHROPIC_API_KEY

echo "========================================="
echo "  CMDR E2E Demo: Real LLM Governance"
echo "========================================="

# Scene 1: Start services
echo "--- Scene 1: Infrastructure ---"
echo "Starting PostgreSQL, CMDR, and agentgateway..."
make dev-up
./bin/cmdr serve &  # background
CMDR_PID=$!
agentgateway -f configs/agw-e2e.yaml &  # background
AGW_PID=$!
sleep 5

# Scene 2: Run agent with Claude (baseline)
echo "--- Scene 2: Capture Baseline (Claude 3.5 Sonnet) ---"
python3 scripts/e2e-agent.py claude-3-5-sonnet-20241022
sleep 2

# Find and mark baseline
TRACE_ID=$(psql "$CMDR_POSTGRES_URL" -t -c \
  "SELECT DISTINCT trace_id FROM replay_traces ORDER BY created_at DESC LIMIT 1" | xargs)
cmdr drift baseline set "$TRACE_ID" --name "e2e-baseline-claude"
echo "Baseline set: $TRACE_ID"

# Scene 3: Run agent with GPT-4o (second trace for drift)
echo "--- Scene 3: Run Variant Agent (GPT-4o) ---"
python3 scripts/e2e-agent.py gpt-4o
sleep 2

# Find variant trace
VARIANT_TRACE=$(psql "$CMDR_POSTGRES_URL" -t -c \
  "SELECT DISTINCT trace_id FROM replay_traces \
   WHERE trace_id != '$TRACE_ID' \
   ORDER BY created_at DESC LIMIT 1" | xargs)

# Scene 4: Drift check
echo "--- Scene 4: Drift Detection ---"
cmdr drift check "$VARIANT_TRACE" --baseline "$TRACE_ID"

# Scene 5: Deployment gate (replay baseline through GPT-4o)
echo "--- Scene 5: Deployment Gate ---"
cmdr gate check --baseline "$TRACE_ID" --model gpt-4o --threshold 0.8

# Scene 6: Show freeze-mcp
echo "--- Scene 6: Frozen Tool Responses ---"
echo "Starting freeze-mcp with baseline trace..."
cd ../freeze-mcp
FREEZE_TRACE_ID="$TRACE_ID" python -m freeze_mcp.server &
FREEZE_PID=$!
sleep 3
echo "freeze-mcp serving frozen tools for trace $TRACE_ID on :9090"

# Summary
echo "--- Results ---"
cmdr drift status --limit 5
cmdr drift baseline list

# Cleanup
kill $CMDR_PID $AGW_PID $FREEZE_PID 2>/dev/null || true
echo "========================================="
echo "  CMDR: Real governance, real models."
echo "========================================="
```

**File:** New `scripts/e2e-demo.sh`

---

## Step 8: .env Updates + Makefile

Update `.env.example` and add new Makefile target:

**.env additions:**
```bash
# agentgateway (for e2e demo)
CMDR_AGENTGATEWAY_URL=http://localhost:3000

# LLM API keys (for e2e demo)
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

**Makefile:**
```makefile
.PHONY: e2e-demo
e2e-demo: build
	@echo "Running e2e demo (requires API keys + make dev-up)..."
	@bash scripts/e2e-demo.sh
```

---

## Files Summary

| File | Action | Purpose |
|---|---|---|
| `configs/agw-e2e.yaml` | **New** | agentgateway config with OpenAI + Anthropic backends + OTEL |
| `scripts/e2e-agent.py` | **New** | Agent driver script making real LLM calls |
| `scripts/e2e-demo.sh` | **New** | Orchestration script for the full e2e flow |
| `pkg/otelreceiver/parser.go` | **Possibly modify** | Adapt to agentgateway's OTEL attribute format (Step 1 outcome) |
| `.env.example` | **Modify** | Add AGW URL + API key placeholders |
| `Makefile` | **Modify** | Add `e2e-demo` target |

---

## Risks and Mitigations

### Risk 1: OTEL Attribute Mismatch (HIGH)
**Problem:** agentgateway may not emit `gen_ai.prompt.N.role/content` attributes. CMDR's `extractPrompt()` would return empty, and `ParseLLMSpan()` would return nil (line 98-101).

**Mitigation:** Step 1 is specifically to validate this. If attributes differ:
- Option A: Add fallback parsing paths in `extractPrompt()` and `extractCompletion()`
- Option B: The agent script emits its own OTEL spans using the OpenTelemetry Python SDK with exact `gen_ai.*` attributes, bypassing agentgateway's tracing entirely

### Risk 2: Tool Call Capture (MEDIUM)
**Problem:** agentgateway traces may not include tool call events as span events with `tool.*` attributes. CMDR's `ParseToolCalls()` looks for events prefixed with "tool" (line 227).

**Mitigation:** Same as Risk 1 — either adapt the parser or have the agent script emit tool events directly. The agent script already knows which tools it called (it simulates them), so it can emit proper tool events.

### Risk 3: Trace ID Discovery (LOW)
**Problem:** After the agent script runs, we need to find the trace ID in CMDR's database.

**Mitigation:** Three options in order of preference:
1. Query `replay_traces` for the most recent distinct trace_id
2. Have the agent script pass an `X-Request-ID` header and use that to correlate
3. Use `cmdr drift status --limit 1` to find the latest trace

### Risk 4: Model Behavior Variance (LOW)
**Problem:** Real LLM responses are non-deterministic. The gate check might produce different verdicts each run.

**Mitigation:** This is actually the point — it demonstrates real governance. Set temperature to 0 in the agent script for more deterministic results. The threshold (0.8) can be tuned.

### Risk 5: API Costs (LOW)
**Problem:** Each demo run makes ~10 real LLM API calls.

**Mitigation:** Use `gpt-4o-mini` instead of `gpt-4o` for cheaper runs. Each call is small (short prompts), so cost should be under $0.10 per demo run.

---

## Verification

1. **Prerequisite check:** `agentgateway --version`, `python3 --version`, API keys set
2. **Step 1 validation:** Make one LLM call through agentgateway, verify CMDR receives and parses the span (`SELECT * FROM replay_traces ORDER BY created_at DESC LIMIT 1`)
3. **Agent run:** `python3 scripts/e2e-agent.py claude-3-5-sonnet-20241022` produces 5 replay_trace rows + 5 tool_capture rows in DB
4. **Baseline:** `cmdr drift baseline set <trace-id>` succeeds
5. **Drift check:** `cmdr drift check <variant-trace> --baseline <baseline-trace>` produces a score and verdict
6. **Gate check:** `cmdr gate check --baseline <trace-id> --model gpt-4o --threshold 0.8` produces a real 6-dimension report
7. **freeze-mcp:** Query port 9090 and verify it returns frozen tool results for the baseline trace
8. **Full demo:** `make e2e-demo` runs end-to-end without manual intervention

---

## Suggested Build Order

1. **Step 1 first (validate OTEL compat)** — this is the critical path blocker. Everything else depends on CMDR being able to parse agentgateway's spans.
2. **Step 2 (agw config)** — quick, just a YAML file
3. **Step 3 (agent script)** — the main new code
4. **Steps 4-5 (capture + gate)** — using existing CLI commands, no new code
5. **Step 6 (freeze-mcp)** — integration validation
6. **Steps 7-8 (orchestration)** — wire it all together
