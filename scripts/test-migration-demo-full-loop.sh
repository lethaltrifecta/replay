#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTGATEWAY_DIR="${AGENTGATEWAY_DIR:-$ROOT_DIR/../agentgateway}"
FREEZE_DIR="${FREEZE_DIR:-$ROOT_DIR/../freeze-mcp}"
CMDR_POSTGRES_URL="${CMDR_POSTGRES_URL:-postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable}"
CMDR_OTLP_URL="${CMDR_OTLP_URL:-http://127.0.0.1:4318}"
FREEZE_URL="${FREEZE_URL:-http://127.0.0.1:9090}"
MIGRATION_MCP_URL="${MIGRATION_MCP_URL:-http://127.0.0.1:18082}"
MIGRATION_LLM_URL="${MIGRATION_LLM_URL:-http://127.0.0.1:18083}"
CAPTURE_LLM_URL="${CAPTURE_LLM_URL:-http://127.0.0.1:3102}"
CAPTURE_MCP_URL="${CAPTURE_MCP_URL:-http://127.0.0.1:3103/mcp/}"
REPLAY_LLM_URL="${REPLAY_LLM_URL:-http://127.0.0.1:3202}"
REPLAY_MCP_URL="${REPLAY_MCP_URL:-http://127.0.0.1:3203/mcp/}"
CMDR_LOG="${CMDR_LOG:-/tmp/replay-migration-cmdr.log}"
FREEZE_LOG="${FREEZE_LOG:-/tmp/replay-migration-freeze.log}"
MCP_LOG="${MCP_LOG:-/tmp/replay-migration-mcp.log}"
LLM_LOG="${LLM_LOG:-/tmp/replay-migration-llm.log}"
CAPTURE_AGW_LOG="${CAPTURE_AGW_LOG:-/tmp/replay-migration-capture-agw.log}"
REPLAY_AGW_LOG="${REPLAY_AGW_LOG:-/tmp/replay-migration-replay-agw.log}"
SAFE_VERDICT_LOG="${SAFE_VERDICT_LOG:-/tmp/replay-migration-safe-verdict.log}"
UNSAFE_VERDICT_LOG="${UNSAFE_VERDICT_LOG:-/tmp/replay-migration-unsafe-verdict.log}"
REPORT_SUMMARY_FILE="${REPORT_SUMMARY_FILE:-}"
RUN_LOG_FILE="${RUN_LOG_FILE:-}"
CMDR_BIN="${CMDR_BIN:-}"

PYTHON_BIN="${PYTHON_BIN:-}"
GO_BIN="${GO_BIN:-}"

if [ -z "$PYTHON_BIN" ]; then
  if [ -x "$FREEZE_DIR/.venv/bin/python" ]; then
    PYTHON_BIN="$FREEZE_DIR/.venv/bin/python"
  elif [ -x "/tmp/freeze-venv/bin/python" ]; then
    PYTHON_BIN="/tmp/freeze-venv/bin/python"
  else
    PYTHON_BIN="python3"
  fi
fi

if [ -z "$GO_BIN" ]; then
  GO_BIN="$(command -v go 2>/dev/null || true)"
  if [ -z "$GO_BIN" ]; then
    echo "go not found in PATH; set GO_BIN explicitly"
    exit 1
  fi
fi

CMDR_PID=""
FREEZE_PID=""
MCP_PID=""
LLM_PID=""
CAPTURE_AGW_PID=""
REPLAY_AGW_PID=""

