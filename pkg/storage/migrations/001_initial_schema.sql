-- CMDR Database Schema v1
-- Migration: 001_initial_schema

-- ============================================================================
-- OTEL Traces (raw capture from OTLP)
-- A single trace contains multiple spans (one per operation).
-- ============================================================================
CREATE TABLE IF NOT EXISTS otel_traces (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,
    parent_span_id VARCHAR(255),
    service_name VARCHAR(255) NOT NULL,
    span_kind VARCHAR(50) NOT NULL,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    attributes JSONB,
    events JSONB,
    status JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(trace_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_otel_traces_trace_id ON otel_traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_otel_traces_start_time ON otel_traces(start_time);
CREATE INDEX IF NOT EXISTS idx_otel_traces_service ON otel_traces(service_name);

-- ============================================================================
-- Parsed Traces (replay-specific)
-- Each row is one LLM call (span). A multi-step agent run produces multiple
-- rows sharing the same trace_id, ordered by step_index.
-- ============================================================================
CREATE TABLE IF NOT EXISTS replay_traces (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,
    run_id VARCHAR(255) NOT NULL,
    step_index INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    provider VARCHAR(100) NOT NULL,
    model VARCHAR(255) NOT NULL,
    prompt JSONB NOT NULL,
    completion TEXT NOT NULL,
    parameters JSONB,
    prompt_tokens INT NOT NULL DEFAULT 0,
    completion_tokens INT NOT NULL DEFAULT 0,
    total_tokens INT NOT NULL DEFAULT 0,
    latency_ms INT NOT NULL DEFAULT 0,
    metadata JSONB,
    UNIQUE(trace_id, span_id)
);

CREATE INDEX IF NOT EXISTS idx_replay_traces_trace_id ON replay_traces(trace_id);
CREATE INDEX IF NOT EXISTS idx_replay_traces_model ON replay_traces(model);
CREATE INDEX IF NOT EXISTS idx_replay_traces_created ON replay_traces(created_at);

-- ============================================================================
-- Tool Captures (for Freeze-Tools)
-- ============================================================================
CREATE TABLE IF NOT EXISTS tool_captures (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL,
    span_id VARCHAR(255) NOT NULL,
    step_index INT NOT NULL,
    tool_name VARCHAR(255) NOT NULL,
    args JSONB NOT NULL,
    args_hash VARCHAR(64) NOT NULL,
    result JSONB,
    error TEXT,
    latency_ms INT NOT NULL DEFAULT 0,
    risk_class VARCHAR(50) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(trace_id, span_id, step_index)
);

CREATE INDEX IF NOT EXISTS idx_tool_captures_trace ON tool_captures(trace_id);
CREATE INDEX IF NOT EXISTS idx_tool_captures_span ON tool_captures(trace_id, span_id);
CREATE INDEX IF NOT EXISTS idx_tool_captures_hash ON tool_captures(tool_name, args_hash);

-- ============================================================================
-- Experiments
-- baseline_trace_id refers to a trace (agent run), not a single span.
-- ============================================================================
CREATE TABLE IF NOT EXISTS experiments (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    baseline_trace_id VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    progress FLOAT NOT NULL DEFAULT 0.0,
    config JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_experiments_status ON experiments(status);
CREATE INDEX IF NOT EXISTS idx_experiments_created ON experiments(created_at);

-- ============================================================================
-- Experiment Runs (one per variant)
-- ============================================================================
CREATE TABLE IF NOT EXISTS experiment_runs (
    id UUID PRIMARY KEY,
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    run_type VARCHAR(50) NOT NULL,
    variant_config JSONB NOT NULL,
    trace_id VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    error TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_experiment_runs_exp ON experiment_runs(experiment_id);
CREATE INDEX IF NOT EXISTS idx_experiment_runs_status ON experiment_runs(status);

-- ============================================================================
-- Analysis Results (4D comparative analysis)
-- ============================================================================
CREATE TABLE IF NOT EXISTS analysis_results (
    id SERIAL PRIMARY KEY,
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    baseline_run_id UUID NOT NULL REFERENCES experiment_runs(id),
    candidate_run_id UUID NOT NULL REFERENCES experiment_runs(id),
    behavior_diff JSONB,
    first_divergence JSONB,
    safety_diff JSONB,
    similarity_score FLOAT,
    quality_metrics JSONB,
    token_delta INT,
    cost_delta FLOAT,
    latency_delta INT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_analysis_exp ON analysis_results(experiment_id);

-- ============================================================================
-- Evaluators (configuration)
-- ============================================================================
CREATE TABLE IF NOT EXISTS evaluators (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    type VARCHAR(50) NOT NULL,
    config JSONB,
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ============================================================================
-- Evaluation Runs (per experiment)
-- ============================================================================
CREATE TABLE IF NOT EXISTS evaluation_runs (
    id UUID PRIMARY KEY,
    experiment_run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    started_at TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_eval_runs_exp_run ON evaluation_runs(experiment_run_id);

-- ============================================================================
-- Evaluator Results (individual scores)
-- ============================================================================
CREATE TABLE IF NOT EXISTS evaluator_results (
    id SERIAL PRIMARY KEY,
    evaluation_run_id UUID NOT NULL REFERENCES evaluation_runs(id) ON DELETE CASCADE,
    evaluator_id INT NOT NULL REFERENCES evaluators(id),
    scores JSONB,
    overall_score FLOAT NOT NULL,
    passed BOOLEAN NOT NULL,
    reasoning TEXT,
    metadata JSONB,
    evaluated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_results_run ON evaluator_results(evaluation_run_id);
CREATE INDEX IF NOT EXISTS idx_eval_results_evaluator ON evaluator_results(evaluator_id);

-- ============================================================================
-- Human Evaluation Queue
-- ============================================================================
CREATE TABLE IF NOT EXISTS human_evaluation_queue (
    id UUID PRIMARY KEY,
    evaluation_run_id UUID NOT NULL REFERENCES evaluation_runs(id) ON DELETE CASCADE,
    experiment_run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    output TEXT NOT NULL,
    context JSONB,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    assigned_to VARCHAR(255),
    scores JSONB,
    feedback TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    assigned_at TIMESTAMP,
    reviewed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_human_eval_status ON human_evaluation_queue(status);
CREATE INDEX IF NOT EXISTS idx_human_eval_assigned ON human_evaluation_queue(assigned_to);

-- ============================================================================
-- Ground Truth (reference answers)
-- ============================================================================
CREATE TABLE IF NOT EXISTS ground_truth (
    id SERIAL PRIMARY KEY,
    task_id VARCHAR(255) UNIQUE NOT NULL,
    task_type VARCHAR(100) NOT NULL,
    input JSONB,
    expected_output TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ground_truth_task ON ground_truth(task_id);
CREATE INDEX IF NOT EXISTS idx_ground_truth_type ON ground_truth(task_type);

-- ============================================================================
-- Evaluation Summary (aggregated results)
-- ============================================================================
CREATE TABLE IF NOT EXISTS evaluation_summary (
    id SERIAL PRIMARY KEY,
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    experiment_run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    overall_score FLOAT NOT NULL,
    passed BOOLEAN NOT NULL,
    evaluator_scores JSONB,
    rank INT NOT NULL,
    is_best BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_eval_summary_exp ON evaluation_summary(experiment_id);
CREATE INDEX IF NOT EXISTS idx_eval_summary_rank ON evaluation_summary(experiment_id, rank);
