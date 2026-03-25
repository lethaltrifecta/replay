"use client";

import { useEffect, useState } from "react";

import { formatApiError, getExperimentInbox } from "@/lib/governance";

import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  formatDate,
  LoadingState,
  PageHeader,
  Panel,
  shortId,
  verdictTone,
} from "./view-kit";

export function ExperimentsScreen() {
  const [items, setItems] = useState<Awaited<ReturnType<typeof getExperimentInbox>>>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string>();

  useEffect(() => {
    void loadExperiments();
  }, []);

  async function loadExperiments() {
    try {
      setLoading(true);
      setError(undefined);
      setItems(await getExperimentInbox());
    } catch (err) {
      setError(formatApiError(err));
    } finally {
      setLoading(false);
    }
  }

  if (loading && items.length === 0) {
    return <LoadingState label="Loading Gauntlet reports..." />;
  }

  const counts = {
    running: items.filter((item) => item.status === "running" || item.status === "pending").length,
    approved: items.filter((item) => item.verdict === "pass").length,
    rejected: items.filter((item) => item.verdict === "fail").length,
    failed: items.filter((item) => item.status === "failed").length,
  };

  return (
    <>
      <PageHeader
        eyebrow="The Gauntlet"
        title="Canonical trial reports"
        description="The Gauntlet report route is the contract center for trial review. This list stays intentionally lean so the real explanation work happens inside the report view."
        theme="gauntlet"
        actions={
          <Button onClick={() => void loadExperiments()} tone="secondary">
            Refresh
          </Button>
        }
      />

      {error ? <ErrorState message={error} /> : null}

      {!loading && items.length === 0 ? (
        <EmptyState
          title="No Gauntlet runs yet"
          description="Trigger a Gauntlet trial through the API or seed demo data to populate this review surface."
        />
      ) : null}

      {items.length > 0 ? (
        <div className="section-grid">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            <ExperimentCount label="Running" value={counts.running} tone="info" />
            <ExperimentCount label="Survived" value={counts.approved} tone="success" />
            <ExperimentCount label="Cut" value={counts.rejected} tone="warning" />
            <ExperimentCount label="System failed" value={counts.failed} tone="danger" />
          </div>

          <Panel title="Gauntlet roster" description="Open a report whenever the list alone stops answering the approval question.">
            <div className="grid gap-3">
              {items.map((item) => (
                <div key={item.id} className="rounded-lg border bg-muted/70 p-4">
                  <div className="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
                    <div>
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="m-0 text-lg font-semibold tracking-[-0.03em]">{item.name}</p>
                        <Badge tone={item.status === "failed" ? "danger" : item.status === "running" ? "info" : "neutral"}>
                          {item.status}
                        </Badge>
                        {item.verdict ? <Badge tone={verdictTone(item.verdict)}>{item.verdict}</Badge> : null}
                      </div>
                      <p className="mt-2 text-sm leading-6 text-muted-foreground">
                        {shortId(item.id)} · baseline {shortId(item.baselineTraceId, 8)} · {item.model || "unknown model"} {item.provider ? `via ${item.provider}` : ""}
                      </p>
                      <p className="mt-1 text-sm leading-6 text-muted-foreground">
                        Started {formatDate(item.createdAt)}{item.completedAt ? ` · Completed ${formatDate(item.completedAt)}` : ""}
                      </p>
                    </div>

                    <div className="flex flex-wrap gap-2">
                      <Button href={`/gauntlet/${encodeURIComponent(item.id)}`} tone="primary">
                        Open trial report
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </Panel>
        </div>
      ) : null}
    </>
  );
}

function ExperimentCount({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: "success" | "warning" | "danger" | "info";
}) {
  return (
    <div className="rounded-lg border bg-muted/70 p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p
        className={`mt-3 text-4xl font-semibold tracking-[-0.06em] ${
          tone === "success"
            ? "text-[hsl(var(--success))]"
            : tone === "warning"
              ? "text-[hsl(var(--warning))]"
              : tone === "danger"
                ? "text-destructive"
                : "text-[hsl(var(--info))]"
        }`}
      >
        {value}
      </p>
    </div>
  );
}
