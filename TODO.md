# CMDR Implementation TODO

## ✅ Completed

### Phase 1: Foundation (90% Complete)
- [x] Project structure and build system
- [x] Configuration management with envconfig
- [x] Structured logging with zap
- [x] CLI framework with Cobra (all commands scaffolded)
- [x] Docker and docker-compose setup
- [x] GitHub Actions CI pipeline
- [x] Local development environment (.env, scripts)
- [x] **Database Layer** - PostgreSQL with 12 tables, migrations, full CRUD
- [x] **OTLP Receiver** - gRPC + HTTP, parser, storage integration
- [x] Fixed port conflicts (Jaeger vs CMDR)

## 🚧 Current Sprint: Complete Phase 1

### 1. Freeze-Tools MCP Server (HIGH PRIORITY)
**Why**: Core feature for deterministic replay

**Files to create**:
- [ ] `pkg/freezetools/server.go` - MCP server implementation
- [ ] `pkg/freezetools/capture.go` - Capture mode (record tool calls)
- [ ] `pkg/freezetools/freeze.go` - Freeze mode (return captured results)
- [ ] `pkg/freezetools/matcher.go` - Argument normalization and matching
- [ ] `pkg/freezetools/mcp.go` - MCP protocol handlers
- [ ] `pkg/freezetools/freezetools_test.go` - Tests
- [ ] Integration in `serve.go` - Start Freeze-Tools server

**Tasks**:
- [ ] Research MCP protocol specification
- [ ] Implement MCP server (stdio or HTTP)
- [ ] Implement capture mode (proxy to real tools + record)
- [ ] Implement freeze mode (lookup captured results)
- [ ] Implement JSON argument normalization (canonical form)
- [ ] Implement argument matching (fuzzy match for equivalent JSON)
- [ ] Handle missing captures gracefully
- [ ] Add comprehensive tests

**Estimated**: 2-3 days

### 2. Phase 1 E2E Testing
- [ ] End-to-end test: Send OTLP trace → Verify storage → Test Freeze-Tools
- [ ] Integration test with mock agentgateway
- [ ] Verify migrations work on fresh database
- [ ] Performance test (1000 traces)

**Estimated**: 1 day

---

## 📋 Phase 2: Replay Engine (Next Sprint)

### 3. Agentgateway HTTP Client
**Files to create**:
- [ ] `pkg/agwclient/client.go` - HTTP client with retry
- [ ] `pkg/agwclient/request_builder.go` - Build LLM requests
- [ ] `pkg/agwclient/options.go` - Functional options pattern
- [ ] `pkg/agwclient/agwclient_test.go` - Tests

**Tasks**:
- [ ] HTTP client with connection pooling
- [ ] Retry logic with exponential backoff
- [ ] Request builder (convert replay trace → agentgateway format)
- [ ] Configure tool endpoint (point to Freeze-Tools MCP)
- [ ] Tests with mock HTTP server

**Estimated**: 1-2 days

### 4. Replay Engine
**Files to create**:
- [ ] `pkg/replayengine/engine.go` - Orchestration
- [ ] `pkg/replayengine/workerpool.go` - Worker pool
- [ ] `pkg/replayengine/job.go` - Job types and state
- [ ] `pkg/replayengine/scheduler.go` - Task scheduling
- [ ] `pkg/replayengine/replayengine_test.go` - Tests

**Tasks**:
- [ ] Experiment orchestration logic
- [ ] Worker pool for concurrent replays (reference agentgateway pattern)
- [ ] Job status tracking and progress updates
- [ ] Rate limiting per model
- [ ] Cancellation support
- [ ] Integration with storage, agwclient, and Freeze-Tools

**Estimated**: 2-3 days

---

## 📋 Phase 3: Analysis & Evaluation (Sprint 3)

### 5. Analyzer Package (4D Analysis)
**Files to create**:
- [ ] `pkg/analyzer/interface.go` - Analyzer interface
- [ ] `pkg/analyzer/behavior.go` - Tool sequence, arg drift
- [ ] `pkg/analyzer/safety.go` - Risk class tracking
- [ ] `pkg/analyzer/quality.go` - Similarity scoring
- [ ] `pkg/analyzer/efficiency.go` - Token/cost/latency
- [ ] `pkg/analyzer/analyzer_test.go` - Tests

**Tasks**:
- [ ] Behavior analyzer: tool sequence diff, first divergence
- [ ] Safety analyzer: risk class changes
- [ ] Quality analyzer: semantic similarity, rubric scoring
- [ ] Efficiency analyzer: token/cost/latency deltas
- [ ] Tests for all analyzers

