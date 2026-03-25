import {
  compareTraces as compareTracesRequest,
  createBaseline as createBaselineRequest,
  createGateCheck as createGateCheckRequest,
  deleteBaseline as deleteBaselineRequest,
  getDriftResult as getDriftResultRequest,
  getExperiment as getExperimentRequest,
  getExperimentReport as getExperimentReportRequest,
  getGateStatus as getGateStatusRequest,
  listBaselines as listBaselinesRequest,
  listDriftResults as listDriftResultsRequest,
  listExperiments as listExperimentsRequest,
  listTraces as listTracesRequest,
  type GateCheckRequest,
} from "./generated";
import { client } from "./generated/client.gen";

export type {
  AnalysisResult,
  Baseline,
  DriftDetails,
  DriftResult,
  Experiment,
  ExperimentDetail,
  ExperimentReport,
  ExperimentRun,
  GateCheckRequest,
  GateCheckResponse,
  ToolCapture,
  TraceComparison,
  TraceDetail,
  TraceStep,
  TraceSummary,
  VariantConfig,
} from "./generated";

let configured = false;
const REQUEST_OPTIONS = {
  responseStyle: "fields" as const,
  throwOnError: false as const,
};

export function configureApiClient() {
  if (configured) {
    return;
  }

  client.setConfig({
    baseUrl: process.env.NEXT_PUBLIC_REPLAY_API_BASE?.replace(/\/$/, "") || "/api/v1",
  });
  configured = true;
}

export class ApiError extends Error {
  body?: unknown;
  status?: number;

  constructor(message: string, options?: { body?: unknown; status?: number }) {
    super(message);
    this.name = "ApiError";
    this.body = options?.body;
    this.status = options?.status;
  }
}

export const DriftVerdicts = {
  verdict: {
    PASS: "pass",
    WARN: "warn",
    FAIL: "fail",
    PENDING: "pending",
  },
} as const;

export const ExperimentRunKinds = {
  runType: {
    BASELINE: "baseline",
    VARIANT: "variant",
  },
} as const;

export const ToolRiskLevels = {
  riskClass: {
    READ: "read",
    WRITE: "write",
    DESTRUCTIVE: "destructive",
  },
} as const;

function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) {
    return error;
  }
  if (typeof error === "string") {
    return new ApiError(error);
  }
  if (error && typeof error === "object") {
    const message =
      "error" in error && typeof error.error === "string"
        ? error.error
        : "message" in error && typeof error.message === "string"
          ? error.message
          : "Unexpected API failure.";
    const status = "status" in error && typeof error.status === "number" ? error.status : undefined;
    return new ApiError(message, { status, body: error });
  }
  if (error instanceof Error) {
    const maybeStatus = "status" in error && typeof error.status === "number" ? error.status : undefined;
    const maybeBody = "body" in error ? error.body : undefined;
    return new ApiError(error.message, { status: maybeStatus, body: maybeBody });
  }
  return new ApiError("Unexpected API failure.");
}

type ResponseEnvelope<T> = {
  data: T | undefined;
  error?: unknown;
  request: Request;
  response: Response;
};

async function wrapRequest<T>(request: Promise<ResponseEnvelope<T>>) {
  try {
    const { data, error, response } = await request;
    if (error !== undefined) {
      throw toApiError({
        body: error,
        error: typeof error === "object" && error && "error" in error ? error.error : undefined,
        status: response.status,
      });
    }
    if (data === undefined) {
      throw new ApiError("Unexpected empty API response.", { status: response.status });
    }
    return data;
  } catch (error) {
    throw toApiError(error);
  }
}

export class GovernanceService {
  static listBaselines() {
    return wrapRequest(listBaselinesRequest(REQUEST_OPTIONS));
  }

  static createBaseline(traceId: string, requestBody?: { name?: string; description?: string }) {
    return wrapRequest(createBaselineRequest({ ...REQUEST_OPTIONS, path: { traceId }, body: requestBody }));
  }

  static deleteBaseline(traceId: string) {
    return wrapRequest(deleteBaselineRequest({ ...REQUEST_OPTIONS, path: { traceId } }));
  }

  static listDriftResults(limit = 20, offset?: number) {
    return wrapRequest(listDriftResultsRequest({ ...REQUEST_OPTIONS, query: { limit, offset } }));
  }

  static getDriftResult(traceId: string, baselineTraceId?: string) {
    return wrapRequest(getDriftResultRequest({ ...REQUEST_OPTIONS, path: { traceId }, query: { baselineTraceId } }));
  }

  static createGateCheck(requestBody: GateCheckRequest) {
    return wrapRequest(createGateCheckRequest({ ...REQUEST_OPTIONS, body: requestBody }));
  }

  static getGateStatus(id: string) {
    return wrapRequest(getGateStatusRequest({ ...REQUEST_OPTIONS, path: { id } }));
  }
}

export class ExperimentsService {
  static listExperiments(
    status?: "pending" | "running" | "completed" | "failed" | "cancelled",
    limit = 20,
    offset?: number,
  ) {
    return wrapRequest(listExperimentsRequest({ ...REQUEST_OPTIONS, query: { status, limit, offset } }));
  }

  static getExperiment(id: string) {
    return wrapRequest(getExperimentRequest({ ...REQUEST_OPTIONS, path: { id } }));
  }

  static getExperimentReport(id: string) {
    return wrapRequest(getExperimentReportRequest({ ...REQUEST_OPTIONS, path: { id } }));
  }
}

export class TracesService {
  static listTraces(model?: string, provider?: string, limit = 20, offset?: number) {
    return wrapRequest(listTracesRequest({ ...REQUEST_OPTIONS, query: { model, provider, limit, offset } }));
  }

  static compareTraces(baselineTraceId: string, candidateTraceId: string) {
    return wrapRequest(compareTracesRequest({ ...REQUEST_OPTIONS, path: { baselineTraceId, candidateTraceId } }));
  }
}
