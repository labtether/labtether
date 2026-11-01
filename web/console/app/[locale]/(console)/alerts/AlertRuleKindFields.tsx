"use client";

import { Check } from "lucide-react";
import { useTranslations } from "next-intl";
import type { AlertRule } from "../../../console/models";
import { Field, invalid, numberValue, shownNumber } from "./ruleFields";
import type { RuleFormErrors, RuleFormState } from "./alertRuleFormState";
import { AGGREGATES, OPERATORS } from "./alertsPageTypes";

type Props = {
  form: RuleFormState;
  set: <K extends keyof RuleFormState>(key: K, value: RuleFormState[K]) => void;
  errors: RuleFormErrors;
  rules: AlertRule[];
};

/** The fields a rule's kind has: a metric and threshold, silence or heartbeat time, a log pattern, a check, or rules to combine. */
export function AlertRuleKindFields({ form, set, errors, rules }: Props) {
  const t = useTranslations("alerts.editor");
  const msg = (key?: string) => (key ? t(`errors.${key}`) : undefined);

  const seconds = (id: string, label: string, key: "maxSilenceSeconds" | "maxStaleSeconds") => (
    <Field id={id} label={label} error={msg(errors.seconds)}>
      <span className="al-when">
        <input id={id} type="number" inputMode="numeric" className="al-in al-num" value={shownNumber(form[key])} onChange={(e) => set(key, numberValue(e.target.value))} {...invalid(id, errors.seconds)} />
        <span className="al-unit">{t("seconds")}</span>
      </span>
    </Field>
  );
  const count = (id: string, label: string, unit: string, key: "minOccurrences" | "consecutiveFailures") => (
    <Field id={id} label={label} error={msg(errors.count)}>
      <span className="al-when">
        <input id={id} type="number" inputMode="numeric" className="al-in al-num" value={shownNumber(form[key])} onChange={(e) => set(key, numberValue(e.target.value))} {...invalid(id, errors.count)} />
        <span className="al-unit">{unit}</span>
      </span>
    </Field>
  );
  const metric = (
    <Field id="rf-metric" label={t("metric")} error={msg(errors.metric)}>
      <input id="rf-metric" className="al-in al-mono" value={form.metric} onChange={(e) => set("metric", e.target.value)} placeholder="cpu_used_percent" autoComplete="off" spellCheck={false} {...invalid("rf-metric", errors.metric)} />
    </Field>
  );

  switch (form.kind) {
    case "metric_threshold":
      return (
        <>
          {metric}
          <Field id="rf-thr" label={t("when")} error={msg(errors.threshold)}>
            <span className="al-when">
              <select className="al-in" aria-label={t("aggregate")} value={form.aggregate} onChange={(e) => set("aggregate", e.target.value)}>
                {AGGREGATES.map((a) => <option key={a} value={a}>{t(`aggregates.${a}`)}</option>)}
              </select>
              <select className="al-in al-op" aria-label={t("operator")} value={form.operator} onChange={(e) => set("operator", e.target.value)}>
                {OPERATORS.map((op) => <option key={op} value={op}>{op}</option>)}
              </select>
              <input id="rf-thr" type="number" className="al-in al-num" aria-label={t("threshold")} value={shownNumber(form.thresholdValue)} onChange={(e) => set("thresholdValue", numberValue(e.target.value))} {...invalid("rf-thr", errors.threshold)} />
            </span>
          </Field>
        </>
      );
    case "metric_deadman":
      return <>{metric}{seconds("rf-silent", t("silentFor"), "maxSilenceSeconds")}</>;
    case "heartbeat_stale":
      return seconds("rf-stale", t("missedFor"), "maxStaleSeconds");
    case "log_pattern":
      return (
        <>
          <Field id="rf-pattern" label={t("pattern")} error={msg(errors.pattern)}>
            <input id="rf-pattern" className="al-in al-mono" value={form.pattern} onChange={(e) => set("pattern", e.target.value)} placeholder="ERROR|FATAL|panic" autoComplete="off" spellCheck={false} {...invalid("rf-pattern", errors.pattern)} />
          </Field>
          {count("rf-count", t("count"), t("timesInWindow"), "minOccurrences")}
        </>
      );
    case "synthetic_check":
      return (
        <>
          <Field id="rf-check" label={t("check")} error={msg(errors.check)}>
            <input id="rf-check" className="al-in al-mono" value={form.checkId} onChange={(e) => set("checkId", e.target.value)} autoComplete="off" spellCheck={false} {...invalid("rf-check", errors.check)} />
          </Field>
          {count("rf-fails", t("failures"), t("inARow"), "consecutiveFailures")}
        </>
      );
    case "composite": {
      const others = rules.filter((r) => r.kind !== "composite");
      const toggle = (id: string) => set("subRuleIds", form.subRuleIds.includes(id) ? form.subRuleIds.filter((x) => x !== id) : [...form.subRuleIds, id]);
      return (
        <>
          <Field id="rf-join" label={t("join")}>
            <select id="rf-join" className="al-in" value={form.compositeOperator} onChange={(e) => set("compositeOperator", e.target.value === "or" ? "or" : "and")}>
              <option value="and">{t("joins.and")}</option>
              <option value="or">{t("joins.or")}</option>
            </select>
          </Field>
          <fieldset className="al-wide al-timing">
            <legend className="al-lbl">{t("rules")}</legend>
            {others.length === 0 ? <p className="al-hint">{t("noOtherRules")}</p> : (
              <div className="al-parts">
                {others.map((r) => {
                  const on = form.subRuleIds.includes(r.id);
                  return (
                    <button key={r.id} type="button" className="al-part" role="checkbox" aria-checked={on} onClick={() => toggle(r.id)}>
                      <span className={`al-box${on ? " on" : ""}`} aria-hidden="true">{on ? <Check strokeWidth={3} /> : null}</span>
                      <span className="al-trunc">{r.name}</span>
                    </button>
                  );
                })}
              </div>
            )}
            {errors.rules ? <p className="al-hint bad" id="rf-parts-err">{msg(errors.rules)}</p> : null}
          </fieldset>
        </>
      );
    }
  }
}
