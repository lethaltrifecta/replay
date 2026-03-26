"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";

import { approveTraceAsBaseline, formatApiError, getDriftReview } from "@/lib/governance";

import {
  Badge,
  Button,
  ChangeContextBanner,
  DefinitionList,
  EmptyState,
  ErrorState,
  formatScore,
  LoadingState,
  PageHeader,
  Panel,
  shortId,
  verdictTone,
} from "./view-kit";
import { ConfirmAction } from "@/components/confirm-action";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function DriftDetailScreen({
  traceId,
  baselineTraceId,
}: {
  traceId: string;
  baselineTraceId?: string;
}) {
  const router = useRouter();
  const [review, setReview] = useState<Awaited<ReturnType<typeof getDriftReview>>>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [pendingApproval, setPendingApproval] = useState(false);

  const loadReview = useCallback(async () => {
    if (!baselineTraceId) {
      return;
    }
    try {
      setLoading(true);
      setError(undefined);
      setReview(await getDriftReview(traceId, baselineTraceId));
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  }, [baselineTraceId, traceId]);

  useEffect(() => {
    if (!baselineTraceId) {
      setLoading(false);
      setError("Drift detail requires a baseline trace ID because this endpoint is pair-sensitive.");
      return;
    }
    void loadReview();
  }, [traceId, baselineTraceId, loadReview]);

  async function handleApprove() {
    if (!review?.approval.canApprove || !review.approval.candidateTraceId) {
      return;
    }
    try {
      setPendingApproval(true);
      await approveTraceAsBaseline(review.approval.candidateTraceId);
      toast.success(`Candidate ${shortId(review.approval.candidateTraceId)} is now an approved baseline.`);
      router.push("/launchpad");
      router.refresh();
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setPendingApproval(false);
    }
  }

  if (loading && !review) {
    return <LoadingState label="Loading drift detail..." />;
  }

  if (error && !review) {
    return (
      <ErrorState
        message={error}
        action={
          <Button href="/divergence" tone="secondary">
            Back to inbox
          </Button>
        }
      />
    );
  }

  if (!review) {
    return <EmptyState title="Drift detail unavailable" description="The selected pair could not be loaded." />;
  }

  return (
    <>
      <PageHeader
        eyebrow="Divergence Engine"
        title={`${review.item.verdict.toUpperCase()} for ${shortId(review.item.traceId, 10)}`}
        description="Divergence detail stays verdict-first but keeps the Shadow Replay summary close enough to answer what changed and whether the candidate is still approvable."
        theme="divergence"
        actions={
          <>
            <Button href="/divergence" tone="secondary">
              Back to queue
            </Button>
            <Button
              href={`/shadow-replay?baseline=${encodeURIComponent(review.item.baselineTraceId)}&candidate=${encodeURIComponent(review.item.traceId)}`}
              tone="secondary"
            >
              Open Shadow Replay
            </Button>
            <ConfirmAction
              title="Approve candidate as a new baseline?"
              description={`Trace ${shortId(review.approval.candidateTraceId, 10)} will become an approved baseline and future drift reviews can compare against it.`}
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
        <ChangeContextBanner ctx={review.compare?.changeContext} />

        <Panel
          title="Divergence review"
          description="Tabbed review surface using the same primitives as the sibling apps."
          accent={review.item.verdict === "fail" ? "danger" : review.item.verdict === "warn" ? "warning" : "success"}
        >
          <Tabs defaultValue="decision">
            <TabsList>
              <TabsTrigger value="decision">Decision</TabsTrigger>
              <TabsTrigger value="context">Context</TabsTrigger>
              <TabsTrigger value="evidence">Evidence</TabsTrigger>
            </TabsList>

            <TabsContent value="decision">
              <DefinitionList
                items={[
                  { label: "Verdict", value: <Badge tone={verdictTone(review.item.verdict)}>{review.item.verdict}</Badge> },
                  { label: "Similarity", value: formatScore(review.item.driftScore) },
                  { label: "Why it moved", value: review.item.reason ?? "No compact reason captured." },
                  { label: "Can I approve this?", value: review.approval.reason },
                ]}
              />
            </TabsContent>

            <TabsContent value="context">
              <div className="grid gap-4 xl:grid-cols-[0.85fr_1.15fr]">
                <div className="grid gap-4 sm:grid-cols-2">
                  <TraceMiniCard label="Baseline" traceId={review.item.baselineTraceId} model={review.item.baseline?.models.join(", ")} />
                  <TraceMiniCard label="Candidate" traceId={review.item.traceId} model={review.item.candidate?.models.join(", ")} />
                </div>

                <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
                  <MetricCard label="Divergence" value={review.compare.summary.divergenceReason ?? "Not captured"} />
                  <MetricCard label="Step" value={review.compare.summary.divergenceStepIndex ?? "n/a"} />
                  <MetricCard label="Blast Radius" value={review.compare.summary.riskEscalated ? "Escalated" : "Stable"} />
                  <MetricCard
                    label="DeepFreeze delta"
                    value={review.compare.summary.changedTools.length ? review.compare.summary.changedTools.join(", ") : "None"}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent value="evidence">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Step</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Baseline</TableHead>
                    <TableHead>Candidate</TableHead>
                    <TableHead>Tool change</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {review.compare.steps.slice(0, 5).map((step) => (
                    <TableRow key={step.stepIndex}>
                      <TableCell className="font-medium">#{step.stepIndex}</TableCell>
                      <TableCell>
                        <Badge tone={step.isDivergence ? "danger" : "info"}>
                          {step.isDivergence ? "First divergence" : "Changed context"}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[260px]">
                        <CompactExcerpt text={step.baselineStep?.completion} />
                      </TableCell>
                      <TableCell className="max-w-[260px]">
                        <CompactExcerpt text={step.candidateStep?.completion} />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          {step.candidateTools.length > 0 ? (
                            step.candidateTools.map((tool) => (
                              <Badge key={`${step.stepIndex}-${tool.name}`} tone={verdictTone(tool.risk === "destructive" ? "fail" : tool.risk === "write" ? "warn" : "pass")}>
                                {tool.name}
                              </Badge>
                            ))
                          ) : (
                            <span className="text-sm text-muted-foreground">No tool change</span>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TabsContent>
          </Tabs>
        </Panel>
      </div>
    </>
  );
}

function TraceMiniCard({ label, traceId, model }: { label: string; traceId: string; model?: string }) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-3 text-lg font-semibold tracking-[-0.04em]">{shortId(traceId, 10)}</p>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">{model || "unknown model"}</p>
    </div>
  );
}

function MetricCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-3 text-sm leading-6 text-foreground/85">{value}</p>
    </div>
  );
}

function CompactExcerpt({ text }: { text?: string }) {
  return <p className="line-clamp-3 text-sm leading-6 text-foreground/85">{text || "No completion captured."}</p>;
}
