import type { AlertInstance, AlertRule } from "../../../console/models";

/** The page's tabs, kept in the address as ?tab=…; Issues is its own route (/incidents). */
export type AlertTab = "alerts" | "rules" | "silences" | "delivery";
export const ALERT_TABS: AlertTab[] = ["alerts", "rules", "silences", "delivery"];
export const parseAlertTab = (value: string | null): AlertTab =>
  ALERT_TABS.includes(value as AlertTab) ? (value as AlertTab) : "alerts";

export type Severity = AlertInstance["severity"];
export const SEVERITIES: Severity[] = ["critical", "high", "medium", "low"];
export type SeverityFilter = Severity | "all";
export type StateFilter = AlertInstance["status"] | "all";
export const STATES: AlertInstance["status"][] = ["firing", "acknowledged", "pending", "resolved"];
export type AlertSort = "severity" | "severityAsc" | "newest" | "oldest";
export const ALERT_SORTS: AlertSort[] = ["severity", "severityAsc", "newest", "oldest"];
export type InboxFilter = { severity: SeverityFilter; state: StateFilter; sort: AlertSort };
export const DEFAULT_FILTER: InboxFilter = { severity: "all", state: "all", sort: "severity" };

export type DurationPreset = "1h" | "4h" | "12h" | "24h";
export const durationPresets: Array<{ id: DurationPreset; hours: number }> = [
  { id: "1h", hours: 1 },
  { id: "4h", hours: 4 },
  { id: "12h", hours: 12 },
  { id: "24h", hours: 24 },
];

export type RuleKind = AlertRule["kind"];
export const RULE_KINDS: RuleKind[] = ["metric_threshold", "metric_deadman", "heartbeat_stale", "log_pattern", "composite", "synthetic_check"];
export type RuleSeverity = Severity;
export type TargetType = AlertRule["target_scope"];
export const TARGET_TYPES: TargetType[] = ["global", "group", "asset"];
export const OPERATORS = [">", "<", ">=", "<=", "=="] as const;
export const AGGREGATES = ["avg", "max", "min", "last"] as const;

/** One notification the hub sent (or tried to send) for an alert: /api/notifications/history. */
export type DeliveryStatus = "sent" | "failed" | "pending";
export type DeliveryRecord = {
  id: string;
  channel_id: string;
  alert_instance_id: string;
  route_id: string;
  /** The alert rule's name when the message was sent; it outlives the rule and the alert. */
  alert_name?: string;
  status: DeliveryStatus;
  sent_at: string | null;
  error: string;
  retry_count: number;
  max_retries: number;
  next_retry_at: string | null;
  created_at: string;
};

export type SilencePayload = {
  matchers: Record<string, string>;
  starts_at: string;
  ends_at: string;
  reason?: string;
};

/** One label=value row in the silence and rule forms; rows without a label are left out. */
export type LabelRow = { key: string; value: string };
export const blankLabel = (): LabelRow => ({ key: "", value: "" });
export const labelsFromRows = (rows: LabelRow[]): Record<string, string> =>
  Object.fromEntries(rows.filter((r) => r.key.trim()).map((r) => [r.key.trim(), r.value.trim()]));
