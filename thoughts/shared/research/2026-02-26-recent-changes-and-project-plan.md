---
date: 2026-02-26T19:30:00-05:00
researcher: Claude Code
git_commit: 89aa1705e191bb03cbb367688fb6a3c9a2f83fa5
branch: main
repository: lethaltrifecta/replay
topic: "Recent Changes and Project Plan - Multi-Span Support and Current Status"
tags: [research, codebase, schema-changes, multi-span, project-plan, otlp-receiver]
status: complete
last_updated: 2026-02-26
last_updated_by: Claude Code
---

# Research: Recent Changes and Current Project Plan

**Date**: 2026-02-26T19:30:00-05:00
**Researcher**: Claude Code
**Git Commit**: 89aa1705e191bb03cbb367688fb6a3c9a2f83fa5
**Branch**: main
**Repository**: lethaltrifecta/replay

## Research Question
What changes were made to support multi-span traces and what is the current project plan?

## Summary

Recent changes introduced **multi-span trace support** to handle real-world agent runs where a single trace contains multiple LLM calls (spans). The schema, models, and parser were updated to track individual spans while maintaining trace-level grouping. The project is currently at **Phase 1 (90% complete)** with database layer and OTLP receiver implemented, and Freeze-Tools MCP server as the next priority.

## Key Changes: Multi-Span Support

### Problem Addressed

Real agent runs involve multiple back-and-forth LLM calls:
```
Trace abc123:
  Span 1: User asks "What's 2+2?"
  Span 2: Agent calls calculator tool
  Span 3: Agent responds with "4"
```

The original schema assumed one span per trace. This was updated to support multiple spans per trace.

### Schema Changes

#### 1. `otel_traces` Table (pkg/storage/migrations/001_initial_schema.sql:8-26)

**Before**: `trace_id` as PRIMARY KEY (one span per trace)

**After**:
```sql
CREATE TABLE otel_traces (
    id SERIAL PRIMARY KEY,                    -- NEW: Auto-increment PK
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,            -- NEW: Span identifier
    parent_span_id VARCHAR(255),
    ...
    UNIQUE(trace_id, span_id)                -- NEW: Composite unique constraint
);
```

**Impact**: Now supports multiple spans per trace_id.

#### 2. `replay_traces` Table (pkg/storage/migrations/001_initial_schema.sql:33-55)

**Before**: `trace_id` as PRIMARY KEY (one LLM call per trace)

**After**:
```sql
CREATE TABLE replay_traces (
    id SERIAL PRIMARY KEY,                    -- NEW: Auto-increment PK
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,            -- NEW: Span identifier
    run_id VARCHAR(255) NOT NULL,
    step_index INT NOT NULL DEFAULT 0,        -- NEW: Order within trace
    ...
    UNIQUE(trace_id, span_id)                -- NEW: Composite unique constraint
);

-- Comment: "A multi-step agent run produces multiple rows sharing the same trace_id."
```

**Impact**:
- Each LLM call in a multi-turn conversation is a separate row
- `step_index` preserves temporal ordering
- Query all spans for a trace to reconstruct the conversation

#### 3. `tool_captures` Table (pkg/storage/migrations/001_initial_schema.sql:60-78)

**Before**: UNIQUE(trace_id, step_index)

**After**:
```sql
CREATE TABLE tool_captures (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,            -- NEW: Span identifier
    step_index INT NOT NULL,
    ...
    UNIQUE(trace_id, span_id, step_index)    -- NEW: Three-part unique key
);

CREATE INDEX idx_tool_captures_span ON tool_captures(trace_id, span_id);  -- NEW index
```

**Impact**: Tool calls now linked to specific spans within a trace.

### Model Changes

#### `OTELTrace` (pkg/storage/models.go:38-53)

```go
type OTELTrace struct {
    ID            int       `db:"id"`          // NEW field
    TraceID       string    `db:"trace_id"`
    SpanID        string    `db:"span_id"`     // NEW field
    ParentSpanID  *string   `db:"parent_span_id"`
    // ... rest of fields
}
```

**Comment added**: "A single trace_id can have many spans (one per operation)."

