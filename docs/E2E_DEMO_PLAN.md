# Phase 4: End-to-End Demo with Real Components

## Context

Phase 3 delivered a self-contained `cmdr demo` command using mock data and a canned completer. It proves the drift detection and deployment gate logic works, but everything is synthetic. A real e2e demo demonstrates the full pipeline: a live LLM agent running through agentgateway, CMDR capturing real OTEL traces, freeze-mcp serving frozen tool responses, and the gate producing a verdict from actual model outputs.

This plan wires up all real components — agentgateway, CMDR, freeze-mcp, and live LLM providers (OpenAI + Anthropic) — into a working demo flow.

**Two demo lanes:**
- **Lane A: Real Gate via agentgateway** — Agent script makes live LLM calls through agentgateway, CMDR captures traces via OTLP, then `cmdr gate check` replays the baseline with a different model through agentgateway. This is the core governance loop.
- **Lane B: freeze-mcp Contract Demo** — Standalone demonstration that freeze-mcp can serve frozen tool responses from CMDR's `tool_captures` table. This is a separate demo because freeze-mcp is **not** in the `cmdr gate check` runtime path today — the gate replays LLM prompts but does not execute tools. freeze-mcp matters for future deterministic agent replay where tools must return identical results.

---

## Architecture

### Lane A: Real Gate via agentgateway

```
                   +-----------------+
                   | Agent Script    |
                   | (Python)        |
                   +--------+--------+
                            |
                   POST /v1/chat/completions
                   (X-Run-ID header for correlation)
                            |
                            v
                   +-----------------+
                   | agentgateway    |  :3000 (listener)
                   | - OpenAI route  |
                   | - OTEL tracing  +------> CMDR OTLP :4317 (gRPC)
                   +--------+--------+
                            |
                            v
                   +----------------+
                   | OpenAI API     |
                   | (gpt-4o-mini)  |
                   +----------------+

                   +-----------------+
                   | CMDR            |  :4317 OTLP gRPC, :4318 HTTP
                   | - OTLP receiver |
                   | - parser        |
                   | - drift/gate    |
                   +--------+--------+
                            |
                            v
                   +-----------------+
                   | PostgreSQL      |  :5432
                   +-----------------+
```

### Lane B: freeze-mcp Contract Demo

```
                   +-----------------+
                   | MCP client      |
                   | (curl / script) |
                   +--------+--------+
                            |
                   SSE / tool call
                            |
                            v
                   +-----------------+
                   | freeze-mcp      |  :9090
                   | - reads frozen  |
                   |   tool_captures |
                   +--------+--------+
                            |
                            v
                   +-----------------+
                   | PostgreSQL      |  :5432
                   | (shared w/ CMDR)|
                   +-----------------+
```

---

## Prerequisites

| Requirement | How to get it |
|---|---|
| PostgreSQL 15 | `make dev-up` (existing docker-compose) |
| Go 1.26 | Already installed (per go.mod) |
| Python >= 3.12 | For freeze-mcp (per pyproject.toml) |
| agentgateway binary | `curl -sL https://agentgateway.dev/install \| bash` |
| OpenAI API key | `OPENAI_API_KEY` env var |
| freeze-mcp repo | Clone `../freeze-mcp` (sibling dir) |

**Note:** Lane A starts with OpenAI only (gpt-4o-mini). Anthropic routing is added as a follow-up once the single-provider path is validated end-to-end. This reduces setup surface and isolates failures.

---

## Step 1: Validate agentgateway OTEL Compatibility

**Why this is first:** The entire e2e pipeline depends on agentgateway emitting OTEL spans that CMDR's parser can understand. CMDR's parser (`pkg/otelreceiver/parser.go:137-165`) expects specific attributes:

- `gen_ai.prompt.N.role` and `gen_ai.prompt.N.content` (for each message)
- `gen_ai.completion.0.content` (or `gen_ai.response.text` or `gen_ai.completion`)
- `gen_ai.request.model`, `gen_ai.system`
- `gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`
- Tool calls as span events with `tool.*` attributes

**OTLP transport detail:** agentgateway's `config.tracing.otlpEndpoint` must point to CMDR's **gRPC** OTLP receiver (`localhost:4317`), not the HTTP endpoint (`localhost:4318`). The endpoint format is `http://host:port` (no path suffix). If agentgateway uses HTTP/protobuf instead of gRPC, the endpoint must target `:4318/v1/traces`. Confirm which transport agentgateway uses by checking its logs on startup or docs for `otlpProtocol` config.

