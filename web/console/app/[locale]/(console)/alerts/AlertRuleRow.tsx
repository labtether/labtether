"use client";

import { useLocale, useTranslations } from "next-intl";
import { useEffect, useRef, type ReactNode } from "react";
import { Button } from "../../../components/ui/Button";
import { Switch } from "../../../components/ui/Switch";
import type { AlertRule } from "../../../console/models";
import { useDuration } from "./alertBits";
import { fullTime } from "./alertsModel";
import { ruleCondition, type RuleCondition } from "./rulesModel";

/** "cpu_used_percent > 90 (average)", "No heartbeat for 5 min" …: what a rule watches, in words. */
export function useRuleSummary() {
  const t = useTranslations("alerts.rules.summary");
  const tAgg = useTranslations("alerts.editor.aggregates");
  const duration = useDuration();
  return (c: RuleCondition | null) => {
    if (!c) return "";
    switch (c.kind) {
      case "metric_threshold": return t("metric", { metric: c.metric, op: c.op, value: c.value, aggregate: tAgg.has(c.aggregate) ? tAgg(c.aggregate) : c.aggregate });
      case "metric_deadman": return t("deadman", { metric: c.metric, time: duration(c.seconds) });
      case "heartbeat_stale": return t("heartbeat", { time: duration(c.seconds) });
      case "log_pattern": return t("log", { pattern: c.pattern, count: c.count });
      case "synthetic_check": return t("check", { count: c.count });
      case "composite": return t("composite", { join: c.join, count: c.count });
    }
  };
}

type Props = {
  rule: AlertRule;
  scope: string;
  open: boolean;
  /** The switch: on, or the state it is being switched to. */
  on: boolean;
  switching: boolean;
  /** The editor, shown in place of the settings while the rule is being changed. */
  editor?: ReactNode;
  openAlerts: number;
  onToggle: () => void;
  onSwitch: (on: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
};

/** One rule: its name and what it watches, severity, what it applies to and its switch. Open, it shows its settings. */
export function AlertRuleRow({ rule, scope, open, on, switching, editor, openAlerts, onToggle, onSwitch, onEdit, onDelete }: Props) {
  const t = useTranslations("alerts.rules");
  const tKind = useTranslations("alerts.kinds");
  const tSev = useTranslations("alerts.severity");
  const locale = useLocale();
  const summary = useRuleSummary();
  const duration = useDuration();
  const rowRef = useRef<HTMLDivElement>(null);
  const watch = summary(ruleCondition(rule));
  const extra = rule as AlertRule & { evaluation_interval_seconds?: number; reopen_after_seconds?: number };
  const labels = Object.entries(rule.labels ?? {});

  useEffect(() => {
    if (open) rowRef.current?.scrollIntoView({ block: "nearest" });
  }, [open]);

  const kv = (label: string, value: string) => (value ? (
    <div className="al-kv"><dt>{label}</dt><dd><span>{value}</span></dd></div>
  ) : null);

  return (
    <>
      <div ref={rowRef} className={`al-rrow al-rule${open ? " on" : ""}${on ? "" : " off"}`} onClick={onToggle}>
        <span className="al-rln">
          <button type="button" className="al-rname" aria-expanded={open} onClick={(e) => { e.stopPropagation(); onToggle(); }}>{rule.name}</button>
          <span className="al-rsum">{[tKind.has(rule.kind) ? tKind(rule.kind) : rule.kind, watch].filter(Boolean).join(" · ")}</span>
        </span>
        <span className="al-rsev">{tSev.has(rule.severity) ? tSev(rule.severity) : rule.severity}</span>
        <span className="al-rscope">{scope}</span>
        {/* The span keeps a click on the switch, enabled or not, from opening the row. */}
        <span className="al-r al-rstate" onClick={(e) => e.stopPropagation()}>
          <Switch size="sm" checked={on} disabled={switching} onChange={onSwitch} aria-label={t("switch", { name: rule.name })} />
        </span>
      </div>
      {open && editor ? editor : open ? (
        <div className="al-well" role="group" aria-label={rule.name}>
          <dl className="al-facts">
            {kv(t("facts.kind"), tKind.has(rule.kind) ? tKind(rule.kind) : rule.kind)}
            {kv(t("facts.condition"), watch)}
            {kv(t("facts.appliesTo"), scope)}
            {kv(t("facts.window"), duration(rule.window_seconds))}
            {typeof extra.evaluation_interval_seconds === "number" ? kv(t("facts.interval"), duration(extra.evaluation_interval_seconds)) : null}
            {kv(t("facts.cooldown"), duration(rule.cooldown_seconds))}
            {typeof extra.reopen_after_seconds === "number" ? kv(t("facts.reopen"), duration(extra.reopen_after_seconds)) : null}
            {kv(t("facts.lastChecked"), rule.last_evaluated_at ? fullTime(rule.last_evaluated_at, locale) : t("facts.never"))}
            {kv(t("facts.openAlerts"), String(openAlerts))}
            {kv(t("facts.description"), rule.description ?? "")}
            {labels.length ? kv(t("facts.labels"), labels.map(([k, v]) => `${k}=${v}`).join(", ")) : null}
          </dl>
          <div className="al-ed-acts">
            <Button variant="ghost" className="al-danger" onClick={onDelete}>{t("deleteAsk")}</Button>
            <span className="al-sp" />
            <Button onClick={onEdit}>{t("edit")}</Button>
          </div>
        </div>
      ) : null}
    </>
  );
}
