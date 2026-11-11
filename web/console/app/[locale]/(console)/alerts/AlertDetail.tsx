"use client";

import { useLocale, useTranslations } from "next-intl";
import type { Ref } from "react";
import { Link } from "../../../../i18n/navigation";
import type { AlertInstance, AlertRule, AlertSilence } from "../../../console/models";
import { alertAssetIds } from "../../../console/models/alerts";
import { deviceFreshnessLabel } from "../../../console/formatters";
import { useFastStatus } from "../../../contexts/StatusContext";
import { AlertDetailActions, type AlertActions } from "./AlertDetailActions";
import { SevLabel } from "./alertBits";
import { alertTimeline, fullTime, shortTime } from "./alertsModel";
import type { DeliveryRecord } from "./alertsPageTypes";
import { useAlertIssue } from "./useAlertIssue";

export type { AlertActions } from "./AlertDetailActions";

type Props = {
  ref?: Ref<HTMLElement>;
  alert: AlertInstance;
  title: string;
  rule: AlertRule | null;
  silence: AlertSilence | null;
  deliveries: DeliveryRecord[];
  channelNames: Map<string, string>;
  actions: AlertActions;
};

/** The chosen alert: its title, severity and state, the actions, then its facts, labels and timeline. */
export function AlertDetail({ ref, alert, title, rule, silence, deliveries, channelNames, actions }: Props) {
  const t = useTranslations("alerts.detail");
  const locale = useLocale();
  const now = Date.now();
  const status = useFastStatus();
  const issue = useAlertIssue(alert);
  const devices = alertAssetIds(alert).map((id) => {
    const asset = status?.assets?.find((a) => a.id === id);
    // The device's state word when it is not answering ("Not responding" in amber, "Offline" in red), as in the mockup.
    const freshness = asset ? deviceFreshnessLabel(asset) : "online";
    return { id, name: asset?.name ?? id, freshness, state: freshness === "offline" ? "bad" : freshness === "unresponsive" ? "warn" : "" };
  });
  // The rule name and device already show as the title and the Device row.
  const labels = Object.entries(alert.labels ?? {}).filter(([key]) => key !== "rule_name" && key !== "asset_id");
  const notes = Object.entries(alert.annotations ?? {}).filter(([key]) => key !== "summary");
  const timeline = alertTimeline(alert, deliveries);
  const fired = alert.last_fired_at && alert.last_fired_at !== alert.started_at;
  const headingId = `al-dt-${alert.id}`;
  const channel = (id?: string) => (id && channelNames.get(id)) || t("timeline.unknownChannel");

  return (
    <section ref={ref} className="al-sheet al-detail" aria-labelledby={headingId}>
      <h2 className="al-dt" id={headingId}>{title || t("unnamed")}</h2>
      <p className="al-meta">
        <SevLabel severity={alert.severity} />
        {alert.status === "resolved" ? (
          <span>{t("resolvedAt", { time: shortTime(alert.resolved_at ?? alert.updated_at, locale, now) })}</span>
        ) : (
          <span>{t("since", { time: shortTime(alert.started_at, locale, now) })}</span>
        )}
        {alert.status === "acknowledged" ? <span>{t("acknowledgedAt", { time: shortTime(alert.updated_at, locale, now) })}</span> : null}
        {alert.status === "pending" ? <span>{t("pending")}</span> : null}
        {silence ? <span>{t("silencedUntil", { time: shortTime(silence.ends_at, locale, now) })}</span> : null}
      </p>

      <AlertDetailActions alert={alert} rule={rule} silence={silence} issueId={issue?.id ?? null} actions={actions} />

      {alert.annotations?.summary ? <p className="al-why">{alert.annotations.summary}</p> : null}

      <dl className="al-facts">
        {devices.map((device) => (
          <div key={device.id} className="al-kv">
            <dt>{t("device")}</dt>
            <dd>
              <Link href={`/nodes/${encodeURIComponent(device.id)}`} className="al-lnk">{device.name}</Link>
              {device.state ? <span className={`al-st ${device.state}`}>{t(`deviceState.${device.freshness}`)}</span> : null}
            </dd>
          </div>
        ))}
        {rule ? (
          <div className="al-kv">
            <dt>{t("rule")}</dt>
            <dd><button type="button" className="al-lnk" onClick={() => actions.openRule(rule.id)}>{rule.name}</button></dd>
          </div>
        ) : null}
        {issue ? (
          <div className="al-kv">
            <dt>{t("issue")}</dt>
            <dd><Link href={`/incidents/${encodeURIComponent(issue.id)}`} className="al-lnk">{issue.title}</Link></dd>
          </div>
        ) : null}
        <div className="al-kv">
          <dt>{t("started")}</dt>
          <dd>{fullTime(alert.started_at, locale)}</dd>
        </div>
        {fired ? (
          <div className="al-kv">
            <dt>{t("lastFired")}</dt>
            <dd>{fullTime(alert.last_fired_at, locale)}</dd>
          </div>
        ) : null}
      </dl>

      {labels.length > 0 ? (
        <>
          <h3 className="al-cap">{t("labels")}</h3>
          <dl className="al-facts">
            {labels.map(([key, value]) => (
              <div key={key} className="al-kv"><dt>{key}</dt><dd><span>{value}</span></dd></div>
            ))}
          </dl>
        </>
      ) : null}

      {notes.length > 0 ? (
        <>
          <h3 className="al-cap">{t("annotations")}</h3>
          <dl className="al-facts">
            {notes.map(([key, value]) => (
              <div key={key} className="al-kv"><dt>{key}</dt><dd><span>{value}</span></dd></div>
            ))}
          </dl>
        </>
      ) : null}

      <h3 className="al-cap">{t("timeline.title")}</h3>
      <ol className="al-tl">
        {timeline.map((entry, i) => (
          <li key={`${entry.kind}-${i}`} className={entry.tone ? `hot ${entry.tone}` : undefined}>
            <time dateTime={entry.at}>{shortTime(entry.at, locale, now)}</time>
            <span>{t(`timeline.${entry.kind}`, { channel: channel(entry.channelId) })}</span>
          </li>
        ))}
      </ol>
    </section>
  );
}
