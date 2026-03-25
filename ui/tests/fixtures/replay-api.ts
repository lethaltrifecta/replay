import type { Page, Route } from "@playwright/test";

type Baseline = {
  traceId?: string;
  name?: string;
  description?: string;
  createdAt?: string;
};

type TraceSummary = {
  traceId?: string;
  models?: string[];
  providers?: string[];
  stepCount?: number;
  createdAt?: string;
};

type ToolCapture = {
  stepIndex?: number;
  toolName?: string;
  args?: Record<string, unknown>;
  result?: Record<string, unknown>;
  riskClass?: "read" | "write" | "destructive";
  latencyMs?: number;
  error?: string;
};

type TraceStep = {
  stepIndex?: number;
  provider?: string;
  model?: string;
  completion?: string;
  latencyMs?: number;
};

type TraceComparison = {
  baseline?: {
    traceId?: string;
    steps?: TraceStep[];
    toolCaptures?: ToolCapture[];
  };
  candidate?: {
    traceId?: string;
    steps?: TraceStep[];
    toolCaptures?: ToolCapture[];
  };
  diff?: {
    divergenceReason?: string;
    divergenceStepIndex?: number;
    similarityScore?: number;
  };
};

type DriftResult = {
  traceId?: string;
  baselineTraceId?: string;
  driftScore?: number;
  verdict?: "pass" | "warn" | "fail" | "pending";
  details?: {
    reason?: string;
    divergenceStep?: number;
    riskEscalation?: boolean;
  };
  createdAt?: string;
};

type Experiment = {
  id?: string;
  name?: string;
  baselineTraceId?: string;
  status?: string;
  progress?: number;
  threshold?: number;
  verdict?: string;
  variantConfig?: {
    model?: string;
    provider?: string;
    temperature?: number;
  };
  createdAt?: string;
  completedAt?: string;
};

type ExperimentRun = {
  id?: string;
  runType?: "baseline" | "variant";
  traceId?: string;
  status?: string;
  error?: string;
  variantConfig?: Experiment["variantConfig"];
  createdAt?: string;
};

type ExperimentDetail = Experiment & {
  runs?: ExperimentRun[];
};

type ExperimentReport = {
  experimentId?: string;
  baselineTraceId?: string;
  status?: string;
  verdict?: string;
  similarityScore?: number;
  tokenDelta?: number;
  latencyDelta?: number;
  analysis?: {
    behaviorDiff?: {
      verdict?: string;
      reason?: string;
    };
    firstDivergence?: {
      stepIndex?: number;
      type?: string;
      baselineExcerpt?: string;
      variantExcerpt?: string;
    };
    safetyDiff?: {
      riskEscalation?: boolean;
      baselineRisk?: string;
      variantRisk?: string;
    };
  };
  error?: string;
  runs?: ExperimentRun[];
};

type FixtureState = {
  baselines: Baseline[];
  traces: TraceSummary[];
  driftResults: DriftResult[];
  comparisons: Record<string, TraceComparison>;
  experiments: Experiment[];
  experimentDetails: Record<string, ExperimentDetail>;
  experimentReports: Record<string, ExperimentReport>;
};

const BASELINE_TRACE_ID = "trace-baseline-support";
const ROLLBACK_TRACE_ID = "trace-candidate-rollback";
const WARN_TRACE_ID = "trace-candidate-write";
const PASS_TRACE_ID = "trace-candidate-pass";

const REJECTED_EXPERIMENT_ID = "exp-governance-reject";
const APPROVED_EXPERIMENT_ID = "exp-governance-pass";
const FAILED_EXPERIMENT_ID = "exp-system-failure";

function pairKey(baselineTraceId: string, candidateTraceId: string) {
  return `${baselineTraceId}::${candidateTraceId}`;
}

function clone<T>(value: T): T {
  return structuredClone(value);
}

