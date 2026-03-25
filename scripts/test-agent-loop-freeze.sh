#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGW_CONFIG="${AGW_CONFIG:-$ROOT_DIR/scripts/agentgateway-freeze-loop.yaml}"
AGW_BIN="${AGW_BIN:-}"
AGW_CARGO_TARGET_DIR="${AGW_CARGO_TARGET_DIR:-/tmp/agentgateway-quick-target}"
CMDR_POSTGRES_URL="${CMDR_POSTGRES_URL:-postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable}"
CMDR_OTLP_URL="${CMDR_OTLP_URL:-http://127.0.0.1:4318}"
CMDR_API_URL="${CMDR_API_URL:-http://127.0.0.1:8080/api/v1}"
FREEZE_URL="${FREEZE_URL:-http://127.0.0.1:9090}"
AGW_PORT="${AGW_PORT:-3002}"
MCP_PROXY_PORT="${MCP_PROXY_PORT:-3003}"
MOCK_PORT="${MOCK_PORT:-18081}"
TRACE_ID="${FREEZE_TRACE_ID:-}"
BASELINE_SOURCE="${BASELINE_SOURCE:-cmdr}"
MCP_TRANSPORT_MODE="${MCP_TRANSPORT_MODE:-agentgateway}"
TOOL_NAME="${TOOL_NAME:-calculator}"
TOOL_ARGS_JSON="${TOOL_ARGS_JSON:-{\"operation\":\"add\",\"a\":5,\"b\":3}}"
EXPECTED_RESULT_JSON="${EXPECTED_RESULT_JSON:-{\"result\":8}}"
PROMPT_TEXT="${PROMPT_TEXT:-Use the calculator to add 5 and 3.}"
EXPECT_SUBSTRING="${EXPECT_SUBSTRING:-8}"
BASELINE_COMPLETION_TEXT="${BASELINE_COMPLETION_TEXT:-I will use the calculator.}"
MOCK_LOG="${MOCK_LOG:-/tmp/replay-mock-toolloop.log}"
AGW_LOG="${AGW_LOG:-/tmp/replay-agentgateway-toolloop.log}"
GO_BIN="${GO_BIN:-}"
CMDR_BIN="${CMDR_BIN:-}"

MOCK_PID=""
AGW_PID=""

resolve_repo_dir() {
  local env_value="$1"
  local label="$2"
  shift 2

  if [ -n "$env_value" ]; then
    printf '%s\n' "$env_value"
    return 0
  fi

  local candidates=()
  local candidate
  for candidate in "$@"; do
    if [ -d "$candidate" ]; then
      candidates+=("$candidate")
    fi
  done

  if [ "${#candidates[@]}" -eq 0 ]; then
    echo "$label checkout not found; set ${label^^}_DIR explicitly" >&2
    return 1
  fi

  if [ "${#candidates[@]}" -gt 1 ]; then
    echo "multiple $label checkouts found; set ${label^^}_DIR explicitly" >&2
    printf '  candidate: %s\n' "${candidates[@]}" >&2
    return 1
  fi

  printf '%s\n' "${candidates[0]}"
}

AGENTGATEWAY_DIR="$(resolve_repo_dir "${AGENTGATEWAY_DIR:-}" "agentgateway" "$ROOT_DIR/../../agentgateway" "$ROOT_DIR/../agentgateway")"
FREEZE_DIR="$(resolve_repo_dir "${FREEZE_DIR:-}" "freeze" "$ROOT_DIR/../../freeze-mcp" "$ROOT_DIR/../freeze-mcp")"

ensure_agentgateway_bin() {
  if [ -n "$AGW_BIN" ]; then
    return 0
  fi

  AGW_BIN="$AGW_CARGO_TARGET_DIR/quick-release/agentgateway"
  if [ -x "$AGW_BIN" ]; then
    return 0
  fi

  (
    cd "$AGENTGATEWAY_DIR"
    CARGO_TARGET_DIR="$AGW_CARGO_TARGET_DIR" cargo build --profile quick-release -p agentgateway-app
  )
}

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

