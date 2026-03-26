import type {
  AnalysisResult,
  Baseline,
  DriftResult,
  Experiment,
  ExperimentDetail,
  ExperimentReport,
  ExperimentRun,
  TraceComparison,
  TraceStep,
} from "@/lib/api";

export type RiskClass = "read" | "write" | "destructive" | "unknown";

export type ChangeContext = {
  kind: string;
  target: string;
  baselineLabel?: string;
  candidateLabel?: string;
  summary?: string;
};
export type ExperimentOutcome = "approved" | "rejected" | "system_failure" | "running";

export type TraceRecord = {
  traceId: string;
  models: string[];
  providers: string[];
  stepCount: number;
  createdAt?: string;
  baseline?: Baseline;
  baselineLatestVerdict?: string;
  baselineCandidateCount?: number;
};

export type SelectionViewModel = {
  baselines: TraceRecord[];
  traces: TraceRecord[];
  driftInboxSize: number;
};

export type DriftInboxItem = {
  traceId: string;
  baselineTraceId: string;
  driftScore?: number;
  verdict: string;
  createdAt?: string;
  reason?: string;
  divergenceStep?: number;
  riskEscalation: boolean;
  candidate?: TraceRecord;
  baseline?: TraceRecord;
  raw: DriftResult;
};

export type ToolSummary = {
  name: string;
  count: number;
  risk: RiskClass;
  errors: number;
};

export type ComparisonSummary = {
  similarityScore?: number;
  divergenceReason?: string;
  divergenceStepIndex?: number;
  baselineStepCount: number;
  candidateStepCount: number;
  baselineRisk: RiskClass;
  candidateRisk: RiskClass;
  riskEscalated: boolean;
  changedTools: string[];
};

export type StepPair = {
  stepIndex: number;
  baselineStep?: TraceStep;
  candidateStep?: TraceStep;
  baselineTools: ToolSummary[];
  candidateTools: ToolSummary[];
  isDivergence: boolean;
};

export type CompareViewModel = {
  baselineTraceId: string;
  candidateTraceId: string;
  comparison: TraceComparison;
  summary: ComparisonSummary;
  steps: StepPair[];
  changeContext?: ChangeContext;
};

export type ApprovalState = {
  canApprove: boolean;
  reason: string;
  candidateTraceId?: string;
};

export type DriftReviewModel = {
  item: DriftInboxItem;
  compare: CompareViewModel;
  approval: ApprovalState;
};

export type ExperimentListItem = {
  id: string;
  name: string;
  baselineTraceId: string;
  status: string;
  verdict?: string;
  threshold?: number;
  progress?: number;
  model?: string;
  provider?: string;
  createdAt?: string;
  completedAt?: string;
  raw: Experiment;
};

export type ExperimentReportViewModel = {
  experimentId: string;
  name: string;
  baselineTraceId: string;
  baselineRun?: ExperimentRun;
  variantRun?: ExperimentRun;
  detail: ExperimentDetail;
  report: ExperimentReport;
  compare?: CompareViewModel;
  outcome: ExperimentOutcome;
  headline: string;
  explanation: string;
  approval: ApprovalState;
  analysis?: AnalysisResult;
};
