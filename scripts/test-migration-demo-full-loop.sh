#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGW_BIN="${AGW_BIN:-}"
AGW_CARGO_TARGET_DIR="${AGW_CARGO_TARGET_DIR:-/tmp/agentgateway-quick-target}"
CMDR_POSTGRES_URL="${CMDR_POSTGRES_URL:-postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable}"
CMDR_OTLP_URL="${CMDR_OTLP_URL:-http://127.0.0.1:4318}"
CMDR_API_URL="${CMDR_API_URL:-http://127.0.0.1:8080/api/v1}"
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
GO_BIN="${GO_BIN:-}"

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

run_migration_demo_agent() {
  run_cmdr demo internal migration-agent "$@"
}

run_migration_demo_verify() {
  run_cmdr demo internal helper "$@"
}

run_freeze_cmd() {
  (
    cd "$FREEZE_DIR"
    "$GO_BIN" run "$@"
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

wait_for_trace_detail() {
  local trace_id="$1"
  local expected_steps="$2"
  local expected_tools="$3"
  local label="$4"
  run_migration_demo_verify wait-trace \
    --api-url "$CMDR_API_URL" \
    --trace-id "$trace_id" \
    --steps "$expected_steps" \
    --tools "$expected_tools" \
    --timeout 30s || {
      echo "timed out waiting for $label"
      return 1
    }
}

wait_for_trace_tool_error() {
  local trace_id="$1"
  local expected_substring="$2"
  local label="$3"
  run_migration_demo_verify wait-trace \
    --api-url "$CMDR_API_URL" \
    --trace-id "$trace_id" \
    --steps 2 \
    --tools 1 \
    --error-substring "$expected_substring" \
    --timeout 30s || {
      echo "timed out waiting for $label"
      return 1
    }
}

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo not found in PATH"
  exit 1
fi

ensure_agentgateway_bin

docker compose -f docker-compose.dev.yml up -d postgres >/dev/null

if ! run_migration_demo_verify wait-postgres --url "$CMDR_POSTGRES_URL" --timeout 30s; then
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
wait_for_http "$CMDR_API_URL/health" "CMDR API"

if ! curl -sSf "$FREEZE_URL/health" >/dev/null 2>&1; then
  (
    export CMDR_POSTGRES_URL="$CMDR_POSTGRES_URL"
    run_freeze_cmd ./cmd/freeze-mcp-migrate
    FREEZE_TRACE_ID="migration-demo-default" run_freeze_cmd ./cmd/freeze-mcp
  ) >"$FREEZE_LOG" 2>&1 &
  FREEZE_PID=$!
  disown "$FREEZE_PID" >/dev/null 2>&1 || true
  wait_for_http "$FREEZE_URL/health" "freeze-mcp"
fi

(
  cd "$ROOT_DIR"
  "$GO_BIN" run ./cmd/mock-migration-mcp
) >"$MCP_LOG" 2>&1 &
MCP_PID=$!
disown "$MCP_PID" >/dev/null 2>&1 || true
wait_for_http "$MIGRATION_MCP_URL/health" "mock migration MCP server"

(
  cd "$ROOT_DIR"
  "$GO_BIN" run ./cmd/mock-openai-upstream --mode migration --port 18083
) >"$LLM_LOG" 2>&1 &
LLM_PID=$!
disown "$LLM_PID" >/dev/null 2>&1 || true
wait_for_port "18083" "mock migration OpenAI upstream"

(
  "$AGW_BIN" -f "$ROOT_DIR/scripts/agentgateway-migration-capture.yaml"
) >"$CAPTURE_AGW_LOG" 2>&1 &
CAPTURE_AGW_PID=$!
disown "$CAPTURE_AGW_PID" >/dev/null 2>&1 || true
wait_for_port "3102" "capture agentgateway AI proxy"
wait_for_port "3103" "capture agentgateway MCP proxy"

BASELINE_TRACE_ID="$(run_migration_demo_verify random-hex --bytes 16)"
SAFE_REPLAY_TRACE_ID="$(run_migration_demo_verify random-hex --bytes 16)"
UNSAFE_REPLAY_TRACE_ID="$(run_migration_demo_verify random-hex --bytes 16)"

echo "Running baseline capture trace $BASELINE_TRACE_ID"
run_migration_demo_agent \
  --mode capture \
  --model migration-safe \
  --behavior safe \
  --trace-id "$BASELINE_TRACE_ID" \
  --llm-url "$CAPTURE_LLM_URL" \
  --mcp-url "$CAPTURE_MCP_URL" \
  --expect-final-substring "Migration completed safely"

wait_for_trace_detail "$BASELINE_TRACE_ID" "5" "4" "baseline trace detail"

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
  "$AGW_BIN" -f "$ROOT_DIR/scripts/agentgateway-migration-replay.yaml"
) >"$REPLAY_AGW_LOG" 2>&1 &
REPLAY_AGW_PID=$!
disown "$REPLAY_AGW_PID" >/dev/null 2>&1 || true
wait_for_port "3202" "replay agentgateway AI proxy"
wait_for_port "3203" "replay agentgateway MCP proxy"

echo "Running safe frozen replay trace $SAFE_REPLAY_TRACE_ID"
run_migration_demo_agent \
  --mode replay \
  --model migration-safe \
  --behavior safe \
  --trace-id "$SAFE_REPLAY_TRACE_ID" \
  --freeze-trace-id "$BASELINE_TRACE_ID" \
  --llm-url "$REPLAY_LLM_URL" \
  --mcp-url "$REPLAY_MCP_URL" \
  --expect-final-substring "Migration completed safely"

wait_for_trace_detail "$SAFE_REPLAY_TRACE_ID" "5" "4" "safe replay trace detail"

echo "Running unsafe frozen replay trace $UNSAFE_REPLAY_TRACE_ID"
run_migration_demo_agent \
  --mode replay \
  --model migration-unsafe \
  --behavior unsafe \
  --trace-id "$UNSAFE_REPLAY_TRACE_ID" \
  --freeze-trace-id "$BASELINE_TRACE_ID" \
  --llm-url "$REPLAY_LLM_URL" \
  --mcp-url "$REPLAY_MCP_URL" \
  --expect-final-substring "blocked" \
  --expect-tool-error

wait_for_trace_detail "$UNSAFE_REPLAY_TRACE_ID" "2" "1" "unsafe replay trace detail"
wait_for_trace_tool_error "$UNSAFE_REPLAY_TRACE_ID" "tool_not_captured" "unsafe replay tool error"

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
  run_migration_demo_verify write-summary \
    --output "$REPORT_SUMMARY_FILE" \
    --baseline-trace-id "$BASELINE_TRACE_ID" \
    --safe-replay-trace-id "$SAFE_REPLAY_TRACE_ID" \
    --unsafe-replay-trace-id "$UNSAFE_REPLAY_TRACE_ID" \
    --run-log "$RUN_LOG_FILE" \
    --cmdr-log "$CMDR_LOG" \
    --freeze-log "$FREEZE_LOG" \
    --migration-mcp-log "$MCP_LOG" \
    --mock-llm-log "$LLM_LOG" \
    --capture-agentgateway-log "$CAPTURE_AGW_LOG" \
    --replay-agentgateway-log "$REPLAY_AGW_LOG" \
    --safe-verdict-log "$SAFE_VERDICT_LOG" \
    --unsafe-verdict-log "$UNSAFE_VERDICT_LOG"
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
