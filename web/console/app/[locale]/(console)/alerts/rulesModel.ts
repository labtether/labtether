import type { AlertRule, Asset, Group } from "../../../console/models";
import { sevRank } from "./alertsModel";

/** What a rule watches, from its condition: the parts its kind has. null when the condition lacks them. */
export type RuleCondition =
  | { kind: "metric_threshold"; metric: string; op: string; value: string; aggregate: string }
  | { kind: "metric_deadman"; metric: string; seconds: number }
  | { kind: "heartbeat_stale"; seconds: number }
  | { kind: "log_pattern"; pattern: string; count: number }
  | { kind: "synthetic_check"; count: number }
  | { kind: "composite"; join: "and" | "or"; count: number };

const str = (v: unknown) => (typeof v === "string" ? v.trim() : typeof v === "number" ? String(v) : "");
const num = (v: unknown, fallback: number) => (typeof v === "number" && Number.isFinite(v) ? v : fallback);

export function ruleCondition(rule: Pick<AlertRule, "kind" | "condition">): RuleCondition | null {
  const c = rule.condition ?? {};
  switch (rule.kind) {
    case "metric_threshold": {
      const metric = str(c.metric);
      const value = str(c.value) || str(c.threshold);
      return metric && value ? { kind: rule.kind, metric, op: str(c.operator) || ">", value, aggregate: str(c.aggregate) || "avg" } : null;
    }
    case "metric_deadman": {
      const metric = str(c.metric);
      return metric ? { kind: rule.kind, metric, seconds: num(c.max_silence_seconds, 300) } : null;
    }
    case "heartbeat_stale":
      return { kind: rule.kind, seconds: num(c.max_stale_seconds, 300) };
    case "log_pattern": {
      const pattern = str(c.pattern);
      return pattern ? { kind: rule.kind, pattern, count: num(c.min_occurrences, 1) } : null;
    }
    case "synthetic_check":
      return { kind: rule.kind, count: num(c.consecutive_failures, 1) };
    case "composite":
      return { kind: rule.kind, join: str(c.operator) === "or" ? "or" : "and", count: Array.isArray(c.rule_ids) ? c.rule_ids.length : 0 };
    default:
      return null;
  }
}

/** What a rule applies to: everything, or the names of its devices and groups (an ID only when the name is gone). */
export function ruleTargets(rule: AlertRule, assetsByID: Map<string, Asset>, groupsByID: Map<string, Group>): { all: true } | { names: string[] } {
  if (rule.target_scope === "global") return { all: true };
  const names = (rule.targets ?? []).map((target) => {
    if (target.asset_id) return assetsByID.get(target.asset_id)?.name ?? target.asset_id;
    if (target.group_id) return groupsByID.get(target.group_id)?.name ?? target.group_id;
    return "";
  }).filter(Boolean);
  return { names };
}

export type RuleShow = "all" | "on" | "off";
export const RULE_SHOWS: RuleShow[] = ["all", "on", "off"];
export type RuleSortKey = "name" | "severity" | "scope";
export type RuleSort = { key: RuleSortKey; dir: 1 | -1 };

export const ruleIsOn = (rule: AlertRule) => rule.status !== "paused";
export const ruleShown = (rule: AlertRule, show: RuleShow) => show === "all" || ruleIsOn(rule) === (show === "on");

/** The rules the list shows: those the On/Off filter lets through, in the chosen column's order (name breaks ties). */
export function visibleRules(rules: AlertRule[], show: RuleShow, sort: RuleSort, scopeLabel: (rule: AlertRule) => string): AlertRule[] {
  const byName = (a: AlertRule, b: AlertRule) => a.name.localeCompare(b.name);
  const by: Record<RuleSortKey, (a: AlertRule, b: AlertRule) => number> = {
    name: byName,
    severity: (a, b) => sevRank(a.severity) - sevRank(b.severity),
    scope: (a, b) => scopeLabel(a).localeCompare(scopeLabel(b)),
  };
  return rules.filter((rule) => ruleShown(rule, show)).sort((a, b) => sort.dir * by[sort.key](a, b) || byName(a, b));
}
