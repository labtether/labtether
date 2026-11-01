import type { AlertRule, AlertRuleTemplate } from "../../../console/models";
import { labelsFromRows, type LabelRow, type RuleKind, type RuleSeverity, type TargetType } from "./alertsPageTypes";

/** The rule editor's values. Seconds stay seconds, as the hub stores them. */
export type RuleFormState = {
  name: string;
  description: string;
  kind: RuleKind;
  severity: RuleSeverity;
  targetType: TargetType;
  targetId: string;
  windowSeconds: number;
  cooldownSeconds: number;
  reopenAfterSeconds: number;
  evaluationIntervalSeconds: number;
  metric: string;
  operator: string;
  thresholdValue: number;
  aggregate: string;
  maxSilenceSeconds: number;
  maxStaleSeconds: number;
  pattern: string;
  minOccurrences: number;
  checkId: string;
  consecutiveFailures: number;
  subRuleIds: string[];
  compositeOperator: "and" | "or";
  /** The rule's labels; silences match against them. */
  labels: LabelRow[];
};

export const defaultRuleFormState: RuleFormState = {
  name: "",
  description: "",
  kind: "metric_threshold",
  severity: "high",
  targetType: "global",
  targetId: "",
  windowSeconds: 300,
  cooldownSeconds: 300,
  reopenAfterSeconds: 120,
  evaluationIntervalSeconds: 30,
  metric: "",
  operator: ">",
  thresholdValue: 90,
  aggregate: "avg",
  maxSilenceSeconds: 300,
  maxStaleSeconds: 300,
  pattern: "",
  minOccurrences: 5,
  checkId: "",
  consecutiveFailures: 3,
  subRuleIds: [],
  compositeOperator: "and",
  labels: [],
};

export function createRuleFormState(overrides: Partial<RuleFormState> = {}): RuleFormState {
  return { ...defaultRuleFormState, ...overrides };
}

