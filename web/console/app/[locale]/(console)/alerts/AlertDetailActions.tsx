"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import type { AlertInstance, AlertRule, AlertSilence } from "../../../console/models";
import { alertAssetIds } from "../../../console/models/alerts";
import { durationPresets, type DurationPreset } from "./alertsPageTypes";

export type AlertActions = {
  /** The alert (or silence) whose action is in flight, so its buttons wait. */
  busy: string | null;
  acknowledge: (alert: AlertInstance) => Promise<void>;
  resolve: (alert: AlertInstance) => Promise<void>;
  silence: (alert: AlertInstance, preset: DurationPreset) => Promise<void>;
  endSilence: (silence: AlertSilence) => Promise<void>;
  openRule: (ruleId: string) => void;
};

/**
 * Acknowledge, Resolve and Silence as plain buttons; the next step is the one rose button (Acknowledge while it fires,
 * Resolve once acknowledged). Silence needs labels: the hub matches a silence against the alert's rule labels.
 */
export function AlertDetailActions({ alert, rule, silence, issueId, actions }: {
  alert: AlertInstance;
  rule: AlertRule | null;
  silence: AlertSilence | null;
  /** The issue that holds this alert, when known: ⋯ then opens it rather than the Issues list. */
  issueId: string | null;
  actions: AlertActions;
}) {
  const t = useTranslations("alerts.detail");
  const busy = actions.busy !== null && (actions.busy === alert.id || actions.busy === silence?.id);
  const firing = alert.status === "firing";
  const open = alert.status !== "resolved";
  const canResolve = firing || alert.status === "acknowledged";
  const hasLabels = Object.keys(alert.labels ?? {}).length > 0;
  const assetIds = alertAssetIds(alert);
  const assetId = assetIds.length === 1 ? assetIds[0] : "";

  return (
    <div className="al-acts">
      {firing ? (
        <Button variant="primary" disabled={busy} onClick={() => void actions.acknowledge(alert)}>{t("acknowledge")}</Button>
      ) : null}
      {canResolve ? (
        <Button variant={firing ? "secondary" : "primary"} disabled={busy} onClick={() => void actions.resolve(alert)}>{t("resolve")}</Button>
      ) : null}
      {open && silence ? (
        <Button disabled={busy} onClick={() => void actions.endSilence(silence)}>{t("endSilence")}</Button>
      ) : null}
      {open && !silence ? (
        <Menu.Root>
          <Menu.Trigger className="al-btn" disabled={busy || !hasLabels} title={hasLabels ? undefined : t("silenceNoLabels")}>
            {t("silence")}
            <ChevronDown aria-hidden="true" strokeWidth={2} />
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="lt-menu" align="start" sideOffset={6}>
              {durationPresets.map((preset) => (
                <Menu.Item key={preset.id} className="lt-menu-item" onSelect={() => void actions.silence(alert, preset.id)}>
                  {t(`silenceFor.${preset.id}`)}
                </Menu.Item>
              ))}
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      ) : null}
      <Menu.Root>
        <Menu.Trigger className="al-btn icon" aria-label={t("more")}>
          <MoreHorizontal aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="start" sideOffset={6}>
            {rule ? (
              <Menu.Item className="lt-menu-item" onSelect={() => actions.openRule(rule.id)}>{t("openRule")}</Menu.Item>
            ) : null}
            {assetId ? (
              <Menu.Item className="lt-menu-item" asChild>
                <Link href={`/nodes/${encodeURIComponent(assetId)}`}>{t("openDevice")}</Link>
              </Menu.Item>
            ) : null}
            <Menu.Item className="lt-menu-item" asChild>
              {issueId ? (
                <Link href={`/incidents/${encodeURIComponent(issueId)}`}>{t("openIssue")}</Link>
              ) : (
                <Link href="/incidents">{t("openIssues")}</Link>
              )}
            </Menu.Item>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
