"use client";

import { clsx } from "clsx";
import type { PropsWithChildren, ReactNode } from "react";

import type { ChangeContext } from "@/lib/governance-types";
import { Badge as UIBadge } from "@/components/ui/badge";
import { Button as UIButton } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  theme = "selection",
}: {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
  theme?: "selection" | "divergence" | "shadow" | "gauntlet";
}) {
  return (
    <div className={clsx("hero-shell mb-8 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between", `hero-theme-${theme}`)}>
      <div aria-hidden="true" className="holo-orbit" />
      <div aria-hidden="true" className="holo-orbit-shadow" />
      {theme === "selection" ? null : (
        <svg
          aria-hidden="true"
          className="hero-shell-arcs"
          fill="none"
          viewBox="0 0 640 280"
        >
          <path
            className="hero-arc hero-arc-primary"
            d="M8 194C56 174 85 204 128 186C170 168 175 116 228 122C276 128 290 198 340 190C394 181 409 110 468 98C523 87 550 129 596 110C615 102 626 84 632 66"
          />
          <path
            className="hero-arc hero-arc-secondary"
            d="M24 148C76 140 88 102 142 101C197 100 215 164 266 168C326 173 346 118 404 126C454 132 474 184 530 185C574 186 600 157 628 142"
          />
          <path
            className="hero-arc-glow"
            d="M420 12C396 44 404 70 382 102C359 136 313 150 304 188C295 223 318 246 330 268"
          />
        </svg>
      )}
      {theme === "selection" ? null : (
        <svg
          aria-hidden="true"
          className="hero-shell-bolt"
          fill="none"
          viewBox="0 0 160 160"
        >
          <path
            d="M110 12 76 70h24L58 148l18-56H52l58-80Z"
            fill="currentColor"
            fillOpacity="0.18"
            stroke="currentColor"
            strokeOpacity="0.85"
            strokeWidth="2.5"
          />
        </svg>
      )}
      <div>
        <p className="m-0 inline-flex rounded-full border border-primary/18 bg-primary/10 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.3em] text-primary">
          {eyebrow}
        </p>
        <h2 className="hero-title mt-4 text-4xl font-semibold tracking-[-0.05em] text-foreground md:text-5xl">{title}</h2>
        <p className="hero-copy mt-3 text-sm leading-6 text-muted-foreground md:text-[15px]">{description}</p>
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-3">{actions}</div> : null}
    </div>
  );
}

export function Panel({
  title,
  description,
  children,
  accent,
  actions,
  className,
}: PropsWithChildren<{
  title: string;
  description?: string;
  accent?: "warning" | "danger" | "success" | "info";
  actions?: ReactNode;
  className?: string;
}>) {
  return (
    <Card
      className={clsx(
        "cinematic-panel",
        accent === "warning" && "border-[hsl(var(--warning)/0.35)]",
        accent === "danger" && "border-destructive/35",
        accent === "success" && "border-[hsl(var(--success)/0.35)]",
        accent === "info" && "border-primary/35",
        className,
      )}
    >
      <CardHeader className="pb-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle className="text-lg tracking-[-0.03em]">{title}</CardTitle>
            {description ? <CardDescription className="mt-2 leading-6">{description}</CardDescription> : null}
          </div>
          {actions}
        </div>
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

export function Stat({
  label,
  value,
  tone = "neutral",
  meta,
}: {
  label: string;
  value: ReactNode;
  tone?: "neutral" | "success" | "warning" | "danger" | "info";
  meta?: string;
}) {
  return (
    <div className="cinematic-card rounded-lg border p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p
        className={clsx(
          "mt-3 text-3xl font-semibold tracking-[-0.06em]",
          tone === "success" && "text-[hsl(var(--success))]",
          tone === "warning" && "text-[hsl(var(--warning))]",
          tone === "danger" && "text-destructive",
          tone === "info" && "text-[hsl(var(--info))]",
        )}
      >
        {value}
      </p>
      {meta ? <p className="mt-2 text-xs uppercase tracking-[0.2em] text-muted-foreground">{meta}</p> : null}
    </div>
  );
}

export function Badge({
  children,
  tone = "neutral",
}: PropsWithChildren<{
  tone?: "neutral" | "success" | "warning" | "danger" | "info";
}>) {
  const variant =
    tone === "success"
      ? "success"
      : tone === "warning"
        ? "warning"
        : tone === "danger"
          ? "destructive"
          : tone === "info"
            ? "info"
            : "secondary";
  return (
    <UIBadge variant={variant} className="energy-badge uppercase tracking-[0.22em]">
      {children}
    </UIBadge>
  );
}

export function Button({
  children,
  href,
  tone = "secondary",
  disabled,
  onClick,
  type = "button",
}: PropsWithChildren<{
  href?: string;
  tone?: "primary" | "secondary" | "ghost" | "danger";
  disabled?: boolean;
  onClick?: () => void;
  type?: "button" | "submit";
}>) {
  const variant = tone === "primary" ? "default" : tone === "secondary" ? "outline" : tone === "ghost" ? "ghost" : "destructive";

  return href ? (
    <UIButton disabled={disabled} href={href} variant={variant}>
      {children}
    </UIButton>
  ) : (
    <UIButton disabled={disabled} onClick={onClick} type={type} variant={variant}>
      {children}
    </UIButton>
  );
}

export function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <Card className="rounded-lg p-12 text-center">
      <p className="m-0 text-[11px] uppercase tracking-[0.26em] text-muted-foreground">No evidence yet</p>
      <h3 className="mt-3 text-2xl font-semibold tracking-[-0.04em]">{title}</h3>
      <p className="mx-auto mt-3 max-w-xl text-sm leading-6 text-muted-foreground">{description}</p>
      {action ? <div className="mt-6 flex justify-center">{action}</div> : null}
    </Card>
  );
}

