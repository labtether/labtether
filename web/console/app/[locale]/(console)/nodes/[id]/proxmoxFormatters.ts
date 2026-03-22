export type ProxmoxTaskLike = {
  status?: string;
  exitstatus?: string;
};

export function formatProxmoxValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number") return Number.isFinite(value) ? String(value) : "";
  if (typeof value === "boolean") return value ? "true" : "false";
  if (Array.isArray(value)) {
    return value.map((item) => formatProxmoxValue(item)).filter(Boolean).join(", ");
  }
  if (typeof value === "object") {
    try {
      const entries = Object.entries(value as Record<string, unknown>);
      if (entries.length === 0) return "";
      return entries
        .map(([k, v]) => `${k}: ${formatProxmoxValue(v)}`)
        .join(", ");
    } catch {
      return String(value);
    }
  }
  return String(value);
}

export function formatProxmoxEpoch(value?: number): string {
  if (typeof value !== "number" || !Number.isFinite(value) || value <= 0) {
    return "n/a";
  }
  return new Date(value * 1000).toLocaleString();
}

export function taskStatusBadge(task: ProxmoxTaskLike): "ok" | "pending" | "bad" {
  const status = (task.status ?? "").toLowerCase();
  const exitStatus = (task.exitstatus ?? "").toLowerCase();
  if (status === "running") {
    return "pending";
  }
  if (exitStatus !== "" && exitStatus !== "ok") {
    return "bad";
  }
  if (status === "error") {
    return "bad";
  }
  return "ok";
}
