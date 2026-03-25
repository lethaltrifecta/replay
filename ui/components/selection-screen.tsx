"use client";

import { startTransition, useDeferredValue, useEffect, useState } from "react";
import { toast } from "sonner";

import {
  approveTraceAsBaseline,
  formatApiError,
  getSelectionViewModel,
  removeBaseline,
  type TraceRecord,
} from "@/lib/governance";

import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  FieldLabel,
  formatDate,
  Input,
  LoadingState,
  PageHeader,
  Panel,
  shortId,
  verdictTone,
} from "./view-kit";
import { ConfirmAction } from "@/components/confirm-action";

export function SelectionScreen() {
  const [surface, setSurface] = useState<Awaited<ReturnType<typeof getSelectionViewModel>>>();
  const [selectedBaselineId, setSelectedBaselineId] = useState<string>();
  const [selectedCandidateId, setSelectedCandidateId] = useState<string>();
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();
  const [pendingAction, setPendingAction] = useState<string>();
  const deferredSearch = useDeferredValue(search);

  useEffect(() => {
    void loadSurface();
  }, []);

  async function loadSurface() {
    try {
      setLoading(true);
      setError(undefined);
      const next = await getSelectionViewModel();
      setSurface(next);
      setSelectedBaselineId((current) => pickBaseline(current, next.baselines, next.traces));
      setSelectedCandidateId((current) => pickCandidate(current, next.traces, next.baselines));
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  }

  async function handleApprove(traceId: string) {
    try {
      setPendingAction(traceId);
      await approveTraceAsBaseline(traceId);
      toast.success(`Baseline approved for ${shortId(traceId)}.`);
      await loadSurface();
      startTransition(() => {
        setSelectedBaselineId(traceId);
      });
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setPendingAction(undefined);
    }
  }

  async function handleRemove(traceId: string) {
    try {
      setPendingAction(traceId);
      await removeBaseline(traceId);
      toast.success(`Baseline removed for ${shortId(traceId)}.`);
      await loadSurface();
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setPendingAction(undefined);
    }
  }

  const filteredTraces = surface?.traces.filter((trace) => {
    const needle = deferredSearch.trim().toLowerCase();
    if (!needle) {
      return true;
    }
    const haystack = [trace.traceId, ...trace.models, ...trace.providers, trace.baseline?.name ?? ""].join(" ").toLowerCase();
    return haystack.includes(needle);
  });

  const compareHref =
    selectedBaselineId && selectedCandidateId && selectedBaselineId !== selectedCandidateId
      ? `/shadow-replay?baseline=${encodeURIComponent(selectedBaselineId)}&candidate=${encodeURIComponent(selectedCandidateId)}`
      : undefined;
  const filteredCount = filteredTraces?.length ?? 0;

  return (
    <>
      <PageHeader
        eyebrow="Launchpad"
        title="Choose the review pair first"
        description="Launchpad is where the approval flow starts. Pick an approved baseline, pick a candidate trace, and move into Divergence or Shadow Replay without losing governance context."
        theme="selection"
        actions={
          <>
            <Button href="/divergence" tone="secondary">
              Open Divergence
            </Button>
            <Button href={compareHref} tone="primary" disabled={!compareHref}>
              Open Shadow Replay
            </Button>
          </>
        }
      />

      {error ? (
        <div className="mb-4">
          <ErrorState
            message={error}
            action={
              <Button onClick={() => void loadSurface()} tone="secondary">
                Retry
              </Button>
            }
          />
        </div>
      ) : null}

      {loading && !surface ? <LoadingState label="Loading baselines and trace inventory..." /> : null}

      {!loading && surface && surface.traces.length === 0 ? (
        <EmptyState
          title="No traces available yet"
          description="Run a capture or seed the demo data first. This screen depends on the canonical /traces and /baselines surfaces."
        />
      ) : null}

      {surface ? (
        <div className="section-grid">
          <div className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
            <Panel
              title="Approved baselines"
              description="These traces anchor drift review. The baseline cards also carry the latest drift pressure so you can see which approved references are attracting noisy candidates."
            >
              {surface.baselines.length === 0 ? (
                <EmptyState
                  title="No baselines approved yet"
                  description="Pick a candidate trace from the catalog below and approve it to establish the first governance reference point."
                />
              ) : (
                <div className="grid gap-3 lg:grid-cols-2">
                  {surface.baselines.map((baseline) => {
                    const isSelected = baseline.traceId === selectedBaselineId;
                    return (
                      <div
                        key={baseline.traceId}
                        className={`rounded-lg border p-4 text-left transition ${
                          isSelected
                            ? "border-[var(--border-strong)] bg-[var(--primary-soft)]"
                            : "border-[var(--border)] bg-[var(--bg-muted)] hover:bg-[var(--surface-hover)]"
                        }`}
                      >
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <p className="m-0 text-lg font-semibold tracking-[-0.03em]">
                              {baseline.baseline?.name || shortId(baseline.traceId)}
                            </p>
                            <p className="mt-2 text-sm text-[var(--text-muted)]">{shortId(baseline.traceId, 10)}</p>
                          </div>
                          <Badge tone={verdictTone(baseline.baselineLatestVerdict)}>
                            {baseline.baselineLatestVerdict ?? "stable"}
                          </Badge>
                        </div>

                        <div className="mt-4 grid gap-3 sm:grid-cols-2">
                          <div>
                            <FieldLabel>Models</FieldLabel>
                            <p className="mt-2 text-sm text-[var(--text-soft)]">{baseline.models.join(", ") || "unknown"}</p>
                          </div>
                          <div>
                            <FieldLabel>Candidate pressure</FieldLabel>
                            <p className="mt-2 text-sm text-[var(--text-soft)]">{baseline.baselineCandidateCount ?? 0} trace(s)</p>
                          </div>
                        </div>

                        <div className="mt-4 flex flex-wrap gap-2">
                          <Button
                            tone={isSelected ? "primary" : "secondary"}
                            onClick={() => startTransition(() => setSelectedBaselineId(baseline.traceId))}
                          >
                            {isSelected ? "Selected baseline" : "Select baseline"}
                          </Button>
                          <ConfirmAction
                            title="Remove approved baseline?"
                            description={`Trace ${shortId(baseline.traceId)} will no longer anchor governance review until it is approved again.`}
                            confirmLabel="Remove baseline"
                            pendingLabel="Removing..."
                            disabled={pendingAction === baseline.traceId}
                            pending={pendingAction === baseline.traceId}
                            onConfirm={() => handleRemove(baseline.traceId)}
                          >
                            <Button tone="danger" disabled={pendingAction === baseline.traceId}>
                              Remove baseline
                            </Button>
                          </ConfirmAction>
                            <Button
                            tone="secondary"
                            href={`/shadow-replay?baseline=${encodeURIComponent(baseline.traceId)}&candidate=${encodeURIComponent(
                              selectedCandidateId && selectedCandidateId !== baseline.traceId
                                ? selectedCandidateId
                                : pickCandidate(undefined, surface.traces, [baseline]) ?? baseline.traceId,
                            )}`}
                          >
                            Compare from here
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </Panel>

            <Panel
              title="Current compare cart"
              description="This is the shortest path into raw evidence. Keep the selected pair here while you decide whether the candidate is ready for drift review or manual inspection."
              accent="info"
            >
              <div className="grid gap-4">
                <SelectionSummaryCard label="Selected baseline" trace={surface.traces.find((trace) => trace.traceId === selectedBaselineId)} />
                <SelectionSummaryCard label="Selected candidate" trace={surface.traces.find((trace) => trace.traceId === selectedCandidateId)} />
                <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-muted)] p-4">
                  <FieldLabel>Next move</FieldLabel>
                  <p className="mt-3 text-sm leading-6 text-[var(--text-soft)]">
                    {compareHref
                      ? "Launchpad is valid. Move into Shadow Replay or open Divergence for verdict-oriented review."
                      : "Pick two different traces before opening compare."}
                  </p>
                  <div className="mt-4 flex flex-wrap gap-2">
                    <Button href={compareHref} tone="primary" disabled={!compareHref}>
                      Open Shadow Replay
                    </Button>
                    <Button href="/divergence" tone="secondary">
                      Review Divergence
                    </Button>
                  </div>
                </div>
              </div>
            </Panel>
          </div>

          <Panel
            title="Trace catalog"
            description="Search by trace ID, model, provider, or approved baseline name. Baseline promotion is intentionally available here so the UI can validate whether the contract supports a clean approval decision."
            actions={
              <div className="min-w-64">
                <Input
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  placeholder="Search trace IDs, models, providers..."
                  className="w-full"
                />
              </div>
            }
          >
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border/70 bg-muted/50 px-4 py-3">
              <p className="m-0 text-sm text-muted-foreground">
                {filteredCount} trace{filteredCount === 1 ? "" : "s"} visible
                {deferredSearch ? ` for “${deferredSearch}”` : ""}
              </p>
              <p className="m-0 text-sm text-muted-foreground">
                {surface.baselines.length} approved baseline{surface.baselines.length === 1 ? "" : "s"} in scope
              </p>
            </div>

            {filteredCount === 0 ? (
              <EmptyState
                title="No traces match this search"
                description="Try a trace ID, model, provider, or clear the search field to restore the full catalog."
              />
            ) : (
              <div className="grid gap-3">
                {filteredTraces?.map((trace) => {
                const isSelectedBaseline = trace.traceId === selectedBaselineId;
                const isSelectedCandidate = trace.traceId === selectedCandidateId;
                return (
                  <div key={trace.traceId} className="rounded-lg border border-[var(--border)] bg-[var(--bg-muted)] p-4">
                    <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="m-0 text-lg font-semibold tracking-[-0.03em]">{shortId(trace.traceId, 12)}</p>
                          {trace.baseline ? <Badge tone="success">Approved baseline</Badge> : null}
                          {isSelectedBaseline ? <Badge tone="info">Selected baseline</Badge> : null}
                          {isSelectedCandidate ? <Badge tone="warning">Selected candidate</Badge> : null}
                        </div>
                        <p className="mt-2 text-sm text-[var(--text-muted)]">
                          {trace.models.join(", ") || "unknown model"} via {trace.providers.join(", ") || "unknown provider"} · {trace.stepCount} step(s) · {formatDate(trace.createdAt)}
                        </p>
                      </div>

                      <div className="flex flex-wrap gap-2">
                        <Button onClick={() => startTransition(() => setSelectedBaselineId(trace.traceId))} tone="secondary">
                          Use as baseline
                        </Button>
                        <Button onClick={() => startTransition(() => setSelectedCandidateId(trace.traceId))} tone="secondary">
                          Use as candidate
                        </Button>
                        <ConfirmAction
                          title="Approve trace as a baseline?"
                          description={`Trace ${shortId(trace.traceId)} will become an approved governance reference for future drift review.`}
                          confirmLabel="Approve baseline"
                          pendingLabel="Approving..."
                          disabled={Boolean(trace.baseline) || pendingAction === trace.traceId}
                          pending={pendingAction === trace.traceId}
                          onConfirm={() => handleApprove(trace.traceId)}
                        >
                          <Button
                            tone="primary"
                            disabled={Boolean(trace.baseline) || pendingAction === trace.traceId}
                          >
                            {trace.baseline ? "Already approved" : "Approve as baseline"}
                          </Button>
                        </ConfirmAction>
                        <Button
                          href={`/shadow-replay?baseline=${encodeURIComponent(selectedBaselineId ?? trace.traceId)}&candidate=${encodeURIComponent(trace.traceId)}`}
                          tone="secondary"
                        >
                          Quick compare
                        </Button>
                      </div>
                    </div>
                  </div>
                );
                })}
              </div>
            )}
          </Panel>
        </div>
      ) : null}
    </>
  );
}

function pickBaseline(current: string | undefined, baselines: TraceRecord[], traces: TraceRecord[]) {
  if (current && baselines.some((baseline) => baseline.traceId === current)) {
    return current;
  }
  return baselines[0]?.traceId ?? traces[0]?.traceId;
}

function pickCandidate(current: string | undefined, traces: TraceRecord[], baselines: TraceRecord[]) {
  if (current && traces.some((trace) => trace.traceId === current)) {
    return current;
  }
  const baselineIds = new Set(baselines.map((baseline) => baseline.traceId));
  return traces.find((trace) => !baselineIds.has(trace.traceId))?.traceId ?? traces[0]?.traceId;
}

function SelectionSummaryCard({ label, trace }: { label: string; trace?: TraceRecord }) {
  return (
    <div className="rounded-lg border border-[var(--border)] bg-[var(--bg-muted)] p-4">
      <FieldLabel>{label}</FieldLabel>
      {trace ? (
        <>
          <p className="mt-3 text-xl font-semibold tracking-[-0.04em]">{shortId(trace.traceId, 12)}</p>
          <p className="mt-2 text-sm leading-6 text-[var(--text-muted)]">
            {trace.models.join(", ") || "unknown model"} · {trace.providers.join(", ") || "unknown provider"}
          </p>
        </>
      ) : (
        <p className="mt-3 text-sm leading-6 text-[var(--text-muted)]">Nothing selected yet.</p>
      )}
    </div>
  );
}
