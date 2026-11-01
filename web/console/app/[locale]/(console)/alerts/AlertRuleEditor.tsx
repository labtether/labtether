"use client";

import { useTranslations } from "next-intl";
import { useMemo, useRef, useState, type FormEvent } from "react";
import { Button } from "../../../components/ui/Button";
import type { AlertRule, AlertRuleTemplate, Asset, Group } from "../../../console/models";
import { AlertRuleKindFields } from "./AlertRuleKindFields";
import { LabelRows } from "./LabelRows";
import { Field, invalid, numberValue, shownNumber } from "./ruleFields";
import { formStateFromTemplate, ruleFormErrors, type RuleFormErrors, type RuleFormState } from "./alertRuleFormState";
import { RULE_KINDS, SEVERITIES, TARGET_TYPES, type RuleKind, type RuleSeverity, type TargetType } from "./alertsPageTypes";

type Props = {
  initial: RuleFormState;
  /** The rule being changed; without it the editor adds a new rule. */
  editing?: AlertRule;
  /** What the rule applies to, shown fixed when the form cannot hold it as one device or group. */
  fixedTarget?: string;
  templates: AlertRuleTemplate[];
  rules: AlertRule[];
  assets: Asset[];
  groups: Group[];
  onCancel: () => void;
  /** `before` is the form as it opened, so a change sends only what differs. */
  onSubmit: (form: RuleFormState, before: RuleFormState) => Promise<void>;
};

/**
 * The inline rule editor (alerts-rule-form in the mockup): where to start, name, kind, severity, what it applies to,
 * the kind's own fields, timing and a description. Changing a rule keeps its kind and scope: the hub cannot change
 * them. Errors show after the first Add or Save.
 */
