"use client";

import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";

import { formatApiError, getCompareView } from "@/lib/governance";

import {
  Badge,
  Button,
  ChangeContextBanner,
  EmptyState,
  ErrorState,
  FieldLabel,
  JsonBlock,
  LoadingState,
  PageHeader,
  Panel,
  riskTone,
  shortId,
} from "./view-kit";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

export function CompareScreen({
  baselineTraceId,
  candidateTraceId,
}: {
  baselineTraceId?: string;
  candidateTraceId?: string;
}) {
  const [compare, setCompare] = useState<Awaited<ReturnType<typeof getCompareView>>>();
  const [loading, setLoading] = useState(Boolean(baselineTraceId && candidateTraceId));
  const [error, setError] = useState<string>();

  const loadCompare = useCallback(async () => {
    if (!baselineTraceId || !candidateTraceId) {
      return;
    }
    try {
      setLoading(true);
      setError(undefined);
      setCompare(await getCompareView(baselineTraceId, candidateTraceId));
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  }, [baselineTraceId, candidateTraceId]);

  useEffect(() => {
    if (!baselineTraceId || !candidateTraceId) {
      setLoading(false);
      return;
    }
    void loadCompare();
  }, [baselineTraceId, candidateTraceId, loadCompare]);

  if (!baselineTraceId || !candidateTraceId) {
    return (
      <EmptyState
        title="No pair selected"
        description="Open Shadow Replay from Launchpad, Divergence, or Gauntlet so this page receives both baseline and candidate trace IDs."
        action={
          <Button href="/launchpad" tone="primary">
            Pick traces
          </Button>
        }
      />
    );
  }

  if (loading && !compare) {
    return <LoadingState label="Loading side-by-side comparison..." />;
  }

  if (error && !compare) {
    return (
      <ErrorState
        message={error}
        action={
          <Button onClick={() => void loadCompare()} tone="secondary">
            Retry
          </Button>
        }
      />
    );
  }

  if (!compare) {
    return <EmptyState title="Shadow Replay unavailable" description="The selected pair could not be loaded from the canonical Shadow Replay surface." />;
  }

  return (
    <>
      <PageHeader
        eyebrow="Shadow Replay"
        title={`${shortId(compare.baselineTraceId, 10)} → ${shortId(compare.candidateTraceId, 10)}`}
        description="Shadow Replay is the raw evidence lane. It answers what changed, but it deliberately avoids inventing a verdict. Use Divergence or the Gauntlet report when you need an approvability decision."
        theme="shadow"
        actions={
            <Button href="/launchpad" tone="secondary">
            Pick another pair
          </Button>
        }
      />

      {error ? <div className="mb-4"><ErrorState message={error} /></div> : null}

      <div className="section-grid">
        <ChangeContextBanner ctx={compare.changeContext} />

        <Panel title="Replay summary" description="Compact meaning from the canonical Shadow Replay contract." accent="info">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            <SummaryRailCard label="Similarity" value={compare.summary.similarityScore !== undefined ? `${Math.round(compare.summary.similarityScore * 100)}%` : "n/a"} />
            <SummaryRailCard label="Divergence step" value={compare.summary.divergenceStepIndex ?? "n/a"} />
            <SummaryRailCard label="Why it diverged" value={compare.summary.divergenceReason ?? "No compact reason"} />
            <SummaryRailCard label="Baseline Blast Radius" value={<Badge tone={riskTone(compare.summary.baselineRisk)}>{compare.summary.baselineRisk}</Badge>} />
            <SummaryRailCard label="Candidate Blast Radius" value={<Badge tone={riskTone(compare.summary.candidateRisk)}>{compare.summary.candidateRisk}</Badge>} />
          </div>
        </Panel>

        <Panel title="Approval context" description="This page is evidence only. Approval stays with the Divergence or Gauntlet review objects.">
          <p className="m-0 text-sm leading-6 text-foreground/85">
            Shadow Replay can show divergence, DeepFreeze deltas, and Blast Radius movement, but it intentionally does not decide whether the candidate is approvable. That keeps the review contract centered on Divergence results and Gauntlet reports.
          </p>
        </Panel>

        <Panel title="Shadow Replay evidence" description="Tabbed evidence view for the left-vs-right replay review.">
          <Tabs defaultValue="timeline">
            <TabsList>
              <TabsTrigger value="timeline">Timeline</TabsTrigger>
              <TabsTrigger value="tool-delta">DeepFreeze delta</TabsTrigger>
            </TabsList>

            <TabsContent value="timeline">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Step</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Baseline</TableHead>
                    <TableHead>Candidate</TableHead>
                    <TableHead>DeepFreeze</TableHead>
                    <TableHead className="w-[120px]">Evidence</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {compare.steps.map((step) => (
                    <TableRow key={step.stepIndex}>
                      <TableCell className="font-medium">#{step.stepIndex}</TableCell>
                      <TableCell>
                        <Badge tone={step.isDivergence ? "danger" : "info"}>
                          {step.isDivergence ? "Diverged" : "Aligned"}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[280px]">
                        <StepExcerpt text={step.baselineStep?.completion} />
                      </TableCell>
                      <TableCell className="max-w-[280px]">
                        <StepExcerpt text={step.candidateStep?.completion} />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap gap-2">
                          {step.candidateTools.length > 0 ? (
                            step.candidateTools.map((tool) => (
                              <Badge key={`${step.stepIndex}-${tool.name}`} tone={riskTone(tool.risk)}>
                                {tool.name} × {tool.count}
                              </Badge>
                            ))
                          ) : (
                            <span className="text-sm text-muted-foreground">No DeepFreeze delta</span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell>
                        <EvidenceDialog
                          step={step}
                          baselineRawTools={compare.comparison.baseline?.toolCaptures?.filter((tool) => tool.stepIndex === step.stepIndex)}
                          candidateRawTools={compare.comparison.candidate?.toolCaptures?.filter((tool) => tool.stepIndex === step.stepIndex)}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TabsContent>

            <TabsContent value="tool-delta">
              <div className="grid gap-4 md:grid-cols-2">
                <ToolPanel title="Baseline DeepFreeze" tools={flattenTools(compare.steps, "baselineTools")} />
                <ToolPanel title="Candidate DeepFreeze" tools={flattenTools(compare.steps, "candidateTools")} />
              </div>
            </TabsContent>
          </Tabs>
        </Panel>
      </div>
    </>
  );
}

function SummaryRailCard({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <FieldLabel>{label}</FieldLabel>
      <div className="mt-3 text-sm leading-6 text-foreground/85">{value}</div>
    </div>
  );
}

function StepExcerpt({ text }: { text?: string }) {
  return (
    <p className="line-clamp-3 text-sm leading-6 text-foreground/85">{text || "No completion captured."}</p>
  );
}

function EvidenceDialog({
  step,
  baselineRawTools,
  candidateRawTools,
}: {
  step: Awaited<ReturnType<typeof getCompareView>>["steps"][number];
  baselineRawTools: Array<Record<string, unknown>> | undefined;
  candidateRawTools: Array<Record<string, unknown>> | undefined;
}) {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button tone="secondary">Inspect</Button>
      </DialogTrigger>
      <DialogContent className="max-w-5xl">
        <DialogHeader>
          <DialogTitle>Step {step.stepIndex} shadow evidence</DialogTitle>
          <DialogDescription>
            {step.isDivergence ? "This is the first divergence step." : "This step remained aligned at the contract level."}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 lg:grid-cols-2">
          <div className="space-y-4 rounded-lg border bg-muted/70 p-4">
            <div className="flex flex-wrap items-center gap-2">
              <h4 className="text-sm font-semibold">Baseline</h4>
              {step.baselineStep?.model ? <Badge tone="neutral">{step.baselineStep.model}</Badge> : null}
            </div>
            <FieldLabel>Completion</FieldLabel>
            <p className="whitespace-pre-wrap text-sm leading-6 text-foreground/85">{step.baselineStep?.completion || "No completion captured."}</p>
            <JsonBlock value={{ prompt: step.baselineStep?.prompt, latencyMs: step.baselineStep?.latencyMs, rawTools: baselineRawTools }} />
          </div>

          <div className="space-y-4 rounded-lg border bg-muted/70 p-4">
            <div className="flex flex-wrap items-center gap-2">
              <h4 className="text-sm font-semibold">Candidate</h4>
              {step.candidateStep?.model ? <Badge tone="neutral">{step.candidateStep.model}</Badge> : null}
            </div>
            <FieldLabel>Completion</FieldLabel>
            <p className="whitespace-pre-wrap text-sm leading-6 text-foreground/85">{step.candidateStep?.completion || "No completion captured."}</p>
            <JsonBlock value={{ prompt: step.candidateStep?.prompt, latencyMs: step.candidateStep?.latencyMs, rawTools: candidateRawTools }} />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ToolPanel({
  title,
  tools,
}: {
  title: string;
  tools: Array<{ name: string; count: number; risk: string }>;
}) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <FieldLabel>{title}</FieldLabel>
      <div className="mt-3 flex flex-wrap gap-2">
        {tools.length > 0 ? (
          tools.map((tool) => (
            <Badge key={`${title}-${tool.name}-${tool.count}`} tone={riskTone(tool.risk)}>
              {tool.name} × {tool.count}
            </Badge>
          ))
        ) : (
          <span className="text-sm text-muted-foreground">No tools captured.</span>
        )}
      </div>
    </div>
  );
}

function flattenTools(
  steps: Awaited<ReturnType<typeof getCompareView>>["steps"],
  key: "baselineTools" | "candidateTools",
) {
  return steps.flatMap((step) => step[key]);
}
