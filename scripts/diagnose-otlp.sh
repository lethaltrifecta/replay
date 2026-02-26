#!/bin/bash

echo "🔍 CMDR OTLP Receiver Diagnostics"
echo "=================================="
echo ""

echo "1. Checking if CMDR process is running..."
if pgrep -f "cmdr serve" > /dev/null; then
    echo "   ✅ CMDR process is running"
    ps aux | grep "[c]mdr serve"
else
    echo "   ❌ CMDR process NOT running"
    echo "   → Start with: make run"
    exit 1
fi

echo ""
echo "2. Checking if ports are listening..."

if lsof -i :4318 > /dev/null 2>&1; then
    echo "   ✅ Port 4318 (OTLP HTTP) is listening"
    lsof -i :4318 | grep -v COMMAND
else
    echo "   ❌ Port 4318 (OTLP HTTP) is NOT listening"
    echo "   → OTLP receiver failed to start"
fi

if lsof -i :4317 > /dev/null 2>&1; then
    echo "   ✅ Port 4317 (OTLP gRPC) is listening"
    lsof -i :4317 | grep -v COMMAND
else
    echo "   ❌ Port 4317 (OTLP gRPC) is NOT listening"
    echo "   → OTLP receiver failed to start"
fi

echo ""
echo "3. Checking database connection..."
export CMDR_POSTGRES_URL=${CMDR_POSTGRES_URL:-"postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable"}

if psql "$CMDR_POSTGRES_URL" -c "SELECT 1" > /dev/null 2>&1; then
    echo "   ✅ Database connection works"
else
    echo "   ❌ Cannot connect to database"
    echo "   → Check: make dev-up"
    exit 1
fi

echo ""
echo "4. Checking database tables..."
TABLE_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('otel_traces', 'replay_traces', 'tool_captures');" 2>/dev/null)

if [ "$TABLE_COUNT" -eq 3 ]; then
    echo "   ✅ All required tables exist"
else
    echo "   ❌ Missing tables (found $TABLE_COUNT of 3)"
    echo "   → Migrations might have failed"
fi

echo ""
echo "5. Testing HTTP endpoint..."
HTTP_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:4318/health 2>/dev/null)

if [ "$HTTP_RESPONSE" = "200" ]; then
    echo "   ✅ HTTP endpoint responds (health check)"
else
    echo "   ⚠️  HTTP endpoint not responding (got $HTTP_RESPONSE)"
    echo "   → Receiver might not be fully started"
fi

echo ""
echo "6. Sending test trace..."
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "test"}}]},
      "scopeSpans": [{
        "spans": [{
          "traceId": "99999999999999999999999999999999",
          "spanId": "9999999999999999",
          "name": "test",
          "startTimeUnixNano": "1000000000000000000",
          "endTimeUnixNano": "1000000001000000000",
          "attributes": [
            {"key": "gen_ai.request.model", "value": {"stringValue": "test-model"}},
            {"key": "gen_ai.system", "value": {"stringValue": "test-provider"}},
            {"key": "gen_ai.prompt.0.role", "value": {"stringValue": "user"}},
            {"key": "gen_ai.prompt.0.content", "value": {"stringValue": "test"}},
            {"key": "gen_ai.completion.0.content", "value": {"stringValue": "test"}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": "1"}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": "1"}}
          ]
        }]
      }]
    }]
  }')

if [ "$RESPONSE" = "200" ]; then
    echo "   ✅ Trace accepted (HTTP 200)"
else
    echo "   ❌ Trace rejected (HTTP $RESPONSE)"
fi

echo ""
echo "7. Checking if trace was stored (wait 2 seconds)..."
sleep 2

OTEL_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "SELECT COUNT(*) FROM otel_traces;" 2>/dev/null | xargs)
REPLAY_COUNT=$(psql "$CMDR_POSTGRES_URL" -t -c "SELECT COUNT(*) FROM replay_traces;" 2>/dev/null | xargs)

echo "   OTEL traces: $OTEL_COUNT"
echo "   Replay traces: $REPLAY_COUNT"

if [ "$OTEL_COUNT" -gt 0 ]; then
    echo "   ✅ Raw OTEL traces are being stored"
else
    echo "   ❌ No OTEL traces stored - receiver not processing"
fi

if [ "$REPLAY_COUNT" -gt 0 ]; then
    echo "   ✅ Replay traces are being parsed and stored"
else
    echo "   ⚠️  Replay traces not stored - parser might not detect LLM spans"
fi

echo ""
echo "=================================="
echo "Diagnosis:"
echo ""

if [ "$OTEL_COUNT" -eq 0 ]; then
    echo "❌ PROBLEM: Traces not reaching storage"
    echo "   Possible causes:"
    echo "   1. OTLP receiver failed to start (check ports above)"
    echo "   2. HTTP unmarshaling is failing"
    echo "   3. Database connection issue during storage"
    echo ""
    echo "   Check CMDR logs for errors:"
    echo "   Look for 'Failed to unmarshal' or 'Failed to store' messages"
elif [ "$REPLAY_COUNT" -eq 0 ]; then
    echo "⚠️  PROBLEM: Traces received but not parsed as LLM spans"
    echo "   Possible causes:"
    echo "   1. IsLLMSpan() not detecting gen_ai.* attributes"
    echo "   2. ParseLLMSpan() returning nil"
    echo ""
    echo "   Run parser unit test:"
    echo "   go run test/manual/test_parser.go"
else
    echo "✅ Everything working! Traces are being stored."
fi

echo ""
