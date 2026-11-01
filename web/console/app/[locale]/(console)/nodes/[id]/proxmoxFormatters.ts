export type ProxmoxTaskLike = {
  status?: string;
  exitstatus?: string;
};

/** A Proxmox config value as plain text: arrays joined, objects as "key: value" pairs. */
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

/** A Proxmox task's state: running, finished OK, or failed (an error status or any exit status other than OK). */
export function proxmoxTaskState(task: ProxmoxTaskLike): "running" | "ok" | "failed" {
  const status = (task.status ?? "").toLowerCase();
  const exitStatus = (task.exitstatus ?? "").toLowerCase();
  if (status === "running") return "running";
  if (exitStatus !== "" && exitStatus !== "ok") return "failed";
  if (status === "error") return "failed";
  return "ok";
}

/** The address of a Proxmox API path with the collector added when there is one. */
export function withCollector(path: string, collectorID: string): string {
  if (!collectorID) return path;
  return `${path}${path.includes("?") ? "&" : "?"}collector_id=${encodeURIComponent(collectorID)}`;
}
