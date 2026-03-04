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
- [x] Storage methods: `MarkTraceAsBaseline`, `UnmarkBaseline`, `ListBaselines`, `GetBaseline`
- [x] Migration: `baselines` table (`002_baselines_and_drift.sql`)
- [x] CLI: `cmdr drift baseline set <trace_id>`
- [x] CLI: `cmdr drift baseline list`
- [x] CLI: `cmdr drift baseline remove <trace_id>`

### 1.2 Behavior Fingerprinting (`pkg/drift/`)
- [x] `fingerprint.go` — extract behavioral signature from a trace:
  - Tool call sequence (ordered list of tool names)
  - Tool call frequency (counts per tool name)
  - Risk class distribution (read/write/destructive percentages)
  - Token usage (input/output averages)
  - Model and provider
- [x] `fingerprint_test.go` — unit tests

### 1.3 Drift Comparison (`pkg/drift/`)
- [x] `compare.go` — compare two fingerprints, produce a drift score:
  - Sequence similarity (Levenshtein or LCS on tool call order)
  - Frequency divergence (cosine distance or simple delta)
  - Risk escalation detection (any read→write or write→destructive shift)
  - Token budget change
  - Overall drift score (weighted composite)
- [x] `compare_test.go` — unit tests
- [x] Configurable thresholds for pass/warn/fail

### 1.4 Drift CLI Commands
- [x] `cmd/cmdr/commands/drift.go`:
  - `cmdr drift check <trace-id>` — compare a trace against its baseline
  - `cmdr drift status` — show drift scores for recent traces
  - `cmdr drift watch` — continuous poll mode with retry + overflow handling
- [x] `cmd/cmdr/commands/drift_test.go` — unit tests for poll, cursor, dedup, retry logic

### 1.5 Storage for Drift Results
- [x] Migration: `drift_results` table + unique constraint (`002` + `003`)
- [x] Storage methods: `CreateDriftResult` (idempotent), `GetDriftResults`, `GetDriftResultsByBaseline`, `GetLatestDriftResult`, `ListDriftResults`, `HasDriftResultForBaseline`

---

## Phase 2: Deployment Gate (Prompt Replay)

**Goal**: Before deploying a model/prompt change, replay captured scenarios and verify behavior doesn't regress.

### 2.1 agentgateway Client (`pkg/agwclient/`)
- [x] `client.go` — HTTP client for agentgateway
  - Send LLM completion requests with model override
  - Retry with exponential backoff on 429/5xx, fail-fast on other 4xx
  - Parse OpenAI-compatible responses
- [x] `client_test.go` — tests with httptest mock server

### 2.2 Prompt Replay Engine (`pkg/replay/`)
- [x] `engine.go` — replay orchestration:
  - Load baseline trace steps (ordered by step_index)
  - For each step: extract the prompt, send to agentgateway with variant model config
  - Capture variant responses as new ReplayTrace rows
  - Manage experiment + experiment_run lifecycle (create, progress, finalize/fail)
- [x] `engine_test.go` — unit tests (mock completer + mock storage)
- [x] Integration with existing `experiments` / `experiment_runs` tables

### 2.3 Behavior Diff (`pkg/diff/`)
- [x] `diff.go` — structural comparison of baseline vs replayed variant:
  - Step count similarity (weight 0.4)
  - Token budget similarity with log-dampened scoring (weight 0.3)
  - Latency similarity (weight 0.3)
  - Configurable threshold, pass/fail verdict
- [x] `diff_test.go` — table-driven unit tests
- [ ] Tool call comparison: same tools? same args? same order?
- [ ] Risk class changes: did the variant escalate risk?
- [ ] Response divergence: structural comparison of completions

### 2.4 Gate CLI Commands
- [x] `cmd/cmdr/commands/gate.go`:
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
