"use client";

import { useLocale, useTranslations } from "next-intl";
import { useId, type ReactNode } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { safeHttpURL } from "../../../../lib/safeExternalURL";
import {
  compatConnectorLabel,
  formatCompatConfidence,
  formatResponseTime,
  proxyProviderLabel,
} from "./servicesLayoutHelpers";
import { formatCheckTime, latencyStats, outagesFromChecks } from "./servicesListModel";

function statusClass(status: string): string {
  const s = status.trim().toLowerCase();
  return s === "down" ? "down" : s === "up" ? "" : "unk";
}

/** One fact in the details grid; nothing when the hub sent no value. */
function Fact({ label, children, mono = false }: { label: string; children: ReactNode; mono?: boolean }) {
  if (children == null || children === "") return null;
  return (
    <div>
      <dt>{label}</dt>
      <dd className={mono ? "mono" : undefined}>{children}</dd>
    </div>
  );
}

/**
 * The open row's checks and facts in a sunk well under it: a strip of the recent checks (red when down), uptime and
 * answer times over the hub's window, outages, the recent checks, then where LabTether found the service.
 */
export function ServiceDetailPanel({ service, hostName }: { service: WebService; hostName: string }) {
  const t = useTranslations("services.details");
  const locale = useLocale();
  const checksID = useId();
  const at = (iso?: string) => formatCheckTime(iso, locale);
  /** "10 minutes", "1 hour 28 minutes": from one check time to another, or to now. */
  const duration = (from: string, to?: string) => {
    const minutes = Math.max(1, Math.round(((to ? Date.parse(to) : Date.now()) - Date.parse(from)) / 60_000));
    const hours = Math.floor(minutes / 60);
    const rest = minutes % 60;
    return hours === 0 ? t("minutes", { count: minutes }) : [t("hours", { count: hours }), rest ? t("minutes", { count: rest }) : ""].filter(Boolean).join(" ");
  };
  const meta = service.metadata ?? {};
  const health = service.health && service.health.checks > 0 ? service.health : null;
  const recent = health?.recent ?? [];
  const outages = outagesFromChecks(recent);
  const { avg, peak } = latencyStats(recent);
  const href = safeHttpURL(service.url);
  const compat = compatConnectorLabel(meta.compat_connector ?? "");
  const confidence = formatCompatConfidence(meta.compat_confidence);
  const altURLs = (service.alt_urls ?? []).map((alt) => alt.url).concat(meta.alt_urls ? [meta.alt_urls] : []);
  const source = [t(`sources.${["docker", "proxy", "scan", "manual"].includes(service.source) ? service.source : "other"}`),
    meta.proxy_provider ? proxyProviderLabel(meta.proxy_provider) : "",
    meta.scan_scope === "lan" ? t("lan") : ""].filter(Boolean).join(" · ");

  return (
    <div className="sv-dt" role="region" aria-label={t("label", { name: service.name })}>
      {health && (
        <>
          {recent.length > 0 && (
            <>
              <div className="sv-strip" role="img" aria-label={outages.length ? t("downTimes", { count: outages.length }) : t("upAll")}>
                {recent.map((point, i) => (
                  <i key={`${point.at}-${i}`} className={statusClass(point.status)} title={`${at(point.at)} · ${formatResponseTime(point.response_ms ?? 0) || t("noAnswer")}`} />
                ))}
              </div>
              <div className="sv-ax"><span>{at(recent[0]?.at)}</span><span>{at(recent[recent.length - 1]?.at)}</span></div>
            </>
          )}
          <dl className="sv-kv">
            <Fact label={t("window", { window: health.window || "24h" })}>
              {[t("uptime", { pct: health.uptime_percent.toFixed(1) }), avg != null ? t("avg", { ms: formatResponseTime(avg) }) : "",
                peak != null ? t("peak", { ms: formatResponseTime(peak) }) : ""].filter(Boolean).join(" · ")}
            </Fact>
            <Fact label={t("outages")}>
              {outages.length === 0 ? <span className="sv-q">{t("none")}</span> : (
                <ul className="sv-out">
                  {outages.map((o) => (
                    <li key={o.start}>
                      <span>{at(o.start)}</span>
                      <span>{o.end ? duration(o.start, o.end) : t("downFor", { duration: duration(o.start) })}</span>
                    </li>
                  ))}
                </ul>
              )}
            </Fact>
            <Fact label={t("lastCheck")}>{at(health.last_checked_at)}</Fact>
            <Fact label={t("lastChange")}>{at(health.last_change_at)}</Fact>
            {recent.length > 0 && (
              <div>
                <dt id={checksID}>{t("recentChecks")}</dt>
                <dd>
                  <ul className="sv-checks" aria-labelledby={checksID}>
                    {recent.slice(-6).reverse().map((point, i) => (
                      <li key={`${point.at}-r${i}`}>
                        <span className={`sv-chk ${statusClass(point.status)}`}>{t(`status.${statusClass(point.status) || "up"}`)}</span>
                        <span>{at(point.at)}</span>
                        <span>{formatResponseTime(point.response_ms ?? 0) || "—"}</span>
                      </li>
                    ))}
                  </ul>
                </dd>
              </div>
            )}
          </dl>
        </>
      )}
      <dl className="sv-kv">
        <Fact label={t("url")}>{href ? <a className="mono sv-lnk" href={href} target="_blank" rel="noopener noreferrer">{service.url}</a> : service.url}</Fact>
        <Fact label={t("device")}>{hostName}</Fact>
        <Fact label={t("source")}>{source}</Fact>
        <Fact label={t("scanned")} mono>{meta.scan_target_host}</Fact>
        <Fact label={t("container")} mono>{service.source === "docker" ? service.container_id?.slice(0, 12) : ""}</Fact>
        <Fact label={t("image")} mono>{meta.image}</Fact>
        <Fact label={t("unit")} mono>{service.service_unit}</Fact>
        <Fact label={t("router")}>{meta.router_name}</Fact>
        <Fact label={t("backend")} mono>{meta.backend_url}</Fact>
        <Fact label={t("direct")} mono>{meta.raw_url}</Fact>
        <Fact label={t("altUrls")} mono>{altURLs.join(", ")}</Fact>
        <Fact label={t("compat")}>{compat ? (confidence ? `${compat} · ${confidence}` : compat) : ""}</Fact>
        <Fact label={t("authHint")}>{meta.compat_auth_hint}</Fact>
        <Fact label={t("profile")} mono>{meta.compat_profile}</Fact>
        <Fact label={t("evidence")} mono>{meta.compat_evidence}</Fact>
      </dl>
    </div>
  );
}
