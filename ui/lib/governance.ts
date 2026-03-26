import {
  ApiError,
  ExperimentRunKinds,
  ExperimentsService,
  GovernanceService,
  ToolRiskLevels,
  TracesService,
  configureApiClient,
  type Baseline,
  type DriftResult,
  type Experiment,
  type ExperimentDetail,
  type ExperimentReport,
  type ExperimentRun as ExperimentRunRecord,
  type TraceComparison,
  type TraceDetail,
  type TraceSummary,
  type ToolCapture as ToolCaptureRecord,
} from "@/lib/api";

import type {
  ApprovalState,
  ChangeContext,
  CompareViewModel,
  ComparisonSummary,
  DriftInboxItem,
  DriftReviewModel,
  ExperimentListItem,
  ExperimentOutcome,
  ExperimentReportViewModel,
  RiskClass,
  SelectionViewModel,
  StepPair,
  ToolSummary,
  TraceRecord,
} from "./governance-types";

export type {
  ApprovalState,
  ChangeContext,
  CompareViewModel,
  ComparisonSummary,
  DriftInboxItem,
  DriftReviewModel,
  ExperimentListItem,
  ExperimentOutcome,
  ExperimentReportViewModel,
  RiskClass,
  SelectionViewModel,
  StepPair,
  ToolSummary,
  TraceRecord,
} from "./governance-types";

const DEFAULT_LIMIT = 100;

export async function getSelectionViewModel(): Promise<SelectionViewModel> {
  configureApiClient();
  const [baselines, traces, driftResults] = await Promise.all([
    GovernanceService.listBaselines(),
    TracesService.listTraces(undefined, undefined, DEFAULT_LIMIT),
    GovernanceService.listDriftResults(DEFAULT_LIMIT),
  ]);

  const traceMap = new Map(traces.map((trace) => [trace.traceId ?? "", trace]));
  const baselineVerdicts = summarizeBaselineVerdicts(driftResults);

  const baselineRecords = baselines
    .map((baseline) => hydrateTraceRecord(traceMap.get(baseline.traceId ?? ""), baseline, baselineVerdicts))
    .filter((record): record is TraceRecord => record !== undefined);

  const baselineMap = new Map(baselines.map((baseline) => [baseline.traceId ?? "", baseline]));
  const traceRecords = traces
    .map((trace) => hydrateTraceRecord(trace, baselineMap.get(trace.traceId ?? ""), baselineVerdicts))
    .filter((record): record is TraceRecord => record !== undefined);

  return {
    baselines: baselineRecords,
    traces: traceRecords,
    driftInboxSize: driftResults.length,
  };
}

export async function approveTraceAsBaseline(traceId: string, input?: { name?: string; description?: string }) {
  configureApiClient();
  return GovernanceService.createBaseline(traceId, input);
}

export async function removeBaseline(traceId: string) {
  configureApiClient();
  await GovernanceService.deleteBaseline(traceId);
}

export async function getDriftInbox(): Promise<DriftInboxItem[]> {
  configureApiClient();
  const [results, traces, baselines] = await Promise.all([
    GovernanceService.listDriftResults(DEFAULT_LIMIT),
    TracesService.listTraces(undefined, undefined, DEFAULT_LIMIT),
    GovernanceService.listBaselines(),
  ]);

  const traceMap = new Map(
    traces
      .map((trace) => normalizeTraceRecord(trace))
      .filter((trace): trace is TraceRecord => trace !== undefined)
      .map((trace) => [trace.traceId, trace]),
  );
  const baselineMap = new Map(
    baselines
      .map((baseline) => baseline.traceId)
      .filter((value): value is string => Boolean(value))
      .map((traceId) => [traceId, traceMap.get(traceId)]),
  );

  return results
    .map((result) => mapDriftInboxItem(result, traceMap, baselineMap))
    .sort(sortDriftInbox);
}

export async function getDriftReview(traceId: string, baselineTraceId: string): Promise<DriftReviewModel> {
  configureApiClient();
  const [result, comparison] = await Promise.all([
    GovernanceService.getDriftResult(traceId, baselineTraceId),
    TracesService.compareTraces(baselineTraceId, traceId),
  ]);

  const compare = buildCompareViewModel(comparison, baselineTraceId, traceId);
  return {
    item: mapDriftInboxItem(result),
    compare,
    approval: approvalFromDrift(result),
  };
}

export async function getCompareView(baselineTraceId: string, candidateTraceId: string): Promise<CompareViewModel> {
  configureApiClient();
  const comparison = await TracesService.compareTraces(baselineTraceId, candidateTraceId);
  return buildCompareViewModel(comparison, baselineTraceId, candidateTraceId);
}

