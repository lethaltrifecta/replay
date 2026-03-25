#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGW_CONFIG="${AGW_CONFIG:-$ROOT_DIR/scripts/agentgateway-cmdr-capture.yaml}"
AGW_PORT="${AGW_PORT:-3001}"
MOCK_PORT="${MOCK_PORT:-18080}"
AGW_BIN="${AGW_BIN:-}"
AGW_CARGO_TARGET_DIR="${AGW_CARGO_TARGET_DIR:-/tmp/agentgateway-quick-target}"
CMDR_OTLP_HEALTH_URL="${CMDR_OTLP_HEALTH_URL:-http://127.0.0.1:4318/health}"
CMDR_API_URL="${CMDR_API_URL:-http://127.0.0.1:8080/api/v1}"
GO_BIN="${GO_BIN:-}"
CMDR_BIN="${CMDR_BIN:-}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PROMPT_TEXT="${PROMPT_TEXT:-Explain the outage briefly. run ${RUN_ID}}"
MOCK_TEXT="${MOCK_TEXT:-Mock response from local upstream.}"

MOCK_LOG="${MOCK_LOG:-/tmp/replay-mock-openai.log}"
AGW_LOG="${AGW_LOG:-/tmp/replay-agentgateway.log}"
REQUEST_BODY="{\"model\":\"mock-model\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT_TEXT\"}],\"stream\":false}"

MOCK_PID=""
AGW_PID=""

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

if ! curl -sSf "$CMDR_OTLP_HEALTH_URL" >/dev/null 2>&1; then
  echo "CMDR OTLP receiver is not listening on $CMDR_OTLP_HEALTH_URL"
  echo "Start it first with cmdr serve"
  exit 1
fi

if ! curl -sSf "$CMDR_API_URL/health" >/dev/null 2>&1; then
  echo "CMDR API is not listening on $CMDR_API_URL/health"
  echo "Start it first with cmdr serve"
  exit 1
fi

(
  cd "$ROOT_DIR"
  "$GO_BIN" run ./cmd/mock-openai-upstream \
    --mode basic \
    --port "$MOCK_PORT" \
    --response-text "$MOCK_TEXT"
) >"$MOCK_LOG" 2>&1 &
MOCK_PID=$!
disown "$MOCK_PID" >/dev/null 2>&1 || true

for _ in $(seq 1 30); do
  if lsof -nP -iTCP:"$MOCK_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! lsof -nP -iTCP:"$MOCK_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "mock upstream failed to start"
  exit 1
fi

(
  "$AGW_BIN" -f "$AGW_CONFIG"
) >"$AGW_LOG" 2>&1 &
AGW_PID=$!
disown "$AGW_PID" >/dev/null 2>&1 || true

for _ in $(seq 1 300); do
  if lsof -nP -iTCP:"$AGW_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! lsof -nP -iTCP:"$AGW_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
  echo "agentgateway failed to start; see $AGW_LOG"
  exit 1
fi

curl -sS "http://127.0.0.1:$AGW_PORT/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d "$REQUEST_BODY"
echo

TRACE_DETAIL_JSON="$(
  run_cmdr demo internal helper wait-trace-content \
    --api-url "$CMDR_API_URL" \
    --prompt "$PROMPT_TEXT" \
    --completion "$MOCK_TEXT" \
    --timeout 30s
)"

echo "$TRACE_DETAIL_JSON"

echo "mock upstream log: $MOCK_LOG"
echo "agentgateway log: $AGW_LOG"
