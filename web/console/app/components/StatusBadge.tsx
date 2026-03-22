const statusMap: Record<string, { label: string; className: string }> = {
  // Severity
  critical: { label: "Critical", className: "bad" },
  major: { label: "Major", className: "bad" },
  warning: { label: "Warning", className: "pending" },
  minor: { label: "Minor", className: "pending" },
  info: { label: "Info", className: "" },

  // Alert state
  firing: { label: "Active", className: "bad" },
  acknowledged: { label: "Acknowledged", className: "pending" },
  resolved: { label: "Resolved", className: "ok" },

  // Incident status
  open: { label: "Open", className: "bad" },
  investigating: { label: "Investigating", className: "pending" },
  mitigating: { label: "Mitigating", className: "pending" },
  closed: { label: "Closed", className: "" },

  // Asset freshness
  online: { label: "Online", className: "ok" },
  stale: { label: "Unresponsive", className: "pending" },
  unresponsive: { label: "Unresponsive", className: "pending" },
  offline: { label: "Offline", className: "bad" },

  // Generic
  ok: { label: "Healthy", className: "ok" },
  bad: { label: "Error", className: "bad" },
  pending: { label: "In Progress", className: "pending" },
  active: { label: "Active", className: "ok" },
  inactive: { label: "Inactive", className: "" },
  maintenance: { label: "Maintenance", className: "pending" },
  expired: { label: "Expired", className: "" },
  enabled: { label: "Enabled", className: "ok" },
  disabled: { label: "Disabled", className: "" },

  // Action/run status
  succeeded: { label: "Succeeded", className: "ok" },
  failed: { label: "Failed", className: "bad" },
  running: { label: "Running", className: "pending" },
  queued: { label: "Queued", className: "" },
};

export function StatusBadge({ status }: { status: string }) {
  const mapped = statusMap[status.toLowerCase()];
  const label = mapped?.label ?? status;
  const className = mapped?.className ?? "";

  return <span className={`statusDot ${className}`}>{label}</span>;
}
