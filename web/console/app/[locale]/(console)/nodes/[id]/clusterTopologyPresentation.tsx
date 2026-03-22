import type { ReactNode } from "react";

export function flowEdgeBadge(text: string, tooltip: string, tone: "info" | "success" | "danger"): ReactNode {
  const toneClass = tone === "success"
    ? "bg-[var(--ok-glow)] text-[var(--ok)]"
    : tone === "danger"
      ? "bg-[var(--bad-glow)] text-[var(--bad)]"
      : "bg-[var(--accent-subtle)] text-[var(--accent-text)]";

  return (
    <span
      title={tooltip}
      className={`pointer-events-auto rounded px-1.5 py-0.5 text-[10px] font-semibold shadow-sm ${toneClass}`}
    >
      {text}
    </span>
  );
}

export function sourceBadgeLabel(source?: string): string {
  const normalized = (source ?? "").trim().toLowerCase();
  if (normalized === "truenas") {
    return "TrueNAS";
  }
  if (normalized === "docker") {
    return "Docker";
  }
  if (normalized === "portainer") {
    return "Portainer";
  }
  if (!normalized) {
    return "External";
  }
  return normalized.toUpperCase();
}

export function haStateColor(state?: string): string {
  if (!state) return "text-[var(--muted)]";
  const s = state.toLowerCase();
  if (s === "started" || s === "running" || s === "enabled") return "text-[var(--ok)]";
  if (s === "stopped" || s === "disabled") return "text-[var(--muted)]";
  if (s === "error" || s === "fence" || s === "recovery") return "text-[var(--bad)]";
  if (s === "migrate" || s === "relocate" || s === "freeze") return "text-[var(--warn)]";
  return "text-[var(--muted)]";
}

export function guestStatusColor(status?: string): string {
  if (!status) return "text-[var(--muted)]";
  if (status === "running" || status === "online" || status === "active") return "text-[var(--ok)]";
  if (status === "stopped" || status === "paused" || status === "offline") return "text-[var(--muted)]";
  if (status === "error" || status === "failed") return "text-[var(--bad)]";
  return "text-[var(--muted)]";
}
