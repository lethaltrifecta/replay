# Testing OTLP Receiver - Step by Step

This guide walks you through testing the OTLP receiver to ensure it correctly ingests and stores OTEL traces.

## Prerequisites

Before testing:
- [ ] Go dependencies downloaded (`go mod download`)
- [ ] CMDR binary built (`make build`)
- [ ] PostgreSQL running (`make dev-up`)
- [ ] `.env` file configured

## Step 1: Download Dependencies and Build

```bash
# Download OpenTelemetry dependencies
go mod download

# Tidy dependencies
go mod tidy

# Build CMDR
make build
```

**Expected output**:
```
Building cmdr...
✅ Binary created at: bin/cmdr
```

## Step 2: Start Development Services

```bash
# Start PostgreSQL and Jaeger
make dev-up
```

**Expected output**:
```
Starting development services with docker compose...
✅ Development services started
   - PostgreSQL: localhost:5432
   - Jaeger UI:  http://localhost:16686
```

**Verify services are running**:
```bash
docker compose ps
```

You should see:
- `cmdr-postgres` - healthy
- `cmdr-jaeger` - running

## Step 3: Verify Configuration

```bash
# Check .env file
cat .env | grep -v '^#' | grep -v '^$'
```

**Required settings**:
```
CMDR_API_PORT=8080
CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable
CMDR_AGENTGATEWAY_URL=http://localhost:8080
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318
```

## Step 4: Start CMDR Service

In a terminal, run:

```bash
make run
```

**Expected output**:
```json
{"level":"info","msg":"Starting CMDR service","version":"0.1.0","api_port":8080,"otlp_grpc":"0.0.0.0:4317","otlp_http":"0.0.0.0:4318"}
{"level":"info","msg":"Database connected and migrated successfully"}
{"level":"info","msg":"OTLP receiver started","grpc_endpoint":"0.0.0.0:4317","http_endpoint":"0.0.0.0:4318"}
{"level":"info","msg":"CMDR service started successfully"}
```

**If you see errors**:
- Database connection error → Check PostgreSQL is running (`docker compose ps postgres`)
- Port already in use → Change ports in `.env`

## Step 5: Run Automated Test (Recommended)

In another terminal:

```bash
# Make script executable
chmod +x scripts/test-otlp.sh

# Run test
./scripts/test-otlp.sh
```

**Expected output**:
```
🧪 Testing OTLP Receiver...
📤 Sending test OTLP trace via HTTP...
✅ Trace sent successfully!
🔍 Checking database for stored traces...

Traces in database:
 trace_id | model                         | provider   | total_tokens | latency_ms
----------+-------------------------------+------------+--------------+------------
 01020... | claude-3-5-sonnet-20241022   | anthropic  |           18 |       1000

Tool captures:
 trace_id | step_index | tool_name  | risk_class
----------+------------+------------+------------
 01020... |          0 | calculator | read

✅ Test complete!
```

## Step 6: Manual Testing (Alternative)

### Test 1: Send Simple HTTP Trace

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "test-service"}
        }]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
          "spanId": "1234567890abcdef",
          "name": "llm.completion",
          "startTimeUnixNano": "1234567890000000000",
          "endTimeUnixNano": "1234567891000000000",
          "attributes": [
            {
              "key": "gen_ai.request.model",
              "value": {"stringValue": "gpt-4"}
            },
            {
              "key": "gen_ai.system",
              "value": {"stringValue": "openai"}
            },
            {
              "key": "gen_ai.prompt.0.role",
              "value": {"stringValue": "user"}
            },
            {
              "key": "gen_ai.prompt.0.content",
              "value": {"stringValue": "Hello!"}
            },
            {
              "key": "gen_ai.completion.0.content",
              "value": {"stringValue": "Hi there!"}
            },
            {
              "key": "gen_ai.usage.input_tokens",
              "value": {"intValue": "5"}
            },
            {
              "key": "gen_ai.usage.output_tokens",
              "value": {"intValue": "3"}
            }
          ]
        }]
      }]
    }]
  }'
```

**Expected response**: `{}`

### Test 2: Verify in Database

```bash
# Set database URL
export CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable

# Query replay traces
psql $CMDR_POSTGRES_URL -c "SELECT trace_id, model, provider, prompt_tokens, completion_tokens, total_tokens FROM replay_traces;"