**Action:** Install agentgateway, configure it with a single OpenAI backend and OTEL tracing pointed at CMDR, make one test LLM call, and inspect what CMDR receives.

```bash
# 1. Install agentgateway
curl -sL https://agentgateway.dev/install | bash

# 2. Create minimal config (OpenAI only, gRPC OTLP)
cat > agw-config.yaml << 'EOF'
version: v1
config:
  tracing:
    otlpEndpoint: "http://localhost:4317"   # CMDR gRPC OTLP receiver
    # If agentgateway requires HTTP transport instead:
    # otlpEndpoint: "http://localhost:4318"
    # otlpProtocol: "http/protobuf"         # check agw docs for this field
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

# 3. Start CMDR + agentgateway
./bin/cmdr serve &
agentgateway -f agw-config.yaml &

# 4. Make a test call
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Say hello"}]}'

# 5. Verify CMDR received and parsed the span
psql "$CMDR_POSTGRES_URL" -c \
  "SELECT trace_id, model, completion, prompt_tokens FROM replay_traces ORDER BY created_at DESC LIMIT 1"

# 6. Also check raw otel_traces for attribute inspection
psql "$CMDR_POSTGRES_URL" -c \
  "SELECT trace_id, attributes FROM otel_traces ORDER BY created_at DESC LIMIT 1"
```

**Critical file:** `pkg/otelreceiver/parser.go` — `extractPrompt()` at line 137, `extractCompletion()` at line 168, `ParseToolCalls()` at line 215.

**If attributes don't match:** We have two options:
- **(A) Adapt CMDR's parser** to also handle agentgateway's attribute format (low-risk, additive change to `extractPrompt`/`extractCompletion`)
- **(B) Build a thin instrumented agent script** that calls agentgateway for LLM responses but emits its own OTEL traces to CMDR with the exact `gen_ai.*` attributes the parser expects

Recommendation: Start with (A). If agentgateway's format is too different, fall back to (B).

**Deliverable:** A mapping doc of agentgateway's actual OTEL attributes vs CMDR's expected attributes, plus any parser changes needed.

---

## Step 2: agentgateway Configuration

**Phase 1 (this step): OpenAI only.** Start with a single provider to reduce failure surface.

Create `configs/agw-e2e.yaml`:

```yaml
version: v1
config:
  tracing:
    otlpEndpoint: "http://localhost:4317"   # CMDR gRPC OTLP
listeners:
  - name: llm-listener
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
```

**Phase 2 (follow-up): Add Anthropic route** once OpenAI path works end-to-end:

