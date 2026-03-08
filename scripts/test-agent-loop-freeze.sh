#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTGATEWAY_DIR="${AGENTGATEWAY_DIR:-$ROOT_DIR/../agentgateway}"
FREEZE_DIR="${FREEZE_DIR:-$ROOT_DIR/../freeze-mcp}"
AGW_CONFIG="${AGW_CONFIG:-$ROOT_DIR/scripts/agentgateway-freeze-loop.yaml}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-$ROOT_DIR/pkg/storage/migrations}"
CMDR_POSTGRES_URL="${CMDR_POSTGRES_URL:-postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable}"
CMDR_OTLP_URL="${CMDR_OTLP_URL:-http://127.0.0.1:4318}"
FREEZE_URL="${FREEZE_URL:-http://127.0.0.1:9090}"
AGW_PORT="${AGW_PORT:-3002}"
MOCK_PORT="${MOCK_PORT:-18081}"
TRACE_ID="${FREEZE_TRACE_ID:-}"
BASELINE_SOURCE="${BASELINE_SOURCE:-cmdr}"
TOOL_NAME="${TOOL_NAME:-calculator}"
TOOL_ARGS_JSON="${TOOL_ARGS_JSON:-{\"operation\":\"add\",\"a\":5,\"b\":3}}"
EXPECTED_RESULT_JSON="${EXPECTED_RESULT_JSON:-{\"result\":8}}"
PROMPT_TEXT="${PROMPT_TEXT:-Use the calculator to add 5 and 3.}"
EXPECT_SUBSTRING="${EXPECT_SUBSTRING:-8}"
BASELINE_COMPLETION_TEXT="${BASELINE_COMPLETION_TEXT:-I will use the calculator.}"
MOCK_LOG="${MOCK_LOG:-/tmp/replay-mock-toolloop.log}"
AGW_LOG="${AGW_LOG:-/tmp/replay-agentgateway-toolloop.log}"

PYTHON_BIN="${PYTHON_BIN:-}"
if [ -z "$PYTHON_BIN" ]; then
  if [ -x "$FREEZE_DIR/.venv/bin/python" ]; then
    PYTHON_BIN="$FREEZE_DIR/.venv/bin/python"
  else
    PYTHON_BIN="python3"
  fi
fi

if [ -z "$TRACE_ID" ]; then
  TRACE_ID="$("$PYTHON_BIN" -c 'import secrets; print(secrets.token_hex(16))')"
fi

MOCK_PID=""
AGW_PID=""

cleanup() {
  if [ -n "$AGW_PID" ] && kill -0 "$AGW_PID" >/dev/null 2>&1; then
    kill -INT "$AGW_PID" >/dev/null 2>&1 || true
    sleep 1
    kill "$AGW_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$MOCK_PID" ] && kill -0 "$MOCK_PID" >/dev/null 2>&1; then
    kill "$MOCK_PID" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

wait_for_count() {
  local query="$1"
  local expected="$2"
  local label="$3"

  for _ in $(seq 1 30); do
    local count
    count="$(psql "$CMDR_POSTGRES_URL" -XtAc "$query" | tr -d '[:space:]')"
    if [ "$count" = "$expected" ]; then
      return 0
    fi
    sleep 1
  done

  echo "timed out waiting for $label"
  return 1
}

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo not found in PATH"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found in PATH"
  exit 1
fi

if ! psql "$CMDR_POSTGRES_URL" -XtAc "SELECT 1" >/dev/null 2>&1; then
  echo "PostgreSQL is not reachable at $CMDR_POSTGRES_URL"
  exit 1
fi

if [ ! -f "$MIGRATIONS_DIR/001_initial_schema.sql" ] || [ ! -f "$MIGRATIONS_DIR/002_baselines_and_drift.sql" ] || [ ! -f "$MIGRATIONS_DIR/003_drift_unique_constraint.sql" ]; then
  echo "CMDR migration files not found in $MIGRATIONS_DIR"
  exit 1
fi

if [ "$(psql "$CMDR_POSTGRES_URL" -XtAc "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'tool_captures'")" != "1" ]; then
  echo "tool_captures not found; applying CMDR migrations..."
  psql "$CMDR_POSTGRES_URL" -v ON_ERROR_STOP=1 -f "$MIGRATIONS_DIR/001_initial_schema.sql" >/dev/null
  psql "$CMDR_POSTGRES_URL" -v ON_ERROR_STOP=1 -f "$MIGRATIONS_DIR/002_baselines_and_drift.sql" >/dev/null
  psql "$CMDR_POSTGRES_URL" -v ON_ERROR_STOP=1 -f "$MIGRATIONS_DIR/003_drift_unique_constraint.sql" >/dev/null
fi

if ! curl -sSf "$FREEZE_URL/health" >/dev/null 2>&1; then
  echo "freeze-mcp is not listening on $FREEZE_URL"
  echo "Start it first from $FREEZE_DIR"
  exit 1
