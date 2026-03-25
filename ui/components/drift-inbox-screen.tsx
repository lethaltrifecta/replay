"use client";

import { useEffect, useState } from "react";

import { formatApiError, getDriftInbox } from "@/lib/governance";

import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  formatDate,
  formatScore,
  LoadingState,
  PageHeader,
  Panel,
  shortId,
  verdictTone,
} from "./view-kit";

const lanes = ["fail", "warn", "pass", "pending"] as const;

export function DriftInboxScreen() {
  const [items, setItems] = useState<Awaited<ReturnType<typeof getDriftInbox>>>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    void loadInbox();
  }, []);

  async function loadInbox() {
    try {
      setLoading(true);
      setError(undefined);
      setItems(await getDriftInbox());
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  }

  if (loading && items.length === 0) {
    return <LoadingState label="Loading drift inbox..." />;
  }

  return (
    <>
      <PageHeader
        eyebrow="Divergence Engine"
        title="Verdict-first divergence queue"
        description="This is the Divergence queue. Candidates are grouped by verdict so the operator can triage hard failures first, then review warnings, then sweep through safe passes."
        theme="divergence"
        actions={
          <Button onClick={() => void loadInbox()} tone="secondary">
            Refresh queue
          </Button>
        }
      />

      {error ? (
        <ErrorState
          message={error}
          action={
            <Button onClick={() => void loadInbox()} tone="secondary">
              Retry
            </Button>
          }
        />
      ) : null}

      {!loading && items.length === 0 ? (
        <EmptyState title="No divergences yet" description="Run the Divergence Engine or seed demo data to populate this queue." />
      ) : null}

      {items.length > 0 ? (
        <div className="grid gap-4 2xl:grid-cols-4">
          {lanes.map((lane) => {
            const laneItems = items.filter((item) => item.verdict === lane);
            return (
              <Panel
                key={lane}
                title={lane.toUpperCase()}
                description={`${laneItems.length} item(s) currently in this review lane.`}
                accent={lane === "fail" ? "danger" : lane === "warn" ? "warning" : lane === "pass" ? "success" : "info"}
              >
                {laneItems.length === 0 ? (
                  <p className="text-sm leading-6 text-muted-foreground">No items in this lane.</p>
                ) : (
                  <div className="grid gap-3">
                    {laneItems.map((item) => (
                      <div key={`${item.baselineTraceId}:${item.traceId}`} className="rounded-lg border bg-muted/70 p-4">
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="m-0 text-base font-semibold tracking-[-0.03em]">{shortId(item.traceId, 10)}</p>
                            <p className="mt-2 text-xs uppercase tracking-[0.22em] text-muted-foreground">
                              vs {shortId(item.baselineTraceId, 8)}
                            </p>
                          </div>
                          <Badge tone={verdictTone(item.verdict)}>{item.verdict}</Badge>
                        </div>

                        <p className="mt-4 text-sm leading-6 text-foreground/85">
                          {item.reason || "No compact drift explanation was captured for this result."}
                        </p>

                        <div className="mt-4 flex flex-wrap gap-2">
                          <Badge tone="info">{formatScore(item.driftScore)}</Badge>
                          {item.riskEscalation ? <Badge tone="danger">Blast Radius escalated</Badge> : null}
                          {item.divergenceStep !== undefined ? <Badge tone="warning">Step {item.divergenceStep}</Badge> : null}
                        </div>

                        <div className="mt-4 text-sm leading-6 text-muted-foreground">
                          <div>{item.candidate?.models.join(", ") || "unknown model"}</div>
                          <div>{formatDate(item.createdAt)}</div>
                        </div>

                        <div className="mt-5 flex flex-wrap gap-2">
                          <Button href={`/divergence/${encodeURIComponent(item.traceId)}?baseline=${encodeURIComponent(item.baselineTraceId)}`} tone="primary">
                            Open detail
                          </Button>
                          <Button href={`/shadow-replay?baseline=${encodeURIComponent(item.baselineTraceId)}&candidate=${encodeURIComponent(item.traceId)}`} tone="secondary">
                            Shadow Replay
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </Panel>
            );
          })}
        </div>
      ) : null}
    </>
  );
}
