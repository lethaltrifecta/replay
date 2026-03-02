# Refactoring Opportunities

This is a prioritized list based on the current codebase state.

## 1) Split `storage.Storage` into narrow interfaces

Problem:
- `pkg/storage/interface.go` defines a very large interface used everywhere.
- Unit tests (for example `pkg/otelreceiver/receiver_test.go`) must implement many unrelated stubs.

Recommendation:
- Create focused interfaces by feature area:
  - `TraceWriter` (`CreateOTELTrace`, `CreateReplayTrace`, `CreateToolCapture`)
  - `DriftStore` (baseline + drift result methods)
  - `ExperimentStore`, `EvalStore`, etc.
- Depend on smallest required interface per package.

Impact:
- Smaller mocks, easier tests, clearer boundaries.

## 2) Extract shared DB bootstrap logic from CLI commands

Problem:
- `cmd/cmdr/commands/drift.go` has command-local DB bootstrapping (`connectDB`) including config load + migration.
- Similar setup exists in `serve.go`.

Recommendation:
- Introduce a shared helper (or command context) for config/logger/storage initialization.
- Keep migration policy explicit (e.g., `serve` runs migrations, read-only commands may opt out).

Impact:
- Less duplication, consistent startup behavior, easier future commands.

## 3) Replace map-based drift details with typed structs

Problem:
- `pkg/drift/compare.go` stores report details in `map[string]interface{}`.
- CLI/tests rely on type assertions and string keys.

Recommendation:
- Define typed structs for:
  - dimension scores
  - risk details
  - model/provider change metadata
  - scoring weights
- Serialize typed structs into JSONB when persisting.

Impact:
- Compile-time safety and easier evolution of drift output schema.

## 4) Improve `serve` startup/shutdown lifecycle

Problem:
- Startup uses fixed `time.Sleep(500ms)` to infer readiness.
- Shutdown path contains TODOs and doesn’t coordinate all components.

Recommendation:
- Add explicit readiness signaling from receiver start routines.
- Use `errgroup` + context cancellation for lifecycle management.
- Ensure graceful receiver stop and deterministic exit codes.

Impact:
- More reliable local/dev/prod behavior and easier debugging.

## 5) Harden OTLP HTTP handler input limits

Problem:
- `pkg/otelreceiver/receiver.go` reads full request body without size limits.

Recommendation:
- Add `http.MaxBytesReader` and reject oversized payloads.
- Optionally validate `Content-Type` and return clearer error responses.

Impact:
- Better resilience against malformed/large requests.

## 6) Migrations: run all files in deterministic order

Problem:
- `Migrate()` manually reads hardcoded files (`001`, `002`).

Recommendation:
- Enumerate migration files from embedded FS, sort lexicographically, execute in order.
- Keep idempotent SQL semantics for reruns.

Impact:
- Lower maintenance burden when adding migrations.

## 7) Break up monolithic drift command function

Problem:
- `runDriftCheck` in `cmd/cmdr/commands/drift.go` mixes:
  - baseline resolution
  - storage reads
  - fingerprint extraction
  - scoring
  - persistence
  - output formatting

Recommendation:
- Split into small helpers or a service object:
  - `resolveBaseline()`
  - `loadTraceData()`
  - `computeDrift()`
  - `printDriftSummary()`

Impact:
- Better readability, testability, and easier addition of `drift status/watch`.

## 8) Align operational scripts with current runtime behavior

Problem:
- Some scripts/docs still assume an HTTP API on `:8080` for readiness checks.

Recommendation:
- Use OTLP HTTP health endpoint (`:4318/health`) consistently.
- Keep script assumptions synchronized with `serve` behavior.

Impact:
- Fewer false negatives during local validation.
