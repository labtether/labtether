"use client";

import { ArrowDown, ArrowUp } from "lucide-react";
import { useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import type { AlertInstance, AlertRule, AlertRuleTemplate } from "../../../console/models";
import { useFastStatus, useSlowStatus } from "../../../contexts/StatusContext";
import { useToast } from "../../../contexts/ToastContext";
import { AlertRuleEditor } from "./AlertRuleEditor";
import { AlertRuleRow } from "./AlertRuleRow";
import { createRuleFormState, formStateFromRule, rulePayload, ruleUpdatePayload } from "./alertRuleFormState";
import { RULE_SHOWS, ruleIsOn, ruleShown, ruleTargetFixed, ruleTargets, visibleRules, type RuleShow, type RuleSort, type RuleSortKey } from "./rulesModel";

/**
 * Which rule is open: the editor for a new one (seq tells one "New rule" click from the next), a rule's facts, or
 * the editor for changing a rule.
 */
export type RuleOpen = { kind: "new"; seq: number } | { kind: "rule" | "edit"; id: string } | null;

type Props = {
  loading: boolean;
  rules: AlertRule[];
  templates: AlertRuleTemplate[];
  instances: AlertInstance[];
  open: RuleOpen;
  onOpen: (open: RuleOpen) => void;
  createRule: (rule: Record<string, unknown>) => Promise<void>;
  updateRule: (id: string, changes: Record<string, unknown>) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
};

/**
 * Rules as a 2.0 list: On/Off filter, sortable columns and a switch per rule; a row opens its facts and Edit, New
 * rule opens the editor on top.
 */
export function AlertRulesTab({ loading, rules, templates, instances, open, onOpen, createRule, updateRule, deleteRule }: Props) {
  const t = useTranslations("alerts.rules");
  const { addToast } = useToast();
  const status = useFastStatus();
  const slow = useSlowStatus();
  const [show, setShow] = useState<RuleShow>("all");
  const [sort, setSort] = useState<RuleSort>({ key: "name", dir: 1 });
  const [deleting, setDeleting] = useState<AlertRule | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);
  // Rule ID → the state its switch was moved to, while the hub takes the change.
  const [switching, setSwitching] = useState<Record<string, boolean>>({});

  const assets = useMemo(() => [...(status?.assets ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [status?.assets]);
  const groups = useMemo(() => [...(slow?.groups ?? [])].sort((a, b) => a.name.localeCompare(b.name)), [slow?.groups]);
  const assetsByID = useMemo(() => new Map(assets.map((a) => [a.id, a])), [assets]);
  const groupsByID = useMemo(() => new Map(groups.map((g) => [g.id, g])), [groups]);
  const scopeLabel = (rule: AlertRule) => {
    const target = ruleTargets(rule, assetsByID, groupsByID);
    return "all" in target ? t("everything") : target.names.join(", ") || t(`scope.${rule.target_scope}`);
  };
  const shown = visibleRules(rules, show, sort, scopeLabel);
  const alertCount = (rule: AlertRule) => instances.filter((a) => a.rule_id === rule.id).length;
  const isOpen = (id: string) => open !== null && open.kind !== "new" && open.id === id;

  const sortBy = (key: RuleSortKey) => setSort((s) => ({ key, dir: s.key === key ? (s.dir === 1 ? -1 : 1) : 1 }));
  const head = (key: RuleSortKey) => (
    <button type="button" className={`al-sort${sort.key === key ? " on" : ""}`} onClick={() => sortBy(key)}>
      {t(`cols.${key}`)}
      {sort.key === key ? (sort.dir === 1 ? <ArrowDown aria-hidden="true" /> : <ArrowUp aria-hidden="true" />) : null}
    </button>
  );

  async function confirmDelete() {
    if (!deleting) return;
    setDeleteBusy(true);
    try {
      await deleteRule(deleting.id);
      addToast("success", t("deleted", { name: deleting.name }));
      if (isOpen(deleting.id)) onOpen(null);
      setDeleting(null);
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("deleteFailed"));
    } finally {
      setDeleteBusy(false);
    }
  }

  async function switchRule(rule: AlertRule, on: boolean) {
    setSwitching((s) => ({ ...s, [rule.id]: on }));
    try {
      await updateRule(rule.id, { status: on ? "active" : "paused" });
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("switchFailed", { state: on ? "on" : "off", name: rule.name }));
    } finally {
      setSwitching((s) => {
        const next = { ...s };
        delete next[rule.id];
        return next;
      });
    }
  }

  // A new rule, or a change to `rule`; either way the editor closes on save.
  const editorFor = (rule: AlertRule | null, key: string) => (
    <AlertRuleEditor
      key={key}
      initial={rule ? formStateFromRule(rule) : createRuleFormState()}
      editing={rule ?? undefined}
      fixedTarget={rule && ruleTargetFixed(rule) ? scopeLabel(rule) : undefined}
      templates={templates}
      rules={rules}
      assets={assets}
      groups={groups}
      onCancel={() => onOpen(rule ? { kind: "rule", id: rule.id } : null)}
      onSubmit={async (form, before) => {
        const name = form.name.trim();
        if (rule) {
          await updateRule(rule.id, ruleUpdatePayload(rule, before, form));
          addToast("success", t("saved", { name }));
          onOpen({ kind: "rule", id: rule.id });
        } else {
          await createRule(rulePayload(form));
          addToast("success", t("added", { name }));
          onOpen(null);
        }
      }}
    />
  );
  const editor = open?.kind === "new" ? editorFor(null, String(open.seq)) : null;

  return (
    <>
      {rules.length > 0 ? (
        <div className="al-tools">
          <div className="al-seg" role="group" aria-label={t("showLabel")}>
            {RULE_SHOWS.map((key) => (
              <button key={key} type="button" className={show === key ? "on" : undefined} aria-pressed={show === key} onClick={() => setShow(key)}>
                {t(`show.${key}`)}
                <span className="al-n">{rules.filter((r) => ruleShown(r, key)).length}</span>
              </button>
            ))}
          </div>
        </div>
      ) : null}

      {rules.length === 0 && !editor ? (
        <div className="al-sheet al-empty">
          <b>{loading ? t("loading") : t("empty")}</b>
        </div>
      ) : (
        <div className="al-sheet al-rules" aria-label={t("listLabel")} role="group">
          <div className="al-rrow al-rcols">
            {head("name")}
            {head("severity")}
            {head("scope")}
            <span className="al-r">{t("cols.state")}</span>
          </div>
          {editor}
          {shown.map((rule) => (
            <AlertRuleRow
              key={rule.id}
              rule={rule}
              scope={scopeLabel(rule)}
              open={isOpen(rule.id)}
              on={switching[rule.id] ?? ruleIsOn(rule)}
              switching={rule.id in switching}
              editor={open?.kind === "edit" && open.id === rule.id ? editorFor(rule, `edit-${rule.id}`) : undefined}
              openAlerts={instances.filter((a) => a.rule_id === rule.id && a.status !== "resolved").length}
              onToggle={() => onOpen(isOpen(rule.id) ? null : { kind: "rule", id: rule.id })}
              onSwitch={(on) => void switchRule(rule, on)}
              onEdit={() => onOpen({ kind: "edit", id: rule.id })}
              onDelete={() => setDeleting(rule)}
            />
          ))}
          {rules.length > 0 && shown.length === 0 ? <p className="al-none">{t("emptyFiltered")}</p> : null}
          {show !== "all" ? (
            <div className="al-foot">
              <span>{t("shown", { shown: shown.length, total: rules.length })}</span>
              <button type="button" onClick={() => setShow("all")}>{t("clear")}</button>
            </div>
          ) : null}
        </div>
      )}

      <ConfirmDialog
        open={deleting !== null}
        onClose={() => (deleteBusy ? undefined : setDeleting(null))}
        onConfirm={() => void confirmDelete()}
        title={deleting ? t("deleteTitle", { name: deleting.name }) : ""}
        message={deleting ? t("deleteBody", { count: alertCount(deleting) }) : ""}
        confirmLabel={t("delete")}
        danger
        loading={deleteBusy}
      />
    </>
  );
}