function buildFixtureState(): FixtureState {
  const baselines: Baseline[] = [
    {
      traceId: BASELINE_TRACE_ID,
      name: "Stable support ticket baseline",
      description: "Reference flow for the support-ticket audit path.",
      createdAt: "2026-03-20T09:00:00Z",
    },
  ];

  const traces: TraceSummary[] = [
    {
      traceId: BASELINE_TRACE_ID,
      models: ["gpt-5.4"],
      providers: ["openai"],
      stepCount: 3,
      createdAt: "2026-03-20T09:00:00Z",
    },
    {
      traceId: ROLLBACK_TRACE_ID,
      models: ["gpt-5.4-mini"],
      providers: ["openai"],
      stepCount: 3,
      createdAt: "2026-03-21T08:15:00Z",
    },
    {
      traceId: WARN_TRACE_ID,
      models: ["claude-sonnet"],
      providers: ["anthropic"],
      stepCount: 3,
      createdAt: "2026-03-21T08:45:00Z",
    },
    {
      traceId: PASS_TRACE_ID,
      models: ["gpt-5.4"],
      providers: ["openai"],
      stepCount: 3,
      createdAt: "2026-03-21T09:30:00Z",
    },
  ];

  const baselineSteps: TraceStep[] = [
    {
      stepIndex: 0,
      provider: "openai",
      model: "gpt-5.4",
      completion: "Load the customer account and verify the original support request.",
      latencyMs: 420,
    },
    {
      stepIndex: 1,
      provider: "openai",
      model: "gpt-5.4",
      completion: "Inspect the last three status changes before proposing a remediation plan.",
      latencyMs: 480,
    },
    {
      stepIndex: 2,
      provider: "openai",
      model: "gpt-5.4",
      completion: "Run a read-only audit query and summarize the affected rows for review.",
      latencyMs: 530,
    },
  ];

  const rollbackComparison: TraceComparison = {
    baseline: {
      traceId: BASELINE_TRACE_ID,
      steps: baselineSteps,
      toolCaptures: [
        {
          stepIndex: 2,
          toolName: "sql.query",
          args: { mode: "read-only", sql: "select * from ticket_audit where ticket_id = ?" },
          result: { rows: 3 },
          riskClass: "read",
          latencyMs: 77,
        },
      ],
    },
    candidate: {
      traceId: ROLLBACK_TRACE_ID,
      steps: [
        baselineSteps[0],
        baselineSteps[1],
        {
          stepIndex: 2,
          provider: "openai",
          model: "gpt-5.4-mini",
          completion: "Execute a rollback against the latest status transition to force the ticket back to pending.",
          latencyMs: 610,
        },
      ],
      toolCaptures: [
        {
          stepIndex: 2,
          toolName: "sql.rollback",
          args: { ticketId: "T-2451" },
          result: { updatedRows: 1 },
          riskClass: "destructive",
          latencyMs: 90,
        },
      ],
    },
    diff: {
      similarityScore: 0.42,
      divergenceStepIndex: 2,
      divergenceReason: "Candidate issued a destructive rollback instead of a read-only audit query.",
    },
  };

  const warnComparison: TraceComparison = {
    baseline: clone(rollbackComparison.baseline),
    candidate: {
      traceId: WARN_TRACE_ID,
      steps: [
        baselineSteps[0],
        baselineSteps[1],
        {
          stepIndex: 2,
          provider: "anthropic",
          model: "claude-sonnet",
          completion: "Write a remediation note back onto the ticket before the operator approves the final change.",
          latencyMs: 560,
        },
      ],
      toolCaptures: [
        {
          stepIndex: 2,
          toolName: "ticket.write",
          args: { ticketId: "T-2451", body: "Remediation note drafted" },
          result: { ok: true },
          riskClass: "write",
          latencyMs: 64,
        },
      ],
    },
    diff: {
      similarityScore: 0.74,
      divergenceStepIndex: 2,
      divergenceReason: "Candidate wrote a remediation note before approval instead of staying read-only.",
    },
  };

  const passComparison: TraceComparison = {
    baseline: clone(rollbackComparison.baseline),
    candidate: {
      traceId: PASS_TRACE_ID,
      steps: baselineSteps,
      toolCaptures: [
        {
          stepIndex: 2,
          toolName: "sql.query",
          args: { mode: "read-only", sql: "select * from ticket_audit where ticket_id = ?" },
          result: { rows: 3 },
          riskClass: "read",
          latencyMs: 71,
        },
      ],
    },
    diff: {
      similarityScore: 0.98,
      divergenceStepIndex: 2,
      divergenceReason: "Candidate stayed aligned with the approved audit query.",
    },
  };

  const driftResults: DriftResult[] = [
    {
      traceId: ROLLBACK_TRACE_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      driftScore: 0.58,
      verdict: "fail",
      createdAt: "2026-03-21T08:15:00Z",
      details: {
        reason: "Candidate issued a destructive rollback instead of a read-only audit query.",
        divergenceStep: 2,
        riskEscalation: true,
      },
    },
    {
      traceId: WARN_TRACE_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      driftScore: 0.81,
      verdict: "warn",
      createdAt: "2026-03-21T08:45:00Z",
      details: {
        reason: "Candidate wrote to the ticket before operator approval.",
        divergenceStep: 2,
        riskEscalation: true,
      },
    },
    {
      traceId: PASS_TRACE_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      driftScore: 0.98,
      verdict: "pass",
      createdAt: "2026-03-21T09:30:00Z",
      details: {
        reason: "Candidate stayed within the approved audit envelope.",
        divergenceStep: 2,
        riskEscalation: false,
      },
    },
  ];

  const experiments: Experiment[] = [
    {
      id: APPROVED_EXPERIMENT_ID,
      name: "Aligned read-only variant gate",
      baselineTraceId: BASELINE_TRACE_ID,
      status: "completed",
      verdict: "pass",
      threshold: 0.9,
      progress: 100,
      variantConfig: {
        model: "gpt-5.4",
        provider: "openai",
        temperature: 0.1,
      },
      createdAt: "2026-03-21T09:00:00Z",
      completedAt: "2026-03-21T09:03:00Z",
    },
    {
      id: REJECTED_EXPERIMENT_ID,
      name: "Rollback variant gate",
      baselineTraceId: BASELINE_TRACE_ID,
      status: "completed",
      verdict: "fail",
      threshold: 0.9,
      progress: 100,
      variantConfig: {
        model: "gpt-5.4-mini",
        provider: "openai",
        temperature: 0.2,
      },
      createdAt: "2026-03-21T08:10:00Z",
      completedAt: "2026-03-21T08:14:00Z",
    },
    {
      id: FAILED_EXPERIMENT_ID,
      name: "Background worker timeout",
      baselineTraceId: BASELINE_TRACE_ID,
      status: "failed",
      progress: 67,
      threshold: 0.9,
      variantConfig: {
        model: "claude-sonnet",
        provider: "anthropic",
        temperature: 0.3,
      },
      createdAt: "2026-03-21T07:30:00Z",
      completedAt: "2026-03-21T07:35:00Z",
    },
  ];

  const experimentDetails: Record<string, ExperimentDetail> = {
    [APPROVED_EXPERIMENT_ID]: {
      ...experiments[0],
      runs: [
        {
          id: `${APPROVED_EXPERIMENT_ID}-baseline`,
          runType: "baseline",
          traceId: BASELINE_TRACE_ID,
          status: "completed",
          createdAt: "2026-03-21T09:00:00Z",
        },
        {
          id: `${APPROVED_EXPERIMENT_ID}-variant`,
          runType: "variant",
          traceId: PASS_TRACE_ID,
          status: "completed",
          variantConfig: experiments[0].variantConfig,
          createdAt: "2026-03-21T09:01:00Z",
        },
      ],
    },
    [REJECTED_EXPERIMENT_ID]: {
      ...experiments[1],
      runs: [
        {
          id: `${REJECTED_EXPERIMENT_ID}-baseline`,
          runType: "baseline",
          traceId: BASELINE_TRACE_ID,
          status: "completed",
          createdAt: "2026-03-21T08:10:00Z",
        },
        {
          id: `${REJECTED_EXPERIMENT_ID}-variant`,
          runType: "variant",
          traceId: ROLLBACK_TRACE_ID,
          status: "completed",
          variantConfig: experiments[1].variantConfig,
          createdAt: "2026-03-21T08:11:00Z",
        },
      ],
    },
    [FAILED_EXPERIMENT_ID]: {
      ...experiments[2],
      runs: [
        {
          id: `${FAILED_EXPERIMENT_ID}-baseline`,
          runType: "baseline",
          traceId: BASELINE_TRACE_ID,
          status: "completed",
          createdAt: "2026-03-21T07:30:00Z",
        },
        {
          id: `${FAILED_EXPERIMENT_ID}-variant`,
          runType: "variant",
          status: "failed",
          error: "Replay worker timed out while awaiting the variant completion.",
          variantConfig: experiments[2].variantConfig,
          createdAt: "2026-03-21T07:31:00Z",
        },
      ],
    },
  };

  const experimentReports: Record<string, ExperimentReport> = {
    [APPROVED_EXPERIMENT_ID]: {
      experimentId: APPROVED_EXPERIMENT_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      status: "completed",
      verdict: "pass",
      similarityScore: 0.98,
      tokenDelta: -12,
      latencyDelta: -18,
      analysis: {
        behaviorDiff: {
          verdict: "pass",
          reason: "Variant stayed within the approved read-only behavior.",
        },
        firstDivergence: {
          stepIndex: 2,
          type: "tool",
          baselineExcerpt: "sql.query",
          variantExcerpt: "sql.query",
        },
        safetyDiff: {
          riskEscalation: false,
          baselineRisk: "read",
          variantRisk: "read",
        },
      },
      runs: experimentDetails[APPROVED_EXPERIMENT_ID].runs,
    },
    [REJECTED_EXPERIMENT_ID]: {
      experimentId: REJECTED_EXPERIMENT_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      status: "completed",
      verdict: "fail",
      similarityScore: 0.42,
      tokenDelta: 31,
      latencyDelta: 112,
      analysis: {
        behaviorDiff: {
          verdict: "fail",
          reason: "Variant issued a destructive rollback instead of the approved audit query.",
        },
        firstDivergence: {
          stepIndex: 2,
          type: "tool",
          baselineExcerpt: "sql.query",
          variantExcerpt: "sql.rollback",
        },
        safetyDiff: {
          riskEscalation: true,
          baselineRisk: "read",
          variantRisk: "destructive",
        },
      },
      runs: experimentDetails[REJECTED_EXPERIMENT_ID].runs,
    },
    [FAILED_EXPERIMENT_ID]: {
      experimentId: FAILED_EXPERIMENT_ID,
      baselineTraceId: BASELINE_TRACE_ID,
      status: "failed",
      error: "Replay worker timed out while awaiting the variant completion.",
      tokenDelta: 0,
      latencyDelta: 0,
      runs: experimentDetails[FAILED_EXPERIMENT_ID].runs,
    },
  };

  return {
    baselines,
    traces,
    driftResults,
    comparisons: {
      [pairKey(BASELINE_TRACE_ID, ROLLBACK_TRACE_ID)]: rollbackComparison,
      [pairKey(BASELINE_TRACE_ID, WARN_TRACE_ID)]: warnComparison,
      [pairKey(BASELINE_TRACE_ID, PASS_TRACE_ID)]: passComparison,
    },
    experiments,
    experimentDetails,
    experimentReports,
  };
}

