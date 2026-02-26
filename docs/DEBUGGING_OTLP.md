# Debugging OTLP Receiver - Traces Not Being Stored

## Quick Diagnosis

Run this test program to isolate the issue:

```bash
# Start PostgreSQL
make dev-up

# Run parser test (this tests the parser without HTTP)
go run test/manual/test_parser.go
```

This will show you exactly where the problem is:
- ✅ If parser works → Issue is in HTTP/gRPC receiver
- ❌ If parser fails → Issue is in parsing logic

## Step-by-Step Debugging

### 1. Check CMDR is Running

```bash
# Terminal 1: Start CMDR
make run
```

Look for these log lines:
```json
{"level":"info","msg":"OTLP receiver started","grpc_endpoint":"0.0.0.0:4317","http_endpoint":"0.0.0.0:4318"}
```

If you don't see this → OTLP receiver didn't start

### 2. Check Ports are Open

```bash
# Check if ports are listening
lsof -i :4317  # gRPC
lsof -i :4318  # HTTP
```

Should show `cmdr` process listening.

### 3. Send Simple Test Trace

```bash
# Terminal 2: Send test trace
curl -v -X POST http://localhost:4318/v1/traces \
  -H "Content-Type: application/json" \
  -d '{
    "resourceSpans": [{
      "resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "test"}}]},
      "scopeSpans": [{
        "spans": [{
          "traceId": "01020304050607080910111213141516",
          "spanId": "0102030405060708",
          "name": "test",
          "startTimeUnixNano": "1000000000000000000",
          "endTimeUnixNano": "1000000001000000000",
          "attributes": [
            {"key": "gen_ai.request.model", "value": {"stringValue": "claude-3-5-sonnet-20241022"}},
            {"key": "gen_ai.system", "value": {"stringValue": "anthropic"}},
            {"key": "gen_ai.prompt.0.role", "value": {"stringValue": "user"}},
            {"key": "gen_ai.prompt.0.content", "value": {"stringValue": "test"}},
            {"key": "gen_ai.completion.0.content", "value": {"stringValue": "test"}},
            {"key": "gen_ai.usage.input_tokens", "value": {"intValue": "5"}},
            {"key": "gen_ai.usage.output_tokens", "value": {"intValue": "3"}}
          ]
        }]
      }]
    }]
  }'
```

**What to look for**:
- HTTP status code (should be 200)
- Response body (should be `{}`)

### 4. Check CMDR Logs

In Terminal 1 (where CMDR is running), you should see:

**If working correctly**:
```json
{"level":"debug","msg":"Stored LLM trace","trace_id":"01020304...","model":"claude-3-5-sonnet-20241022","tokens":8}
```

**If not working**:
- No debug logs → Trace received but not processed
- Warning logs → Storage error
- Error logs → Parsing error

**Common log messages**:
```json
{"level":"warn","msg":"Failed to store OTEL trace","trace_id":"...","error":"..."}
{"level":"warn","msg":"Failed to store replay trace","trace_id":"...","error":"..."}
```

### 5. Check Database Tables

```bash
# Set database URL
export CMDR_POSTGRES_URL=postgres://cmdr:cmdr_dev_password@localhost:5432/cmdr?sslmode=disable

# Check OTEL traces (raw - should exist even if parsing fails)
psql $CMDR_POSTGRES_URL -c "SELECT COUNT(*) FROM otel_traces;"

# Check replay traces (parsed - only if gen_ai.* attributes found)
psql $CMDR_POSTGRES_URL -c "SELECT COUNT(*) FROM replay_traces;"

# Check for any data
psql $CMDR_POSTGRES_URL -c "
SELECT
    (SELECT COUNT(*) FROM otel_traces) as otel_count,
    (SELECT COUNT(*) FROM replay_traces) as replay_count,
    (SELECT COUNT(*) FROM tool_captures) as tool_count;
"
```

**Diagnosis**:
- `otel_count > 0, replay_count = 0` → Traces received but not recognized as LLM spans
- `otel_count = 0` → Traces not reaching storage at all
- `replay_count > 0` → It's working! (Check trace IDs)

### 6. Check Raw OTEL Attributes

```bash
# See what attributes were actually stored
psql $CMDR_POSTGRES_URL -c "
SELECT
    trace_id,
    attributes::text
FROM otel_traces
ORDER BY created_at DESC
LIMIT 1;
" | head -50
```

This shows the actual OTEL attributes received. Look for `gen_ai.*` keys.

### 7. Enable More Verbose Logging

Edit `.env`:
```bash
CMDR_LOG_LEVEL=debug
```

Restart CMDR:
```bash
# Stop: Ctrl+C
# Start again:
make run
```

Now send trace again and watch for detailed logs.

## Common Issues and Fixes

### Issue 1: "No gen_ai.* attributes found"

**Symptom**: `otel_traces` has data but `replay_traces` is empty

**Cause**: Spans don't have `gen_ai.*` attributes

**Fix**: Check your trace format. The span must have at least:
```json
{"key": "gen_ai.request.model", "value": {"stringValue": "..."}}
```

### Issue 2: "Failed to store replay trace: duplicate key"

**Symptom**: Warning in logs about duplicate trace_id

**Cause**: Trying to store same trace twice

**Fix**: This is expected if you send the same trace multiple times. Use unique trace IDs for testing.

### Issue 3: "Failed to store replay trace: foreign key violation"

**Symptom**: Error about replay_traces foreign key

**Cause**: Database schema issue

**Fix**: Reset database
```bash
make dev-reset
make run
```

### Issue 4: HTTP unmarshaling error

**Symptom**: "Failed to unmarshal HTTP traces"

**Cause**: Invalid OTLP JSON format

**Fix**: Use the exact format from `scripts/test-otlp.sh` or run the manual test:
```bash
go run test/manual/test_parser.go
```

## Debug Checklist

Run through this checklist:

- [ ] PostgreSQL is running (`docker compose ps postgres` shows healthy)
- [ ] CMDR binary built successfully (`make build`)
- [ ] CMDR is running (`make run` shows "service started")
- [ ] OTLP receiver started (logs show both endpoints)
- [ ] Log level is debug (`CMDR_LOG_LEVEL=debug` in `.env`)
- [ ] HTTP request returns 200 OK
- [ ] Trace has `gen_ai.request.model` attribute
- [ ] `otel_traces` table has entries
- [ ] CMDR logs show "Stored LLM trace" (debug level)

## Share Logs for Help

If still not working, share:

1. **CMDR startup logs** (first 10 lines):
```bash
make run 2>&1 | head -20
```

2. **HTTP test response**:
```bash
curl -v http://localhost:4318/v1/traces ...
```

3. **Database counts**:
```bash
psql $CMDR_POSTGRES_URL -c "SELECT COUNT(*) FROM otel_traces, replay_traces;"
```

4. **Parser test output**:
```bash
go run test/manual/test_parser.go
```

This will help identify exactly where the issue is!
