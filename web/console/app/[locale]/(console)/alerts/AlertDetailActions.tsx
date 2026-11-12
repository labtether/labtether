"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, MoreHorizontal } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import type { AlertInstance, AlertRule, AlertSilence } from "../../../console/models";
import { alertAssetIds } from "../../../console/models/alerts";
import { shortTime } from "./alertsModel";
import { durationPresets, type DurationPreset } from "./alertsPageTypes";
import { useCanWrite } from "../../../hooks/useCanWrite";

export type AlertActions = {
  /** The alert (or silence) whose action is in flight, so its buttons wait. */
  busy: string | null;
  acknowledge: (alert: AlertInstance) => Promise<void>;
  resolve: (alert: AlertInstance) => Promise<void>;
  /** Silences the alert's rule on every device the alert fired for, or on the one device given. */
  silence: (alert: AlertInstance, preset: DurationPreset, deviceId?: string) => Promise<void>;
  /** Ends the silences that cover the alert, or those that name the one device given. */
  endSilence: (alert: AlertInstance, deviceId?: string) => Promise<void>;
  openRule: (ruleId: string) => void;
};

/** Silence and its durations. `name` is read out in place of "Silence" when the button stands for one device. */
function SilenceMenu({ name, quiet, disabled, onPick }: {
  name?: string;
  quiet?: boolean;
  disabled: boolean;
  onPick: (preset: DurationPreset) => void;
}) {
  const t = useTranslations("alerts.detail");
  return (
    <Menu.Root>
      <Menu.Trigger className={quiet ? "al-btn quiet" : "al-btn"} aria-label={name} disabled={disabled}>
        {t("silence")}
        <ChevronDown aria-hidden="true" strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu" align="start" sideOffset={6}>
          {durationPresets.map((preset) => (
            <Menu.Item key={preset.id} className="lt-menu-item" onSelect={() => onPick(preset.id)}>
              {t(`silenceFor.${preset.id}`)}
            </Menu.Item>
          ))}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/**
 * One device of an alert that fired for several: Silence mutes the alert's rule on this device only. Once a silence
 * names the device, the row says until when and offers End silence. `silences` are the running ones that name it;
 * `covered` is true when a silence already covers the whole alert.
 */
export function AlertDeviceSilence({ alert, device, silences, covered, actions }: {
  alert: AlertInstance;
  device: { id: string; name: string };
  silences: AlertSilence[];
  covered: boolean;
  actions: AlertActions;
}) {
  const t = useTranslations("alerts.detail");
  const locale = useLocale();
  // A viewer gets no button: the Hub refuses their silence.
  const canWrite = useCanWrite();
  if (!canWrite || alert.status === "resolved") return null;
  const busy = actions.busy !== null;
  if (silences.length > 0) {
    const until = silences.reduce((latest, s) => (Date.parse(s.ends_at) > Date.parse(latest) ? s.ends_at : latest), silences[0].ends_at);
    return (
      <span className="al-dev-sil">
        <span className="al-st">{t("silencedUntil", { time: shortTime(until, locale, Date.now()) })}</span>
        <button type="button" className="al-btn quiet" aria-label={t("endSilenceDevice", { device: device.name })} disabled={busy} onClick={() => void actions.endSilence(alert, device.id)}>
          {t("endSilence")}
        </button>
      </span>
    );
  }
  if (covered) return null;
  return <SilenceMenu quiet name={t("silenceDevice", { device: device.name })} disabled={busy} onPick={(preset) => void actions.silence(alert, preset, device.id)} />;
}

/**
 * Acknowledge, Resolve and Silence as plain buttons; the next step is the one rose button (Acknowledge while it fires,
 * Resolve once acknowledged). Silence mutes this alert's rule on the alert's devices only.
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
  // A viewer gets only the links: the Hub refuses their acknowledge, resolve and silence.
  const canWrite = useCanWrite();
  const firing = canWrite && alert.status === "firing";
  const open = canWrite && alert.status !== "resolved";
  const canResolve = firing || (canWrite && alert.status === "acknowledged");
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
        <Button disabled={busy} onClick={() => void actions.endSilence(alert)}>{t("endSilence")}</Button>
      ) : null}
      {open && !silence ? (
        <SilenceMenu disabled={busy} onPick={(preset) => void actions.silence(alert, preset)} />
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
