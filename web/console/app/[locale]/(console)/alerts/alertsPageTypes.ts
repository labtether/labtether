export type AlertSeverityFilter = "critical" | "high" | "medium" | "low" | "all";
export type AlertStateFilter = "pending" | "firing" | "acknowledged" | "resolved" | "all";
export type AlertTab = "inbox" | "rules" | "silences" | "delivery";
export type DurationPreset = "1h" | "4h" | "12h" | "24h";
export type RuleKind = "metric_threshold" | "metric_deadman" | "heartbeat_stale" | "log_pattern" | "composite" | "synthetic_check";
export type RuleSeverity = "critical" | "high" | "medium" | "low";
export type TargetType = "group" | "asset" | "global";

export const ruleKindOptions: Array<{ id: RuleKind; label: string }> = [
  { id: "metric_threshold", label: "Metric Threshold" },
  { id: "metric_deadman", label: "Metric Deadman" },
  { id: "heartbeat_stale", label: "Heartbeat Stale" },
  { id: "log_pattern", label: "Log Pattern" },
  { id: "composite", label: "Composite" },
  { id: "synthetic_check", label: "Synthetic Check" },
];

export const ruleSeverityOptions: Array<{ id: RuleSeverity; label: string }> = [
  { id: "critical", label: "Critical" },
  { id: "high", label: "High" },
  { id: "medium", label: "Medium" },
  { id: "low", label: "Low" },
];

export const targetTypeOptions: Array<{ id: TargetType; label: string }> = [
  { id: "global", label: "Global" },
  { id: "group", label: "Group" },
  { id: "asset", label: "Asset" },
];

export const durationPresets: Array<{ id: DurationPreset; label: string; hours: number }> = [
  { id: "1h", label: "1 hour", hours: 1 },
  { id: "4h", label: "4 hours", hours: 4 },
  { id: "12h", label: "12 hours", hours: 12 },
  { id: "24h", label: "24 hours", hours: 24 },
];