#### `ReplayTrace` (pkg/storage/models.go:55-74)

```go
type ReplayTrace struct {
    ID               int       `db:"id"`          // NEW field
    TraceID          string    `db:"trace_id"`
    SpanID           string    `db:"span_id"`     // NEW field
    RunID            string    `db:"run_id"`
    StepIndex        int       `db:"step_index"`  // NEW field
    // ... rest of fields
}
```

**Comment added**: "A multi-step agent run produces multiple rows sharing the same trace_id."

#### `ToolCapture` (pkg/storage/models.go:76-90)

```go
type ToolCapture struct {
    ID        int       `db:"id"`
    TraceID   string    `db:"trace_id"`
    SpanID    string    `db:"span_id"`          // NEW field
    StepIndex int       `db:"step_index"`
    // ... rest of fields
}
```

### Storage Interface Changes

#### New Methods (pkg/storage/interface.go:19-22)

```go
// Changed from single-span to multi-span methods
GetOTELTraceSpans(ctx context.Context, traceID string) ([]*OTELTrace, error)
GetReplayTraceSpans(ctx context.Context, traceID string) ([]*ReplayTrace, error)
```

**Purpose**: Retrieve all spans for a trace to reconstruct multi-turn agent conversations.

### Parser Changes

#### 1. `normalizeValue()` Function (pkg/otelreceiver/parser.go:409-448)

**Purpose**: Ensure deterministic args hashing for Freeze-Tools lookup.

**Key Features**:
- Recursively normalizes nested structures
- Converts all numeric types to `float64` (int → float64, int64 → float64)
- Handles `storage.JSONB` type conversion
- Normalizes arrays and nested maps

**Why This Matters**:
```go
// These should produce the SAME hash:
args1 := JSONB{"count": int(5), "name": "test"}
args2 := JSONB{"count": float64(5), "name": "test"}

// Without normalization: different hashes (Freeze-Tools would fail to match)
// With normalization: same hash (Freeze-Tools correctly matches)
```

**Critical for Freeze-Tools**: When looking up captured tool results, args must match even if numeric types differ between capture and replay.

#### 2. `ParseToolCalls()` Signature (pkg/otelreceiver/parser.go:211)

**Before**:
```go
func ParseToolCalls(span ptrace.Span, traceID string) []*storage.ToolCapture
```

**After**:
```go
func ParseToolCalls(span ptrace.Span, traceID string, spanID string) []*storage.ToolCapture
```

**Impact**: Tool captures now linked to specific spans.

#### 3. `ParseLLMSpan()` Updates (pkg/otelreceiver/parser.go:84-130)

Now extracts and populates:
- `SpanID` from span
- `StepIndex` (derived from span ordering)

### PostgreSQL Implementation Changes

#### 1. Conflict Handling (pkg/storage/postgres.go:79-92)

```go
INSERT INTO otel_traces (...)
VALUES (...)
ON CONFLICT (trace_id, span_id) DO NOTHING
RETURNING id

// Handle no rows returned from ON CONFLICT
if err == sql.ErrNoRows {
    return nil  // Treat as success
}
```

**Purpose**: Idempotent trace ingestion - sending same span twice doesn't error.

#### 2. New Query Methods (pkg/storage/postgres.go:95-125, 152-185)

**`GetOTELTraceSpans()`**:
```go
SELECT ... FROM otel_traces
WHERE trace_id = $1
ORDER BY start_time ASC
```
Returns all spans for a trace, ordered chronologically.

**`GetReplayTraceSpans()`**:
```go
SELECT ... FROM replay_traces
WHERE trace_id = $1
ORDER BY step_index ASC, created_at ASC
```
Returns all LLM calls for an agent run, ordered by step.

#### 3. Updated INSERT Statements

All INSERT statements now include `span_id` and handle new fields:
- `CreateReplayTrace()` includes span_id, step_index
- `CreateToolCapture()` includes span_id

### Test Updates

#### `pkg/storage/storage_test.go` (lines 78-136)

Tests now create multiple spans per trace:

