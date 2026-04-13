"use client";

import { Link } from "../../../../i18n/navigation";
import { useTranslations } from "next-intl";
import { Zap } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Input } from "../../../components/ui/Input";
import { Select } from "../../../components/ui/Input";
import { useActions } from "../../../hooks/useActions";

export default function ActionsPage() {
  const t = useTranslations('actions');
  const {
    actionParameters,
    actionParamValues,
    connectors,
    selectedConnector,
    setSelectedConnector,
    connectorActions,
    selectedConnectorAction,
    setSelectedConnectorAction,
    selectedActionDescriptor,
    actionRequiresTarget,
    actionSupportsDryRun,
    actionTarget,
    setActionTarget,
    setActionParamValue,
    actionDryRun,
    setActionDryRun,
    actionSubmitting,
    actionMessage,
    connectorActionsError,
    actionRuns,
    submitConnectorAction
  } = useActions();

  return (
    <>
      <PageHeader title={t('title')} subtitle={t('subtitle')} />
      <Card>
        <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('runAction.heading')}</h2>
        <form className="space-y-3" onSubmit={submitConnectorAction}>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <label className="flex flex-col gap-1 text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
              <span data-tooltip={t('runAction.integration.tooltip')}>{t('runAction.integration.label')}</span>
              <Select
                value={selectedConnector}
                onChange={(event) => setSelectedConnector(event.target.value)}
              >
                {connectors.map((connector) => (
                  <option key={connector.id} value={connector.id}>
                    {connector.display_name}
                  </option>
                ))}
              </Select>
            </label>
            <label className="flex flex-col gap-1 text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
              {t('runAction.action')}
              <Select
                value={selectedConnectorAction}
                onChange={(event) => setSelectedConnectorAction(event.target.value)}
              >
                {connectorActions.map((action) => (
                  <option key={action.id} value={action.id}>
                    {action.name}
                  </option>
                ))}
              </Select>
            </label>
          </div>
          {actionRequiresTarget ? (
            <label className="flex flex-col gap-1 text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
              <span data-tooltip={t('runAction.device.tooltip')}>{t('runAction.device.label')}</span>
              <Input
                value={actionTarget}
                onChange={(event) => setActionTarget(event.target.value)}
                placeholder={t('runAction.device.placeholder')}
              />
            </label>
          ) : (
            <p className="text-xs text-[var(--muted)]">{t('runAction.device.notRequired')}</p>
          )}
          {actionParameters.length > 0 ? (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              {actionParameters.map((parameter) => (
                <label key={parameter.key} className="flex flex-col gap-1 text-xs font-mono uppercase tracking-wider text-[var(--muted)]">
                  <span title={parameter.description}>
                    {parameter.label}
                    {parameter.required ? ` ${t('runAction.parameters.required')}` : ""}
                  </span>
                  <Input
                    value={actionParamValues[parameter.key] ?? ""}
                    onChange={(event) => setActionParamValue(parameter.key, event.target.value)}
                    placeholder={parameter.description || parameter.key}
                  />
                </label>
              ))}
            </div>
          ) : (
            <p className="text-xs text-[var(--muted)]">{t('runAction.parameters.none')}</p>
          )}
          <label className={`flex items-center gap-2 text-sm text-[var(--text)] select-none ${actionSupportsDryRun ? "cursor-pointer" : "cursor-not-allowed opacity-60"}`}>
            <span
              role="checkbox"
              aria-checked={actionSupportsDryRun && actionDryRun}
              aria-disabled={!actionSupportsDryRun}
              tabIndex={actionSupportsDryRun ? 0 : -1}
              onClick={() => {
                if (actionSupportsDryRun) {
                  setActionDryRun(!actionDryRun);
                }
              }}
              onKeyDown={(event) => {
                if (!actionSupportsDryRun) return;
                if (event.key === " " || event.key === "Enter") {
                  event.preventDefault();
                  setActionDryRun(!actionDryRun);
                }
              }}
              className={`inline-flex items-center justify-center w-4 h-4 rounded border transition-colors ${(actionSupportsDryRun && actionDryRun) ? "border-[var(--accent)] bg-[var(--accent)] text-white" : "border-[var(--line)] bg-transparent"}`}
            >
              {actionSupportsDryRun && actionDryRun ? <span className="text-[10px] leading-none">&#10003;</span> : null}
            </span>
            <span data-tooltip={actionSupportsDryRun ? t('runAction.previewOnly.tooltip') : t('runAction.previewOnly.unsupportedTooltip')}>
              {t('runAction.previewOnly.label')}
            </span>
          </label>
          <Button type="submit" variant="primary" disabled={actionSubmitting || !selectedActionDescriptor}>
            {actionSubmitting ? t('runAction.submitting') : t('runAction.submit')}
          </Button>
        </form>
        {selectedActionDescriptor?.description ? <p className="mt-3 text-xs text-[var(--muted)]">{selectedActionDescriptor.description}</p> : null}
        {connectorActionsError ? <p className="mt-3 text-xs text-[var(--bad)]">{connectorActionsError}</p> : null}
        {actionMessage ? <p className="mt-3 text-xs text-[var(--muted)]">{actionMessage}</p> : null}
      </Card>

      <Card>
        <h2 className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-2">{t('history.heading')}</h2>
        <ul className="divide-y divide-[var(--line)]">
          {actionRuns.slice(0, 8).map((run) => (
            <li key={run.id} className="flex items-center justify-between gap-3 py-2.5">
              <span>
                {run.connector_id ? `${run.connector_id}:${run.action_id}` : run.command || run.type}
                {run.status === "failed" ? <> &middot; <Link href="/logs" className="nodeLink">{t('history.viewLogs')}</Link></> : null}
              </span>
              <Badge
                status={run.status === "succeeded" ? "ok" : run.status === "failed" ? "bad" : "pending"}
                size="sm"
                dot
              />
            </li>
          ))}
          {actionRuns.length === 0 ? (
            <li>
              <EmptyState
                icon={Zap}
                title={t('history.empty.title')}
                description={t('history.empty.description')}
              />
            </li>
          ) : null}
        </ul>
      </Card>
    </>
  );
}
