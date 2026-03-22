# Hackathon Demo Runbook

This runbook is designed for a 2-3 minute live demo with deterministic outputs.

## Goal

Show that CMDR can:
- detect drift against a known-good baseline
- block a risky rollout in a CI-friendly way (`exit 1`)
- allow a safe rollout (`exit 0`)

## Prerequisites

```bash
make dev-up
make build
```

If your binary path is custom, set:

```bash
export CMDR=/path/to/cmdr
```

## One-Command Demo

```bash
make demo
```

This runs:
1. `cmdr demo seed`
2. `cmdr drift check demo-baseline-001 demo-drifted-002`
3. `cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger` (expected fail)
4. `cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet` (expected pass)

## Manual Demo (Presenter Mode)

### 1) Seed deterministic data

```bash
./bin/cmdr demo seed
```

Expected:
- baseline trace `demo-baseline-001` seeded
- drifted trace `demo-drifted-002` seeded
- baseline marker set for `demo-baseline-001`

Note: seeding is idempotent; reruns skip duplicate tool-capture rows.

### 2) Show drift detection

```bash
./bin/cmdr drift check demo-baseline-001 demo-drifted-002
```

Expected:
- non-pass verdict due to destructive behavior divergence

### 3) Show deployment gate blocks dangerous model

```bash
./bin/cmdr demo gate --baseline demo-baseline-001 --model gpt-4o-danger
echo $?
```

Expected:
- verdict `FAIL`
- risk escalation highlighted
- exit code `1` (CI block)

### 4) Show deployment gate allows safe model

```bash
./bin/cmdr demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet
echo $?
```

Expected:
- verdict `PASS`
- no risk escalation
- exit code `0`

## Talking Points (30s each)

1. Baseline behavior is read/write only.
2. Dangerous profile introduces destructive DB operations.
3. Gate output explains *why* it fails (tool/risk/response dimensions).
4. Exit code is CI-friendly and blocks bad rollout automatically.

## Suggested Submission Assets

Capture these artifacts for the submission:
1. Drift check output showing divergence.
2. Dangerous gate run showing `FAIL` and exit code `1`.
3. Safe gate run showing `PASS` and exit code `0`.
