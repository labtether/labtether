"use client";

import type { AlertRuleTemplate } from "../../../console/models";
import type { RuleKind, RuleSeverity, TargetType } from "./alertsPageTypes";

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
  subRuleIds: string;
  compositeOperator: "and" | "or";
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
  subRuleIds: "",
  compositeOperator: "and",
};

export function createRuleFormState(overrides: Partial<RuleFormState> = {}): RuleFormState {
  return {
    ...defaultRuleFormState,
    ...overrides,
  };
}

export function numberFromUnknown(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

export function stringFromUnknown(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

export function formStateFromTemplate(template: AlertRuleTemplate): RuleFormState {
  const condition = template.condition ?? {};
  return createRuleFormState({
    name: template.name,
    description: template.description,
    kind: template.kind,
    severity: template.severity,
    targetType: template.target_scope,
    windowSeconds: template.window_seconds,
    cooldownSeconds: template.cooldown_seconds,
    reopenAfterSeconds: template.reopen_after_seconds,
    evaluationIntervalSeconds: template.evaluation_interval_seconds,
    metric: stringFromUnknown(condition.metric),
    operator: stringFromUnknown(condition.operator, ">"),
    thresholdValue: numberFromUnknown(condition.value, 0),
    aggregate: stringFromUnknown(condition.aggregate, "avg"),
    maxSilenceSeconds: numberFromUnknown(condition.max_silence_seconds, defaultRuleFormState.maxSilenceSeconds),
    maxStaleSeconds: numberFromUnknown(condition.max_stale_seconds, defaultRuleFormState.maxStaleSeconds),
    pattern: stringFromUnknown(condition.pattern),
    minOccurrences: numberFromUnknown(condition.min_occurrences, defaultRuleFormState.minOccurrences),
    checkId: stringFromUnknown(condition.check_id),
    consecutiveFailures: numberFromUnknown(condition.consecutive_failures, defaultRuleFormState.consecutiveFailures),
    subRuleIds: Array.isArray(condition.rule_ids)
      ? condition.rule_ids.map((value) => String(value)).join(", ")
      : "",
    compositeOperator: stringFromUnknown(condition.operator) === "or" ? "or" : "and",
  });
}