**Estimated**: 3-4 days

### 6. Evaluator Framework
**Files to create**:
- [ ] `pkg/evaluator/interface.go` - Evaluator interface
- [ ] `pkg/evaluator/rule_based.go` - Rule evaluator
- [ ] `pkg/evaluator/llm_judge.go` - LLM-as-judge
- [ ] `pkg/evaluator/rubric.go` - Rubric scorer
- [ ] `pkg/evaluator/ground_truth.go` - Ground truth comparison
- [ ] `pkg/evaluator/human_loop.go` - Human review queue
- [ ] `pkg/evaluator/aggregate.go` - Score aggregation, winner determination
- [ ] `pkg/evaluator/evaluator_test.go` - Tests

**Tasks**:
- [ ] All 5 evaluator types
- [ ] Composable evaluators with weights
- [ ] Pass/fail thresholds
- [ ] Winner determination and ranking
- [ ] Tests for all evaluators

**Estimated**: 3-4 days

### 7. Report Generator
**Files to create**:
- [ ] `pkg/reporter/reporter.go` - Report orchestration
- [ ] `pkg/reporter/json.go` - JSON format
- [ ] `pkg/reporter/markdown.go` - Markdown format
- [ ] `pkg/reporter/templates.go` - Report templates
- [ ] `pkg/reporter/reporter_test.go` - Tests

**Tasks**:
- [ ] JSON report generation
- [ ] Markdown report generation
- [ ] Include analysis diffs, evaluation scores, winner
- [ ] Template system for customization

**Estimated**: 1-2 days

---

## 📋 Phase 4: API & Production (Sprint 4)

### 8. REST API
**Files to create**:
- [ ] `pkg/api/server.go` - HTTP server
- [ ] `pkg/api/handlers_experiments.go` - Experiment endpoints
- [ ] `pkg/api/handlers_evaluation.go` - Evaluation endpoints
- [ ] `pkg/api/handlers_human.go` - Human review endpoints
- [ ] `pkg/api/handlers_ground_truth.go` - Ground truth endpoints
- [ ] `pkg/api/middleware.go` - Logging, auth, CORS
- [ ] `pkg/api/routes.go` - Route definitions
- [ ] `pkg/api/api_test.go` - Tests

**Estimated**: 2-3 days

### 9. CLI Implementation
**Files to update** (skeleton already exists):
- [ ] `cmd/cmdr/commands/experiment.go` - Implement experiment commands
- [ ] `cmd/cmdr/commands/eval.go` - Implement eval commands
- [ ] `cmd/cmdr/commands/ground_truth.go` - Implement ground truth commands

**Estimated**: 2-3 days

### 10. Production Hardening
- [ ] Authentication/authorization (JWT)
- [ ] Rate limiting
- [ ] Request validation
- [ ] Error handling improvements
- [ ] Prometheus metrics
- [ ] Kubernetes manifests
- [ ] Production deployment guide

**Estimated**: 2-3 days

---

## 🎯 Immediate Next Steps (This Week)

### Priority 1: Freeze-Tools MCP Server
This is the **most critical** component - without it, we can't do deterministic replay.

**Start with**:
1. Research MCP protocol (Model Context Protocol)
2. Design server interface (stdio vs HTTP)
3. Implement capture mode
4. Implement freeze mode
5. Test with simple tools

### Priority 2: Test OTLP Receiver E2E
Now that port conflict is fixed, verify:
1. Traces are being stored
2. Parser works correctly
3. Tool calls are captured

---

## Timeline Estimate

Based on DRAFT_PLAN2.md 4-week plan:

**Week 1** (Current - Phase 1):
- [x] Days 1-5: Foundation + Database + OTLP ✅
- [ ] Days 6-7: Freeze-Tools MCP server 🚧

**Week 2** (Phase 2):
- [ ] Agentgateway client
- [ ] Replay engine
- [ ] E2E replay test

**Week 3** (Phase 3):
- [ ] 4D Analysis
- [ ] Evaluation framework
- [ ] Report generation

**Week 4** (Phase 4):
- [ ] REST API
- [ ] CLI implementation
- [ ] Production hardening

---

## What Should We Do Next?

**Option A**: Implement Freeze-Tools MCP server (enables deterministic replay)
**Option B**: Test OTLP receiver thoroughly first (verify what we have works)
**Option C**: Implement REST API first (enables external interaction)

My recommendation: **Option A (Freeze-Tools)** - it's the core differentiator and blocking for replay functionality.

What would you like to tackle next?
