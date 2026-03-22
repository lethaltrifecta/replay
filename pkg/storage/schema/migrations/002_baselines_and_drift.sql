-- baselines: marks a trace as known-good for drift comparison
CREATE TABLE IF NOT EXISTS baselines (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255),
    description TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- drift_results: stores outcome of comparing a trace against its baseline
CREATE TABLE IF NOT EXISTS drift_results (
    id SERIAL PRIMARY KEY,
    trace_id VARCHAR(255) NOT NULL,
    baseline_trace_id VARCHAR(255) NOT NULL,
    drift_score FLOAT NOT NULL CHECK (drift_score >= 0 AND drift_score <= 1),
    verdict VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (verdict IN ('pass', 'warn', 'fail', 'pending')),
    details JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_drift_results_trace ON drift_results(trace_id);
CREATE INDEX IF NOT EXISTS idx_drift_results_baseline ON drift_results(baseline_trace_id);
CREATE INDEX IF NOT EXISTS idx_drift_results_created ON drift_results(created_at);