export function AlertRuleEditor({ initial, editing, fixedTarget, templates, rules, assets, groups, onCancel, onSubmit }: Props) {
  const t = useTranslations("alerts.editor");
  const tKind = useTranslations("alerts.kinds");
  const tSev = useTranslations("alerts.severity");
  const [before] = useState<RuleFormState>(initial);
  const [form, setForm] = useState<RuleFormState>(initial);
  const [presetId, setPresetId] = useState("");
  const [tried, setTried] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const formRef = useRef<HTMLFormElement>(null);

  const sortedTemplates = useMemo(() => [...templates].sort((a, b) =>
    Number(b.metadata?.category === "starter") - Number(a.metadata?.category === "starter") || a.name.localeCompare(b.name)), [templates]);
  const preset = sortedTemplates.find((tpl) => tpl.id === presetId) ?? null;
  const ruleIds = new Set(rules.map((r) => r.id));
  const otherNames = rules.filter((r) => r.id !== editing?.id).map((r) => r.name);
  const clean: RuleFormState = { ...form, subRuleIds: form.subRuleIds.filter((id) => ruleIds.has(id)) };
  const check = (f: RuleFormState): RuleFormErrors => {
    const err = ruleFormErrors(f, otherNames);
    if (fixedTarget) delete err.target;
    return err;
  };
  const errors: RuleFormErrors = tried ? check(clean) : {};
  const msg = (key?: string) => (key ? t(`errors.${key}`) : undefined);
  const set = <K extends keyof RuleFormState>(key: K, value: RuleFormState[K]) => setForm((f) => ({ ...f, [key]: value }));

  async function submit(event: FormEvent) {
    event.preventDefault();
    setFailure(null);
    if (Object.keys(check(clean)).length > 0) {
      setTried(true);
      requestAnimationFrame(() => formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus());
      return;
    }
    setSubmitting(true);
    try {
      await onSubmit(clean, before);
    } catch (err) {
      setFailure(err instanceof Error && err.message ? err.message : t(editing ? "errors.update" : "errors.save"));
      setSubmitting(false);
    }
  }

  const timing: Array<[keyof RuleFormState & ("windowSeconds" | "evaluationIntervalSeconds" | "cooldownSeconds" | "reopenAfterSeconds"), keyof RuleFormErrors, string]> = [
    ["windowSeconds", "window", "window"],
    ["evaluationIntervalSeconds", "interval", "interval"],
    ["cooldownSeconds", "cooldown", "cooldown"],
    ["reopenAfterSeconds", "reopen", "reopen"],
  ];

  return (
    <form ref={formRef} className="al-well al-ed" aria-label={editing ? t("editTitle", { name: editing.name }) : t("title")} onSubmit={(e) => void submit(e)} noValidate>
      <div className="al-grid">
        {!editing && sortedTemplates.length > 0 ? (
          <Field id="rf-from" label={t("from")} wide>
            <select
              id="rf-from"
              className="al-in"
              value={presetId}
              onChange={(e) => {
                const next = sortedTemplates.find((tpl) => tpl.id === e.target.value);
                setPresetId(e.target.value);
                if (next) setForm(formStateFromTemplate(next));
              }}
            >
              <option value="">{t("blank")}</option>
              {sortedTemplates.map((tpl) => <option key={tpl.id} value={tpl.id}>{tpl.name}</option>)}
            </select>
            {preset?.description ? <p className="al-hint">{preset.description}</p> : null}
          </Field>
        ) : null}

        <Field id="rf-name" label={t("name")} error={msg(errors.name)}>
          <input id="rf-name" className="al-in" value={form.name} onChange={(e) => set("name", e.target.value)} placeholder={t("namePlaceholder")} autoComplete="off" autoFocus {...invalid("rf-name", errors.name)} />
        </Field>
        <Field id="rf-kind" label={t("kind")}>
          <select id="rf-kind" className="al-in" value={form.kind} disabled={!!editing} onChange={(e) => set("kind", e.target.value as RuleKind)}>
            {RULE_KINDS.map((k) => <option key={k} value={k}>{tKind(k)}</option>)}
          </select>
        </Field>
        <Field id="rf-sev" label={t("severity")}>
          <select id="rf-sev" className="al-in" value={form.severity} onChange={(e) => set("severity", e.target.value as RuleSeverity)}>
            {SEVERITIES.map((s) => <option key={s} value={s}>{tSev(s)}</option>)}
          </select>
        </Field>
        <Field id="rf-scope" label={t("appliesTo")}>
          <select id="rf-scope" className="al-in" value={form.targetType} disabled={!!editing} onChange={(e) => setForm((f) => ({ ...f, targetType: e.target.value as TargetType, targetId: "" }))}>
            {TARGET_TYPES.map((k) => <option key={k} value={k}>{t(`targets.${k}`)}</option>)}
          </select>
        </Field>
        {form.targetType !== "global" ? (
          <Field id="rf-target" label={form.targetType === "asset" ? t("device") : t("group")} error={msg(errors.target)} wide>
            {fixedTarget ? (
              <input id="rf-target" className="al-in" value={fixedTarget} disabled />
            ) : (form.targetType === "asset" ? assets.length : groups.length) > 0 ? (
              <select id="rf-target" className="al-in" value={form.targetId} onChange={(e) => set("targetId", e.target.value)} {...invalid("rf-target", errors.target)}>
                <option value="">{form.targetType === "asset" ? t("pickDevice") : t("pickGroup")}</option>
                {form.targetType === "asset"
                  ? assets.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)
                  : groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
              </select>
            ) : (
              <input id="rf-target" className="al-in al-mono" value={form.targetId} onChange={(e) => set("targetId", e.target.value)} autoComplete="off" spellCheck={false} {...invalid("rf-target", errors.target)} />
            )}
          </Field>
        ) : null}

        <AlertRuleKindFields form={form} set={set} errors={errors} rules={rules} />

        <div className="al-wide al-timing-grid" role="group" aria-label={t("timing")}>
          {timing.map(([key, errKey, label]) => (
            <Field key={key} id={`rf-${errKey}`} label={t(label)} error={msg(errors[errKey])}>
              <span className="al-when">
                <input id={`rf-${errKey}`} type="number" inputMode="numeric" className="al-in al-num" value={shownNumber(form[key])} onChange={(e) => set(key, numberValue(e.target.value))} {...invalid(`rf-${errKey}`, errors[errKey])} />
                <span className="al-unit">{t("seconds")}</span>
              </span>
            </Field>
          ))}
        </div>

        <LabelRows legend={t("labels")} rows={form.labels} onRows={(rows) => set("labels", rows)} hint={t("labelsHint")} />

        <Field id="rf-desc" label={t("description")} wide>
          <input id="rf-desc" className="al-in" value={form.description} onChange={(e) => set("description", e.target.value)} autoComplete="off" />
        </Field>
      </div>

      {failure ? <p className="al-hint bad" role="alert">{failure}</p> : null}
      <div className="al-ed-acts">
        <span className="al-sp" />
        <Button type="button" variant="ghost" onClick={onCancel}>{t("cancel")}</Button>
        <Button type="submit" variant="primary" disabled={submitting}>
          {editing ? (submitting ? t("saving") : t("save")) : (submitting ? t("adding") : t("add"))}
        </Button>
      </div>
    </form>
  );
}