```yaml
      # Add after openai-route, under the same listener
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

With model-based routing, the agent script controls which backend is used by setting the `model` field in the request body. agentgateway translates Anthropic's native API format to/from OpenAI-compatible format automatically.

**File:** New `configs/agw-e2e.yaml`

---

## Step 3: Agent Driver Script

Create `scripts/e2e-agent.py` — a simple Python script that simulates a multi-step coding agent by making sequential LLM calls through agentgateway.

**Purpose:** Produce a realistic multi-step agent trace that CMDR can capture. Not an actual agent framework — just sequential prompts that simulate the same auth-refactoring task from the Phase 3 demo.

**Run correlation:** The script generates a `RUN_ID` (UUID) at startup and passes it as an `X-Run-ID` header on every request. This tag propagates into agentgateway's OTEL spans (as a header attribute or we inject it into the prompt metadata), allowing deterministic trace lookup scoped to this run — no "find the latest row" fragility.

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
import uuid

AGW_URL = "http://localhost:3000/v1/chat/completions"

SYSTEM_PROMPT = (
    "You are a coding assistant. Help the user refactor code safely. "
    "Always run tests after making changes. Use the provided tools."
)
USER_PROMPT = (
    "Refactor the auth module in src/auth/module.ts to use JWT tokens "
    "instead of session-based authentication. Run tests after making changes."
)

TOOLS = [
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read a file from the repository",
            "parameters": {
                "type": "object",
                "properties": {"path": {"type": "string"}},
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "edit_file",
            "description": "Edit a file in the repository",
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                },
                "required": ["path", "content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "run_tests",
            "description": "Run the test suite",
            "parameters": {
                "type": "object",
                "properties": {"suite": {"type": "string"}},
                "required": ["suite"],
            },
        },
    },
]

# Canned tool results (same data as Phase 3 demo seed)
TOOL_RESULTS = {
    "read_file": {
        "src/auth/module.ts": {
            "content": "export class AuthModule { constructor() { this.sessions = new Map(); } ... }"
        },
        "src/auth/module.test.ts": {
            "content": "describe('AuthModule', () => { test('authenticate returns session id', ...) });"
        },
    },
    "edit_file": {"status": "ok", "bytes_written": 247},
    "run_tests": {"passed": 3, "failed": 0, "total": 3},
}


def simulate_tool(name: str, arguments: str) -> dict:
    args = json.loads(arguments)
    if name == "read_file" and args.get("path") in TOOL_RESULTS.get("read_file", {}):
        return TOOL_RESULTS["read_file"][args["path"]]
    return TOOL_RESULTS.get(name, {"status": "ok"})


def run_agent(model: str, run_id: str):
    messages = [
        {"role": "system", "content": SYSTEM_PROMPT},
        {"role": "user", "content": USER_PROMPT},
    ]
    headers = {
        "Content-Type": "application/json",
        "X-Run-ID": run_id,
    }

    step = 0
    max_steps = 10  # safety limit

    while step < max_steps:
        resp = requests.post(
            AGW_URL,
            json={
                "model": model,
                "messages": messages,
                "tools": TOOLS,
                "temperature": 0,  # deterministic
            },
            headers=headers,
        )
        resp.raise_for_status()
        data = resp.json()
        assistant_msg = data["choices"][0]["message"]
        messages.append(assistant_msg)
        step += 1

        finish_reason = data["choices"][0].get("finish_reason", "")

        if assistant_msg.get("tool_calls"):
            for tc in assistant_msg["tool_calls"]:
                result = simulate_tool(tc["function"]["name"], tc["function"]["arguments"])
                messages.append({
                    "role": "tool",
                    "tool_call_id": tc["id"],
                    "content": json.dumps(result),
                })
            print(f"  Step {step}: {[tc['function']['name'] for tc in assistant_msg['tool_calls']]}")
        elif finish_reason == "stop":
            print(f"  Step {step}: agent finished (stop)")
            break
        else:
            print(f"  Step {step}: {finish_reason or 'text response'}")
            break

    print(f"Agent completed {step} LLM calls with model {model}")
    print(f"Run ID: {run_id}")


if __name__ == "__main__":
    model = sys.argv[1] if len(sys.argv) > 1 else "gpt-4o-mini"
    run_id = sys.argv[2] if len(sys.argv) > 2 else str(uuid.uuid4())
    run_agent(model, run_id)
```

**Key details:**
- `temperature: 0` for more deterministic results
- `X-Run-ID` header for trace correlation (avoids fragile "latest row" queries)
- `max_steps` safety limit instead of fixed loop count — the agent may finish in fewer steps
- Uses `gpt-4o-mini` as default (cheap, fast) rather than `gpt-4o`
- Tool execution is simulated with canned responses (same data as Phase 3 demo seed)

**File:** New `scripts/e2e-agent.py`

---

## Step 4: Capture Baseline + Mark It

After the agent script runs, CMDR has the trace in `replay_traces` and `tool_captures`.

**Trace ID discovery:** The agent script prints its `RUN_ID`. We look up the corresponding trace_id using a run-scoped query:

```bash
# The agent script prints: "Run ID: <uuid>"
# Use that to find the trace_id (agentgateway sets trace_id, but run_id correlates)

# Option A: If agentgateway propagates X-Run-ID into span attributes
TRACE_ID=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT trace_id
  FROM otel_traces
  WHERE attributes->>'x-run-id' = '$RUN_ID'
  LIMIT 1
" | xargs)

# Option B: If we can't correlate by run_id, use time-windowed + model filter
TRACE_ID=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT trace_id
  FROM replay_traces
  WHERE created_at > NOW() - INTERVAL '2 minutes'
    AND model = 'gpt-4o-mini'
  GROUP BY trace_id
  ORDER BY MAX(created_at) DESC
  LIMIT 1
" | xargs)

# Mark as baseline
./bin/cmdr drift baseline set "$TRACE_ID" --name "e2e-baseline"
```

**Note:** The `GROUP BY trace_id ORDER BY MAX(created_at) DESC` pattern avoids the invalid `SELECT DISTINCT ... ORDER BY created_at` that Postgres rejects when `created_at` is not in the SELECT list.

---