export async function getExperimentInbox(): Promise<ExperimentListItem[]> {
  configureApiClient();
  const experiments = await ExperimentsService.listExperiments(undefined, DEFAULT_LIMIT);
  return experiments
    .map(mapExperimentListItem)
    .filter((item): item is ExperimentListItem => item !== undefined)
    .sort((left, right) => {
      const leftTime = new Date(left.createdAt ?? 0).getTime();
      const rightTime = new Date(right.createdAt ?? 0).getTime();
      return rightTime - leftTime;
    });
}

export async function getExperimentReview(id: string): Promise<ExperimentReportViewModel> {
  configureApiClient();
  const [detail, report] = await Promise.all([
    ExperimentsService.getExperiment(id),
    ExperimentsService.getExperimentReport(id),
  ]);

  const baselineRun = findRun(detail.runs, ExperimentRunKinds.runType.BASELINE);
  const variantRun = findRun(detail.runs, ExperimentRunKinds.runType.VARIANT);

  const compare =
    baselineRun?.traceId && variantRun?.traceId
      ? buildCompareViewModel(
          await TracesService.compareTraces(baselineRun.traceId, variantRun.traceId),
          baselineRun.traceId,
          variantRun.traceId,
        )
      : undefined;

  const outcome = deriveOutcome(report);
  const approval = approvalFromExperiment(report, variantRun?.traceId);

  return {
    experimentId: report.experimentId ?? id,
    name: detail.name ?? "Untitled gate run",
    baselineTraceId: report.baselineTraceId ?? detail.baselineTraceId ?? "",
    baselineRun,
    variantRun,
    detail,
    report,
    compare,
    outcome,
    headline: outcomeHeadline(outcome),
    explanation: outcomeExplanation(report, outcome),
    approval,
    analysis: report.analysis,
  };
}

