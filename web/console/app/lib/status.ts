export type StatusEntry = {
  label: string;
  color: "emerald" | "red" | "amber" | "zinc";
};

export const statusConfig: Record<string, StatusEntry> = {
  // Severity
  critical: { label: "Critical", color: "red" },
  major: { label: "Major", color: "red" },
  warning: { label: "Warning", color: "amber" },
  minor: { label: "Minor", color: "amber" },
  info: { label: "Info", color: "zinc" },

  // Alert state
  firing: { label: "Active", color: "red" },
  acknowledged: { label: "Acknowledged", color: "amber" },
  resolved: { label: "Resolved", color: "emerald" },

  // Incident status
  open: { label: "Open", color: "red" },
  investigating: { label: "Investigating", color: "amber" },
  mitigating: { label: "Mitigating", color: "amber" },
  mitigated: { label: "Mitigated", color: "amber" },
  closed: { label: "Closed", color: "zinc" },

  // Asset freshness
  online: { label: "Online", color: "emerald" },
  stale: { label: "Unresponsive", color: "amber" },
  unresponsive: { label: "Unresponsive", color: "amber" },
  offline: { label: "Offline", color: "red" },

  // Generic
  ok: { label: "Healthy", color: "emerald" },
  bad: { label: "Error", color: "red" },
  pending: { label: "In Progress", color: "amber" },
  active: { label: "Active", color: "emerald" },
  inactive: { label: "Inactive", color: "zinc" },
  maintenance: { label: "Maintenance", color: "amber" },
  expired: { label: "Expired", color: "zinc" },
  enabled: { label: "Enabled", color: "emerald" },
  disabled: { label: "Disabled", color: "zinc" },
  paused: { label: "Paused", color: "zinc" },

  // Action/run status
  succeeded: { label: "Succeeded", color: "emerald" },
  failed: { label: "Failed", color: "red" },
  running: { label: "Running", color: "amber" },
  queued: { label: "Queued", color: "zinc" },
};

const colorMap: Record<string, { dot: string; bg: string; text: string }> = {
  emerald: {
    dot: "bg-[var(--ok)]",
    bg: "bg-[var(--ok-glow)]",
    text: "text-[var(--ok)]",
  },
  red: {
    dot: "bg-[var(--bad)]",
    bg: "bg-[var(--bad-glow)]",
    text: "text-[var(--bad)]",
  },
  amber: {
    dot: "bg-[var(--warn)]",
    bg: "bg-[var(--warn-glow)]",
    text: "text-[var(--warn)]",
  },
  zinc: {
    dot: "bg-[var(--muted)]",
    bg: "bg-[var(--surface)]",
    text: "text-[var(--muted)]",
  },
};

export function getStatusColors(status: string) {
  const entry = statusConfig[status.toLowerCase()];
  const color = entry?.color ?? "zinc";
  return colorMap[color] ?? colorMap.zinc;
}

export function getStatusLabel(status: string): string {
  const entry = statusConfig[status.toLowerCase()];
  return entry?.label ?? status;
}