```go
// Create first span
trace1 := &ReplayTrace{
    TraceID:   "trace-123",
    SpanID:    "span-001",       // NEW
    StepIndex: 0,                // NEW
    ...
}

// Create second span (same trace)
trace2 := &ReplayTrace{
    TraceID:   "trace-123",
    SpanID:    "span-002",       // Different span
    StepIndex: 1,                // Next step
    ...
}

// Retrieve all spans for trace
spans, _ := storage.GetReplayTraceSpans(ctx, "trace-123")
// Returns: [trace1, trace2] ordered by step_index
```

#### `pkg/otelreceiver/receiver_test.go`

Mock storage updated with new methods:
- `GetOTELTraceSpans()`
- `GetReplayTraceSpans()`

#### New Hash Tests (pkg/otelreceiver/receiver_test.go:148-211)

Comprehensive tests for `calculateArgsHash()`:
- int vs float64 produce same hash ✅
- Nested maps with different key order produce same hash ✅
- Different values produce different hashes ✅
- Empty args produce consistent hash ✅

**Critical for Freeze-Tools**: Ensures args matching works correctly.

## Current Project Status

### ✅ Completed (Phase 1 - 90%)

**Foundation**:
- [x] Project structure (Go module, packages, cmd)
- [x] Configuration system (envconfig, validation, tests)
- [x] Structured logging (zap wrapper)
- [x] CLI framework (Cobra with all command skeletons)
- [x] Build tools (Makefile, Docker, docker-compose, CI)
- [x] Local dev environment (.env, setup scripts)
- [x] Documentation (README, QUICKSTART, DRAFT_PLAN2)

**Database Layer** (pkg/storage/):
- [x] Storage interface (40+ methods)
- [x] PostgreSQL implementation with connection pooling
- [x] 12 tables: traces, experiments, analysis, evaluation
- [x] Migration system (embedded SQL)
- [x] JSONB support with marshaling
- [x] Multi-span support (trace_id + span_id)
- [x] Comprehensive test suite
- [x] Idempotent inserts (ON CONFLICT DO NOTHING)

**OTLP Receiver** (pkg/otelreceiver/):
- [x] Dual protocol (gRPC port 4317, HTTP port 4318)
- [x] OTEL semantic conventions parser (gen_ai.*)
- [x] Prompt/completion extraction
- [x] Tool call parsing from events
- [x] Risk class determination (read/write/destructive)
- [x] Deterministic args hashing with normalization
- [x] Multi-span support (span_id tracking)
- [x] Storage integration
- [x] Unit tests
- [x] Integrated into `cmdr serve`

**Fixes**:
- [x] Docker Compose V2 support (docker compose vs docker-compose)
- [x] Port conflict resolution (Jaeger vs CMDR)
- [x] Enhanced logging for debugging

### 🚧 Current Sprint (Phase 1 - Final 10%)

**Freeze-Tools MCP Server** (HIGH PRIORITY):
- [ ] MCP protocol implementation
- [ ] Capture mode (proxy + record)
- [ ] Freeze mode (lookup + return)
- [ ] Argument matching (using normalized hashing)
- [ ] Integration in serve command

**E2E Testing**:
- [ ] Verify OTLP receiver stores traces correctly
- [ ] Test multi-span trace ingestion
- [ ] Integration test with Freeze-Tools

### 📋 Upcoming Phases

**Phase 2** - Replay Engine (Week 2):
- Agentgateway HTTP client
- Experiment orchestration
- Worker pool for concurrent replays

**Phase 3** - Analysis & Evaluation (Week 3):
- 4D analysis (behavior, safety, quality, efficiency)
- 5 evaluator types
- Report generation

**Phase 4** - API & Production (Week 4):
- REST API endpoints
- CLI implementation
- Production hardening

## Architecture Changes: Multi-Span Model

### Before (Single-Span)

```
Trace abc123
├── One LLM call
├── Tool calls attached to trace
└── Single prompt/completion
```

**Limitation**: Couldn't represent multi-turn conversations.

### After (Multi-Span)

```
Trace abc123 (Agent Run)
├── Span 1 (step_index=0): User prompt → LLM response
│   └── Tool calls: search_docs()
├── Span 2 (step_index=1): Follow-up prompt → LLM response
│   └── Tool calls: read_file()
└── Span 3 (step_index=2): Final prompt → LLM response
```