export function formatApiError(error: unknown): string {
  if (error instanceof ApiError) {
    const payload = error.body;
    if (payload && typeof payload === "object" && "error" in payload && typeof payload.error === "string") {
      return payload.error;
    }
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "Unexpected UI failure.";
}

function summarizeBaselineVerdicts(results: DriftResult[]) {
  const summary = new Map<string, { verdict: string; count: number }>();
  for (const result of results) {
    const baselineTraceId = result.baselineTraceId;
    if (!baselineTraceId) {
      continue;
    }
    const current = summary.get(baselineTraceId);
    const nextVerdict = current ? higherVerdict(current.verdict, result.verdict ?? "pending") : result.verdict ?? "pending";
    summary.set(baselineTraceId, { verdict: nextVerdict, count: (current?.count ?? 0) + 1 });
  }
  return summary;
}

function hydrateTraceRecord(
  trace: TraceSummary | undefined,
  baseline: Baseline | undefined,
  baselineVerdicts: Map<string, { verdict: string; count: number }>,
): TraceRecord | undefined {
  const record = normalizeTraceRecord(trace, baseline);
  if (!record) {
    return undefined;
  }
  const verdict = baselineVerdicts.get(record.traceId);
  return {
    ...record,
    baselineLatestVerdict: verdict?.verdict,
    baselineCandidateCount: verdict?.count,
  };
}

function normalizeTraceRecord(trace?: TraceSummary, baseline?: Baseline): TraceRecord | undefined {
  if (!trace?.traceId && !baseline?.traceId) {
    return undefined;
  }
  return {
    traceId: trace?.traceId ?? baseline?.traceId ?? "",
    models: trace?.models ?? [],
    providers: trace?.providers ?? [],
    stepCount: trace?.stepCount ?? 0,
    createdAt: trace?.createdAt ?? baseline?.createdAt,
    baseline,
  };
}

function mapDriftInboxItem(
  result: DriftResult,
  traces?: Map<string, TraceRecord>,
  baselines?: Map<string, TraceRecord | undefined>,
): DriftInboxItem {
  return {
    traceId: result.traceId ?? "",
    baselineTraceId: result.baselineTraceId ?? "",
    driftScore: result.driftScore,
    verdict: result.verdict ?? "pending",
    createdAt: result.createdAt,
    reason: result.details?.reason,
    divergenceStep: result.details?.divergenceStep,
    riskEscalation: Boolean(result.details?.riskEscalation),
    candidate: traces?.get(result.traceId ?? ""),
    baseline: baselines?.get(result.baselineTraceId ?? "") ?? traces?.get(result.baselineTraceId ?? ""),
    raw: result,
  };
}

function sortDriftInbox(left: DriftInboxItem, right: DriftInboxItem) {
  const leftRank = verdictRank(left.verdict);
  const rightRank = verdictRank(right.verdict);
  if (leftRank !== rightRank) {
    return rightRank - leftRank;
  }
  const leftTime = new Date(left.createdAt ?? 0).getTime();
  const rightTime = new Date(right.createdAt ?? 0).getTime();
  return rightTime - leftTime;
}

function buildCompareViewModel(
  comparison: TraceComparison,
  baselineTraceId: string,
  candidateTraceId: string,
): CompareViewModel {
  return {
    baselineTraceId,
    candidateTraceId,
    comparison,
    summary: summarizeComparison(comparison),
    steps: pairSteps(comparison),
    changeContext: extractChangeContext(comparison),
  };
}

function extractChangeContext(comparison: TraceComparison): ChangeContext | undefined {
  const allSteps = [...(comparison.baseline?.steps ?? []), ...(comparison.candidate?.steps ?? [])];
  for (const step of allSteps) {
    const ctx = step.metadata?.["change_context"];
    if (ctx && typeof ctx === "object" && "kind" in ctx && "target" in ctx) {
      return ctx as ChangeContext;
    }
  }
  return undefined;
}

function summarizeComparison(comparison: TraceComparison): ComparisonSummary {
  const baseline = comparison.baseline;
  const candidate = comparison.candidate;
  const baselineTools = summarizeTools(baseline?.toolCaptures ?? []);
  const candidateTools = summarizeTools(candidate?.toolCaptures ?? []);

  const baselineRisk = highestRisk(baselineTools);
  const candidateRisk = highestRisk(candidateTools);

  const baselineNames = new Set(baselineTools.map((tool) => tool.name));
  const candidateNames = new Set(candidateTools.map((tool) => tool.name));
  const changedTools = Array.from(new Set([...baselineNames, ...candidateNames])).filter(
    (name) => (baselineNames.has(name) ? baselineTools.find((tool) => tool.name === name)?.count : 0) !==
      (candidateNames.has(name) ? candidateTools.find((tool) => tool.name === name)?.count : 0),
  );

  return {
    similarityScore: comparison.diff?.similarityScore,
    divergenceReason: comparison.diff?.divergenceReason,
    divergenceStepIndex: comparison.diff?.divergenceStepIndex,
    baselineStepCount: baseline?.steps?.length ?? 0,
    candidateStepCount: candidate?.steps?.length ?? 0,
    baselineRisk,
    candidateRisk,
    riskEscalated: riskRank(candidateRisk) > riskRank(baselineRisk),
    changedTools,
  };
}

function pairSteps(comparison: TraceComparison): StepPair[] {
  const baselineSteps = indexByStep(comparison.baseline?.steps ?? []);
  const candidateSteps = indexByStep(comparison.candidate?.steps ?? []);
  const baselineTools = groupToolsByStep(comparison.baseline?.toolCaptures ?? []);
  const candidateTools = groupToolsByStep(comparison.candidate?.toolCaptures ?? []);

  const indexes = new Set<number>([
    ...baselineSteps.keys(),
    ...candidateSteps.keys(),
    ...baselineTools.keys(),
    ...candidateTools.keys(),
  ]);

  return Array.from(indexes)
    .sort((left, right) => left - right)
    .map((stepIndex) => ({
      stepIndex,
      baselineStep: baselineSteps.get(stepIndex),
      candidateStep: candidateSteps.get(stepIndex),
      baselineTools: summarizeTools(baselineTools.get(stepIndex) ?? []),
      candidateTools: summarizeTools(candidateTools.get(stepIndex) ?? []),
      isDivergence: stepIndex === comparison.diff?.divergenceStepIndex,
    }));
}

function indexByStep(steps: TraceDetail["steps"]) {
  const map = new Map<number, NonNullable<TraceDetail["steps"]>[number]>();
  for (const step of steps ?? []) {
    if (step.stepIndex === undefined) {
      continue;
    }
    map.set(step.stepIndex, step);
  }
  return map;
}

function groupToolsByStep(captures: TraceDetail["toolCaptures"]) {
  const map = new Map<number, ToolCaptureRecord[]>();
  for (const capture of captures ?? []) {
    const stepIndex = capture.stepIndex ?? -1;
    const bucket = map.get(stepIndex) ?? [];
    bucket.push(capture);
    map.set(stepIndex, bucket);
  }
  return map;
}

function summarizeTools(captures: ToolCaptureRecord[]): ToolSummary[] {
  const grouped = new Map<string, ToolSummary>();
  for (const capture of captures) {
    const name = capture.toolName ?? "unknown";
    const existing = grouped.get(name);
    const risk = normalizeRisk(capture.riskClass);
    if (!existing) {
      grouped.set(name, {
        name,
        count: 1,
        risk,
        errors: capture.error ? 1 : 0,
      });
      continue;
    }
    existing.count += 1;
    existing.errors += capture.error ? 1 : 0;
    if (riskRank(risk) > riskRank(existing.risk)) {
      existing.risk = risk;
    }
  }
  return Array.from(grouped.values()).sort((left, right) => right.count - left.count || left.name.localeCompare(right.name));
}

function highestRisk(tools: ToolSummary[]): RiskClass {
  return tools.reduce<RiskClass>((current, tool) => (riskRank(tool.risk) > riskRank(current) ? tool.risk : current), "unknown");
}

function normalizeRisk(risk?: ToolCaptureRecord["riskClass"]): RiskClass {
  switch (risk) {
    case ToolRiskLevels.riskClass.READ:
      return "read";
    case ToolRiskLevels.riskClass.WRITE:
      return "write";
    case ToolRiskLevels.riskClass.DESTRUCTIVE:
      return "destructive";
    default:
      return "unknown";
  }
}

function riskRank(risk: RiskClass) {
  switch (risk) {
    case "read":
      return 1;
    case "write":
      return 2;
    case "destructive":
      return 3;
    default:
      return 0;
  }
}

function approvalFromDrift(result: DriftResult): ApprovalState {
  if (result.verdict === "pass") {
    return {
      canApprove: true,
      candidateTraceId: result.traceId,
      reason: "Candidate stayed within the approved governance envelope.",
    };
  }
  if (result.verdict === "pending") {
    return {
      canApprove: false,
      candidateTraceId: result.traceId,
      reason: "Drift review is still pending.",
    };
  }
  return {
    canApprove: false,
    candidateTraceId: result.traceId,
    reason: result.details?.reason ?? "CMDR found behavior drift that still needs review.",
  };
}

function mapExperimentListItem(experiment: Experiment): ExperimentListItem | undefined {
  if (!experiment.id || !experiment.baselineTraceId || !experiment.status) {
    return undefined;
  }
  return {
    id: experiment.id,
    name: experiment.name ?? "Untitled gate run",
    baselineTraceId: experiment.baselineTraceId,
    status: experiment.status,
    verdict: experiment.verdict,
    threshold: experiment.threshold,
    progress: experiment.progress,
    model: experiment.variantConfig?.model,
    provider: experiment.variantConfig?.provider,
    createdAt: experiment.createdAt,
    completedAt: experiment.completedAt,
    raw: experiment,
  };
}

function findRun(
  runs: ExperimentDetail["runs"],
  runType:
    | ExperimentRunRecord["runType"]
    | (typeof ExperimentRunKinds.runType)[keyof typeof ExperimentRunKinds.runType],
) {
  return runs?.find((run) => run.runType === runType);
}

function deriveOutcome(report: ExperimentReport): ExperimentOutcome {
  if (report.status === "completed" && report.verdict === "pass") {
    return "approved";
  }
  if (report.status === "completed" && report.verdict === "fail") {
    return "rejected";
  }
  if (report.status === "failed" && report.error) {
    return "system_failure";
  }
  return "running";
}

function approvalFromExperiment(report: ExperimentReport, candidateTraceId?: string): ApprovalState {
  const outcome = deriveOutcome(report);
  if (!candidateTraceId) {
    return {
      canApprove: false,
      reason: "No candidate trace is available to approve.",
    };
  }
  if (outcome === "approved") {
    return {
      canApprove: true,
      candidateTraceId,
      reason: "Replay completed and the governance verdict is pass.",
    };
  }
  if (outcome === "rejected") {
    return {
      canApprove: false,
      candidateTraceId,
      reason: report.analysis?.behaviorDiff?.reason ?? "Replay completed but CMDR rejected the candidate behavior.",
    };
  }
  if (outcome === "system_failure") {
    return {
      canApprove: false,
      candidateTraceId,
      reason: report.error ?? "Evaluation failed before CMDR could make a governance decision.",
    };
  }
  return {
    canApprove: false,
    candidateTraceId,
    reason: "Gate review is still running.",
  };
}

function outcomeHeadline(outcome: ExperimentOutcome) {
  switch (outcome) {
    case "approved":
      return "Governance approved";
    case "rejected":
      return "Governance rejected";
    case "system_failure":
      return "System failure";
    default:
      return "Gate still running";
  }
}

function outcomeExplanation(report: ExperimentReport, outcome: ExperimentOutcome) {
  switch (outcome) {
    case "approved":
      return report.analysis?.behaviorDiff?.reason ?? "Replay completed and the candidate stayed within the approved baseline.";
    case "rejected":
      return report.analysis?.behaviorDiff?.reason ?? "Replay completed, but the candidate behavior diverged from the approved baseline.";
    case "system_failure":
      return report.error ?? "The replay pipeline failed before a governance verdict was produced.";
    default:
      return "Waiting for the replay pipeline to finish.";
  }
}

function verdictRank(verdict: string) {
  switch (verdict) {
    case "fail":
      return 3;
    case "warn":
      return 2;
    case "pending":
      return 1;
    default:
      return 0;
  }
}

function higherVerdict(left: string, right: string) {
  return verdictRank(left) >= verdictRank(right) ? left : right;
}
