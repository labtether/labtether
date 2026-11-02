"use client";

import { Plus } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { useState } from "react";
import { useRouter } from "../../../../i18n/navigation";
import { PageHeader } from "../../../components/PageHeader";
import { PageError } from "../../../components/ui/PageError";
import { Button } from "../../../components/ui/Button";
import type { AlertInstance, AlertSilence } from "../../../console/models";
import { useToast } from "../../../contexts/ToastContext";
import { useAlerts } from "../../../hooks/useAlerts";
import { AlertDeliveryLogTab } from "./AlertDeliveryLogTab";
import { AlertInbox } from "./AlertInbox";
import { AlertInboxToolbar } from "./AlertInboxToolbar";
import { AlertRulesTab, type RuleOpen } from "./AlertRulesTab";
import { AlertSilencesTab } from "./AlertSilencesTab";
import { AlertTabs } from "./AlertTabs";
import type { AlertActions } from "./AlertDetail";
import { shortTime } from "./alertsModel";
import { DEFAULT_FILTER, durationPresets, parseAlertTab, type DurationPreset, type InboxFilter } from "./alertsPageTypes";

/**
 * Alerts: the open alerts beside the chosen one, Rules (and the rule editor), Silences and the delivery log, one
 * tab each (?tab=…). Issues is its own page in the same tab row. ?alert=<id> opens on that alert.
 */
export default function AlertsPage() {
  const t = useTranslations("alerts");
  const tCommon = useTranslations("common");
  const locale = useLocale();
  const params = useSearchParams();
  const router = useRouter();
  const { addToast } = useToast();
  const tab = parseAlertTab(params.get("tab"));
  const data = useAlerts();
  const [selectedAlert, setSelectedAlert] = useState<string | null>(params.get("alert"));
  const [ruleOpen, setRuleOpen] = useState<RuleOpen>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [filter, setFilter] = useState<InboxFilter>(DEFAULT_FILTER);

  const openRules = (open: RuleOpen) => {
    setRuleOpen(open);
    if (tab !== "rules") router.push("/alerts?tab=rules");
  };
  const showAlert = (id: string) => {
    setSelectedAlert(id);
    router.push("/alerts");
  };

  async function run(alert: AlertInstance, work: () => Promise<void>, done?: string) {
    setBusy(alert.id);
    try {
      await work();
      if (done) addToast("success", done);
    } catch (err) {
      addToast("error", err instanceof Error && err.message ? err.message : t("errors.action"));
    } finally {
      setBusy(null);
    }
  }

  const actions: AlertActions = {
    busy,
    acknowledge: (alert) => run(alert, () => data.ackAlert(alert.id)),
    resolve: (alert) => run(alert, () => data.resolveAlert(alert.id)),
    silence: (alert, preset: DurationPreset) => {
      const hours = durationPresets.find((p) => p.id === preset)?.hours ?? 1;
      const start = new Date();
      const end = new Date(start.getTime() + hours * 3_600_000);
      return run(alert, () => data.createSilence({
        matchers: { ...(alert.labels ?? {}) },
        starts_at: start.toISOString(),
        ends_at: end.toISOString(),
        reason: alert.labels?.rule_name || data.rules.find((r) => r.id === alert.rule_id)?.name || undefined,
      }), t("detail.silenced", { time: shortTime(end.toISOString(), locale, Date.now()) }));
    },
    endSilence: async (silence: AlertSilence) => {
      setBusy(silence.id);
      try {
        await data.deleteSilence(silence.id);
        addToast("success", t("detail.silenceEnded"));
      } catch (err) {
        addToast("error", err instanceof Error && err.message ? err.message : t("errors.action"));
      } finally {
        setBusy(null);
      }
    },
    openRule: (ruleId) => openRules({ kind: "rule", id: ruleId }),
  };

  const newRuleMain = tab === "rules" && ruleOpen?.kind !== "new";
  // Nothing has loaded and the last try failed: the error, not "No alerts".
  const blank = data.loadFailed && data.instances.length === 0 && data.rules.length === 0;
  return (
    <>
      <PageHeader
        title={t("title")}
        className="al-ph"
        action={(
          <Button variant={newRuleMain ? "primary" : "secondary"} onClick={() => openRules({ kind: "new", seq: Date.now() })}>
            <Plus aria-hidden="true" size={14} strokeWidth={2} />
            {t("newRule")}
          </Button>
        )}
      />
      <AlertTabs current={tab} tools={tab === "alerts" && data.instances.length > 0 ? <AlertInboxToolbar filter={filter} onFilter={setFilter} /> : undefined} />
      {data.loadFailed && !blank ? <p role="alert" className="al-error">{t("errors.load")}</p> : null}
      {data.error && !data.loadFailed ? <p role="alert" className="al-error">{data.error}</p> : null}
      {blank && tab !== "delivery" ? <PageError title={t("errors.load")} retryLabel={tCommon("retry")} onRetry={() => void data.refresh()} /> : null}

      {!blank && tab === "alerts" ? (
        <AlertInbox
          loading={data.loading}
          instances={data.instances}
          rules={data.rules}
          silences={data.silences}
          selectedId={selectedAlert}
          onSelect={setSelectedAlert}
          filter={filter}
          onFilter={setFilter}
          actions={actions}
        />
      ) : null}

      {!blank && tab === "rules" ? (
        <AlertRulesTab
          loading={data.loading}
          rules={data.rules}
          templates={data.templates}
          instances={data.instances}
          open={ruleOpen}
          onOpen={setRuleOpen}
          createRule={data.createRule}
          updateRule={data.updateRule}
          deleteRule={data.deleteRule}
        />
      ) : null}

      {!blank && tab === "silences" ? (
        <AlertSilencesTab
          silences={data.silences}
          createSilence={data.createSilence}
          deleteSilence={data.deleteSilence}
        />
      ) : null}

      {tab === "delivery" ? <AlertDeliveryLogTab instances={data.instances} rules={data.rules} onShowAlert={showAlert} /> : null}
    </>
  );
}
