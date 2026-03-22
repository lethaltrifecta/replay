-- Deduplicate existing rows: keep the lowest id for each (trace_id, baseline_trace_id) pair.
DELETE FROM drift_results
WHERE id NOT IN (
    SELECT MIN(id) FROM drift_results GROUP BY trace_id, baseline_trace_id
);

-- Add unique constraint so concurrent inserts cannot create duplicates.
CREATE UNIQUE INDEX IF NOT EXISTS idx_drift_results_trace_baseline
    ON drift_results (trace_id, baseline_trace_id);
