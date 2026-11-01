"use client";

import { useLocale, useTranslations } from "next-intl";
import { useEffect, useMemo } from "react";
import { useCron } from "../../../../hooks/useCron";
import { copyText, ListState, RefreshButton, RowMenu, shortTime } from "./systemViewParts";

/** "timer", "launchd" or "cron" for the hub's source names; anything else as sent. */
function kindOf(source: string): string {
  const normalized = source.trim().toLowerCase();
  if (normalized === "systemd-timer") return "timer";
  if (normalized === "launchd") return "launchd";
  if (normalized === "crontab") return "cron";
  return "";
}

/** Scheduled work on the device, read only: cron jobs, systemd timers and launchd jobs, with their next and last run. */
export function CronTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const locale = useLocale();
  const { entries, loading, error, refresh } = useCron(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const rows = useMemo(() => [...entries].sort((left, right) =>
    left.source.localeCompare(right.source) || left.user.localeCompare(right.user) || left.schedule.localeCompare(right.schedule)), [entries]);

  return (
    <div className="dsy">
      <div className="dv-bar">
        <span className="dv-sp" />
        <RefreshButton loading={loading} onClick={() => void refresh()} />
      </div>
      <ListState
        loading={loading}
        hasRows={rows.length > 0}
        error={error}
        loadError={t("cron.loadError")}
        empty={t("cron.empty")}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-cron" role="table" aria-label={t("cron.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("cron.cols.schedule")}</span>
            <span role="columnheader">{t("cron.cols.command")}</span>
            <span role="columnheader" className="c-user">{t("cron.cols.user")}</span>
            <span role="columnheader" className="r c-next">{t("cron.cols.next")}</span>
            <span role="columnheader" className="r c-last">{t("cron.cols.last")}</span>
            <span role="columnheader" aria-label={t("actions")} />
          </div>
          {rows.map((entry, index) => {
            const kind = kindOf(entry.source);
            const who = [kind && kind !== "cron" ? t(`cron.kind.${kind}`) : !kind ? entry.source : "", entry.user].filter(Boolean).join(" · ");
            return (
              <div key={`${entry.source}-${entry.user}-${entry.schedule}-${index}`} className="dv-row" role="row" tabIndex={0}>
                <span role="cell" className="dv-mono dsy-trunc">{entry.schedule || "—"}</span>
                <span role="cell" className="dv-mono dsy-trunc" title={entry.command || undefined}>{entry.command || "—"}</span>
                <span role="cell" className="dv-txt c-user">{who || "—"}</span>
                <span role="cell" className="dv-n c-next">{shortTime(entry.next_run, locale)}</span>
                <span role="cell" className="dv-n none c-last">{shortTime(entry.last_run, locale)}</span>
                <RowMenu
                  name={entry.command || entry.schedule}
                  items={[
                    { label: t("cron.copyCommand"), onSelect: () => copyText(entry.command) },
                    { label: t("cron.copySchedule"), onSelect: () => copyText(entry.schedule) },
                  ]}
                />
              </div>
            );
          })}
        </div>
      </ListState>
    </div>
  );
}