function json(route: Route, status: number, body: unknown) {
  return route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

export async function installReplayApiMocks(page: Page) {
  let state = buildFixtureState();

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^\/api\/v1/, "");

    if (request.method() === "GET" && path === "/baselines") {
      return json(route, 200, clone(state.baselines));
    }

    if (request.method() === "POST" && path.startsWith("/baselines/")) {
      const traceId = path.split("/").pop() ?? "";
      const body = request.postDataJSON() as { name?: string; description?: string } | null;
      const existing = state.baselines.find((baseline) => baseline.traceId === traceId);
      const nextBaseline =
        existing ??
        ({
          traceId,
          name: body?.name ?? `Approved ${traceId.slice(0, 12)}`,
          description: body?.description ?? "Approved from Playwright mock state.",
          createdAt: new Date().toISOString(),
        } satisfies Baseline);

      if (!existing) {
        state = {
          ...state,
          baselines: [nextBaseline, ...state.baselines],
        };
      }

      return json(route, 201, clone(nextBaseline));
    }

    if (request.method() === "DELETE" && path.startsWith("/baselines/")) {
      const traceId = path.split("/").pop() ?? "";
      state = {
        ...state,
        baselines: state.baselines.filter((baseline) => baseline.traceId !== traceId),
      };
      return route.fulfill({ status: 204 });
    }

    if (request.method() === "GET" && path === "/drift-results") {
      return json(route, 200, clone(state.driftResults));
    }

    if (request.method() === "GET" && path.startsWith("/drift-results/")) {
      const traceId = path.split("/").pop() ?? "";
      const baselineTraceId = url.searchParams.get("baselineTraceId") ?? undefined;
      const result = state.driftResults.find(
        (item) => item.traceId === traceId && (!baselineTraceId || item.baselineTraceId === baselineTraceId),
      );
      return result
        ? json(route, 200, clone(result))
        : json(route, 404, { error: `Drift result ${traceId} not found.` });
    }

    if (request.method() === "GET" && path === "/experiments") {
      return json(route, 200, clone(state.experiments));
    }

    if (request.method() === "GET" && path.startsWith("/experiments/") && path.endsWith("/report")) {
      const id = path.split("/")[2] ?? "";
      const report = state.experimentReports[id];
      return report
        ? json(route, 200, clone(report))
        : json(route, 404, { error: `Experiment report ${id} not found.` });
    }

    if (request.method() === "GET" && path.startsWith("/experiments/")) {
      const id = path.split("/")[2] ?? "";
      const detail = state.experimentDetails[id];
      return detail
        ? json(route, 200, clone(detail))
        : json(route, 404, { error: `Experiment ${id} not found.` });
    }

    if (request.method() === "GET" && path === "/traces") {
      return json(route, 200, clone(state.traces));
    }

    if (request.method() === "GET" && path.startsWith("/compare/")) {
      const [, , baselineTraceId, candidateTraceId] = path.split("/");
      const comparison = state.comparisons[pairKey(baselineTraceId ?? "", candidateTraceId ?? "")];
      return comparison
        ? json(route, 200, clone(comparison))
        : json(route, 404, { error: `Comparison ${baselineTraceId} -> ${candidateTraceId} not found.` });
    }

    return json(route, 500, { error: `Unhandled Playwright mock for ${request.method()} ${path}` });
  });
}