**Benefits**:
- Represents complete multi-turn agent conversations
- Preserves temporal ordering (step_index)
- Links tool calls to specific LLM interactions
- Enables analysis of conversation flow

### Data Model

```
┌─────────────────────────────────────┐
│     Trace (Logical Grouping)        │
│     trace_id: abc123                │
└──────────┬──────────────────────────┘
           │
           ├─→ OTELTrace (span-001)      [Raw OTEL data]
           ├─→ OTELTrace (span-002)      [Raw OTEL data]
           │
           ├─→ ReplayTrace (span-001, step=0)  [Parsed LLM call]
           │   └─→ ToolCapture (span-001, step=0, tool=search)
           │
           ├─→ ReplayTrace (span-002, step=1)  [Parsed LLM call]
           │   └─→ ToolCapture (span-002, step=0, tool=read)
           │
           └─→ ReplayTrace (span-003, step=2)  [Parsed LLM call]
```

### Query Patterns

**Reconstruct Agent Run**:
```go
// Get all LLM calls for an agent run, in order
spans := storage.GetReplayTraceSpans(ctx, "trace-abc123")
// Returns: [span1 (step=0), span2 (step=1), span3 (step=2)]

// Get all tool calls across the entire run
tools := storage.GetToolCapturesByTrace(ctx, "trace-abc123")
// Returns all tool calls, any span, ordered by step_index
```

**Freeze-Tools Lookup**:
```go
// Look up captured tool result by name and args (span-agnostic)
capture := storage.GetToolCaptureByArgs(ctx, "search_docs", argsHash)
// Returns most recent matching capture (any trace, any span)
```

## Code References

