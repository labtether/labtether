"use client";

import { useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { DeadLetterAnalytics, DeadLetterEvent, EndpointStatus } from "../../../console/models";
import { Section, Status } from "./HealthParts";
import { hubChecks, type HubCheck } from "./healthModel";
import { clock, when } from "./healthTime";

type Version = { version?: string; started_at?: string };

/** The hub's version and start time, as Settings › General shows them. */
function useHubVersion() {
  const [version, setVersion] = useState<Version | null>(null);
  useEffect(() => {
    let live = true;
    fetch("/api/version")
      .then((r) => (r.ok ? r.json() : null))
      .then((payload: unknown) => {
        if (live && payload && typeof payload === "object" && !("error" in payload)) setVersion(payload as Version);
      })
      .catch(() => undefined);
    return () => { live = false; };
  }, []);
  return version;
}

type HubSectionProps = {
  endpoints: EndpointStatus[];
  checkedAt?: string;
  analytics: DeadLetterAnalytics;
  deadLetters: DeadLetterEvent[];
  retentionError: string;
};

/** Hub: the hub process, the endpoints it probes, failed background jobs and data cleanup, each with a status word. */
export function HubSection({ endpoints, checkedAt, analytics, deadLetters, retentionError }: HubSectionProps) {
  const t = useTranslations("reliability.hub");
  const locale = useLocale();
  const version = useHubVersion();
  const now = Date.now();
  const rows = hubChecks({ version, endpoints, analytics, deadLetters, retentionError });

  const right = (row: HubCheck) => {
    if (row.latencyMs !== undefined) return t("latency", { ms: row.latencyMs });
    if (!row.at) return "";
    return row.key === "hub" ? t("since", { time: when(row.at, now, locale) }) : t("lastAt", { time: when(row.at, now, locale) });
  };

  return (
    <Section id="pp-hub" title={t("title")} right={checkedAt ? <span className="pp-meta">{t("checked", { time: clock(checkedAt, locale) })}</span> : null}>
      <div className="pp-list" role="table" aria-labelledby="pp-hub">
        {rows.map((row) => (
          <div key={row.key} className="pp-row pp-hc" role="row">
            <span className="pp-in" role="cell">
              <span className="pp-nm">{"key" in row.name ? t(`names.${row.name.key}`) : row.name.text}</span>
              {row.sub ? <span className="pp-grey">{"key" in row.sub ? t(row.sub.key, row.sub.values) : row.sub.text}</span> : null}
            </span>
            <span role="cell"><Status tone={row.tone}>{t(`words.${row.word}`, row.values)}</Status></span>
            <span className="pp-c pp-r pp-num" role="cell">{right(row)}</span>
          </div>
        ))}
      </div>
    </Section>
  );
}
