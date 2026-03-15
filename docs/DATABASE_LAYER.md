# Database Layer

PostgreSQL storage backend for CMDR. All schema is managed via embedded SQL migrations applied on startup.

## Schema

### Core tables

| Table | Purpose | Unique constraint |
|-------|---------|-------------------|
| `otel_traces` | Raw OTEL spans from OTLP receiver | `(trace_id, span_id)` |
| `replay_traces` | Parsed LLM calls (model, prompt, completion, tokens, latency) | `(trace_id, span_id)` |
| `tool_captures` | Recorded tool calls with args hash + risk class | `(trace_id, span_id, step_index)` |
| `baselines` | Traces marked as known-good for drift comparison | `(trace_id)` |
| `drift_results` | Drift check outcomes (score, verdict, dimension breakdown) | `(candidate_trace_id, baseline_trace_id)` |

### Experiment tables

| Table | Purpose |
|-------|---------|
| `experiments` | Experiment metadata, links to `baseline_trace_id` |
| `experiment_runs` | One row per variant run in an experiment |
| `analysis_results` | 4D comparative analysis (behavior, safety, quality, efficiency) |

### Evaluation tables (scaffolded)

| Table | Purpose |
|-------|---------|
| `evaluators` | Evaluator configurations |
| `evaluation_runs` | Evaluation execution records |
| `evaluator_results` | Individual evaluator scores |
| `evaluation_summary` | Aggregated rankings |

## Migrations

Migrations are embedded via `go:embed` and applied idempotently on startup:

1. `001_initial_schema.sql` — Core tables, indexes, foreign keys
2. `002_baselines_and_drift.sql` — Baselines table + drift_results table
3. `003_drift_unique_constraint.sql` — Unique constraint on drift_results

## Key design decisions

- **Idempotent inserts** — `ON CONFLICT DO NOTHING` for span ingestion (OTLP may deliver duplicates)
- **JSONB columns** — Prompts, configs, and scores stored as JSONB for flexible schema evolution
- **Parameterized queries** — All queries use `$1, $2, ...` placeholders (no string interpolation)
- **CASCADE deletes** — `experiment → experiment_runs`, `experiment → analysis_results`, `evaluation_run → evaluator_results`
- **Connection pooling** — Configurable max connections, 1-hour connection lifetime, idle management

## Storage interface

The `Storage` interface (`pkg/storage/interface.go`) defines all operations. `PostgresStorage` is the only implementation.

Key method groups:

- **Traces** — `CreateOTELTrace`, `CreateReplayTrace`, `ListReplayTraces`
- **Tool captures** — `CreateToolCapture`, `GetToolCapturesByTrace`, `GetToolCaptureByArgs`
- **Baselines** — `MarkTraceAsBaseline`, `UnmarkBaseline`, `ListBaselines`, `GetBaseline`
- **Drift** — `CreateDriftResult`, `GetDriftResults`, `ListDriftResults`, `HasDriftResultForBaseline`
- **Experiments** — `CreateExperiment`, `CreateExperimentRun`, `UpdateExperimentRun`

## Usage

```go
storage, err := storage.NewPostgresStorage(connectionURL, maxConns)
if err != nil {
    return err
}
defer storage.Close()

if err := storage.Migrate(ctx); err != nil {
    return err
}
```

## Testing

```bash
# Requires make dev-up (PostgreSQL must be running)
make test-storage
```
