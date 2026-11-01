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
import { createRuleFormState, formStateFromRule } from "./alertRuleFormState";
import { RULE_SHOWS, ruleShown, ruleTargets, visibleRules, type RuleShow, type RuleSort, type RuleSortKey } from "./rulesModel";

/** Which rule is open: the editor for a new one (seq tells one "New rule" click from the next), or a rule's facts. */
export type RuleOpen = { kind: "new"; seq: number; from: AlertRule | null } | { kind: "rule"; id: string } | null;

type Props = {
  loading: boolean;
  rules: AlertRule[];
  templates: AlertRuleTemplate[];
  instances: AlertInstance[];
  open: RuleOpen;
  onOpen: (open: RuleOpen) => void;
  createRule: (rule: Record<string, unknown>) => Promise<void>;
  deleteRule: (id: string) => Promise<void>;
};

/** Rules as a 2.0 list: On/Off filter, sortable columns; a row opens its facts, New rule opens the editor on top. */
export function AlertRulesTab({ loading, rules, templates, instances, open, onOpen, createRule, deleteRule }: Props) {
  const t = useTranslations("alerts.rules");
  const { addToast } = useToast();
  const status = useFastStatus();
  const slow = useSlowStatus();
  const [show, setShow] = useState<RuleShow>("all");
  const [sort, setSort] = useState<RuleSort>({ key: "name", dir: 1 });
  const [deleting, setDeleting] = useState<AlertRule | null>(null);
  const [deleteBusy, setDeleteBusy] = useState(false);

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
      if (open?.kind === "rule" && open.id === deleting.id) onOpen(null);
      setDeleting(null);
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("deleteFailed"));
    } finally {
      setDeleteBusy(false);
    }
  }

  const editor = open?.kind === "new" ? (
    <AlertRuleEditor
      key={open.seq}
      initial={open.from ? formStateFromRule(open.from) : createRuleFormState()}
      templates={templates}
      rules={rules}
      assets={assets}
      groups={groups}
      onCancel={() => onOpen(null)}
      onSubmit={async (payload, name) => {
        await createRule(payload);
        addToast("success", t("added", { name }));
        onOpen(null);
      }}
    />
  ) : null;

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
              open={open?.kind === "rule" && open.id === rule.id}
              openAlerts={instances.filter((a) => a.rule_id === rule.id && a.status !== "resolved").length}
              onToggle={() => onOpen(open?.kind === "rule" && open.id === rule.id ? null : { kind: "rule", id: rule.id })}
              onCopy={() => onOpen({ kind: "new", seq: Date.now(), from: rule })}
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
