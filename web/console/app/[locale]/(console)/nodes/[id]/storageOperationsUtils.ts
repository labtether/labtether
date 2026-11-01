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

export function confidenceLabel(raw: string): string {
  const value = raw.trim().toLowerCase();
  if (value === "high" || value === "medium" || value === "low") return value;
  return "low";
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