export function LoadingState({ label = "Loading governance surface..." }: { label?: string }) {
  return (
    <Card className="cinematic-panel">
      <CardContent className="p-8">
        <div className="flex items-center gap-4">
          <div className="h-3 w-3 animate-pulse rounded-full bg-[hsl(var(--info))]" />
          <p className="m-0 text-sm text-foreground/80">{label}</p>
        </div>
      </CardContent>
    </Card>
  );
}

export function ErrorState({
  title = "Request failed",
  message,
  action,
}: {
  title?: string;
  message: string;
  action?: ReactNode;
}) {
  return (
    <Card className="border-destructive/35">
      <CardContent className="p-6">
        <Badge tone="danger">Error</Badge>
        <h3 className="mt-4 text-xl font-semibold tracking-[-0.04em]">{title}</h3>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">{message}</p>
        {action ? <div className="mt-5">{action}</div> : null}
      </CardContent>
    </Card>
  );
}

export function InlineNotice({ children }: PropsWithChildren) {
  return (
    <div className="rounded-lg border border-primary/35 bg-accent px-4 py-3 text-sm text-accent-foreground">
      {children}
    </div>
  );
}

export { Input };

export function DefinitionList({ items }: { items: Array<{ label: string; value: ReactNode }> }) {
  return (
    <dl className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      {items.map((item) => (
        <div key={item.label} className="cinematic-card rounded-lg border p-4">
          <dt className="text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{item.label}</dt>
          <dd className="mt-3 text-sm leading-6 text-foreground/85">{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}

export function JsonBlock({ value }: { value: unknown }) {
  return (
    <pre className="scrollbar-thin max-h-72 overflow-auto rounded-lg border bg-[hsl(var(--code-background))] p-4 text-xs leading-6 text-foreground/85 shadow-[inset_0_1px_0_hsl(var(--primary)/0.08)]">
      {JSON.stringify(value ?? {}, null, 2)}
    </pre>
  );
}

export function StepText({ title, text }: { title: string; text?: string }) {
  return (
    <div className="cinematic-card rounded-lg border p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{title}</p>
      <p className="mt-3 whitespace-pre-wrap text-sm leading-6 text-foreground/85">{text || "No content captured."}</p>
    </div>
  );
}

export function FieldLabel({ children }: PropsWithChildren) {
  return <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-muted-foreground">{children}</p>;
}

export function shortId(value?: string, width = 8) {
  if (!value) {
    return "unknown";
  }
  if (value.length <= width * 2) {
    return value;
  }
  return `${value.slice(0, width)}…${value.slice(-width)}`;
}

export function formatDate(value?: string) {
  if (!value) {
    return "unknown";
  }
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function formatScore(value?: number) {
  if (value === undefined || Number.isNaN(value)) {
    return "n/a";
  }
  return `${Math.round(value * 100)}%`;
}

export function verdictTone(verdict?: string): "neutral" | "success" | "warning" | "danger" {
  switch (verdict) {
    case "pass":
      return "success";
    case "warn":
      return "warning";
    case "fail":
      return "danger";
    default:
      return "neutral";
  }
}

export function riskTone(risk?: string): "neutral" | "warning" | "danger" {
  switch (risk) {
    case "write":
      return "warning";
    case "destructive":
      return "danger";
    default:
      return "neutral";
  }
}

export function ChangeContextBanner({ ctx }: { ctx?: ChangeContext }) {
  if (!ctx) return null;
  return (
    <div className="rounded-lg border border-primary/20 bg-primary/5 p-4">
      <p className="m-0 text-[11px] uppercase tracking-[0.24em] text-primary/70">What changed</p>
      <p className="mt-2 text-sm font-semibold text-foreground">
        {ctx.target}
        {ctx.baselineLabel && ctx.candidateLabel ? (
          <span className="ml-2 font-normal text-muted-foreground">
            {ctx.baselineLabel} → {ctx.candidateLabel}
          </span>
        ) : null}
      </p>
      {ctx.summary ? <p className="mt-1 text-sm leading-6 text-muted-foreground">{ctx.summary}</p> : null}
    </div>
  );
}
