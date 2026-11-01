"use client";

import { useLocale, useTranslations } from "next-intl";
import { useMemo, useRef, useState, type KeyboardEvent } from "react";
import type { AlertInstance, AlertRule, AlertSilence } from "../../../console/models";
import { AlertDetail, type AlertActions } from "./AlertDetail";
import { Dot } from "./alertBits";
import { alertTitle, groupAlerts, pickAlert, sevTone, shortTime, silenceFor } from "./alertsModel";
import type { InboxFilter } from "./alertsPageTypes";
import { useDeliveryLog } from "./useDeliveryLog";

const RESOLVED_PREVIEW = 5;

type Props = {
  loading: boolean;
  instances: AlertInstance[];
  rules: AlertRule[];
  silences: AlertSilence[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  /** The Filter and Order menus live in the tab row (the page holds their state). */
  filter: InboxFilter;
  onFilter: (next: InboxFilter) => void;
  actions: AlertActions;
};

/**
 * The inbox: open alerts (most severe first), then Silenced and Resolved, with the chosen alert's facts beside the
 * list (below it when the page is narrow). j/k or the arrows move, a acknowledges, e resolves.
 */
export function AlertInbox({ loading, instances, rules, silences, selectedId, onSelect, filter, onFilter, actions }: Props) {
  const t = useTranslations("alerts.inbox");
  const tSevRaw = useTranslations("alerts.severity");
  const tSev = (s: string) => (tSevRaw.has(s) ? tSevRaw(s) : s);
  const locale = useLocale();
  const [allResolved, setAllResolved] = useState(false);
  const listRef = useRef<HTMLDivElement>(null);
  const detailRef = useRef<HTMLElement>(null);
  const deliveries = useDeliveryLog(instances.length > 0);
  const now = Date.now();
  const ruleNames = useMemo(() => new Map(rules.map((r) => [r.id, r.name])), [rules]);
  const groups = groupAlerts(instances, silences, filter, now);
  const selected = pickAlert(groups, selectedId);
  // The latest few resolved alerts, or all of them when asked (or when the chosen one is further down).
  const showAll = allResolved || (selected !== null && groups.resolved.indexOf(selected) >= RESOLVED_PREVIEW);
  const resolvedShown = showAll ? groups.resolved : groups.resolved.slice(0, RESOLVED_PREVIEW);
  const filtered = filter.severity !== "all" || filter.state !== "all";

  /** Choose an alert; when the detail sits under the list (narrow page), bring it into view. */
  const choose = (id: string, reveal: boolean) => {
    onSelect(id);
    if (!reveal) return;
    requestAnimationFrame(() => {
      const list = listRef.current;
      const detail = detailRef.current;
      if (list && detail && detail.offsetTop > list.offsetTop + 8) {
        const smooth = !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
        detail.scrollIntoView({ behavior: smooth ? "smooth" : "auto", block: "start" });
      }
    });
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const row = (event.target as HTMLElement).closest<HTMLElement>("[data-alert]");
    if (!row || event.metaKey || event.ctrlKey || event.altKey) return;
    const rows = [...(listRef.current?.querySelectorAll<HTMLElement>("[data-alert]") ?? [])];
    const i = rows.indexOf(row);
    const id = row.dataset.alert ?? "";
    const move = ({ j: 1, ArrowDown: 1, k: -1, ArrowUp: -1 } as Record<string, number>)[event.key];
    if (move) {
      event.preventDefault();
      const next = rows[Math.max(0, Math.min(rows.length - 1, i + move))];
      next?.focus();
      if (next?.dataset.alert) onSelect(next.dataset.alert);
      return;
    }
    const alert = instances.find((a) => a.id === id);
    if (event.key === "a" && alert?.status === "firing") {
      event.preventDefault();
      void actions.acknowledge(alert);
    } else if (event.key === "e" && alert && (alert.status === "firing" || alert.status === "acknowledged")) {
      event.preventDefault();
      void actions.resolve(alert);
    }
  };

  const item = (alert: AlertInstance, quiet: boolean) => {
    const on = alert.id === selected?.id;
    const silence = silenceFor(alert, silences, now);
    const tone = quiet ? "off" : sevTone(alert.severity);
    let sub = alert.annotations?.summary ?? "";
    if (alert.status === "resolved") sub = t("row.resolvedAt", { time: shortTime(alert.resolved_at ?? alert.updated_at, locale, now) });
    else if (silence) sub = t("row.silencedUntil", { time: shortTime(silence.ends_at, locale, now) });
    else if (alert.status === "acknowledged") sub = t("row.acknowledgedAt", { time: shortTime(alert.updated_at, locale, now) });
    else if (alert.status === "pending") sub = sub ? t("row.pendingWith", { text: sub }) : t("row.pending");
    else if (!sevTone(alert.severity)) sub = sub ? t("row.sevWith", { severity: tSev(alert.severity), text: sub }) : tSev(alert.severity);
    return (
      <button
        key={alert.id}
        type="button"
        className={`al-item${on ? " on" : ""}${quiet ? " done" : ""}`}
        data-alert={alert.id}
        aria-current={on ? "true" : undefined}
        onClick={() => choose(alert.id, true)}
      >
        <Dot tone={tone} />
        <span className="al-it">
          <span className="al-t">{alertTitle(alert, ruleNames) || t("unnamed")}</span>
          {sub ? <span className="al-s">{sub}</span> : null}
        </span>
        <time className="al-time" dateTime={alert.started_at}>{shortTime(alert.started_at, locale, now)}</time>
      </button>
    );
  };

  if (loading && instances.length === 0) {
    return <div className="al-sheet al-empty" aria-busy="true">{t("loading")}</div>;
  }

  if (!selected) {
    return (
      <div className="al-sheet al-empty">
        <b>{filtered ? t("emptyFiltered") : t("empty")}</b>
        {filtered ? <button type="button" className="al-quiet" onClick={() => onFilter({ ...filter, severity: "all", state: "all" })}>{t("clear")}</button> : null}
      </div>
    );
  }

  return (
    <div className="al-wrap">
      <div className="al-split">
        <div ref={listRef} className="al-sheet al-list" role="group" aria-label={t("list")} onKeyDown={onKeyDown}>
          {groups.open.map((a) => item(a, false))}
          {groups.open.length === 0 ? <p className="al-none">{t("noOpen")}</p> : null}
          {groups.silenced.length > 0 ? <div className="al-group">{t("groups.silenced")}</div> : null}
          {groups.silenced.map((a) => item(a, true))}
          {groups.resolved.length > 0 ? <div className="al-group">{t("groups.resolved")}</div> : null}
          {resolvedShown.map((a) => item(a, true))}
          {groups.resolved.length > resolvedShown.length ? (
            <button type="button" className="al-more-row" onClick={() => setAllResolved(true)}>
              {t("showAllResolved", { count: groups.resolved.length })}
            </button>
          ) : null}
          {filtered ? (
            <div className="al-foot">
              <span>{t("shown", { shown: groups.shown, total: groups.total })}</span>
              <button type="button" onClick={() => onFilter({ ...filter, severity: "all", state: "all" })}>{t("clear")}</button>
            </div>
          ) : null}
        </div>
        <AlertDetail
          ref={detailRef}
          alert={selected}
          title={alertTitle(selected, ruleNames)}
          rule={rules.find((r) => r.id === selected.rule_id) ?? null}
          silence={silenceFor(selected, silences, now)}
          deliveries={deliveries.records}
          channelNames={deliveries.channelNames}
          actions={actions}
        />
      </div>
    </div>
  );
}