export function numberFromUnknown(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export function stringFromUnknown(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

type RuleLike = Pick<AlertRuleTemplate, "name" | "kind" | "severity" | "target_scope" | "window_seconds" | "cooldown_seconds" | "condition" | "labels"> & {
  description?: string;
  reopen_after_seconds?: number;
  evaluation_interval_seconds?: number;
};

function formStateFrom(source: RuleLike, targetId = ""): RuleFormState {
  const condition = source.condition ?? {};
  const d = defaultRuleFormState;
  return createRuleFormState({
    name: source.name,
    description: source.description ?? "",
    kind: source.kind,
    severity: source.severity,
    targetType: source.target_scope,
    targetId,
    windowSeconds: numberFromUnknown(source.window_seconds, d.windowSeconds),
    cooldownSeconds: numberFromUnknown(source.cooldown_seconds, d.cooldownSeconds),
    reopenAfterSeconds: numberFromUnknown(source.reopen_after_seconds, d.reopenAfterSeconds),
    evaluationIntervalSeconds: numberFromUnknown(source.evaluation_interval_seconds, d.evaluationIntervalSeconds),
    metric: stringFromUnknown(condition.metric),
    operator: stringFromUnknown(condition.operator, ">"),
    thresholdValue: numberFromUnknown(condition.value, numberFromUnknown(condition.threshold, 0)),
    aggregate: stringFromUnknown(condition.aggregate, "avg"),
    maxSilenceSeconds: numberFromUnknown(condition.max_silence_seconds, d.maxSilenceSeconds),
    maxStaleSeconds: numberFromUnknown(condition.max_stale_seconds, d.maxStaleSeconds),
    pattern: stringFromUnknown(condition.pattern),
    minOccurrences: numberFromUnknown(condition.min_occurrences, d.minOccurrences),
    checkId: stringFromUnknown(condition.check_id),
    consecutiveFailures: numberFromUnknown(condition.consecutive_failures, d.consecutiveFailures),
    subRuleIds: Array.isArray(condition.rule_ids) ? condition.rule_ids.map((value) => String(value)) : [],
    compositeOperator: stringFromUnknown(condition.operator) === "or" ? "or" : "and",
    labels: Object.entries(source.labels ?? {}).map(([key, value]) => ({ key, value })),
  });
}

export const formStateFromTemplate = (template: AlertRuleTemplate) => formStateFrom(template);

/** An existing rule's values, for changing it. The form holds one target: the first. */
export function formStateFromRule(rule: AlertRule): RuleFormState {
  const first = rule.targets?.[0];
  return formStateFrom(rule as RuleLike, first?.asset_id ?? first?.group_id ?? "");
}

/** Field → the alerts.editor.errors key that says what is wrong with it. */
export type RuleFormErrors = Partial<Record<
  "name" | "metric" | "threshold" | "pattern" | "check" | "rules" | "target" | "seconds" | "count" | "window" | "interval" | "cooldown" | "reopen",
  string
>>;

const whole = (n: number, min: number) => Number.isInteger(n) && n >= min;

/** What stops the hub from taking this rule, per field. Empty when it can be saved. */
export function ruleFormErrors(form: RuleFormState, otherNames: string[]): RuleFormErrors {
  const err: RuleFormErrors = {};
  const name = form.name.trim();
  if (!name) err.name = "name";
  else if (otherNames.some((n) => n.trim().toLowerCase() === name.toLowerCase())) err.name = "nameTaken";
  if (form.targetType !== "global" && !form.targetId.trim()) err.target = "target";
  if (!whole(form.windowSeconds, 1)) err.window = "whole";
  if (!whole(form.evaluationIntervalSeconds, 1)) err.interval = "whole";
  if (!whole(form.cooldownSeconds, 0)) err.cooldown = "wholeZero";
  if (!whole(form.reopenAfterSeconds, 0)) err.reopen = "wholeZero";
  switch (form.kind) {
    case "metric_threshold":
      if (!form.metric.trim()) err.metric = "metric";
      if (!Number.isFinite(form.thresholdValue)) err.threshold = "number";
      break;
    case "metric_deadman":
      if (!form.metric.trim()) err.metric = "metric";
      if (!whole(form.maxSilenceSeconds, 1)) err.seconds = "whole";
      break;
    case "heartbeat_stale":
      if (!whole(form.maxStaleSeconds, 1)) err.seconds = "whole";
      break;
    case "log_pattern":
      if (!form.pattern.trim()) err.pattern = "pattern";
      if (!whole(form.minOccurrences, 1)) err.count = "whole";
      break;
    case "synthetic_check":
      if (!form.checkId.trim()) err.check = "check";
      if (!whole(form.consecutiveFailures, 1)) err.count = "whole";
      break;
    case "composite":
      if (form.subRuleIds.length === 0) err.rules = "rules";
      break;
  }
  return err;
}

function condition(form: RuleFormState): Record<string, unknown> {
  switch (form.kind) {
    case "metric_threshold":
      return { metric: form.metric.trim(), operator: form.operator, value: form.thresholdValue, aggregate: form.aggregate };
    case "metric_deadman":
      return { metric: form.metric.trim(), max_silence_seconds: form.maxSilenceSeconds };
    case "heartbeat_stale":
      return { max_stale_seconds: form.maxStaleSeconds };
    case "log_pattern":
      return { pattern: form.pattern.trim(), min_occurrences: form.minOccurrences };
    case "synthetic_check":
      return { check_id: form.checkId.trim(), consecutive_failures: form.consecutiveFailures };
    case "composite":
      return { rule_ids: form.subRuleIds, operator: form.compositeOperator };
  }
}

/** The POST /api/alerts/rules body for a new, active rule. */
export function rulePayload(form: RuleFormState): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    name: form.name.trim(),
    description: form.description.trim(),
    kind: form.kind,
    severity: form.severity,
    status: "active",
    target_scope: form.targetType,
    window_seconds: form.windowSeconds,
    cooldown_seconds: form.cooldownSeconds,
    reopen_after_seconds: form.reopenAfterSeconds,
    evaluation_interval_seconds: form.evaluationIntervalSeconds,
    condition: condition(form),
  };
  const labels = labelsFromRows(form.labels);
  if (Object.keys(labels).length > 0) payload.labels = labels;
  if (form.targetType === "asset") payload.targets = [{ asset_id: form.targetId.trim() }];
  else if (form.targetType === "group") payload.targets = [{ group_id: form.targetId.trim() }];
  return payload;
}

const CHANGEABLE = ["name", "description", "severity", "window_seconds", "cooldown_seconds", "reopen_after_seconds", "evaluation_interval_seconds"];
const sameJSON = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/**
 * The PATCH /api/alerts/rules/<id> body for a changed rule: only what differs from `before` (the form as it opened).
 * The hub keeps kind, scope and status, and replaces condition, labels and targets whole. So the condition keeps the
 * keys the form does not show, and targets go only when the target changed (a rule may have more than the first).
 */
export function ruleUpdatePayload(rule: AlertRule, before: RuleFormState, after: RuleFormState): Record<string, unknown> {
  const prev = rulePayload(before);
  const next = rulePayload(after);
  const changes: Record<string, unknown> = {};
  for (const key of CHANGEABLE) if (next[key] !== prev[key]) changes[key] = next[key];
  if (!sameJSON(next.condition, prev.condition)) {
    const kept: Record<string, unknown> = { ...(rule.condition ?? {}) };
    // "threshold" is the old name of "value"; the hub refuses a condition where the two disagree.
    if (rule.kind === "metric_threshold") delete kept.threshold;
    changes.condition = { ...kept, ...(next.condition as Record<string, unknown>) };
  }
  const labels = labelsFromRows(after.labels);
  if (!sameJSON(labels, labelsFromRows(before.labels))) changes.labels = labels;
  if (after.targetId.trim() !== before.targetId.trim() && next.targets) changes.targets = next.targets;
  return changes;
}
