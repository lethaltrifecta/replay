# CMDR Implementation TODO

## Target: MCP_HACK//26 — "Secure & Govern MCP" Category
**Deadline: April 3, 2026**

## Completed

- [x] Project structure, build system, CI pipeline
- [x] Configuration management (envconfig, validation)
- [x] Structured logging (zap)
- [x] CLI framework (Cobra, all commands scaffolded)
- [x] Docker + docker-compose setup
- [x] **Database layer** — PostgreSQL, 12 tables, migrations, full CRUD
- [x] **OTLP receiver** — gRPC + HTTP, gen_ai.* parser, tool call extraction, args hashing
- [x] **Freeze-Tools MCP server** — separate repo (`freeze-mcp`), Python, fully built by teammate

## Architecture Change

CMDR is pivoting from a generic eval framework to an **agent governance system**:
- **Drift detection** — compare live agent traces against a known-good baseline
- **Deployment gate** — replay captured scenarios with a different model, block if behavior diverges

freeze-mcp is a separate service (Python/FastAPI/MCP SDK) that reads from CMDR's `tool_captures` table. CMDR does not need to build or manage it — it runs as a sidecar.

---

## Phase 1: Drift Detection

**Goal**: Detect when agent behavior in production deviates from a known-good baseline.

No replay needed — pure trace analysis.

### 1.1 Baseline Management
- [ ] Storage methods: `MarkTraceAsBaseline`, `UnmarkBaseline`, `ListBaselines`, `GetBaselineFingerprint`
- [ ] Migration: add `is_baseline BOOLEAN DEFAULT FALSE` to `replay_traces` or new `baselines` table
- [ ] CLI: `cmdr drift baseline set <trace_id>`
- [ ] CLI: `cmdr drift baseline list`
- [ ] CLI: `cmdr drift baseline remove <trace_id>`

### 1.2 Behavior Fingerprinting (`pkg/drift/`)
- [ ] `fingerprint.go` — extract behavioral signature from a trace:
  - Tool call sequence (ordered list of tool names)
  - Tool call frequency (counts per tool name)
  - Risk class distribution (read/write/destructive percentages)
  - Token usage (input/output averages)
  - Model and provider
- [ ] `fingerprint_test.go` — unit tests

### 1.3 Drift Comparison (`pkg/drift/`)
- [ ] `compare.go` — compare two fingerprints, produce a drift score:
  - Sequence similarity (Levenshtein or LCS on tool call order)
  - Frequency divergence (cosine distance or simple delta)
  - Risk escalation detection (any read→write or write→destructive shift)
  - Token budget change
  - Overall drift score (weighted composite)
- [ ] `compare_test.go` — unit tests
- [ ] Configurable thresholds for pass/warn/fail

### 1.4 Drift CLI Commands
- [ ] `cmd/cmdr/commands/drift.go`:
  - `cmdr drift check [--trace-id <id>]` — compare a trace against its baseline
  - `cmdr drift status` — show drift scores for recent traces
  - `cmdr drift watch` — continuous mode (poll for new traces, check against baseline)

### 1.5 Storage for Drift Results
- [ ] Migration: `drift_results` table (trace_id, baseline_trace_id, drift_score, details JSONB, created_at)
- [ ] Storage methods: `StoreDriftResult`, `GetDriftResults`, `GetDriftResultsByBaseline`

---

## Phase 2: Deployment Gate (Prompt Replay)

**Goal**: Before deploying a model/prompt change, replay captured scenarios and verify behavior doesn't regress.

### 2.1 agentgateway Client (`pkg/agwclient/`)
- [ ] `client.go` — HTTP client for agentgateway
  - Send LLM completion requests with model override
  - Connection pooling, timeouts
  - Parse responses
- [ ] `client_test.go` — tests with mock HTTP server
- [ ] Research agentgateway's HTTP API for sending LLM requests

### 2.2 Prompt Replay Engine (`pkg/replay/`)
- [ ] `engine.go` — replay orchestration:
  - Load baseline trace steps (ordered by step_index)
  - For each step: extract the prompt, send to agentgateway with variant model config
  - Capture variant responses
  - Store as experiment + experiment_run
- [ ] `engine_test.go` — unit tests
- [ ] Integration with existing `experiments` / `experiment_runs` tables

### 2.3 Behavior Diff (`pkg/diff/`)
- [ ] `diff.go` — compare baseline trace vs replayed variant:
  - Tool call comparison: same tools? same args? same order?
  - Risk class changes: did the variant escalate risk?
  - Token efficiency: cost/latency delta
  - Response divergence: structural comparison of completions
- [ ] `diff_test.go` — unit tests
- [ ] Produce a pass/fail verdict with configurable threshold

### 2.4 Gate CLI Commands
- [ ] `cmd/cmdr/commands/gate.go`:
  - `cmdr gate check --baseline <trace_id> --model <model> [--threshold 0.8]`
  - `cmdr gate report <experiment_id>`
  - Exit code 0 = pass, 1 = fail (CI/CD friendly)

---

## Phase 3: Demo Polish

**Goal**: A compelling 2-3 minute demo for hackathon judges.

### 3.1 Output & Reporting
- [ ] Color-coded CLI output for drift checks and gate results
- [ ] Markdown report generation for gate results
- [ ] Summary table: baseline vs variant (tools, risk, tokens, verdict)

### 3.2 Demo Scenario
- [ ] Script a realistic scenario:
  - Agent run captured via agentgateway (e.g., a coding assistant or k8s operator)
  - Show drift detection catching behavior change after model update
  - Show deployment gate blocking a model swap that introduces risk escalation
- [ ] Record demo video (2-3 min)
- [ ] Write blog post

### 3.3 Submission Materials
- [ ] Update README with demo GIF or screenshot
- [ ] Final submission narrative
- [ ] Social post highlighting a surprising finding

---

## What We're NOT Building (Descoped)

These were in the original plan but are cut for hackathon scope:

- ~~REST API~~ — CLI-only for now
- ~~5 evaluator types~~ — replaced by drift + gate verdicts
- ~~Human review queue~~ — not needed for demo
- ~~LLM-as-judge~~ — overkill for the governance angle
- ~~Report templates~~ — simple Markdown is sufficient
- ~~kagent integration~~ — nice-to-have, not blocking
- ~~agentregistry integration~~ — nice-to-have, not blocking
- ~~Production hardening (JWT, rate limiting, Prometheus)~~ — post-hackathon
