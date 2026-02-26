# CMDR Implementation Plan v2

**Replay-Backed Behavior Analysis Lab for LLM Agents**

---

## Executive Summary

CMDR is a deterministic replay and evaluation system for comparing LLM agent behavior across models, prompts, policies, and tool configurations. By capturing real agent runs and replaying them with frozen tool responses, CMDR enables fair, reproducible experiments that isolate model behavior differences from environment variability.

**Core Innovation**: Freeze-Tools deterministic replay + comprehensive evaluation framework

**Target Users**: Engineering teams evaluating models for production deployment, optimizing costs, and ensuring consistent agent behavior.

---

## Table of Contents

1. [Product Definition](#product-definition)
2. [Architectural Decisions](#architectural-decisions)
3. [Core Concepts](#core-concepts)
4. [System Architecture](#system-architecture)
5. [Data Model](#data-model)
6. [Evaluation Framework](#evaluation-framework)
7. [API Specification](#api-specification)
8. [CLI Interface](#cli-interface)
9. [Implementation Phases](#implementation-phases)
10. [Success Criteria](#success-criteria)

---

## Product Definition

### What is CMDR?

CMDR is a **replay-backed behavior analysis lab** that helps engineering teams answer:

1. **How does model behavior change across the same task context?**
   - Compare GPT-4 vs Claude vs Gemini on identical scenarios
   - Measure differences in tool usage, reasoning paths, output quality

2. **Why do results differ between models or scenarios?**
   - Identify first divergence point in agent execution
   - Explain behavioral differences with detailed diffs

3. **Which model/configuration is best for our use case?**
   - Score outputs against quality criteria
   - Rank models by performance, cost, safety
   - Provide recommendations with trade-off analysis

### Primary Use Cases

1. **Model Selection**
   - Test multiple models on real production workloads
   - Compare behavior, quality, cost, and latency
   - Make data-driven model selection decisions

2. **Scenario Experiments**
   - Test prompt variants (different phrasings, instructions)
   - Test policy variants (different safety rules)
   - Test tool availability (subsets of tools)
   - Test retrieval context (different knowledge bases)

3. **Quality Assurance**
   - Evaluate model outputs against rubrics
   - Detect risky behavior changes (new destructive operations)
   - Track consistency across model updates

4. **Cost Optimization**
   - Find cheaper models with acceptable quality trade-offs
   - Measure token/cost deltas across models
   - Identify best value-for-money configurations

### Key Differentiators

Compared to existing solutions (LangSmith, Langfuse, Braintrust, etc.):

1. **Deterministic Environment Pinning (Freeze-Tools)**
   - Tools return pre-captured results during replay
   - Eliminates environment variability from comparisons
   - Enables true apples-to-apples model comparison

2. **Protocol-Native Replay**
   - Works at MCP/tool graph level
   - No SDK changes required in agent code
   - Gateway-agnostic replay mechanism

3. **Behavior-First Analysis**
   - Tool call sequence diffs (not just output comparison)
   - Risk class tracking (read/write/destructive operations)
   - First-divergence explanations

4. **Comprehensive Evaluation Framework**
   - Multiple evaluation strategies (rule-based, LLM-judge, rubric, human-loop)
   - Composable evaluators with configurable weights
   - Winner determination with cost-vs-quality trade-offs

---

## Architectural Decisions

The following architectural decisions define CMDR's implementation:

### 1. Data Source: OTEL Traces from Gateway

**Decision**: Consume OTEL traces emitted by agentgateway via OTEL exporter

**Rationale**:
- Leverages existing OTEL infrastructure
- No custom gateway capture logic needed
- Standard OTEL format enables interoperability
- Can consume from Jaeger/Tempo or direct OTLP endpoint

**Architecture**:
```
Agentgateway (emits OTEL traces)
    ↓
OTEL Collector (optional)
    ↓
CMDR OTLP Receiver → Parse & Store
```

### 2. Determinism: Freeze-Tools

**Decision**: Implement Freeze-Tools MCP server for deterministic replay

**What is Freeze-Tools?**

Freeze-Tools ensures **identical tool execution results** across multiple replays by capturing tool responses during baseline run and returning frozen responses during variant replays.

**The Problem**:
When replaying an agent run with a different model, tools might return different results (data changes, time-dependent APIs, nondeterministic services). This makes it impossible to isolate pure model behavior differences.

**Example Without Freeze-Tools**:
```
Baseline (GPT-4):
1. search_docs("auth") → [10 docs] (live)
2. Process results → Answer A

Replay (Claude):
1. search_docs("auth") → [12 docs] (docs updated!)
2. Process results → Answer B

Question: Is difference due to model or different tool results?
```

**Example With Freeze-Tools**:
```
Baseline (GPT-4) - CAPTURE MODE:
1. search_docs("auth") → [10 docs]
2. CAPTURE: tool="search_docs", args={query:"auth"}, result=[10 docs]
3. Process results → Answer A

Replay (Claude) - FREEZE MODE:
1. search_docs("auth") → [FROZEN 10 docs] (no execution)
2. Process results → Answer B

Answer: Difference is PURELY due to model behavior!
```

**Freeze-Tools Operation**:

1. **Capture Mode** (Baseline):
   - Acts as transparent proxy to real tools
   - Records: tool_name, arguments (JSON), result, error, latency
   - Stores captured data with run_id + step_index

2. **Freeze Mode** (Variants):
   - Replaces real MCP tools
   - Looks up captured result by (tool_name, normalized_args)
   - Returns pre-captured result (no actual execution)
   - Maintains determinism across all replays

**Benefits**:
- Fair model comparison (identical inputs)
- Reproducible experiments (weeks later)
- Cost efficient (no re-execution of expensive APIs)
- Safe (no re-execution of destructive operations)

### 3. Architecture: Standalone Service + Agentgateway Client

**Decision**: Build as standalone Go microservice that uses agentgateway as HTTP client

**Rationale**:
- Independent development and deployment
- No gateway code changes required
- Can evolve independently
- Uses agentgateway's existing LLM request API

**Integration**:
- CMDR sends LLM requests to agentgateway HTTP API
- Configures tool endpoint to point to Freeze-Tools MCP
- Agentgateway processes requests normally
- CMDR collects responses and metrics

### 4. Analysis Scope: Full Behavior + Evaluation

**Decision**: Implement 4-dimensional analysis + comprehensive evaluation framework

**4 Analysis Dimensions**:

1. **Behavior Analysis**
   - Tool call sequence comparison
   - Argument drift detection
   - Tool graph visualization
   - First divergence identification

2. **Safety/Policy Analysis**
   - Risk class tracking (read/write/destructive)
   - Policy decision changes
   - Unauthorized tool access attempts

3. **Quality Analysis**
   - Semantic similarity scoring
   - Rubric-based evaluation
   - First-divergence explanation

4. **Efficiency Analysis**
   - Token count deltas
   - Cost deltas (configurable pricing)
   - Latency comparison (model + e2e)

**Evaluation Framework** (NEW):
- Multiple evaluator types (rule-based, LLM-judge, rubric, human-loop, ground-truth)
- Composable with configurable weights
- Pass/fail thresholds
- Winner determination with rankings
- Trade-off analysis (quality vs cost)

### 5. Technology Stack: Go

**Decision**: Implement entire system in Go

**Stack**:
- **Language**: Go 1.23+
- **HTTP**: `gorilla/mux` router
- **Config**: `kelseyhightower/envconfig`
- **Logging**: `go.uber.org/zap`
- **Retry**: `github.com/avast/retry-go/v4`
- **Database**: PostgreSQL with `lib/pq`
- **CLI**: `spf13/cobra`
- **OTLP**: `go.opentelemetry.io/otel`
- **Testing**: `stretchr/testify`, `golang/mock`

---

## Core Concepts

### Run vs Experiment

**Run**: A single agent execution
- Baseline run: Initial capture with real tools
- Variant run: Replay with Freeze-Tools + changed parameters

**Experiment**: Collection of runs comparing multiple variants
- 1 baseline run
- N variant runs (different models/prompts/policies)
- Analysis comparing all variants to baseline
- Evaluation scoring all outputs

### Trace vs Step

**Trace**: Complete agent execution from start to finish
- Captured as OTEL spans
- Contains all LLM calls, tool calls, policy events

**Step**: Single operation within a trace
- `step_index`: Canonical ordering (0, 1, 2, ...)
- `step_type`: llm, tool, policy, io
- Each step has inputs, outputs, latency, metadata

### Capture vs Freeze

**Capture Mode**: Record tool results during baseline
- Real tools execute normally
- All tool calls captured: name, args, result, error
- Stored for replay

**Freeze Mode**: Return captured results during replay
- Freeze-Tools MCP intercepts tool calls
- Returns pre-captured results
- No actual tool execution

### Analysis vs Evaluation

**Analysis**: Comparative diff of behavior
- How did models behave differently?
- Tool sequences, arguments, efficiency
- Descriptive, not prescriptive

**Evaluation**: Scoring output quality
- Which model produced the best result?
- Rule-based, LLM-judge, rubric, human scoring
- Produces rankings and recommendations

---

## System Architecture

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CAPTURE PHASE                          │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    OTEL spans    ┌──────────────┐        │
│  │ Agentgateway │─────────────────►│ OTEL Exporter│        │
│  │ (with real   │                   │ (Jaeger/     │        │
│  │  tools)      │                   │  Tempo/      │        │
│  └──────────────┘                   │  Direct)     │        │
│         │                            └──────┬───────┘        │
│         │                                   │                │
│         ▼                                   ▼                │
│  ┌──────────────┐                   ┌──────────────┐        │
│  │  Real MCP    │                   │ CMDR Service │        │
│  │  Tools       │                   │ - OTLP RX    │        │
│  └──────────────┘                   │ - Parse      │        │
│                                      │ - Store      │        │
│                                      └──────────────┘        │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                      REPLAY PHASE                           │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────┐              │
│  │ CMDR Service (Orchestrator)              │              │
│  │ ┌────────────────────────────────────┐   │              │
│  │ │ Experiment Engine                  │   │              │
│  │ │ - Load baseline trace              │   │              │
│  │ │ - Generate variant matrix          │   │              │
│  │ │ - Start Freeze-Tools MCP           │   │              │
│  │ └────────┬───────────────────────────┘   │              │
│  │          │                                │              │
│  │          ▼                                │              │
│  │ ┌────────────────────────────────────┐   │              │
│  │ │ Worker Pool                        │   │              │
│  │ │ - Concurrent variant replays       │   │              │
│  │ │ - Rate limiting per model          │   │              │
│  │ └────────┬───────────────────────────┘   │              │
│  └──────────┼───────────────────────────────┘              │
│             │                                               │
│             ▼                                               │
│  ┌──────────────────┐          ┌──────────────┐           │
│  │  Agentgateway    │◄─────────│ Freeze-Tools │           │
│  │  (HTTP Client)   │   MCP    │ MCP Server   │           │
│  │  - Model variant │ endpoint │ (frozen tool │           │
│  │  - Prompt variant│          │  results)    │           │
│  └──────────┬───────┘          └──────────────┘           │
│             │                                               │
│             ▼                                               │
│  ┌──────────────────────────────────────────────────┐     │
│  │           Storage (PostgreSQL)                   │     │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │     │
│  │  │  Traces  │  │ Captures │  │ Experiments  │   │     │
│  │  └──────────┘  └──────────┘  └──────────────┘   │     │
│  └──────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│               ANALYSIS & EVALUATION PHASE                    │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────────────────────────────────┐      │
│  │           Analysis Engine                        │      │
│  │  ┌──────────────┐  ┌──────────────┐            │      │
│  │  │  Behavior    │  │  Safety/     │            │      │
│  │  │  Analyzer    │  │  Policy      │            │      │
│  │  │  - Tool seq  │  │  Analyzer    │            │      │
│  │  │  - Arg drift │  │  - Risk class│            │      │
│  │  └──────────────┘  └──────────────┘            │      │
│  │  ┌──────────────┐  ┌──────────────┐            │      │
│  │  │  Quality     │  │  Efficiency  │            │      │
│  │  │  Analyzer    │  │  Analyzer    │            │      │
│  │  │  - Similarity│  │  - Tokens    │            │      │
│  │  └──────────────┘  └──────────────┘            │      │
│  └──────────────────────────────────────────────────┘      │
│                          │                                  │
│                          ▼                                  │
│  ┌──────────────────────────────────────────────────┐      │
│  │           Evaluation Engine                      │      │
│  │  ┌──────────────┐  ┌──────────────┐            │      │
│  │  │  Rule-Based  │  │  LLM Judge   │            │      │
│  │  │  Evaluator   │  │  Evaluator   │            │      │
│  │  └──────────────┘  └──────────────┘            │      │
│  │  ┌──────────────┐  ┌──────────────┐            │      │
│  │  │  Rubric      │  │  Ground      │            │      │
│  │  │  Evaluator   │  │  Truth Eval  │            │      │
│  │  └──────────────┘  └──────────────┘            │      │
│  │  ┌──────────────┐                               │      │
│  │  │  Human Loop  │                               │      │
│  │  │  Evaluator   │                               │      │
│  │  └──────────────┘                               │      │
│  └──────────────────────────────────────────────────┘      │
│                          │                                  │
│                          ▼                                  │
│  ┌──────────────────────────────────────────────────┐      │
│  │           Report Generator                       │      │
│  │  - Analysis diff report                          │      │
│  │  - Evaluation scorecard                          │      │
│  │  - Winner determination                          │      │
│  │  - Trade-off analysis                            │      │
│  │  - Recommendations                               │      │
│  │  Output: JSON + Markdown                         │      │
│  └──────────────────────────────────────────────────┘      │
│                          │                                  │
│                          ▼                                  │
│  ┌──────────────────────────────────────────────────┐      │
│  │           REST API + CLI                         │      │
│  │  - Experiment submission                         │      │
│  │  - Status tracking                               │      │
│  │  - Results retrieval                             │      │
│  │  - Human review workflow                         │      │
│  └──────────────────────────────────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
```

### Component Diagram

```
cmd/
└── cmdr/
    └── main.go              # CLI entry point

pkg/
├── config/                  # Configuration
│   ├── config.go
│   └── validation.go
│
├── otelreceiver/            # OTLP trace ingestion
│   ├── receiver.go          # gRPC/HTTP OTLP receiver
│   └── parser.go            # Parse OTEL spans
│
├── tracefetcher/            # Historical trace fetching
│   ├── interface.go
│   ├── jaeger_client.go
│   └── tempo_client.go
│
├── storage/                 # Data persistence
│   ├── interface.go
│   ├── postgres.go
│   └── models.go
│
├── freezetools/             # Deterministic replay
│   ├── server.go            # MCP server implementation
│   ├── capture.go           # Capture mode
│   ├── freeze.go            # Freeze mode
│   └── matcher.go           # Argument matching
│
├── agwclient/               # Agentgateway client
│   ├── client.go
│   ├── request_builder.go
│   └── retry.go
│
├── replayengine/            # Experiment orchestration
│   ├── engine.go
│   ├── workerpool.go
│   └── job.go
│
├── analyzer/                # 4D analysis
│   ├── interface.go
│   ├── behavior.go          # Tool sequence analysis
│   ├── safety.go            # Risk class tracking
│   ├── quality.go           # Similarity scoring
│   └── efficiency.go        # Token/cost/latency
│
├── evaluator/               # Evaluation framework
│   ├── interface.go
│   ├── rule_based.go        # Rule evaluator
│   ├── llm_judge.go         # LLM-as-judge
│   ├── rubric.go            # Rubric scorer
│   ├── human_loop.go        # Human review queue
│   ├── ground_truth.go      # Accuracy comparison
│   └── aggregate.go         # Score aggregation
│
├── comparator/              # Orchestrates analysis + eval
│   └── comparator.go
│
├── reporter/                # Report generation
│   ├── json.go
│   └── markdown.go
│
└── api/                     # REST API
    ├── server.go
    ├── handlers.go
    ├── middleware.go
    └── routes.go
```

---

## Data Model

### Database Schema

```sql
-- ============================================================================
-- OTEL Traces (raw capture from OTLP)
-- ============================================================================
CREATE TABLE otel_traces (
    trace_id VARCHAR(255) PRIMARY KEY,
    span_id VARCHAR(255),
    parent_span_id VARCHAR(255),
    service_name VARCHAR(255),
    span_kind VARCHAR(50),
    start_time TIMESTAMP,
    end_time TIMESTAMP,
    attributes JSONB,      -- All OTEL attributes
    events JSONB,          -- OTEL events
    status JSONB,          -- OTEL status
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_otel_traces_start_time ON otel_traces(start_time);
CREATE INDEX idx_otel_traces_service ON otel_traces(service_name);

-- ============================================================================
-- Parsed Traces (replay-specific schema)
-- ============================================================================
CREATE TABLE replay_traces (
    trace_id VARCHAR(255) PRIMARY KEY,
    run_id VARCHAR(255) UNIQUE,
    created_at TIMESTAMP,
    provider VARCHAR(100),
    model VARCHAR(255),
    prompt JSONB,          -- Array of messages [{role, content}]
    completion TEXT,
    parameters JSONB,      -- {temperature, top_p, max_tokens, ...}
    prompt_tokens INT,
    completion_tokens INT,
    total_tokens INT,
    latency_ms INT,
    metadata JSONB
);

CREATE INDEX idx_replay_traces_model ON replay_traces(model);
CREATE INDEX idx_replay_traces_created ON replay_traces(created_at);

-- ============================================================================
-- Tool Captures (for Freeze-Tools)
-- ============================================================================
CREATE TABLE tool_captures (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) REFERENCES replay_traces(trace_id),
    step_index INT,
    tool_name VARCHAR(255),
    args JSONB,                      -- Normalized JSON arguments
    args_hash VARCHAR(64),           -- SHA256(normalized_args) for fast lookup
    result JSONB,                    -- Tool result
    error TEXT,                      -- Error if tool failed
    latency_ms INT,
    risk_class VARCHAR(50),          -- read, write, destructive
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(trace_id, step_index)
);

CREATE INDEX idx_tool_captures_trace ON tool_captures(trace_id);
CREATE INDEX idx_tool_captures_hash ON tool_captures(tool_name, args_hash);

-- ============================================================================
-- Experiments
-- ============================================================================
CREATE TABLE experiments (
    id UUID PRIMARY KEY,
    name VARCHAR(255),
    baseline_trace_id VARCHAR(255) REFERENCES replay_traces(trace_id),
    status VARCHAR(50),              -- pending, running, completed, failed
    progress FLOAT,
    config JSONB,                    -- Variant matrix, weights, thresholds
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX idx_experiments_status ON experiments(status);
CREATE INDEX idx_experiments_created ON experiments(created_at);

-- ============================================================================
-- Experiment Runs (one per variant)
-- ============================================================================
CREATE TABLE experiment_runs (
    id UUID PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id) ON DELETE CASCADE,
    run_type VARCHAR(50),            -- baseline, variant
    variant_config JSONB,            -- {model, prompt, policy, ...}
    trace_id VARCHAR(255) REFERENCES replay_traces(trace_id),
    status VARCHAR(50),              -- pending, running, completed, failed
    error TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX idx_experiment_runs_exp ON experiment_runs(experiment_id);
CREATE INDEX idx_experiment_runs_status ON experiment_runs(status);

-- ============================================================================
-- Analysis Results (4D comparative analysis)
-- ============================================================================
CREATE TABLE analysis_results (
    id SERIAL PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id) ON DELETE CASCADE,
    baseline_run_id UUID REFERENCES experiment_runs(id),
    candidate_run_id UUID REFERENCES experiment_runs(id),

    -- Behavior analysis
    behavior_diff JSONB,             -- Tool sequence comparison
    first_divergence JSONB,          -- {step_index, reason, details}

    -- Safety analysis
    safety_diff JSONB,               -- Risk class changes

    -- Quality analysis
    similarity_score FLOAT,
    quality_metrics JSONB,

    -- Efficiency analysis
    token_delta INT,
    cost_delta FLOAT,
    latency_delta INT,

    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_analysis_exp ON analysis_results(experiment_id);

-- ============================================================================
-- Evaluators (configuration)
-- ============================================================================
CREATE TABLE evaluators (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE,
    type VARCHAR(50),                -- rule, llm_judge, rubric, human, ground_truth
    config JSONB,                    -- Evaluator-specific config
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- Evaluation Runs (per experiment)
-- ============================================================================
CREATE TABLE evaluation_runs (
    id UUID PRIMARY KEY,
    experiment_run_id UUID REFERENCES experiment_runs(id) ON DELETE CASCADE,
    status VARCHAR(50),              -- pending, running, completed, failed
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX idx_eval_runs_exp_run ON evaluation_runs(experiment_run_id);

-- ============================================================================
-- Evaluator Results (individual scores)
-- ============================================================================
CREATE TABLE evaluator_results (
    id SERIAL PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id) ON DELETE CASCADE,
    evaluator_id INT REFERENCES evaluators(id),
    scores JSONB,                    -- {dimension: score, ...}
    overall_score FLOAT,
    passed BOOLEAN,
    reasoning TEXT,
    metadata JSONB,
    evaluated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_eval_results_run ON evaluator_results(evaluation_run_id);
CREATE INDEX idx_eval_results_evaluator ON evaluator_results(evaluator_id);

-- ============================================================================
-- Human Evaluation Queue
-- ============================================================================
CREATE TABLE human_evaluation_queue (
    id UUID PRIMARY KEY,
    evaluation_run_id UUID REFERENCES evaluation_runs(id) ON DELETE CASCADE,
    experiment_run_id UUID REFERENCES experiment_runs(id) ON DELETE CASCADE,
    output TEXT,
    context JSONB,                   -- Task context, baseline output, etc.
    status VARCHAR(50),              -- pending, in_review, completed, skipped
    assigned_to VARCHAR(255),
    scores JSONB,                    -- Human-provided scores
    feedback TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    assigned_at TIMESTAMP,
    reviewed_at TIMESTAMP
);

CREATE INDEX idx_human_eval_status ON human_evaluation_queue(status);
CREATE INDEX idx_human_eval_assigned ON human_evaluation_queue(assigned_to);

-- ============================================================================
-- Ground Truth (reference answers)
-- ============================================================================
CREATE TABLE ground_truth (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(255) UNIQUE,
    task_type VARCHAR(100),
    input JSONB,
    expected_output TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_ground_truth_task ON ground_truth(task_id);
CREATE INDEX idx_ground_truth_type ON ground_truth(task_type);

-- ============================================================================
-- Evaluation Summary (aggregated results)
-- ============================================================================
CREATE TABLE evaluation_summary (
    id SERIAL PRIMARY KEY,
    experiment_id UUID REFERENCES experiments(id) ON DELETE CASCADE,
    experiment_run_id UUID REFERENCES experiment_runs(id) ON DELETE CASCADE,
    overall_score FLOAT,
    passed BOOLEAN,
    evaluator_scores JSONB,          -- Per-evaluator breakdown
    rank INT,
    is_best BOOLEAN,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_eval_summary_exp ON evaluation_summary(experiment_id);
CREATE INDEX idx_eval_summary_rank ON evaluation_summary(experiment_id, rank);
```

---

## Evaluation Framework

### Overview

The evaluation framework scores model outputs to determine **which model/configuration produces the best results**.

### Evaluator Types

#### 1. Rule-Based Evaluator

**Purpose**: Fast, deterministic checks against output structure and content

**Use Cases**:
- Format validation (JSON structure, required fields)
- Content requirements (must include X, must not include Y)
- Length constraints (min/max words/tokens)
- Pattern matching (email format, code syntax)

**Configuration**:
```yaml
type: rule
name: format_validation
rules:
  - name: valid_json
    check: is_valid_json
    weight: 1.0

  - name: required_fields
    check: has_fields
    params:
      fields: [status, message, data]
    weight: 1.0

  - name: max_length
    check: max_tokens
    params:
      max: 500
    weight: 0.5
```

**Implementation**:
```go
type RuleBasedEvaluator struct {
    Rules []Rule
}

type Rule struct {
    Name   string
    Check  func(output string, params map[string]interface{}) (bool, error)
    Weight float64
    Params map[string]interface{}
}
```

#### 2. LLM-as-a-Judge Evaluator

**Purpose**: Use an LLM to evaluate outputs based on subjective criteria

**Use Cases**:
- Subjective quality assessment (tone, style, appropriateness)
- Semantic correctness (factuality, reasoning quality)
- Creative tasks (humor, engagement, originality)
- Complex criteria (cultural sensitivity, nuance)

**Configuration**:
```yaml
type: llm_judge
name: quality_assessment
judge_model: claude-3-5-sonnet-20241022
criteria:
  - "Clarity: Is the output clear and easy to understand?"
  - "Accuracy: Is the information correct?"
  - "Helpfulness: Does it address the user's needs?"
score_scale: 10
weight: 2.0
```

**Evaluation Prompt**:
```
You are evaluating the quality of an AI assistant's output.

Task: {task_description}
Output: {model_output}

Evaluate the output on the following criteria (score 1-10 for each):
1. Clarity: Is the output clear and easy to understand?
2. Accuracy: Is the information correct?
3. Helpfulness: Does it address the user's needs?

Provide scores in JSON format:
{
  "scores": {
    "clarity": <score>,
    "accuracy": <score>,
    "helpfulness": <score>
  },
  "overall": <average_score>,
  "reasoning": "Brief explanation"
}
```

#### 3. Rubric-Based Evaluator

**Purpose**: Standardized scoring against predefined rubric dimensions

**Use Cases**:
- Consistent evaluation across experiments
- Weighted multi-dimensional assessment
- Educational/grading scenarios
- Quality standards enforcement

**Configuration**:
```yaml
type: rubric
name: customer_service_rubric
rubric_file: ./rubrics/customer_service.yaml
weight: 1.5
```

**Rubric Definition** (`customer_service.yaml`):
```yaml
rubric:
  name: Customer Service Email Evaluation
  description: Evaluates quality of customer service emails

  dimensions:
    - name: tone
      weight: 0.25
      levels:
        - score: 10
          description: Warm, empathetic, professional
          indicators:
            - Uses customer name
            - Acknowledges concern
            - Positive language
        - score: 7
          description: Professional but neutral
        - score: 4
          description: Cold or robotic
        - score: 1
          description: Unprofessional

    - name: completeness
      weight: 0.35
      levels:
        - score: 10
          required_elements:
            - refund_amount
            - processing_timeframe
            - next_steps
        - score: 7
          required_elements:
            - refund_amount
            - processing_timeframe
        - score: 4
          required_elements:
            - refund_amount

  pass_threshold: 7.0
```

#### 4. Ground Truth Evaluator

**Purpose**: Compare outputs against known correct answers

**Use Cases**:
- Code generation (compare against reference implementation)
- Data extraction (compare against labeled dataset)
- Math/logic problems (compare against correct answer)
- Translation (compare against reference translation)

**Configuration**:
```yaml
type: ground_truth
name: accuracy_check
similarity_threshold: 0.85
weight: 2.0
```

**Storage**:
```sql
INSERT INTO ground_truth (task_id, task_type, input, expected_output)
VALUES (
  'task-123',
  'code_generation',
  '{"prompt": "Write a function to reverse a string"}',
  'def reverse_string(s: str) -> str:\n    return s[::-1]'
);
```

#### 5. Human-in-the-Loop Evaluator

**Purpose**: Queue outputs for human review when automated evaluation is insufficient

**Use Cases**:
- Edge cases automated evaluation can't handle
- High-stakes decisions requiring human judgment
- Training data collection for future automated evaluators
- Spot-checking automated evaluation accuracy

**Configuration**:
```yaml
type: human
name: manual_review
queue_conditions:
  - "overall_score < 0.7"           # Low automated score
  - "rule:format_validation == 0"   # Failed critical check
  - "random_sample: 0.1"            # 10% random sampling
weight: 3.0
```

**Workflow**:
1. System queues run for review based on conditions
2. Reviewer retrieves pending reviews via API/CLI
3. Reviewer submits scores and feedback
4. System re-calculates overall score with human input
5. Rankings updated if needed

### Evaluation Composition

**Multiple Evaluators**:
```yaml
evaluation:
  evaluators:
    - type: rule
      name: format_check
      weight: 2.0              # Critical

    - type: llm_judge
      name: quality
      weight: 1.0              # Important

    - type: rubric
      name: standards
      weight: 1.5              # Very important

    - type: human
      name: final_review
      weight: 3.0              # Most important

  aggregation: weighted_average
  pass_threshold: 0.75
```

**Score Aggregation**:
```
overall_score = (
    rule_score * 2.0 +
    llm_judge_score * 1.0 +
    rubric_score * 1.5 +
    human_score * 3.0
) / (2.0 + 1.0 + 1.5 + 3.0)

passed = overall_score >= 0.75
```

### Winner Determination

After evaluation, CMDR ranks all variants:

```json
{
  "winner": {
    "run_id": "run-baseline",
    "model": "gpt-4",
    "overall_score": 0.87,
    "rank": 1,
    "reasons": [
      "Highest overall score (0.87)",
      "Passed all evaluators",
      "Best clarity and completeness scores"
    ]
  },
  "runners_up": [
    {
      "run_id": "run-variant-1",
      "model": "claude-3-5-sonnet-20241022",
      "overall_score": 0.82,
      "rank": 2,
      "cost_savings": "25%",
      "trade_offs": "Slightly lower clarity but comparable accuracy"
    }
  ],
  "failed": [
    {
      "run_id": "run-variant-2",
      "model": "gpt-3.5-turbo",
      "overall_score": 0.65,
      "rank": 3,
      "failure_reasons": [
        "Overall score below threshold (0.65 < 0.75)",
        "Failed format validation"
      ]
    }
  ]
}
```

---

## API Specification

### REST API Endpoints

#### Experiments

```
POST   /api/v1/experiments
GET    /api/v1/experiments
GET    /api/v1/experiments/:id
DELETE /api/v1/experiments/:id

GET    /api/v1/experiments/:id/results
GET    /api/v1/experiments/:id/analysis
GET    /api/v1/experiments/:id/report
```

#### Evaluators

```
GET    /api/v1/evaluators
POST   /api/v1/evaluators
GET    /api/v1/evaluators/:id
PUT    /api/v1/evaluators/:id
DELETE /api/v1/evaluators/:id
```

#### Evaluation

```
POST   /api/v1/experiments/:id/evaluate
GET    /api/v1/experiments/:id/evaluation
GET    /api/v1/experiments/:id/evaluation/summary
```

#### Human Review

```
POST   /api/v1/evaluations/human/queue
GET    /api/v1/evaluations/human/pending
GET    /api/v1/evaluations/human/:id
POST   /api/v1/evaluations/human/:id/review
```

#### Ground Truth

```
GET    /api/v1/ground-truth
POST   /api/v1/ground-truth
GET    /api/v1/ground-truth/:id
PUT    /api/v1/ground-truth/:id
DELETE /api/v1/ground-truth/:id
```

#### Health

```
GET    /health
GET    /ready
GET    /metrics
```

### Example: Create Experiment

**Request**:
```bash
POST /api/v1/experiments
Content-Type: application/json

{
  "name": "GPT-4 vs Claude comparison",
  "baseline_trace_id": "abc123",
  "variants": [
    {
      "name": "claude-sonnet",
      "model": "claude-3-5-sonnet-20241022"
    },
    {
      "name": "gpt4",
      "model": "gpt-4"
    }
  ],
  "analysis": {
    "dimensions": ["behavior", "safety", "quality", "efficiency"]
  },
  "evaluation": {
    "evaluators": ["rule_check", "llm_judge", "rubric"],
    "pass_threshold": 0.75
  }
}
```

**Response**:
```json
{
  "experiment_id": "exp-789",
  "status": "pending",
  "created_at": "2026-02-26T10:00:00Z",
  "estimated_completion": "2026-02-26T10:05:00Z"
}
```

### Example: Get Evaluation Summary

**Request**:
```bash
GET /api/v1/experiments/exp-789/evaluation/summary
```

**Response**:
```json
{
  "experiment_id": "exp-789",
  "status": "completed",
  "winner": {
    "run_id": "run-baseline",
    "model": "gpt-4",
    "overall_score": 0.87,
    "rank": 1
  },
  "summary": {
    "total_runs": 3,
    "passed": 2,
    "failed": 1,
    "best_model": "gpt-4",
    "cost_efficient_alternative": {
      "model": "claude-3-5-sonnet-20241022",
      "score": 0.82,
      "cost_savings": "25%"
    }
  },
  "detailed_scores": [
    {
      "run_id": "run-baseline",
      "model": "gpt-4",
      "evaluator_scores": {
        "rule_check": 1.0,
        "llm_judge": 0.85,
        "rubric": 0.80
      },
      "overall_score": 0.87,
      "passed": true,
      "rank": 1
    }
  ]
}
```

---

## CLI Interface

### Commands

#### Experiments

```bash
# Create experiment
cmdr experiment run \
  --baseline-trace abc123 \
  --variants variants.yaml \
  --output report.json

# List experiments
cmdr experiment list

# Get experiment status
cmdr experiment status exp-789

# Get experiment results
cmdr experiment results exp-789 \
  --format json|markdown|table

# Get detailed report
cmdr experiment report exp-789 \
  --output report.md

# Cancel experiment
cmdr experiment cancel exp-789
```

#### Evaluation

```bash
# Run evaluation
cmdr eval run \
  --experiment exp-789 \
  --config evaluation.yaml

# Get evaluation results
cmdr eval results exp-789 \
  --format json|table|markdown

# Get evaluation summary with winner
cmdr eval summary exp-789

# Configure evaluators
cmdr eval config \
  --experiment exp-789 \
  --evaluators rule_check,llm_judge,rubric
```

#### Human Review

```bash
# Queue for human review
cmdr eval human queue \
  --experiment exp-789 \
  --run run-variant-1

# List pending reviews
cmdr eval human pending \
  --assigned-to me

# Submit review
cmdr eval human review review-456 \
  --scores '{"clarity": 8, "accuracy": 9, "helpfulness": 7}' \
  --feedback "Good output but could be more concise" \
  --approve

# Get review details
cmdr eval human get review-456
```

#### Ground Truth

```bash
# Add ground truth
cmdr ground-truth add \
  --task-id task-123 \
  --input input.json \
  --expected-output expected.txt \
  --task-type code_generation

# List ground truth
cmdr ground-truth list \
  --task-type code_generation

# Update ground truth
cmdr ground-truth update task-123 \
  --expected-output new_expected.txt

# Delete ground truth
cmdr ground-truth delete task-123
```

### Configuration Files

**variants.yaml**:
```yaml
baseline:
  trace_id: abc123

variants:
  - name: claude-sonnet
    model: claude-3-5-sonnet-20241022

  - name: gpt4
    model: gpt-4

  - name: prompt-variant
    model: claude-3-5-sonnet-20241022
    prompt_template: alternative_prompt.txt

analysis:
  dimensions:
    - behavior
    - safety
    - quality
    - efficiency
  weights:
    behavior: 0.3
    safety: 0.4
    quality: 0.2
    efficiency: 0.1
  fail_on:
    - new_destructive_tools
    - policy_violations
```

**evaluation.yaml**:
```yaml
evaluation:
  evaluators:
    - type: rule
      name: format_validation
      enabled: true
      rules:
        - name: valid_json
          check: is_valid_json
          weight: 1.0

    - type: llm_judge
      name: quality_assessment
      enabled: true
      judge_model: claude-3-5-sonnet-20241022
      criteria:
        - "Clarity: Is the output clear?"
        - "Accuracy: Is the information correct?"
        - "Helpfulness: Does it help the user?"
      weight: 2.0

    - type: rubric
      name: customer_service
      enabled: true
      rubric_file: ./rubrics/customer_service.yaml
      weight: 1.5

    - type: human
      name: manual_review
      enabled: true
      queue_conditions:
        - "overall_score < 0.7"
        - "random_sample: 0.1"
      weight: 3.0

  aggregation: weighted_average
  pass_threshold: 0.75
  fail_fast:
    - "rule:format_validation < 1.0"
```

---

## Implementation Phases

### Phase 1: Foundation + OTEL + Freeze-Tools (Week 1)

**Goal**: Establish core infrastructure for trace ingestion and deterministic replay

**Tasks**:
1. Initialize Go module and project structure
2. Implement configuration layer (envconfig)
3. Set up structured logging (zap)
4. Implement OTLP receiver (gRPC + HTTP)
5. Implement OTEL span parser (gen_ai.* attributes → replay schema)
6. Implement PostgreSQL storage layer
7. Implement Freeze-Tools MCP server:
   - Capture mode (proxy + record)
   - Freeze mode (lookup + return)
   - Argument normalization and matching
8. Write unit tests for all components
9. Set up CI pipeline (GitHub Actions)

**Deliverables**:
- CMDR service can receive OTEL traces
- Traces parsed and stored in replay schema
- Freeze-Tools MCP server operational
- Unit test coverage > 80%

### Phase 2: Replay Engine + Agentgateway Client (Week 2)

**Goal**: Enable experiment orchestration and variant replay

**Tasks**:
1. Implement agentgateway HTTP client with retry logic
2. Implement experiment orchestration engine
3. Implement worker pool for concurrent replays
4. Integrate Freeze-Tools into replay flow
5. Implement experiment status tracking
6. Write integration tests (with test agentgateway)
7. End-to-end replay test (capture → freeze → replay)

**Deliverables**:
- Experiments can be submitted via Go API
- Baseline trace captured with tool freezing
- Variants replayed with frozen tools
- Worker pool handles concurrent replays
- Integration tests passing

### Phase 3: Analysis + Evaluation Framework (Week 3)

**Goal**: Implement 4D analysis and comprehensive evaluation

**Tasks**:
1. Implement behavior analyzer:
   - Tool sequence diff
   - Argument drift detection
   - First divergence identification
2. Implement safety analyzer:
   - Risk class tracking
   - Policy decision changes
3. Implement quality analyzer:
   - Semantic similarity (embeddings)
   - Rubric scoring engine
4. Implement efficiency analyzer:
   - Token/cost deltas
   - Latency comparison
5. Implement evaluation framework:
   - Evaluator interface and registry
   - Rule-based evaluator
   - LLM-judge evaluator
   - Rubric evaluator
   - Human-loop evaluator (queue + workflow)
   - Ground-truth evaluator
6. Implement evaluation aggregation and winner determination
7. Write tests for all analyzers and evaluators

**Deliverables**:
- 4D analysis working (behavior, safety, quality, efficiency)
- All 5 evaluator types implemented
- Evaluation aggregation and ranking functional
- Human review queue operational
- Test coverage > 80%

### Phase 4: Reporting + API + CLI + Hardening (Week 4)

**Goal**: Complete user-facing interfaces and production readiness

**Tasks**:
1. Implement report generator:
   - JSON format (machine-readable)
   - Markdown format (human-readable)
   - Scorecard with winner and rankings
2. Implement REST API:
   - Experiment endpoints
   - Evaluation endpoints
   - Human review endpoints
   - Ground truth endpoints
   - Health/ready/metrics
3. Implement CLI with Cobra:
   - Experiment commands
   - Evaluation commands
   - Human review commands
   - Ground truth commands
4. Add authentication/authorization (JWT)
5. Performance testing and optimization
6. Documentation:
   - API documentation (OpenAPI spec)
   - Architecture documentation
   - Usage guides with examples
   - Rubric creation guide
7. Create Docker image and Kubernetes manifests
8. Deploy to staging environment
9. Run end-to-end acceptance tests

**Deliverables**:
- Complete REST API documented
- Full-featured CLI
- Report generation (JSON + Markdown)
- Docker image built
- Kubernetes deployment configs
- Documentation complete
- Staging deployment successful
- E2E tests passing

---

## Success Criteria

### Functional Requirements

✅ **Deterministic Replay**
- Baseline trace captured with all tool calls
- Freeze-Tools correctly matches tool calls by normalized arguments
- Variant replays use frozen tool results (zero re-execution)
- Same experiment run twice produces identical results

✅ **Analysis (4 Dimensions)**
- Behavior: Tool sequence diffs, argument drift, first divergence
- Safety: Risk class tracking, policy changes
- Quality: Similarity scoring, rubric evaluation
- Efficiency: Token/cost/latency deltas

✅ **Evaluation Framework**
- All 5 evaluator types functional
- Evaluators composable with weights
- Pass/fail thresholds enforced
- Winner determination with rankings
- Human review queue and workflow
- Ground truth management

✅ **API & CLI**
- REST API accepts experiments, returns results
- CLI submits experiments and retrieves reports
- Human review workflow functional
- Reports available in JSON and Markdown

### Performance Requirements

✅ **Throughput**
- OTLP receiver: 1000 traces/sec
- Worker pool: 10 concurrent replays per model
- API latency: p95 < 200ms (excluding LLM calls)
- Human review queue: 1000+ pending reviews

✅ **Reliability**
- Service uptime: 99.9%
- No data loss on OTLP ingestion
- Graceful degradation on evaluation failures
- Automatic retry on transient errors

✅ **Scalability**
- Horizontal scaling of replay workers
- Database handles 1M+ traces
- Evaluation scales to 100+ evaluators

### Quality Requirements

✅ **Testing**
- Unit test coverage > 80%
- Integration tests with real services
- End-to-end test: capture → replay → analyze → evaluate → report
- Load testing: 100 concurrent experiments

✅ **Documentation**
- API fully documented (OpenAPI)
- Architecture diagrams complete
- Usage guides with examples
- Troubleshooting guide

✅ **Observability**
- Structured logging with trace IDs
- Prometheus metrics exported
- Health and readiness checks
- Error tracking (Sentry/similar)

---

## Appendix

### Environment Variables

```bash
# Service
CMDR_API_PORT=8080
CMDR_LOG_LEVEL=info

# OTLP Receiver
CMDR_OTLP_GRPC_ENDPOINT=0.0.0.0:4317
CMDR_OTLP_HTTP_ENDPOINT=0.0.0.0:4318

# Trace Fetching (optional - for historical traces)
CMDR_JAEGER_URL=http://jaeger:16686
CMDR_TEMPO_URL=http://tempo:3200

# Agentgateway Client
CMDR_AGENTGATEWAY_URL=http://agentgateway:8080
CMDR_AGENTGATEWAY_TIMEOUT=60s
CMDR_AGENTGATEWAY_RETRY_ATTEMPTS=3

# Freeze-Tools MCP
CMDR_FREEZETOOLS_PORT=9090

# Database
CMDR_POSTGRES_URL=postgres://user:pass@localhost:5432/cmdr
CMDR_POSTGRES_MAX_CONNS=50

# Replay Engine
CMDR_WORKER_POOL_SIZE=10
CMDR_MAX_CONCURRENT_REPLAYS=5

# Evaluation
CMDR_LLM_JUDGE_MODEL=claude-3-5-sonnet-20241022
CMDR_EVAL_TIMEOUT=120s

# Authentication (optional)
CMDR_JWT_SECRET=your-secret-key
CMDR_JWT_EXPIRY=24h
```

### Deployment

**Docker**:
```bash
docker build -t cmdr:latest .
docker run -p 8080:8080 \
  -e CMDR_POSTGRES_URL=postgres://... \
  -e CMDR_AGENTGATEWAY_URL=http://... \
  cmdr:latest
```

**Kubernetes**:
```bash
kubectl apply -f deployments/kubernetes/
kubectl get pods -l app=cmdr
kubectl logs -f deployment/cmdr
```

### References

- DRAFT_PLAN.md - Original CMDR vision
- IMPLEMENTATION_PLAN.md - Original LLM Replay Service plan
- Research Document - Architectural decisions and comparisons
- OpenTelemetry Specification - OTLP trace format
- MCP Protocol - Model Context Protocol for tool integration

---

**Document Version**: 2.0
**Last Updated**: 2026-02-26
**Status**: Final - Ready for Implementation
