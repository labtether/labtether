export type AlertInstance = {
  id: string;
  rule_id: string;
  fingerprint: string;
  status: "pending" | "firing" | "acknowledged" | "resolved";
  severity: "critical" | "high" | "medium" | "low";
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  started_at: string;
  resolved_at?: string;
  last_fired_at: string;
  suppressed_by?: string;
  created_at: string;
  updated_at: string;
};

export type AlertRule = {
  id: string;
  name: string;
  description?: string;
  status: "active" | "paused";
  kind:
    | "metric_threshold"
    | "metric_deadman"
    | "heartbeat_stale"
    | "log_pattern"
    | "composite"
    | "synthetic_check";
  severity: "critical" | "high" | "medium" | "low";
  target_scope: "asset" | "group" | "global";
  cooldown_seconds: number;
  window_seconds: number;
  condition: Record<string, unknown>;
  labels?: Record<string, string>;
  targets?: Array<{ id: string; asset_id?: string; group_id?: string }>;
  created_at: string;
  updated_at: string;
  last_evaluated_at?: string;
};

export type AlertRuleTemplate = {
  id: string;
  name: string;
  description: string;
  kind:
    | "metric_threshold"
    | "metric_deadman"
    | "heartbeat_stale"
    | "log_pattern"
    | "composite"
    | "synthetic_check";
  severity: "critical" | "high" | "medium" | "low";
  target_scope: "asset" | "group" | "global";
  cooldown_seconds: number;
  reopen_after_seconds: number;
  evaluation_interval_seconds: number;
  window_seconds: number;
  condition: Record<string, unknown>;
  labels?: Record<string, string>;
  metadata?: Record<string, string>;
};

export type AlertSilence = {
  id: string;
  matchers: Record<string, string>;
  reason?: string;
  created_by: string;
  starts_at: string;
  ends_at: string;
  created_at: string;
};

export type Incident = {
  id: string;
  title: string;
  severity: "critical" | "high" | "medium" | "low";
  status: "open" | "investigating" | "mitigated" | "resolved" | "closed";
  source: "manual" | "alert_auto";
  summary?: string;
  assignee?: string;
  root_cause?: string;
  action_items?: string[];
  lessons_learned?: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
};

export type IncidentEvent = {
  id: string;
  incident_id: string;
  kind: string;
  title: string;
  detail?: string;
  source?: string;
  created_at: string;
};
