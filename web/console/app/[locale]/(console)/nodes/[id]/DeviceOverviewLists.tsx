"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { ServiceIcon } from "../../../../components/ServiceIcon";
import type { LogEvent } from "../../../../console/models";
import type { WebService } from "../../../../hooks/useWebServices";
import { shortWhen } from "./deviceStatusModel";

const SHOWN = 6;

/** The device's web services, down first: icon, name, address, latency. Six until "Show all". Nothing when it has none. */
export function DeviceServicesList({ services }: { services: WebService[] }) {
  const t = useTranslations("devices.page.services");
  const [all, setAll] = useState(false);
  if (!services.length) return null;
  const list = [...services].sort((a, b) => Number(b.status === "down") - Number(a.status === "down") || a.name.localeCompare(b.name));
  const long = list.length > SHOWN + 1;
  const shown = long && !all ? list.slice(0, SHOWN) : list;

  return (
    <section className="dv-sec" aria-labelledby="dv-svc">
      <div className="dv-sh"><h2 id="dv-svc">{t("title")}</h2></div>
      <div className="dv-list dv-svc" role="list" aria-label={t("title")}>
        {shown.map((s) => {
          const down = s.status === "down";
          const web = /^https?:\/\//i.test(s.url);
          const address = s.url.replace(/^https?:\/\//i, "").replace(/\/$/, "");
          return (
            <div key={s.id} className="dv-row" role="listitem">
              <span className="dv-lead">{down ? <i className="dv-dot bad" aria-hidden="true" /> : null}</span>
              <span className="dv-svc-ic"><ServiceIcon iconKey={s.icon_key} size={18} /></span>
              <span className="dv-ln">
                {web ? <a className="dv-nm" href={s.url} target="_blank" rel="noopener noreferrer">{s.name}</a> : <span className="dv-nm">{s.name}</span>}
                {down ? <span className="dv-prob bad">{t("down")}</span> : <span className="dv-grey">{address}</span>}
              </span>
              {down || !(s.response_ms > 0)
                ? <span className="dv-n none">—</span>
                : <span className={`dv-n${s.response_ms > 100 ? " hot" : ""}`}>{Math.round(s.response_ms)} <span className="u">ms</span></span>}
            </div>
          );
        })}
        {long ? (
          <div className="dv-foot">
            <button type="button" aria-expanded={all} onClick={() => setAll(!all)}>{t(all ? "fewer" : "all")}</button>
          </div>
        ) : null}
      </div>
    </section>
  );
}

const LEVEL_TONE: Record<string, "bad" | "warn" | undefined> = { error: "bad", err: "bad", crit: "bad", critical: "bad", fatal: "bad", emerg: "bad", alert: "bad", warning: "warn", warn: "warn" };

/** This device's newest log lines: time and message, warnings and errors marked with a dot. */
export function DeviceActivity({ nodeId, events, loaded }: { nodeId: string; events: LogEvent[]; loaded: boolean }) {
  const t = useTranslations("devices.page.activity");
  const locale = useLocale();
  return (
    <section className="dv-sec" aria-labelledby="dv-act">
      <div className="dv-sh">
        <h2 id="dv-act">{t("title")}</h2>
        <Link href={`/nodes/${encodeURIComponent(nodeId)}?panel=logs`}>{t("allLogs")}</Link>
      </div>
      <div className="dv-list dv-ev">
        {events.length ? events.map((e) => {
          const when = new Date(e.timestamp);
          const tone = LEVEL_TONE[(e.level ?? "").toLowerCase()];
          return (
            <div key={e.id} className="dv-row">
              <time className="dv-n" dateTime={e.timestamp}>{Number.isFinite(when.getTime()) ? shortWhen(when, locale) : ""}</time>
              <span className="dv-ev-msg">
                {tone ? <i className={`dv-dot ${tone}`} aria-hidden="true" /> : null}
                <span className="dv-ev-t">{e.message}</span>
              </span>
            </div>
          );
        }) : <p className="dv-ev-none">{loaded ? t("none") : " "}</p>}
      </div>
    </section>
  );
}