# Query OTEL traces (raw)
psql $CMDR_POSTGRES_URL -c "SELECT trace_id, service_name, span_kind FROM otel_traces;"

# Query tool captures
psql $CMDR_POSTGRES_URL -c "SELECT trace_id, tool_name, args, risk_class FROM tool_captures;"
```

### Test 3: Check CMDR Logs

In the terminal where CMDR is running, you should see:

```json
{"level":"debug","msg":"Stored LLM trace","trace_id":"01020...","model":"claude-3-5-sonnet-20241022","tokens":18}
{"level":"debug","msg":"Stored tool capture","trace_id":"01020...","tool_name":"calculator","step_index":0}
```

## Step 7: Test with Tool Calls

Send a trace with tool calls:

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{
          "key": "service.name",
          "value": {"stringValue": "agent-service"}
        }]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "f1e2d3c4b5a69788970605040302010",
          "spanId": "fedcba9876543210",
          "name": "llm.completion",
          "startTimeUnixNano": "1234567890000000000",
          "endTimeUnixNano": "1234567891500000000",
          "attributes": [
            {"key": "gen_ai.system", "value": {"stringValue": "anthropic"}},
            {"key": "gen_ai.request.model", "value": {"stringValue": "claude-3-5-sonnet-20241022"}},
            {"key": "gen_ai.prompt.0.role", "value": {"stringValue": "user"}},
            {"key": "gen_ai.prompt.0.content", "value": {"stringValue": "Search for documentation on authentication"}},
            {"key": "gen_ai.completion.0.content", "value": {"stringValue": "I will search for authentication documentation."}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": "15"}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": "12"}}
          ],
          "events": [
            {
              "timeUnixNano": "1234567890500000000",
              "name": "tool.call",
              "attributes": [
                {"key": "tool.name", "value": {"stringValue": "search_docs"}},
                {"key": "tool.args", "value": {"stringValue": "{\\\"query\\\": \\\"authentication\\\", \\\"limit\\\": 10}"}},
                {"key": "tool.result", "value": {"stringValue": "{\\\"results\\\": [{\\\"title\\\": \\\"Auth Guide\\\", \\\"content\\\": \\\"...\\\"}]}"}},
                {"key": "tool.latency_ms", "value": {"intValue": "250"}}
              ]
            }
          ]
        }]
      }]
    }]
  }'
```

**Verify tool capture**:
```bash
psql $CMDR_POSTGRES_URL -c "
SELECT
    trace_id,
    step_index,
    tool_name,
    args->>'query' as query,
    risk_class
FROM tool_captures
ORDER BY created_at DESC
LIMIT 5;
"
```

## Step 8: Test Destructive Tool Classification

Send a trace with a destructive tool:

```bash
curl -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {
        "attributes": [{"key": "service.name", "value": {"stringValue": "agent"}}]
      },
      "scopeSpans": [{
        "spans": [{
          "traceId": "aabbccddeeff00112233445566778899",
          "spanId": "1122334455667788",
          "name": "llm.completion",
          "startTimeUnixNano": "1234567890000000000",
          "endTimeUnixNano": "1234567891000000000",
          "attributes": [
            {"key": "gen_ai.system", "value": {"stringValue": "openai"}},
            {"key": "gen_ai.request.model", "value": {"stringValue": "gpt-4"}},
            {"key": "gen_ai.prompt.0.role", "value": {"stringValue": "user"}},
            {"key": "gen_ai.prompt.0.content", "value": {"stringValue": "Delete old files"}},
            {"key": "gen_ai.completion.0.content", "value": {"stringValue": "I will delete the old files."}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": "8"}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": "10"}}
          ],
          "events": [{
            "timeUnixNano": "1234567890500000000",
            "name": "tool.call",
            "attributes": [
              {"key": "tool.name", "value": {"stringValue": "delete_files"}},
              {"key": "tool.args", "value": {"stringValue": "{\\\"pattern\\\": \\\"*.tmp\\\"}"}},
              {"key": "tool.result", "value": {"stringValue": "{\\\"deleted\\\": 5}"}}
            ]
          }]
        }]
      }]
    }]
  }'
```

**Verify risk class**:
```bash
psql $CMDR_POSTGRES_URL -c "
SELECT tool_name, risk_class
FROM tool_captures
WHERE tool_name LIKE '%delete%';
"
```

