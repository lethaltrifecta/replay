#!/bin/bash

# Test OTLP Receiver Script

set -e

echo "🧪 Testing OTLP Receiver..."
echo ""

# Check if CMDR is running
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "⚠️  CMDR service not running. Start it with: make run"
    echo "   (In another terminal)"
    echo ""
    read -p "Press Enter when CMDR is running..."
fi

echo "📤 Sending test OTLP trace via HTTP..."
echo ""

TRACE_ID="01020304050607080910111213141516"
SPAN_ID="0102030405060708"
TIMESTAMP_START="1234567890000000000"
TIMESTAMP_END="1234567891000000000"

curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d "{
    \"resourceSpans\": [{
      \"resource\": {
        \"attributes\": [{
          \"key\": \"service.name\",
          \"value\": {\"stringValue\": \"test-agent\"}
        }]
      },
      \"scopeSpans\": [{
        \"spans\": [{
          \"traceId\": \"$TRACE_ID\",
          \"spanId\": \"$SPAN_ID\",
          \"name\": \"llm.completion\",
          \"kind\": 1,
          \"startTimeUnixNano\": \"$TIMESTAMP_START\",
          \"endTimeUnixNano\": \"$TIMESTAMP_END\",
          \"attributes\": [
            {
              \"key\": \"gen_ai.request.model\",
              \"value\": {\"stringValue\": \"claude-3-5-sonnet-20241022\"}
            },
            {
              \"key\": \"gen_ai.system\",
              \"value\": {\"stringValue\": \"anthropic\"}
            },
            {
              \"key\": \"gen_ai.prompt.0.role\",
              \"value\": {\"stringValue\": \"user\"}
            },
            {
              \"key\": \"gen_ai.prompt.0.content\",
              \"value\": {\"stringValue\": \"What is 2+2?\"}
            },
            {
              \"key\": \"gen_ai.completion.0.content\",
              \"value\": {\"stringValue\": \"2+2 equals 4.\"}
            },
            {
              \"key\": \"gen_ai.usage.input_tokens\",
              \"value\": {\"intValue\": \"10\"}
            },
            {
              \"key\": \"gen_ai.usage.output_tokens\",
              \"value\": {\"intValue\": \"8\"}
            },
            {
              \"key\": \"gen_ai.request.temperature\",
              \"value\": {\"doubleValue\": 0.7}
            },
            {
              \"key\": \"gen_ai.request.max_tokens\",
              \"value\": {\"intValue\": \"1024\"}
            }
          ],
          \"events\": [
            {
              \"timeUnixNano\": \"$TIMESTAMP_START\",
              \"name\": \"tool.call\",
              \"attributes\": [
                {
                  \"key\": \"tool.name\",
                  \"value\": {\"stringValue\": \"calculator\"}
                },
                {
                  \"key\": \"tool.args\",
                  \"value\": {\"stringValue\": \"{\\\"operation\\\": \\\"add\\\", \\\"a\\\": 2, \\\"b\\\": 2}\"}
                },
                {
                  \"key\": \"tool.result\",
                  \"value\": {\"stringValue\": \"{\\\"result\\\": 4}\"}
                },
                {
                  \"key\": \"tool.latency_ms\",
                  \"value\": {\"intValue\": \"5\"}
                }
              ]
            }
          ],
          \"status\": {
            \"code\": 1
          }
        }]
      }]
    }]
  }"

echo ""
echo "✅ Trace sent successfully!"
echo ""

# Wait a moment for processing
sleep 1

# Check database
echo "🔍 Checking database for stored traces..."
echo ""

if [ -z "$CMDR_POSTGRES_URL" ]; then
    export CMDR_POSTGRES_URL="postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable"
fi

echo "Traces in database:"
psql "$CMDR_POSTGRES_URL" -c "SELECT trace_id, model, provider, total_tokens, latency_ms FROM replay_traces ORDER BY created_at DESC LIMIT 5;" 2>/dev/null || echo "⚠️  Could not query database. Install psql or check connection."

echo ""
echo "Tool captures:"
psql "$CMDR_POSTGRES_URL" -c "SELECT trace_id, step_index, tool_name, risk_class FROM tool_captures ORDER BY created_at DESC LIMIT 5;" 2>/dev/null || echo "⚠️  Could not query database. Install psql or check connection."

echo ""
echo "✅ Test complete!"
echo ""
echo "📊 To view more data:"
echo "   psql \$CMDR_POSTGRES_URL"
echo "   cmdr=# SELECT * FROM replay_traces;"
echo "   cmdr=# SELECT * FROM tool_captures;"
echo ""
