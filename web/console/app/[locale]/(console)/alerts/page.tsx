"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { useAlerts } from "../../../hooks/useAlerts";
import { AlertsInboxTab } from "./AlertsInboxTab";
import { AlertRulesTab } from "./AlertRulesTab";
import { AlertDeliveryLogTab } from "./AlertDeliveryLogTab";
import { AlertSilencesTab } from "./AlertSilencesTab";
import type {
  AlertSeverityFilter,
  AlertStateFilter,
  AlertTab,
} from "./alertsPageTypes";

export default function AlertsPage() {
  const t = useTranslations('alerts');
  const {
    instances,
    rules,
    templates,
    silences,
    loading,
    ackAlert,
    resolveAlert,
    createSilence,
    deleteSilence,
    createRule,
    deleteRule,
  } = useAlerts();

  const [activeTab, setActiveTab] = useState<AlertTab>("inbox");
  const [severityFilter, setSeverityFilter] = useState<AlertSeverityFilter>("all");
  const [stateFilter, setStateFilter] = useState<AlertStateFilter>("all");
  const [highlightedRuleId, setHighlightedRuleId] = useState<string | null>(null);

  const firingCount = instances.filter((instance) => instance.status === "firing").length;

  function handleGoToRule(ruleId: string) {
    setHighlightedRuleId(ruleId);
    setActiveTab("rules");
  }

  return (
    <>
      <PageHeader
        title={t('title')}
        subtitle={t('subtitle.label', { activeInfo: firingCount > 0 ? t('subtitle.active', { count: firingCount }) : '' })}
      />

      <Card className="mb-4 flex items-center justify-between">
        <SegmentedTabs
          value={activeTab}
          options={(["inbox", "rules", "silences", "delivery"] as AlertTab[]).map((tab) => ({
            id: tab,
            label: (
              <span className="inline-flex items-center gap-1.5">
                {t(`tabs.${tab}`)}
                {tab === "inbox" && firingCount > 0 ? (
                  <span className="rounded-lg border border-[var(--control-border)] px-1.5 py-0.5 text-[10px] text-[var(--control-fg-muted)]">
                    {firingCount}
                  </span>
                ) : null}
              </span>
            ),
          }))}
          onChange={setActiveTab}
        />
      </Card>

      {activeTab === "inbox" ? (
        <AlertsInboxTab
          loading={loading}
          instances={instances}
          rules={rules}
          severityFilter={severityFilter}
          stateFilter={stateFilter}
          onSeverityFilterChange={setSeverityFilter}
          onStateFilterChange={setStateFilter}
          onAck={(id) => void ackAlert(id)}
          onResolve={(id) => void resolveAlert(id)}
          onGoToRule={handleGoToRule}
        />
      ) : null}

      {activeTab === "rules" ? (
        <AlertRulesTab
          rules={rules}
          templates={templates}
          highlightedRuleId={highlightedRuleId}
          onHighlightedRuleIdChange={setHighlightedRuleId}
          createRule={createRule}
          deleteRule={deleteRule}
        />
      ) : null}

      {activeTab === "silences" ? (
        <AlertSilencesTab
          silences={silences}
          createSilence={createSilence}
          deleteSilence={deleteSilence}
        />
      ) : null}

      {activeTab === "delivery" ? (
        <AlertDeliveryLogTab />
      ) : null}
    </>
  );
}
