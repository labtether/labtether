"use client";

import { useLocale, useTranslations } from "next-intl";
import { useEffect, useMemo } from "react";
import { useUsers } from "../../../../hooks/useUsers";
import { ListState, RefreshButton, shortTime } from "./systemViewParts";

/** Who is signed in to the device now: user, terminal, where from, and since when. */
export function UsersTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("devices.system");
  const locale = useLocale();
  const { sessions, loading, error, refresh } = useUsers(nodeId);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const rows = useMemo(() => [...sessions].sort((left, right) => left.username.localeCompare(right.username)), [sessions]);

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
        loadError={t("users.loadError")}
        empty={t("users.empty")}
        onRetry={() => void refresh()}
      >
        <div className="dv-list dsy-usr" role="table" aria-label={t("users.label")}>
          <div className="dv-cols" role="row">
            <span role="columnheader">{t("users.cols.user")}</span>
            <span role="columnheader" className="r">{t("users.cols.since")}</span>
          </div>
          {rows.map((session, index) => {
            const terminal = session.terminal?.trim() ?? "";
            const host = session.remote_host?.trim() ?? "";
            const grey = terminal && host ? t("users.from", { terminal, host }) : terminal || host;
            return (
              <div key={`${session.username}-${session.terminal}-${index}`} className="dv-row" role="row">
                <span role="cell" className="dv-ln">
                  <span className="dv-nm">{session.username || "—"}</span>
                  {grey ? <span className="dv-grey">{grey}</span> : null}
                </span>
                <span role="cell" className="dv-n" title={session.login_time || undefined}>{shortTime(session.login_time, locale)}</span>
              </div>
            );
          })}
        </div>
      </ListState>
    </div>
  );
}