cleanup() {
  pkill -f "$ROOT_DIR/scripts/agentgateway-migration-capture.yaml" >/dev/null 2>&1 || true
  pkill -f "$ROOT_DIR/scripts/agentgateway-migration-replay.yaml" >/dev/null 2>&1 || true
  for pid_var in REPLAY_AGW_PID CAPTURE_AGW_PID LLM_PID MCP_PID FREEZE_PID CMDR_PID; do
    pid="${!pid_var:-}"
    if [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1; then
      kill "$pid" >/dev/null 2>&1 || true
      for _ in $(seq 1 10); do
        if ! kill -0 "$pid" >/dev/null 2>&1; then
          break
        fi
        sleep 1
      done
      kill -9 "$pid" >/dev/null 2>&1 || true
    fi
  done
}

trap cleanup EXIT

run_cmdr() {
  if [ -n "$CMDR_BIN" ]; then
    "$CMDR_BIN" "$@"
    return
  fi

  (
    cd "$ROOT_DIR"
    "$GO_BIN" run ./cmd/cmdr "$@"
  )
}

wait_for_http() {
  local url="$1"
  local label="$2"
  for _ in $(seq 1 60); do
    if curl -sSf "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for $label at $url"
  return 1
}

wait_for_port() {
  local port="$1"
  local label="$2"
  for _ in $(seq 1 120); do
    if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for $label on port $port"
  return 1
}

wait_for_process_exit() {
  local pid="$1"
  for _ in $(seq 1 15); do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_agentgateway_admin_clear() {
  for _ in $(seq 1 20); do
    if ! lsof -nP -iTCP:15000 -iTCP:15020 -iTCP:15021 -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

wait_for_count() {
  local query="$1"
  local expected="$2"
  local label="$3"
  for _ in $(seq 1 30); do
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

if ! "$PYTHON_BIN" - <<'PY' >/dev/null 2>&1
import importlib
for module_name in ("anyio", "httpx", "mcp.client.streamable_http", "psycopg", "fastapi", "uvicorn", "freeze_mcp"):
    importlib.import_module(module_name)
PY
then
  echo "required Python modules are missing for the migration demo"
  echo "Expected modules: anyio, httpx, mcp, psycopg, fastapi, uvicorn, freeze_mcp"
  exit 1
fi

docker compose up -d postgres >/dev/null

for _ in $(seq 1 30); do
  if psql "$CMDR_POSTGRES_URL" -XtAc "SELECT 1" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! psql "$CMDR_POSTGRES_URL" -XtAc "SELECT 1" >/dev/null 2>&1; then
  echo "PostgreSQL is not reachable at $CMDR_POSTGRES_URL"
  exit 1
fi

if ! curl -sSf "$CMDR_OTLP_URL/health" >/dev/null 2>&1; then
  (
    export CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL"
    run_cmdr serve
  ) >"$CMDR_LOG" 2>&1 &
  CMDR_PID=$!
  disown "$CMDR_PID" >/dev/null 2>&1 || true
  wait_for_http "$CMDR_OTLP_URL/health" "CMDR OTLP receiver"
fi

if ! curl -sSf "$FREEZE_URL/health" >/dev/null 2>&1; then
  CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL" FREEZE_TRACE_ID="migration-demo-default" "$PYTHON_BIN" -m freeze_mcp.server >"$FREEZE_LOG" 2>&1 &
  FREEZE_PID=$!
  disown "$FREEZE_PID" >/dev/null 2>&1 || true
  wait_for_http "$FREEZE_URL/health" "freeze-mcp"
fi

"$PYTHON_BIN" "$ROOT_DIR/scripts/mock_migration_mcp_server.py" >"$MCP_LOG" 2>&1 &
MCP_PID=$!
disown "$MCP_PID" >/dev/null 2>&1 || true
wait_for_http "$MIGRATION_MCP_URL/health" "mock migration MCP server"

"$PYTHON_BIN" "$ROOT_DIR/scripts/mock_migration_openai_upstream.py" >"$LLM_LOG" 2>&1 &
LLM_PID=$!
disown "$LLM_PID" >/dev/null 2>&1 || true
wait_for_port "18083" "mock migration OpenAI upstream"

(
  cd "$AGENTGATEWAY_DIR"
  CARGO_TARGET_DIR=/tmp/agentgateway-target cargo run -p agentgateway-app -- -f "$ROOT_DIR/scripts/agentgateway-migration-capture.yaml"
) >"$CAPTURE_AGW_LOG" 2>&1 &
CAPTURE_AGW_PID=$!
disown "$CAPTURE_AGW_PID" >/dev/null 2>&1 || true
wait_for_port "3102" "capture agentgateway AI proxy"
wait_for_port "3103" "capture agentgateway MCP proxy"

BASELINE_TRACE_ID="$("$PYTHON_BIN" -c 'import secrets; print(secrets.token_hex(16))')"
SAFE_REPLAY_TRACE_ID="$("$PYTHON_BIN" -c 'import secrets; print(secrets.token_hex(16))')"
UNSAFE_REPLAY_TRACE_ID="$("$PYTHON_BIN" -c 'import secrets; print(secrets.token_hex(16))')"

echo "Running baseline capture trace $BASELINE_TRACE_ID"
"$PYTHON_BIN" "$ROOT_DIR/scripts/run_migration_demo_agent.py" \
  --mode capture \
  --model migration-safe \
  --behavior safe \
  --trace-id "$BASELINE_TRACE_ID" \
  --otlp-url "$CMDR_OTLP_URL" \
  --llm-url "$CAPTURE_LLM_URL" \
  --mcp-url "$CAPTURE_MCP_URL" \
  --expect-final-substring "Migration completed safely"

wait_for_count "SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$BASELINE_TRACE_ID'" "5" "baseline replay traces"
wait_for_count "SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$BASELINE_TRACE_ID'" "4" "baseline tool captures"

if [ -n "$CAPTURE_AGW_PID" ] && kill -0 "$CAPTURE_AGW_PID" >/dev/null 2>&1; then
  kill "$CAPTURE_AGW_PID" >/dev/null 2>&1 || true
  if ! wait_for_process_exit "$CAPTURE_AGW_PID"; then
    kill -9 "$CAPTURE_AGW_PID" >/dev/null 2>&1 || true
    wait_for_process_exit "$CAPTURE_AGW_PID" || true
  fi
fi
pkill -f "$ROOT_DIR/scripts/agentgateway-migration-capture.yaml" >/dev/null 2>&1 || true
wait_for_agentgateway_admin_clear || true
CAPTURE_AGW_PID=""

(
  cd "$AGENTGATEWAY_DIR"
  CARGO_TARGET_DIR=/tmp/agentgateway-target cargo run -p agentgateway-app -- -f "$ROOT_DIR/scripts/agentgateway-migration-replay.yaml"
) >"$REPLAY_AGW_LOG" 2>&1 &
REPLAY_AGW_PID=$!
disown "$REPLAY_AGW_PID" >/dev/null 2>&1 || true
wait_for_port "3202" "replay agentgateway AI proxy"
wait_for_port "3203" "replay agentgateway MCP proxy"

echo "Running safe frozen replay trace $SAFE_REPLAY_TRACE_ID"
"$PYTHON_BIN" "$ROOT_DIR/scripts/run_migration_demo_agent.py" \
  --mode replay \
  --model migration-safe \
  --behavior safe \
  --trace-id "$SAFE_REPLAY_TRACE_ID" \
  --freeze-trace-id "$BASELINE_TRACE_ID" \
  --otlp-url "$CMDR_OTLP_URL" \
  --llm-url "$REPLAY_LLM_URL" \
  --mcp-url "$REPLAY_MCP_URL" \
  --expect-final-substring "Migration completed safely"

wait_for_count "SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$SAFE_REPLAY_TRACE_ID'" "5" "safe replay traces"
wait_for_count "SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$SAFE_REPLAY_TRACE_ID'" "4" "safe replay tool captures"

echo "Running unsafe frozen replay trace $UNSAFE_REPLAY_TRACE_ID"
"$PYTHON_BIN" "$ROOT_DIR/scripts/run_migration_demo_agent.py" \
  --mode replay \
  --model migration-unsafe \
  --behavior unsafe \
  --trace-id "$UNSAFE_REPLAY_TRACE_ID" \
  --freeze-trace-id "$BASELINE_TRACE_ID" \
  --otlp-url "$CMDR_OTLP_URL" \
  --llm-url "$REPLAY_LLM_URL" \
  --mcp-url "$REPLAY_MCP_URL" \
  --expect-final-substring "blocked" \
  --expect-tool-error

wait_for_count "SELECT COUNT(*) FROM replay_traces WHERE trace_id = '$UNSAFE_REPLAY_TRACE_ID'" "2" "unsafe replay traces"
wait_for_count "SELECT COUNT(*) FROM tool_captures WHERE trace_id = '$UNSAFE_REPLAY_TRACE_ID'" "1" "unsafe replay tool captures"

echo
echo "CMDR verdict: safe replay"
(
  export CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL"
  run_cmdr demo migration verdict \
    --baseline "$BASELINE_TRACE_ID" \
    --candidate "$SAFE_REPLAY_TRACE_ID" \
    --candidate-label "safe-replay"
) | tee "$SAFE_VERDICT_LOG"

echo
echo "CMDR verdict: unsafe replay"
(
  export CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL"
  run_cmdr demo migration verdict \
    --baseline "$BASELINE_TRACE_ID" \
    --candidate "$UNSAFE_REPLAY_TRACE_ID" \
    --candidate-label "unsafe-replay"
) | tee "$UNSAFE_VERDICT_LOG"

if [ -n "$REPORT_SUMMARY_FILE" ]; then
  REPORT_SUMMARY_FILE="$REPORT_SUMMARY_FILE" \
  BASELINE_TRACE_ID="$BASELINE_TRACE_ID" \
  SAFE_REPLAY_TRACE_ID="$SAFE_REPLAY_TRACE_ID" \
  UNSAFE_REPLAY_TRACE_ID="$UNSAFE_REPLAY_TRACE_ID" \
  CMDR_LOG="$CMDR_LOG" \
  FREEZE_LOG="$FREEZE_LOG" \
  MCP_LOG="$MCP_LOG" \
  LLM_LOG="$LLM_LOG" \
  CAPTURE_AGW_LOG="$CAPTURE_AGW_LOG" \
  REPLAY_AGW_LOG="$REPLAY_AGW_LOG" \
  SAFE_VERDICT_LOG="$SAFE_VERDICT_LOG" \
  UNSAFE_VERDICT_LOG="$UNSAFE_VERDICT_LOG" \
  "$PYTHON_BIN" - <<'PY'
import json
import os
from pathlib import Path

payload = {
    "scenario": "database-migration",
    "baseline_trace_id": os.environ["BASELINE_TRACE_ID"],
    "safe_replay_trace_id": os.environ["SAFE_REPLAY_TRACE_ID"],
    "unsafe_replay_trace_id": os.environ["UNSAFE_REPLAY_TRACE_ID"],
    "logs": {
        "run_log": os.environ.get("RUN_LOG_FILE", ""),
        "cmdr": os.environ["CMDR_LOG"],
        "freeze_mcp": os.environ["FREEZE_LOG"],
        "migration_mcp": os.environ["MCP_LOG"],
        "mock_llm": os.environ["LLM_LOG"],
        "capture_agentgateway": os.environ["CAPTURE_AGW_LOG"],
        "replay_agentgateway": os.environ["REPLAY_AGW_LOG"],
        "safe_verdict": os.environ["SAFE_VERDICT_LOG"],
        "unsafe_verdict": os.environ["UNSAFE_VERDICT_LOG"],
    },
}

summary_path = Path(os.environ["REPORT_SUMMARY_FILE"])
summary_path.parent.mkdir(parents=True, exist_ok=True)
summary_path.write_text(json.dumps(payload, indent=2) + "\n")
PY
fi

echo
echo "Baseline trace: $BASELINE_TRACE_ID"
echo "Safe replay trace: $SAFE_REPLAY_TRACE_ID"
echo "Unsafe replay trace: $UNSAFE_REPLAY_TRACE_ID"
echo
echo "Logs:"
echo "  CMDR: $CMDR_LOG"
echo "  freeze-mcp: $FREEZE_LOG"
echo "  migration MCP: $MCP_LOG"
echo "  mock LLM: $LLM_LOG"
echo "  capture agentgateway: $CAPTURE_AGW_LOG"
echo "  replay agentgateway: $REPLAY_AGW_LOG"
echo "  safe verdict: $SAFE_VERDICT_LOG"
echo "  unsafe verdict: $UNSAFE_VERDICT_LOG"
