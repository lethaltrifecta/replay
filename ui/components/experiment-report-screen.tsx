"use client";

import { useRouter } from "next/navigation";
import { useEffect, useEffectEvent, useState } from "react";
import type { ReactNode } from "react";
import { toast } from "sonner";

import { approveTraceAsBaseline, formatApiError, getExperimentReview } from "@/lib/governance";

import {
  Badge,
  Button,
  DefinitionList,
  ErrorState,
  formatScore,
  LoadingState,
  PageHeader,
  Panel,
  shortId,
  verdictTone,
} from "./view-kit";
import { ConfirmAction } from "@/components/confirm-action";

export function ExperimentReportScreen({ experimentId }: { experimentId: string }) {
  const router = useRouter();
  const [review, setReview] = useState<Awaited<ReturnType<typeof getExperimentReview>>>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [pendingApproval, setPendingApproval] = useState(false);

  const loadReview = useEffectEvent(async () => {
    try {
      setLoading(true);
      setError(undefined);
      setReview(await getExperimentReview(experimentId));
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  });

  useEffect(() => {
    void loadReview();
  }, [experimentId]);

  useEffect(() => {
    if (!review || review.outcome !== "running") {
      return;
    }
    const handle = window.setInterval(() => {
      void loadReview();
    }, 4000);
    return () => window.clearInterval(handle);
  }, [review]);

  async function handleApprove() {
    if (!review?.approval.canApprove || !review.approval.candidateTraceId) {
      return;
    }
    try {
      setPendingApproval(true);
      await approveTraceAsBaseline(review.approval.candidateTraceId);
      toast.success(`Candidate ${shortId(review.approval.candidateTraceId)} is now approved as a baseline.`);
      router.push("/launchpad");
      router.refresh();
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setPendingApproval(false);
    }
  }

  if (loading && !review) {
    return <LoadingState label="Loading Gauntlet report..." />;
  }

  if (error && !review) {
    return (
      <ErrorState
        message={error}
        action={
          <Button href="/gauntlet" tone="secondary">
            Back to reports
          </Button>
        }
      />
    );
  }

  if (!review) {
    return <ErrorState message="The canonical report route returned no data." />;
  }

  return (
    <>
      <PageHeader
        eyebrow="The Gauntlet"
        title={review.headline}
        description="The trial report is where the contract must cleanly answer all four operator questions. If this page feels forced, the contract is wrong."
        theme="gauntlet"
        actions={
          <>
            <Button href="/gauntlet" tone="secondary">
              Back to Gauntlet
            </Button>
            {review.compare ? (
              <Button
                href={`/shadow-replay?baseline=${encodeURIComponent(review.compare.baselineTraceId)}&candidate=${encodeURIComponent(
                  review.compare.candidateTraceId,
                )}`}
                tone="secondary"
              >
                Open Shadow Replay
              </Button>
            ) : null}
            <ConfirmAction
              title="Approve candidate as a baseline?"
              description={`Trace ${shortId(review.approval.candidateTraceId, 10)} will become an approved governance reference.`}
              confirmLabel="Approve candidate"
              pendingLabel="Approving..."
              disabled={!review.approval.canApprove || pendingApproval}
              pending={pendingApproval}
              onConfirm={handleApprove}
            >
              <Button disabled={!review.approval.canApprove || pendingApproval} tone="primary">
                Approve candidate
              </Button>
            </ConfirmAction>
          </>
        }
      />

      {error ? <div className="mb-4"><ErrorState message={error} /></div> : null}

      <div className="section-grid">
          <Panel
            title="Operator answers"
            description="This is the contract validator. The report should answer all four questions without making the reviewer dig through raw storage shapes."
          accent={review.outcome === "approved" ? "success" : review.outcome === "rejected" ? "warning" : review.outcome === "system_failure" ? "danger" : "info"}
        >
          <DefinitionList
            items={[
              {
                label: "What changed?",
                value:
                  review.analysis?.behaviorDiff?.reason ??
                  review.compare?.summary.divergenceReason ??
                  "No structured behavior explanation captured.",
              },
              {
                label: "Why did it fail?",
                value:
                  review.outcome === "approved"
                    ? "It did not fail. The model survived the Gauntlet."
                    : review.explanation,
              },
              {
                label: "Governance rejection or system failure?",
                value:
                  review.outcome === "rejected"
                    ? "Gauntlet rejection. Replay completed and verdict=fail."
                    : review.outcome === "system_failure"
                      ? "System failure. Replay status=failed and operational error is present."
                      : review.outcome === "approved"
                        ? "Neither. Replay completed and the candidate survived the Gauntlet."
                        : "No final classification yet. Replay is still running.",
              },
              {
                label: "Can I approve this?",
                value: review.approval.reason,
              },
            ]}
          />
        </Panel>

        <div className="grid gap-4 xl:grid-cols-[0.72fr_1.28fr]">
          <Panel title="Gauntlet summary" description={review.explanation}>
            <div className="grid gap-4 sm:grid-cols-2">
              <SummaryBox label="Status" value={<Badge tone={review.outcome === "system_failure" ? "danger" : "info"}>{review.report.status ?? "unknown"}</Badge>} />
              <SummaryBox label="Verdict" value={<Badge tone={verdictTone(review.report.verdict)}>{review.report.verdict ?? "pending"}</Badge>} />
              <SummaryBox label="Similarity" value={formatScore(review.report.similarityScore)} />
              <SummaryBox label="Threshold" value={review.detail.threshold !== undefined ? formatScore(review.detail.threshold) : "n/a"} />
              <SummaryBox label="Token delta" value={review.report.tokenDelta ?? "n/a"} />
              <SummaryBox label="Latency delta" value={review.report.latencyDelta ?? "n/a"} />
            </div>
          </Panel>

          <Panel title="Review context" description="Primary identifiers and replay configuration.">
            <DefinitionList
              items={[
                { label: "Experiment", value: shortId(review.experimentId, 10) },
                { label: "Baseline trace", value: shortId(review.baselineTraceId, 10) },
                { label: "Variant trace", value: review.variantRun?.traceId ? shortId(review.variantRun.traceId, 10) : "not captured" },
                { label: "Variant model", value: review.detail.variantConfig?.model ?? "unknown" },
                { label: "Provider", value: review.detail.variantConfig?.provider ?? "unknown" },
                { label: "Replay name", value: review.name },
              ]}
            />
          </Panel>
        </div>

        {review.compare ? (
          <Panel title="Shadow Replay snapshot" description="Direct evidence carried into the report context without duplicating the raw Shadow Replay page.">
            <div className="grid gap-4 lg:grid-cols-4">
              <SummaryBox label="Divergence step" value={review.compare.summary.divergenceStepIndex ?? "n/a"} />
              <SummaryBox label="Divergence reason" value={review.compare.summary.divergenceReason ?? "not captured"} />
              <SummaryBox label="DeepFreeze delta" value={review.compare.summary.changedTools.length ? review.compare.summary.changedTools.join(", ") : "none"} />
              <SummaryBox label="Blast Radius" value={review.compare.summary.riskEscalated ? "Escalated" : "Stable"} />
            </div>
          </Panel>
        ) : (
          <Panel title="Shadow Replay snapshot" description="No baseline/candidate pair was available for side-by-side evidence on this report.">
            <p className="m-0 text-sm leading-6 text-muted-foreground">
              The report still answers the classification, but the Shadow Replay surface cannot be linked until both run trace IDs exist.
            </p>
          </Panel>
        )}

        <Panel title="Run evidence" description="Supporting evidence. The report stays primary; runs are here only to explain pipeline state.">
          <div className="grid gap-3 lg:grid-cols-2">
            {review.detail.runs?.map((run) => (
              <div key={run.id ?? `${run.runType}:${run.traceId}`} className="rounded-lg border bg-muted/70 p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="m-0 text-lg font-semibold tracking-[-0.03em]">{run.runType ?? "run"}</p>
                  <Badge tone={run.status === "failed" ? "danger" : run.status === "completed" ? "success" : "info"}>{run.status ?? "unknown"}</Badge>
                </div>
                <p className="mt-3 text-sm leading-6 text-muted-foreground">
                  Trace {run.traceId ? shortId(run.traceId, 10) : "not captured"}
                </p>
                {run.error ? <p className="mt-2 text-sm leading-6 text-destructive">{run.error}</p> : null}
                {run.variantConfig?.model ? (
                  <p className="mt-2 text-sm leading-6 text-foreground/85">
                    {run.variantConfig.model} {run.variantConfig.provider ? `via ${run.variantConfig.provider}` : ""}
                  </p>
                ) : null}
              </div>
            ))}
          </div>
        </Panel>
      </div>
    </>
  );
}

function SummaryBox({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <div className="mt-3 text-sm leading-6 text-foreground/85">{value}</div>
    </div>
  );
}