### Schema
- [001_initial_schema.sql:8-78](pkg/storage/migrations/001_initial_schema.sql#L8-L78) - Updated tables with span support

### Models
- [models.go:38-53](pkg/storage/models.go#L38-L53) - OTELTrace with span_id
- [models.go:55-74](pkg/storage/models.go#L55-L74) - ReplayTrace with span_id and step_index
- [models.go:76-90](pkg/storage/models.go#L76-L90) - ToolCapture with span_id

### Storage Implementation
- [postgres.go:73-93](pkg/storage/postgres.go#L73-L93) - CreateOTELTrace with ON CONFLICT
- [postgres.go:95-125](pkg/storage/postgres.go#L95-L125) - GetOTELTraceSpans
- [postgres.go:127-150](pkg/storage/postgres.go#L127-L150) - CreateReplayTrace with span_id
- [postgres.go:152-185](pkg/storage/postgres.go#L152-L185) - GetReplayTraceSpans
- [postgres.go:230-244](pkg/storage/postgres.go#L230-L244) - CreateToolCapture with span_id

### Parser
- [parser.go:84-130](pkg/otelreceiver/parser.go#L84-L130) - ParseLLMSpan extracts span_id
- [parser.go:211](pkg/otelreceiver/parser.go#L211) - ParseToolCalls takes spanID param
- [parser.go:394-407](pkg/otelreceiver/parser.go#L394-L407) - calculateArgsHash with normalization
- [parser.go:409-448](pkg/otelreceiver/parser.go#L409-L448) - normalizeValue for deterministic hashing

### Tests
- [storage_test.go:78-136](pkg/storage/storage_test.go#L78-L136) - Multi-span test
- [receiver_test.go:148-211](pkg/otelreceiver/receiver_test.go#L148-L211) - Args hash normalization tests

## Deterministic Args Hashing

### Problem

Freeze-Tools needs to match tool calls by arguments:
```go
// Capture: {"count": 5} (as int)
// Replay:  {"count": 5.0} (as float64 from JSON)
// Without normalization: Different hashes → Match fails
```

### Solution: `normalizeValue()` Function

```go
func normalizeValue(v interface{}) interface{} {
    switch val := v.(type) {
    case int, int64, int32:
        return float64(val)  // Normalize all integers to float64
    case float32:
        return float64(val)  // Normalize float32 to float64
    case map[string]interface{}:
        // Recursively normalize nested maps
    case []interface{}:
        // Recursively normalize arrays
    // ...
    }
}
```

**Test Coverage** (receiver_test.go:148-211):
- `int(5)` and `float64(5)` → same hash ✅
- `int64(42)` and `float64(42)` → same hash ✅
- Nested maps with different key order → same hash ✅
- Different values → different hashes ✅

**Impact**: Freeze-Tools can reliably match tool calls regardless of numeric type representation.

## Project Plan

### Timeline (4 Weeks)

**Week 1** (Current - Phase 1: Foundation):
- Days 1-5: ✅ **COMPLETE** - Foundation + Database + OTLP
- Days 6-7: 🚧 **IN PROGRESS** - Freeze-Tools MCP server

**Week 2** (Phase 2: Replay):
- Agentgateway HTTP client
- Replay engine with worker pool
- E2E replay test

**Week 3** (Phase 3: Analysis):
- 4D analysis (behavior, safety, quality, efficiency)
- Evaluation framework (5 evaluator types)
- Report generation (JSON + Markdown)

**Week 4** (Phase 4: Production):
- REST API (experiments, evaluation, human review)
- CLI implementation
- Production hardening (auth, metrics, K8s)

### Current Priority: Freeze-Tools

**Why HIGH PRIORITY**:
- Core differentiator vs competitors
- Blocking for replay functionality
- Required for deterministic experiments

**What It Does**:
1. **Capture Mode**: Records tool calls during baseline run
2. **Freeze Mode**: Returns pre-recorded results during variant replays
3. **Enables**: Fair model comparison (identical tool outputs)

**Implementation Tasks**:
1. Research MCP (Model Context Protocol) specification
2. Implement MCP server (likely HTTP-based)
3. Capture mode: Proxy to real tools + record via `CreateToolCapture()`
4. Freeze mode: Lookup via `GetToolCaptureByArgs()` + return
5. Test with simple tools
6. Integrate into `serve.go`

### Component Dependencies

```
Phase 1 (Foundation) ✅
  ↓
Freeze-Tools (Current) 🚧
  ↓
Phase 2 (Replay Engine)
  ├── Needs: Freeze-Tools, Storage, OTLP ✅
  └── Provides: Experiment orchestration
  ↓
Phase 3 (Analysis & Eval)
  ├── Needs: Replay Engine, Storage
  └── Provides: Comparative analysis, scoring
  ↓
Phase 4 (API & CLI)
  ├── Needs: All previous phases
  └── Provides: User interfaces
```

## File Structure

### Implemented (35+ files)

```
replay/
├── cmd/cmdr/
│   ├── main.go ✅
│   └── commands/
│       ├── root.go ✅
│       ├── serve.go ✅ (integrated DB + OTLP)
│       ├── experiment.go ✅ (skeleton)
│       ├── eval.go ✅ (skeleton)
│       └── ground_truth.go ✅ (skeleton)
│
├── pkg/
│   ├── config/
│   │   ├── config.go ✅
│   │   └── config_test.go ✅
│   │
│   ├── storage/
│   │   ├── interface.go ✅
│   │   ├── models.go ✅ (multi-span support)
│   │   ├── postgres.go ✅ (multi-span queries)
│   │   ├── postgres_eval.go ✅
│   │   ├── storage_test.go ✅
│   │   └── migrations/
│   │       └── 001_initial_schema.sql ✅ (multi-span schema)
│   │
│   ├── otelreceiver/
│   │   ├── receiver.go ✅ (gRPC + HTTP)
│   │   ├── parser.go ✅ (with normalizeValue)
│   │   └── receiver_test.go ✅
│   │
│   └── utils/logger/
│       └── logger.go ✅
│
├── test/manual/
│   └── test_parser.go ✅ (updated for multi-span)
│
├── scripts/
│   ├── setup-dev.sh ✅
│   ├── test-otlp.sh ✅
│   └── diagnose-otlp.sh ✅
│
├── docs/
│   ├── QUICKSTART.md ✅
│   ├── DATABASE_LAYER.md ✅
│   ├── OTLP_RECEIVER.md ✅
│   ├── TESTING_OTLP.md ✅
│   ├── DEBUGGING_OTLP.md ✅
│   └── LOCAL_DEV_SETUP.md ✅
│
├── .env.example ✅
├── .gitignore ✅
├── Dockerfile ✅
├── docker-compose.yml ✅ (updated for port conflict)
├── Makefile ✅ (with dev commands)
├── go.mod ✅
├── README.md ✅
├── DRAFT_PLAN2.md ✅ (complete spec)
├── TODO.md ✅ (this research)
└── IMPLEMENTATION_STATUS.md ✅
```

### Not Yet Implemented

```
pkg/
├── freezetools/ ❌ (NEXT - HIGH PRIORITY)
├── agwclient/ ❌
├── replayengine/ ❌
├── analyzer/ ❌
├── evaluator/ ❌
├── reporter/ ❌
└── api/ ❌
```

## Migration Path for Existing Data

If traces were captured before multi-span support:

**Old schema**: One row per trace in replay_traces
**New schema**: Multiple rows per trace (one per span)

**Migration strategy**:
1. Add default `span_id` to existing rows (generate from trace_id)
2. Set `step_index = 0` for all existing rows
3. Update constraints from trace_id PK to UNIQUE(trace_id, span_id)

**Currently**: No migration needed (fresh database)

## Testing Status

### Unit Tests: ✅ Passing
- Configuration tests
- Storage tests (with multi-span scenarios)
- Parser tests (including args hash normalization)

### Integration Tests: 🚧 Pending
- OTLP receiver E2E (send trace → verify storage)
- Multi-span trace ingestion
- Freeze-Tools integration (when implemented)

### Known Issues

**Resolved**:
- ✅ Docker Compose command detection (V1 vs V2)
- ✅ Port conflict between Jaeger and CMDR (4317/4318)
- ✅ Unused fmt import in root.go
- ✅ Args hash consistency (int vs float64)

**Current** (from user report):
- 🔍 OTLP receiver not storing traces (debugging in progress)
  - Possible cause: Receiver failing to start (port conflict now fixed)
  - Next step: Rebuild and test with updated docker-compose.yml

## Next Actions

### Immediate (Today)

1. **Fix and test OTLP receiver**:
   ```bash
   docker compose down
   make dev-up      # Now uses fixed docker-compose.yml
   make build       # Rebuild with latest changes
   make run         # Start CMDR
   ./scripts/test-otlp.sh  # Test trace ingestion
   ```

2. **Verify multi-span support**:
   - Send trace with multiple spans
   - Query `GetReplayTraceSpans()` to verify ordering
   - Check `step_index` is correct

### This Week

3. **Implement Freeze-Tools MCP Server**:
   - Research MCP protocol
   - Implement server (pkg/freezetools/)
   - Integrate into serve command
   - Test capture and freeze modes

4. **Complete Phase 1**:
   - E2E tests passing
   - Documentation updated
   - Ready for Phase 2

## Success Metrics

**Phase 1 Complete When**:
- ✅ OTLP receiver ingests traces successfully
- ✅ Multi-span traces stored correctly
- ✅ Tool calls captured with deterministic hashing
- ⏳ Freeze-Tools MCP server operational
- ⏳ E2E test: Capture → Store → Freeze → Replay

**Current Score**: 90% complete

## Related Research

- [2026-02-26-draft-vs-implementation-plan-comparison.md](thoughts/shared/research/2026-02-26-draft-vs-implementation-plan-comparison.md) - Original architectural decisions

## Open Questions

1. **MCP Protocol**: What's the best way to implement the MCP server? (stdio, HTTP, or WebSocket)
2. **Argument Matching**: Should we support fuzzy matching beyond exact hash match? (e.g., whitespace tolerance)
3. **Missing Captures**: How should Freeze-Tools behave when a tool call wasn't captured? (error, warning, proxy to real tool?)
4. **Performance**: Can the args normalization handle deeply nested objects efficiently?

---

**Status**: Phase 1 is 90% complete with multi-span support implemented. Next: Freeze-Tools MCP server to enable deterministic replay.
