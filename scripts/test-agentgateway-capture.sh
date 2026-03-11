#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGENTGATEWAY_DIR="${AGENTGATEWAY_DIR:-$ROOT_DIR/../agentgateway}"
AGW_CONFIG="${AGW_CONFIG:-$ROOT_DIR/scripts/agentgateway-cmdr-capture.yaml}"
CMDR_POSTGRES_URL="${CMDR_POSTGRES_URL:-postgres://cmdr@localhost:5432/cmdr?sslmode=disable}"
AGW_PORT="${AGW_PORT:-3001}"
MOCK_PORT="${MOCK_PORT:-18080}"
RUN_ID="${RUN_ID:-$(date +%s)}"
PROMPT_TEXT="${PROMPT_TEXT:-Explain the outage briefly. run ${RUN_ID}}"
MOCK_TEXT="${MOCK_TEXT:-Mock response from local upstream.}"

MOCK_LOG="${MOCK_LOG:-/tmp/replay-mock-openai.log}"
AGW_LOG="${AGW_LOG:-/tmp/replay-agentgateway.log}"
REQUEST_BODY="{\"model\":\"mock-model\",\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT_TEXT\"}],\"stream\":false}"

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

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo not found in PATH"
  exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 not found in PATH"
  exit 1
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "psql not found in PATH"
  exit 1
fi

if ! curl -sSf http://127.0.0.1:4318/health >/dev/null 2>&1; then
  echo "CMDR is not listening on http://127.0.0.1:4318/health"
  echo "Start it first with cmdr serve"
  exit 1
fi

python3 "$ROOT_DIR/scripts/mock_openai_upstream.py" \
  --port "$MOCK_PORT" \
  --response-text "$MOCK_TEXT" \
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
  echo "mock upstream failed to start"
  exit 1
fi

(
  cd "$AGENTGATEWAY_DIR"
  CARGO_TARGET_DIR=/tmp/agentgateway-target cargo run -p agentgateway-app -- -f "$AGW_CONFIG"
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

TRACE_ID=""
for _ in $(seq 1 30); do
  TRACE_ID="$(psql -X -A -t -P pager=off "$CMDR_POSTGRES_URL" -c \
    "SELECT trace_id
     FROM replay_traces
     WHERE prompt::text LIKE '%$PROMPT_TEXT%'
     ORDER BY created_at DESC
     LIMIT 1;" | tr -d '[:space:]')"
  if [ -n "$TRACE_ID" ]; then
    break
  fi
  sleep 1
done

if [ -z "$TRACE_ID" ]; then
  echo "timed out waiting for replay_traces row for prompt: $PROMPT_TEXT"
  exit 1
fi

psql -X -P pager=off "$CMDR_POSTGRES_URL" -c \
  "SELECT trace_id, provider, model, prompt::text, completion, prompt_tokens, completion_tokens
   FROM replay_traces
   WHERE trace_id = '$TRACE_ID';"

psql -X -P pager=off "$CMDR_POSTGRES_URL" -c \
  "SELECT trace_id, service_name, span_kind, attributes->>'gen_ai.provider.name' AS provider_name,
          attributes->>'gen_ai.prompt.0.content' AS prompt0
   FROM otel_traces
   WHERE trace_id = '$TRACE_ID';"

echo "mock upstream log: $MOCK_LOG"
echo "agentgateway log: $AGW_LOG"
