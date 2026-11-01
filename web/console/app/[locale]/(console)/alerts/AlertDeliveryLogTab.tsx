"use client";

import { RefreshCw } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import { useMemo, useState } from "react";
import { Button } from "../../../components/ui/Button";
import type { AlertInstance, AlertRule } from "../../../console/models";
import { Dot } from "./alertBits";
import { alertTitle, shortTime } from "./alertsModel";
import type { DeliveryRecord, DeliveryStatus } from "./alertsPageTypes";
import { useDeliveryLog } from "./useDeliveryLog";

type StatusFilter = "all" | DeliveryStatus;
const FILTERS: StatusFilter[] = ["all", "sent", "failed", "pending"];

/**
 * The hub's notification history as a plain list: when, which channel, for which alert, and whether it went out.
 * A failure says why under the row; a pending send says which retry it is on.
 */
export function AlertDeliveryLogTab({ instances, rules, onShowAlert }: { instances: AlertInstance[]; rules: AlertRule[]; onShowAlert: (id: string) => void }) {
  const t = useTranslations("notification-center");
  const locale = useLocale();
  const { records, channelNames, loading, refreshing, failed, refresh } = useDeliveryLog();
  const [filter, setFilter] = useState<StatusFilter>("all");
  const now = Date.now();
  const ruleNames = useMemo(() => new Map(rules.map((r) => [r.id, r.name])), [rules]);
  const alerts = useMemo(() => new Map(instances.map((a) => [a.id, a])), [instances]);
  // Newest first, whatever order the hub sends.
  const sorted = useMemo(() => [...records].sort((a, b) => (Date.parse(b.sent_at ?? b.created_at) || 0) - (Date.parse(a.sent_at ?? a.created_at) || 0)), [records]);
  const shown = filter === "all" ? sorted : sorted.filter((r) => r.status === filter);

  const state = (r: DeliveryRecord) => {
    if (r.status === "sent") return r.retry_count > 0 ? t("sentAfter", { count: r.retry_count }) : t("sent");
    if (r.status === "failed") return r.retry_count > 0 ? t("failedAfter", { count: r.retry_count }) : t("failed");
    return r.max_retries > 0 && r.retry_count > 0 ? t("retrying", { count: r.retry_count, max: r.max_retries }) : t("pending");
  };

  return (
    <>
      <div className="al-tools">
        <div className="al-seg" role="group" aria-label={t("filterByStatus")}>
          {FILTERS.map((key) => (
            <button key={key} type="button" className={filter === key ? "on" : undefined} aria-pressed={filter === key} onClick={() => setFilter(key)}>
              {t(key)}
            </button>
          ))}
        </div>
        <span className="al-sp" />
        <Button variant="ghost" onClick={() => void refresh()} disabled={refreshing}>
          <RefreshCw aria-hidden="true" size={14} strokeWidth={2} className={refreshing ? "al-spin" : undefined} />
          {t("refresh")}
        </Button>
      </div>

      {loading ? (
        <div className="al-sheet al-empty" aria-busy="true">{t("loading")}</div>
      ) : failed && records.length === 0 ? (
        <div className="al-sheet al-empty"><b>{t("loadError")}</b></div>
      ) : shown.length === 0 ? (
        <div className="al-sheet al-empty">
          <b>{filter === "all" ? t("noNotifications") : t("emptyFiltered")}</b>
          {filter !== "all" ? <button type="button" className="al-quiet" onClick={() => setFilter("all")}>{t("clear")}</button> : null}
        </div>
      ) : (
        <div className="al-sheet al-plain al-dl" role="group" aria-label={t("title")}>
          <div className="al-dl-row al-rcols">
            <span>{t("sentAt")}</span>
            <span>{t("channel")}</span>
            <span>{t("alert")}</span>
            <span>{t("status")}</span>
          </div>
          {shown.map((r) => {
            const alert = alerts.get(r.alert_instance_id);
            const title = alert ? alertTitle(alert, ruleNames) || t("unknownAlert") : t("unknownAlert");
            const at = r.sent_at ?? r.created_at;
            return (
              <div key={r.id} className={`al-dl-row${r.error ? " two" : ""}`}>
                <time className="al-time" dateTime={at}>{shortTime(at, locale, now)}</time>
                <span className="al-trunc">{channelNames.get(r.channel_id) || t("unknownChannel")}</span>
                <span className="al-trunc">
                  {alert ? <button type="button" className="al-lnk" onClick={() => onShowAlert(alert.id)}>{title}</button> : title}
                </span>
                <span className={`al-dl-state ${r.status}`}>
                  <Dot tone={r.status === "failed" ? "bad" : ""} />
                  {state(r)}
                </span>
                {r.error ? <span className="al-dl-err">{r.error}</span> : null}
              </div>
            );
          })}
          {filter !== "all" ? (
            <div className="al-foot">
              <span>{t("shown", { shown: shown.length, total: records.length })}</span>
              <button type="button" onClick={() => setFilter("all")}>{t("clear")}</button>
            </div>
          ) : null}
        </div>
      )}
    </>
  );
}