run_openai_mock() {
  (
    cd "$ROOT_DIR"
    "$GO_BIN" run ./cmd/mock-openai-upstream "$@"
  )
}

run_freeze_capture() {
  run_cmdr demo internal freeze-capture "$@"
}

run_freeze_agent_loop() {
  run_cmdr demo internal freeze-loop "$@"
}

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo not found in PATH"
  exit 1
fi

ensure_agentgateway_bin

if [ -z "$GO_BIN" ]; then
  GO_BIN="$(command -v go 2>/dev/null || true)"
  if [ -z "$GO_BIN" ]; then
    echo "go not found in PATH; set GO_BIN explicitly"
    exit 1
  fi
fi

if [ -z "$TRACE_ID" ]; then
  TRACE_ID="$(run_cmdr demo internal helper random-hex --bytes 16)"
fi

if ! run_cmdr demo internal helper wait-postgres --url "$CMDR_POSTGRES_URL" --timeout 30s; then
  echo "PostgreSQL is not reachable at $CMDR_POSTGRES_URL"
  exit 1
fi

if ! curl -sSf "$FREEZE_URL/health" >/dev/null 2>&1; then
  echo "freeze-mcp is not listening on $FREEZE_URL"
  echo "Start it first from $FREEZE_DIR"
  exit 1
fi

if [ "$BASELINE_SOURCE" = "cmdr" ]; then
  if ! curl -sSf "$CMDR_OTLP_URL/health" >/dev/null 2>&1; then
    echo "CMDR OTLP receiver is not listening on $CMDR_OTLP_URL"
    echo "Start cmdr serve first, or use BASELINE_SOURCE=seed for the direct fallback"
    exit 1
  fi
  if ! curl -sSf "$CMDR_API_URL/health" >/dev/null 2>&1; then
    echo "CMDR API is not listening on $CMDR_API_URL/health"
    echo "Start cmdr serve first, or use BASELINE_SOURCE=seed for the direct fallback"
    exit 1
  fi

  run_freeze_capture \
    --otlp-url "$CMDR_OTLP_URL" \
    --trace-id "$TRACE_ID" \
    --prompt "$PROMPT_TEXT" \
    --completion "$BASELINE_COMPLETION_TEXT" \
    --tool-name "$TOOL_NAME" \
    --tool-args "$TOOL_ARGS_JSON" \
    --tool-result "$EXPECTED_RESULT_JSON"

  run_cmdr demo internal helper wait-trace \
    --api-url "$CMDR_API_URL" \
    --trace-id "$TRACE_ID" \
    --steps 1 \
    --tools 1 \
    --timeout 30s
else
  run_cmdr demo internal helper seed-tool-capture \
    --postgres-url "$CMDR_POSTGRES_URL" \
    --trace-id "$TRACE_ID" \
    --tool-name "$TOOL_NAME" \
    --tool-args "$TOOL_ARGS_JSON" \
    --tool-result "$EXPECTED_RESULT_JSON"
fi

run_openai_mock \
  --mode toolloop \
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
  "$AGW_BIN" -f "$AGW_CONFIG"
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

MCP_URL="$FREEZE_URL/mcp/"
if [ "$MCP_TRANSPORT_MODE" = "agentgateway" ]; then
  for _ in $(seq 1 120); do
    if lsof -nP -iTCP:"$MCP_PROXY_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  if ! lsof -nP -iTCP:"$MCP_PROXY_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "agentgateway MCP proxy failed to start on port $MCP_PROXY_PORT; see $AGW_LOG"
    exit 1
  fi

  MCP_URL="http://127.0.0.1:$MCP_PROXY_PORT/mcp/"
fi

run_freeze_agent_loop \
  --llm-url "http://127.0.0.1:$AGW_PORT" \
  --mcp-url "$MCP_URL" \
  --freeze-trace-id "$TRACE_ID" \
  --prompt "$PROMPT_TEXT" \
  --expect-substring "$EXPECT_SUBSTRING"

echo "mock upstream log: $MOCK_LOG"
echo "agentgateway log: $AGW_LOG"