fi

if ! PYTHONPATH="$FREEZE_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" - <<'PY' > /dev/null 2>&1
import importlib
for module_name in ("anyio", "httpx", "mcp.client.streamable_http", "mcp.client.session", "psycopg", "freeze_mcp.matcher"):
    importlib.import_module(module_name)
PY
then
  echo "required Python deps are missing for the freeze-mcp agent loop"
  echo "Expected modules: anyio, httpx, mcp, psycopg, freeze_mcp"
  exit 1
fi

if [ "$BASELINE_SOURCE" = "cmdr" ]; then
  if ! curl -sSf "$CMDR_OTLP_URL/health" >/dev/null 2>&1; then
    echo "CMDR OTLP receiver is not listening on $CMDR_OTLP_URL"
    echo "Start cmdr serve first, or use BASELINE_SOURCE=seed for the direct fallback"
    exit 1
  fi

  "$PYTHON_BIN" "$ROOT_DIR/scripts/capture_freeze_baseline.py" \
    --otlp-url "$CMDR_OTLP_URL" \
    --trace-id "$TRACE_ID" \
    --prompt "$PROMPT_TEXT" \
    --completion "$BASELINE_COMPLETION_TEXT" \
    --tool-name "$TOOL_NAME" \
    --tool-args "$TOOL_ARGS_JSON" \
    --tool-result "$EXPECTED_RESULT_JSON"

  wait_for_count "SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$TRACE_ID'" "1" "replay_traces row for $TRACE_ID"
  wait_for_count "SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$TRACE_ID'" "1" "tool_captures row for $TRACE_ID"
else
  export TRACE_ID TOOL_NAME TOOL_ARGS_JSON EXPECTED_RESULT_JSON CMDR_POSTGRES_URL
  PYTHONPATH="$FREEZE_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" - <<'PY'
import json
import os
from datetime import datetime, timezone
from uuid import uuid4

import psycopg

from freeze_mcp.matcher import calculate_args_hash

trace_id = os.environ["TRACE_ID"]
tool_name = os.environ["TOOL_NAME"]
tool_args = json.loads(os.environ["TOOL_ARGS_JSON"])
result = json.loads(os.environ["EXPECTED_RESULT_JSON"])
args_hash = calculate_args_hash(tool_args)
span_id = f"loop-span-{uuid4().hex[:12]}"
step_index = int(datetime.now(timezone.utc).timestamp()) % 100000

with psycopg.connect(os.environ["CMDR_POSTGRES_URL"]) as conn:
    with conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO tool_captures
            (trace_id, span_id, step_index, tool_name, args, args_hash, result, error, latency_ms, risk_class)
            VALUES (%s, %s, %s, %s, %s::jsonb, %s, %s::jsonb, NULL, 5, 'read')
            """,
            (
                trace_id,
                span_id,
                step_index,
                tool_name,
                json.dumps(tool_args),
                args_hash,
                json.dumps(result),
            ),
        )
    conn.commit()

print(f"seeded tool capture trace_id={trace_id} args_hash={args_hash}")
PY
fi

PYTHONPATH="$FREEZE_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" \
  "$ROOT_DIR/scripts/mock_toolcall_openai_upstream.py" \
  --port "$MOCK_PORT" \
  --tool-name "$TOOL_NAME" \
  --tool-args "$TOOL_ARGS_JSON" \
  >"$MOCK_LOG" 2>&1 &
MOCK_PID=$!
disown "$MOCK_PID" >/dev/null 2>&1 || true

for _ in $(seq 1 30); do
  if lsof -nP -iTCP:"$MOCK_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! lsof -nP -iTCP:"$MOCK_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "mock tool-loop upstream failed to start"
  exit 1
fi

(
  cd "$AGENTGATEWAY_DIR"
  CARGO_TARGET_DIR=/tmp/agentgateway-target cargo run -p agentgateway-app -- -f "$AGW_CONFIG"
) >"$AGW_LOG" 2>&1 &
AGW_PID=$!
disown "$AGW_PID" >/dev/null 2>&1 || true

for _ in $(seq 1 120); do
  if lsof -nP -iTCP:"$AGW_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! lsof -nP -iTCP:"$AGW_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "agentgateway tool-loop config failed to start; see $AGW_LOG"
  exit 1
fi

PYTHONPATH="$FREEZE_DIR${PYTHONPATH:+:$PYTHONPATH}" "$PYTHON_BIN" \
  "$ROOT_DIR/scripts/run_freeze_agent_loop.py" \
  --llm-url "http://127.0.0.1:$AGW_PORT" \
  --mcp-url "$FREEZE_URL/mcp/" \
  --freeze-trace-id "$TRACE_ID" \
  --prompt "$PROMPT_TEXT" \
  --expect-substring "$EXPECT_SUBSTRING"

echo "mock upstream log: $MOCK_LOG"
echo "agentgateway log: $AGW_LOG"
