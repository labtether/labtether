import { formatBytes } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";

export type RiskState = "healthy" | "watch" | "action" | "critical";

export function storageKey(value: string): string {
  return value.trim().toLowerCase();
}

export function parseStorageName(asset: Asset): string {
  const rawStorageID = asset.metadata?.storage_id?.trim() ?? "";
  if (rawStorageID !== "") {
    const parts = rawStorageID.split("/");
    const last = parts[parts.length - 1]?.trim() ?? "";
    if (last !== "") return last;
  }
  const rawName = asset.name.trim();
  if (rawName === "") return asset.id;
  const parts = rawName.split("/");
  return parts[parts.length - 1]?.trim() || rawName;
}

export function parsePercent(raw?: string): number | null {
  if (!raw) return null;
  const parsed = Number(raw);
  if (!Number.isFinite(parsed)) return null;
  if (parsed < 0) return 0;
  return Math.min(parsed, 100);
}

export function formatPercent(value: number | null): string {
  if (value == null) return "--";
  return `${Math.round(value)}%`;
}

export function formatDays(value: number | null): string {
  if (value == null) return "--";
  if (value <= 0) return "0d";
  if (value < 1) return "<1d";
  if (value > 365) return ">365d";
  return `${Math.round(value)}d`;
}

export function formatGrowthBytes(value: number | null): string {
  if (value == null) return "--";
  if (value === 0) return "0 B";
  const sign = value > 0 ? "+" : "-";
  return `${sign}${formatBytes(Math.abs(value))}`;
}

export function normalizeHealth(raw: string | undefined): string {
  const health = (raw ?? "").trim().toUpperCase();
  if (health === "") return "UNKNOWN";
  return health;
}

export function isHealthyHealth(health: string): boolean {
  return health === "ONLINE" || health === "OK" || health === "ACTIVE";
}

export function riskStateFromScore(score: number): RiskState {
  if (score >= 75) return "critical";
  if (score >= 50) return "action";
  if (score >= 25) return "watch";
  return "healthy";
}

export function normalizeRiskState(raw: string | undefined, score: number): RiskState {
  const value = (raw ?? "").trim().toLowerCase();
  if (value === "critical" || value === "action" || value === "watch" || value === "healthy") {
    return value;
  }
  return riskStateFromScore(score);
}

export function riskBadgeStatus(state: RiskState): string {
  switch (state) {
    case "critical":
      return "critical";
    case "action":
      return "warning";
    case "watch":
      return "pending";
    default:
      return "ok";
  }
}

export function barColor(value: number | null): string {
  if (value == null) return "bg-[var(--surface)]";
  if (value >= 90) return "bg-[var(--bad)]";
  if (value >= 80) return "bg-[var(--warn)]";
  return "bg-[var(--ok)]";
}

export function recommendationStatus(severity: "critical" | "warning" | "info"): string {
  if (severity === "critical") return "critical";
  if (severity === "warning") return "warning";
  return "info";
}

export function confidenceLabel(raw: string): string {
  const value = raw.trim().toLowerCase();
  if (value === "high" || value === "medium" || value === "low") return value;
  return "low";
}

export function severityBadgeStatus(raw?: string): string {
  const value = (raw ?? "").trim().toLowerCase();
  if (value === "critical" || value === "warning" || value === "info") return value;
  return "info";
}

export function taskStatusBadge(taskStatus?: string, exitStatus?: string): string {
  const status = (taskStatus ?? "").trim().toLowerCase();
  const exit = (exitStatus ?? "").trim().toLowerCase();
  if (status === "running") return "pending";
  if (status === "error" || (exit !== "" && exit !== "ok")) return "bad";
  if (status === "stopped" || exit === "ok") return "ok";
  return "info";
}

export function parseIDList(values?: number[]): number[] {
  if (!Array.isArray(values)) return [];
  return values
    .map((value) => Number(value))
    .filter((value) => Number.isInteger(value) && value > 0);
}
