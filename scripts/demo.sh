#!/bin/bash
set -euo pipefail

# Source .env so CMDR_POSTGRES_URL etc. are available
if [ -f .env ]; then
  set -a; source .env; set +a
else
  echo "Error: .env file not found. Run: cp .env.example .env" >&2
  exit 1
fi

CMDR="${CMDR:-./bin/cmdr}"

echo ""
echo "========================================="
echo "  CMDR: Agent Governance for MCP"
echo "========================================="
echo ""
sleep 1

# Scene 1: Seed data
echo "--- Scene 1: Capture Agent Behavior ---"
echo "Load deterministic baseline + drifted traces for a refactoring task."
echo "These seeded traces mirror what CMDR stores from captured telemetry."
echo ""
$CMDR demo seed
echo ""
sleep 2

# Scene 2: Drift detection
echo "--- Scene 2: Drift Detection ---"
echo "A new agent run comes in. Same task, but the agent goes rogue..."
echo "It calls delete_database instead of run_tests at step 3."
echo ""
$CMDR drift check demo-baseline-001 demo-drifted-002
echo ""
sleep 3

# Scene 3a: Gate FAIL
echo "--- Scene 3a: Deployment Gate (Dangerous Model) ---"
echo "Before deploying gpt-4o, replay the baseline scenario..."
echo ""
set +e
$CMDR demo gate --baseline demo-baseline-001 --model gpt-4o-danger
danger_exit=$?
set -e
echo "Danger profile gate exit code: $danger_exit (expected 1)"
if [ "$danger_exit" -ne 1 ]; then
  echo "Unexpected gate exit code for dangerous profile; expected 1 to prove CI block." >&2
  exit 1
fi
echo ""
sleep 3

# Scene 3b: Gate PASS
echo "--- Scene 3b: Deployment Gate (Safe Model) ---"
echo "Try claude-3-5-sonnet instead..."
echo ""
$CMDR demo gate --baseline demo-baseline-001 --model claude-3-5-sonnet
echo ""
sleep 2

# Summary
echo "--- Results ---"
echo ""
echo "Recent drift results:"
$CMDR drift list --limit 5
echo ""
echo "========================================="
echo "  CMDR: Govern agents before they govern you."
echo "========================================="