## Step 5: Deployment Gate with Real LLM (Lane A)

Use the existing `cmdr gate check` command to replay the baseline with the same or different real model:

```bash
# Replay baseline with gpt-4o-mini through agentgateway
./bin/cmdr gate check \
  --baseline "$TRACE_ID" \
  --model gpt-4o-mini \
  --threshold 0.8
```

This uses the real code path in `gate.go:53-138`:
1. `agwclient.NewClient()` with `CMDR_AGENTGATEWAY_URL=http://localhost:3000`
2. `replay.Engine.Run()` sends each baseline prompt to agentgateway → OpenAI
3. Real LLM responses come back, get stored as variant traces
4. `diff.CompareAll()` produces a real 6-dimension similarity score
5. Verdict: PASS or FAIL based on actual behavioral similarity

**Important:** The gate replays LLM prompts only. It does **not** execute tools — tool calls appear in the LLM response as `tool_calls` metadata, which the diff engine compares. This is why freeze-mcp is a separate lane (Lane B), not part of the gate runtime.

**Key config:** `.env` must have `CMDR_AGENTGATEWAY_URL=http://localhost:3000` (agentgateway's listener port, not the default 8080 from `.env.example`).

---

## Step 6: freeze-mcp Contract Demo (Lane B)

**Purpose:** Demonstrate that freeze-mcp can serve frozen tool responses from CMDR's `tool_captures` table. This is a **contract demo** — proving the integration works — not part of the gate runtime path.

**Why separate:** `cmdr gate check` replays LLM prompts and compares LLM responses. It does not execute tools. freeze-mcp matters for a future feature: deterministic agent replay where the agent loop runs tools through freeze-mcp to get identical results, isolating model behavior changes from tool behavior changes.

**Setup (from project root, no cd):**

```bash
# Install freeze-mcp (from sibling dir)
pip install -e ../freeze-mcp

# Start freeze-mcp with the baseline trace
CMDR_POSTGRES_URL="postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable" \
FREEZE_TRACE_ID="$TRACE_ID" \
python -m freeze_mcp.server &
FREEZE_PID=$!

# Wait for readiness
sleep 3
curl -sf http://localhost:9090/health || echo "freeze-mcp not ready"

# Demo: query a frozen tool response
# (The exact query format depends on freeze-mcp's MCP SSE protocol)
echo "freeze-mcp serving frozen tools for trace $TRACE_ID on :9090"
echo "Tool captures available:"
psql "$CMDR_POSTGRES_URL" -c "
  SELECT tool_name, risk_class, args_hash
  FROM tool_captures
  WHERE trace_id = '$TRACE_ID'
  ORDER BY step_index
"
```

**Demo point:** Shows the governance story — captured tool responses are available for deterministic replay, keyed by `(tool_name, args_hash, trace_id)`.

---

## Step 7: E2E Demo Script

Create `scripts/e2e-demo.sh` that orchestrates the full flow with proper cleanup, readiness checks, and run-scoped correlation:

```bash
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CMDR="${CMDR:-$PROJECT_DIR/bin/cmdr}"

# Source env
if [ -f "$PROJECT_DIR/.env" ]; then
  set -a; source "$PROJECT_DIR/.env"; set +a
else
  echo "Error: .env not found. Run: cp .env.example .env" >&2
  exit 1
fi

# Cleanup on exit
PIDS=()
cleanup() {
  echo ""
  echo "Cleaning up background processes..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

# Readiness check helper
wait_for_port() {
  local port=$1 name=$2 retries=20
  for i in $(seq 1 $retries); do
    if nc -z localhost "$port" 2>/dev/null; then
      echo "$name ready on :$port"
      return 0
    fi
    sleep 0.5
  done
  echo "ERROR: $name not ready on :$port after ${retries} retries" >&2
  exit 1
}

echo ""
echo "========================================="
echo "  CMDR E2E Demo: Real LLM Governance"
echo "========================================="
echo ""

# =============================================
# Lane A: Real Gate via agentgateway
# =============================================

echo "--- Scene 1: Start Infrastructure ---"
echo "Starting PostgreSQL..."
make -C "$PROJECT_DIR" dev-up
echo ""

echo "Starting CMDR..."
"$CMDR" serve &
PIDS+=($!)
wait_for_port 4317 "CMDR OTLP"
echo ""

echo "Starting agentgateway..."
agentgateway -f "$PROJECT_DIR/configs/agw-e2e.yaml" &
PIDS+=($!)
wait_for_port 3000 "agentgateway"
echo ""
sleep 2

# Scene 2: Run agent (baseline capture)
echo "--- Scene 2: Capture Baseline (gpt-4o-mini) ---"
BASELINE_RUN_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
python3 "$PROJECT_DIR/scripts/e2e-agent.py" gpt-4o-mini "$BASELINE_RUN_ID"
sleep 3

# Find trace by time window + model (run-scoped)
BASELINE_TRACE=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT trace_id
  FROM replay_traces
  WHERE created_at > NOW() - INTERVAL '5 minutes'
    AND model = 'gpt-4o-mini'
  GROUP BY trace_id
  ORDER BY MAX(created_at) DESC
  LIMIT 1
" | xargs)

if [ -z "$BASELINE_TRACE" ]; then
  echo "ERROR: No baseline trace found in replay_traces" >&2
  exit 1
fi

"$CMDR" drift baseline set "$BASELINE_TRACE" --name "e2e-baseline"
echo "Baseline set: $BASELINE_TRACE"
echo ""
sleep 2

# Scene 3: Run agent again (variant for drift)
echo "--- Scene 3: Run Variant Agent (gpt-4o-mini, second run) ---"
VARIANT_RUN_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
python3 "$PROJECT_DIR/scripts/e2e-agent.py" gpt-4o-mini "$VARIANT_RUN_ID"
sleep 3

VARIANT_TRACE=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT trace_id
  FROM replay_traces
  WHERE created_at > NOW() - INTERVAL '5 minutes'
    AND trace_id != '$BASELINE_TRACE'
  GROUP BY trace_id
  ORDER BY MAX(created_at) DESC
  LIMIT 1
" | xargs)

if [ -z "$VARIANT_TRACE" ]; then
  echo "ERROR: No variant trace found" >&2
  exit 1
fi
echo ""

# Scene 4: Drift check
echo "--- Scene 4: Drift Detection ---"
"$CMDR" drift check "$VARIANT_TRACE" --baseline "$BASELINE_TRACE"
echo ""
sleep 2

# Scene 5: Deployment gate
echo "--- Scene 5: Deployment Gate ---"
echo "Replaying baseline prompts through gpt-4o-mini..."
set +e
"$CMDR" gate check --baseline "$BASELINE_TRACE" --model gpt-4o-mini --threshold 0.8
GATE_EXIT=$?
set -e
echo "Gate exit code: $GATE_EXIT (0=pass, 1=fail)"
echo ""
sleep 2

# =============================================
# Lane B: freeze-mcp Contract Demo
# =============================================

echo "--- Scene 6: freeze-mcp Contract Demo ---"
echo "Starting freeze-mcp with baseline trace..."
CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL" \
FREEZE_TRACE_ID="$BASELINE_TRACE" \
python3 -m freeze_mcp.server &
PIDS+=($!)
sleep 3

echo "Frozen tool captures for baseline trace:"
psql "$CMDR_POSTGRES_URL" -c "
  SELECT step_index, tool_name, risk_class
  FROM tool_captures
  WHERE trace_id = '$BASELINE_TRACE'
  ORDER BY step_index
"
echo ""

# =============================================
# Verification
# =============================================

echo "--- Verification ---"
REPLAY_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$BASELINE_TRACE'
" | xargs)
TOOL_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$BASELINE_TRACE'
" | xargs)
DRIFT_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "
  SELECT COUNT(*) FROM drift_results
  WHERE candidate_trace_id = '$VARIANT_TRACE'
" | xargs)
echo "Baseline replay_traces: $REPLAY_COUNT rows"
echo "Baseline tool_captures: $TOOL_COUNT rows"
echo "Drift results: $DRIFT_COUNT rows"
echo "Gate exit code: $GATE_EXIT"
echo ""

# Summary
echo "--- Summary ---"
"$CMDR" drift status --limit 5
echo ""
"$CMDR" drift baseline list
echo ""

echo "========================================="
echo "  CMDR: Real governance, real models."
echo "========================================="
```

**File:** New `scripts/e2e-demo.sh`

**Hardening applied:**
- `trap cleanup EXIT` for reliable process cleanup
- `wait_for_port` readiness checks before proceeding
- `$CMDR` / absolute paths throughout (no bare `cmdr`)
- No `cd` that would break relative paths
- `uuidgen` run IDs for correlation
- `GROUP BY ... ORDER BY MAX()` for valid Postgres queries
- Explicit error checks on empty trace IDs
- DB count assertions in verification section
- Gate exit code captured and reported

---

## Step 8: .env Updates + Makefile

Update `.env.example` and add new Makefile target:

**.env additions:**
```bash
# agentgateway (for e2e demo — listener port, not API port)
CMDR_AGENTGATEWAY_URL=http://localhost:3000

# LLM API keys (for e2e demo)
# OPENAI_API_KEY=sk-...
# ANTHROPIC_API_KEY=sk-ant-...  (optional, for Lane A phase 2)
```

**Makefile:**
```makefile
.PHONY: e2e-demo
e2e-demo: build
	@echo "Running e2e demo (requires OPENAI_API_KEY + make dev-up)..."
	@bash scripts/e2e-demo.sh
```

---

## Files Summary

| File | Action | Purpose |
|---|---|---|
| `configs/agw-e2e.yaml` | **New** | agentgateway config with OpenAI backend + OTEL tracing |
| `scripts/e2e-agent.py` | **New** | Agent driver script with run-ID correlation |
| `scripts/e2e-demo.sh` | **New** | Orchestration script with trap cleanup + readiness checks |
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

### Risk 3: OTLP Transport Mismatch (MEDIUM)
**Problem:** agentgateway may use HTTP/protobuf OTLP instead of gRPC, causing silent no-op tracing if pointed at the wrong CMDR endpoint.

**Mitigation:** Step 1 explicitly checks for received spans. If none appear, try switching `otlpEndpoint` from `:4317` (gRPC) to `:4318` (HTTP). Check agentgateway startup logs for OTLP exporter type.

### Risk 4: Model Behavior Variance (LOW)
**Problem:** Real LLM responses are non-deterministic. The gate check might produce different verdicts each run.

**Mitigation:** Set `temperature: 0` in the agent script. The threshold (0.8) can be tuned. Non-determinism is actually the point — it demonstrates real governance value.

### Risk 5: API Costs (LOW)
**Problem:** Each demo run makes ~10 real LLM API calls.

**Mitigation:** Uses `gpt-4o-mini` (not `gpt-4o`). Each call is small (short prompts), cost under $0.05 per demo run.

---

## Verification

Each verification step includes explicit pass/fail criteria:

| # | Check | Pass criteria | Fail action |
|---|---|---|---|
| 1 | Prerequisites | `agentgateway --version` prints version, `python3 --version` >= 3.12, `echo $OPENAI_API_KEY` non-empty | Install missing deps |
| 2 | OTEL compat | `SELECT COUNT(*) FROM replay_traces` increases by 1 after a single agentgateway call | Investigate parser (see Risk 1) |
| 3 | Agent run | `SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$TRACE_ID'` returns >= 3 rows; `SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$TRACE_ID'` returns >= 3 rows | Check agentgateway logs, CMDR logs |
| 4 | Baseline | `cmdr drift baseline list` shows the trace_id | Check `MarkTraceAsBaseline` errors |
| 5 | Drift check | `cmdr drift check` exits 0 and prints score + verdict; `SELECT COUNT(*) FROM drift_results WHERE candidate_trace_id = '$VARIANT'` = 1 | Check trace IDs exist in DB |
| 6 | Gate check | `cmdr gate check` exits 0 (pass) or 1 (fail) and prints 6-dimension report; `SELECT COUNT(*) FROM analysis_results` increases by 1 | Check agentgateway connectivity |
| 7 | freeze-mcp | `curl http://localhost:9090/health` returns 200; `tool_captures` rows exist for baseline trace | Check CMDR_POSTGRES_URL, FREEZE_TRACE_ID |
| 8 | Full demo | `make e2e-demo` runs to completion; cleanup trap fires; all background processes terminate | Check script output for ERROR lines |

---

## Suggested Build Order

1. **Step 1 first (validate OTEL compat)** — critical path blocker. Everything else depends on CMDR being able to parse agentgateway's spans.
2. **Step 2 (agw config)** — quick, just a YAML file (OpenAI only for now)
3. **Step 3 (agent script)** — the main new code
4. **Steps 4-5 (capture + gate)** — using existing CLI commands, no new code
5. **Step 6 (freeze-mcp)** — separate lane, can be done in parallel with Steps 4-5
6. **Steps 7-8 (orchestration)** — wire it all together
7. **Follow-up: Add Anthropic route** to agw config once OpenAI path is solid