**Expected**: `risk_class = 'destructive'`

## Step 9: Query Statistics

```bash
# Count traces by model
psql $CMDR_POSTGRES_URL -c "
SELECT model, provider, COUNT(*) as count
FROM replay_traces
GROUP BY model, provider
ORDER BY count DESC;
"

# Count tool calls by type
psql $CMDR_POSTGRES_URL -c "
SELECT tool_name, risk_class, COUNT(*) as count
FROM tool_captures
GROUP BY tool_name, risk_class
ORDER BY count DESC;
"

# Average tokens by model
psql $CMDR_POSTGRES_URL -c "
SELECT
    model,
    AVG(prompt_tokens) as avg_prompt_tokens,
    AVG(completion_tokens) as avg_completion_tokens,
    AVG(latency_ms) as avg_latency_ms
FROM replay_traces
GROUP BY model;
"
```

## Test Results Checklist

After running tests, verify:

- [ ] HTTP POST to `/v1/traces` returns 200 OK
- [ ] Traces appear in `replay_traces` table
- [ ] Tool calls appear in `tool_captures` table
- [ ] Raw OTEL data stored in `otel_traces` table
- [ ] Model and provider parsed correctly
- [ ] Prompt tokens and completion tokens calculated correctly
- [ ] Tool risk classes determined correctly (read/write/destructive)
- [ ] Args hash calculated (for Freeze-Tools lookup)
- [ ] CMDR logs show successful storage
- [ ] No errors in CMDR output

## Troubleshooting

### HTTP Request Returns Error

**Check CMDR logs**: Look for parsing errors or storage failures

**Common issues**:
- Invalid JSON format
- Missing required trace fields
- Database connection lost

### Traces Not Appearing in Database

**Check**:
1. Is CMDR running? (`ps aux | grep cmdr`)
2. Are spans marked as LLM spans? (must have `gen_ai.*` attributes)
3. Check CMDR debug logs for warnings

**Debug**:
```bash
# Check OTEL traces (raw) were stored
psql $CMDR_POSTGRES_URL -c "SELECT COUNT(*) FROM otel_traces;"

# If raw traces exist but replay_traces doesn't:
# → Parser didn't detect gen_ai.* attributes
# → Check span has gen_ai.request.model attribute
```

### Database Connection Error

```bash
# Verify PostgreSQL is running
docker compose ps postgres

# Test connection
psql $CMDR_POSTGRES_URL -c "SELECT 1;"

# Check connection string in .env
echo $CMDR_POSTGRES_URL
```

## Performance Testing

### Send Multiple Traces

```bash
# Send 10 traces rapidly
for i in {1..10}; do
  curl -X POST http://localhost:4318/v1/traces \
    -H "Content-Type: application/json" \
    -d "{ ... }" &
done
wait

# Check all were stored
psql $CMDR_POSTGRES_URL -c "SELECT COUNT(*) FROM replay_traces;"
```

### Check CMDR Performance

```bash
# Watch CMDR logs for processing time
# Each trace should be processed in < 100ms
```

## Cleanup After Testing

```bash
# Clear test data
psql $CMDR_POSTGRES_URL -c "TRUNCATE TABLE tool_captures, replay_traces, otel_traces CASCADE;"

# Or reset entire database
make dev-reset
```

## Success Criteria

✅ **OTLP Receiver is Working** if:
1. HTTP POST to `/v1/traces` returns 200
2. Traces stored in database within 1 second
3. All gen_ai.* attributes parsed correctly
4. Tool calls extracted from events
5. Risk classes assigned appropriately
6. No errors in logs
7. Service handles multiple concurrent requests

## Next Steps

Once OTLP receiver tests pass:

1. **Test with real agentgateway**:
   - Configure agentgateway to export to `localhost:4317`
   - Run actual agent requests
   - Verify traces are captured

2. **Implement Freeze-Tools**:
   - Now that we can capture tool calls
   - Next: Replay them deterministically

3. **Build experiment orchestration**:
   - Create experiments
   - Replay with variants
   - Run analysis

---

**Current Test Status**: Ready to test

**Run this to test everything**:
```bash
# Terminal 1: Start CMDR
make dev-up && make run

# Terminal 2: Run tests
chmod +x scripts/test-otlp.sh
./scripts/test-otlp.sh
```
