"use client";

import { useTranslations } from "next-intl";
import { ClipboardList, Play } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Button } from "../../../components/ui/Button";
import { Input } from "../../../components/ui/Input";
import { Badge } from "../../../components/ui/Badge";
import { useUpdates } from "../../../hooks/useUpdates";

export default function UpdatesPage() {
  const t = useTranslations('actions');
  const {
    updatePlans,
    updateRuns,
    updatePlanName,
    setUpdatePlanName,
    updatePlanTargets,
    setUpdatePlanTargets,
    updatePlanScopes,
    setUpdatePlanScopes,
    updatePlanDryRun,
    setUpdatePlanDryRun,
    updatePlanSubmitting,
    updateMessage,
    createUpdatePlan,
    executeUpdatePlan
  } = useUpdates();

  return (
    <>
      <PageHeader title={t('updates.title')} subtitle={t('updates.subtitle')} />
      <Card className="mb-4">
        <form className="space-y-3 mb-4" onSubmit={createUpdatePlan}>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--muted)]">{t('updates.planName')}</span>
            <Input value={updatePlanName} onChange={(event) => setUpdatePlanName(event.target.value)} />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--muted)]" data-tooltip={t('updates.devices.tooltip')}>{t('updates.devices.label')}</span>
            <Input
              value={updatePlanTargets}
              onChange={(event) => setUpdatePlanTargets(event.target.value)}
              placeholder={t('updates.devices.placeholder')}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs font-medium text-[var(--muted)]" data-tooltip={t('updates.scopes.tooltip')}>{t('updates.scopes.label')}</span>
            <Input
              value={updatePlanScopes}
              onChange={(event) => setUpdatePlanScopes(event.target.value)}
              placeholder={t('updates.scopes.placeholder')}
            />
          </label>
          <label className="flex items-center gap-2 text-sm text-[var(--text)]">
            <input type="checkbox" checked={updatePlanDryRun} onChange={(event) => setUpdatePlanDryRun(event.target.checked)} />
            {t('updates.previewOnly')}
          </label>
          <Button variant="primary" disabled={updatePlanSubmitting}>{updatePlanSubmitting ? t('updates.saving') : t('updates.createPlan')}</Button>
        </form>
        {updateMessage ? <p className="text-xs text-[var(--muted)]">{updateMessage}</p> : null}

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)] mb-2">{t('updates.plans')}</p>
            <ul className="divide-y divide-[var(--line)]">
              {updatePlans.slice(0, 6).map((plan) => (
                <li key={plan.id} className="flex items-center justify-between gap-3 py-2.5">
                  <span className="text-sm font-medium text-[var(--text)]">{plan.name}</span>
                  <Button variant="ghost" size="sm" onClick={() => void executeUpdatePlan(plan.id)}>
                    {t('updates.run')}
                  </Button>
                </li>
              ))}
              {updatePlans.length === 0 ? (
                <li>
                  <EmptyState
                    icon={ClipboardList}
                    title={t('updates.plansEmpty.title')}
                    description={t('updates.plansEmpty.description')}
                  />
                </li>
              ) : null}
            </ul>
          </div>
          <div>
            <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)] mb-2">{t('updates.recentRuns')}</p>
            <ul className="divide-y divide-[var(--line)]">
              {updateRuns.slice(0, 6).map((run) => (
                <li key={run.id} className="flex items-center justify-between gap-3 py-2.5">
                  <span className="text-sm font-medium text-[var(--text)]">{run.plan_name}</span>
                  <Badge status={run.status === "succeeded" ? "ok" : run.status === "failed" ? "bad" : "pending"} size="sm" />
                </li>
              ))}
              {updateRuns.length === 0 ? (
                <li>
                  <EmptyState
                    icon={Play}
                    title={t('updates.runsEmpty.title')}
                    description={t('updates.runsEmpty.description')}
                  />
                </li>
              ) : null}
            </ul>
          </div>
        </div>
      </Card>
    </>
  );
}
