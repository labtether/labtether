"use client";

import { useTranslations } from "next-intl";
import { useEffect, useMemo, useState } from "react";
import { Link } from "../../../i18n/navigation";
import type { AssetTelemetryDetails } from "../../console/models";
import { friendlySourceLabel } from "../../console/taxonomy";
import { DeviceTile } from "../../[locale]/(console)/nodes/DeviceRow";
import { needsAttention } from "../../[locale]/(console)/nodes/deviceListModel";
import type { DeviceCardData } from "../../[locale]/(console)/nodes/nodesPageUtils";

const HISTORY_REFRESH_MS = 5 * 60_000;

/** CPU over the last 24 hours for each server, refreshed every five minutes. A server without a series draws nothing. */
function useCpuHistory(ids: string[]) {
  const key = ids.join(",");
  const [history, setHistory] = useState<Record<string, number[]>>({});
  useEffect(() => {
    if (!key) return;
    const controller = new AbortController();
    const load = async () => {
      const entries = await Promise.all(key.split(",").map(async (id) => {
        try {
          const res = await fetch(`/api/metrics/assets/${encodeURIComponent(id)}?window=24h`, { cache: "no-store", signal: controller.signal });
          if (!res.ok) return [id, []] as const;
          const data = (await res.json()) as AssetTelemetryDetails;
          const cpu = data.series?.find((s) => s.metric === "cpu_used_percent");
          return [id, (cpu?.points ?? []).map((p) => p.value).filter((v) => Number.isFinite(v))] as const;
        } catch {
          return [id, []] as const;
        }
      }));
      if (!controller.signal.aborted) setHistory(Object.fromEntries(entries));
    };
    void load();
    const timer = window.setInterval(load, HISTORY_REFRESH_MS);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [key]);
  return history;
}

/** A one-hue area curve, scaled so a quiet server still shows its shape (top at 125% of the peak, at least 20%). */
function CpuCurve({ values, label }: { values: number[]; label: string }) {
  if (values.length < 2) return <span className="db-curve" />;
  const step = Math.ceil(values.length / 96);
  const v = values.filter((_, i) => i % step === 0);
  const max = Math.max(20, Math.ceil((Math.max(...v) * 1.25) / 10) * 10);
  const pts = v.map((y, i) => `${((i / (v.length - 1)) * 160).toFixed(1)},${(28 - (Math.min(y, max) / max) * 26).toFixed(1)}`);
  return (
    <svg className="db-curve" viewBox="0 0 160 28" preserveAspectRatio="none" role="img" aria-label={label}>
      <polygon points={`0,28 ${pts.join(" ")} 160,28`} className="db-curve-fill" />
      <polyline points={pts.join(" ")} className="db-curve-line" />
    </svg>
  );
}

const pct = (card: DeviceCardData, value: number | null) => {
  if (needsAttention(card) || value == null) return <span className="db-pct" />;
  const v = Math.round(value);
  return <span className={`db-pct${v >= 85 ? " hot" : ""}`}>{v}%</span>;
};

/** One row per server: its tile and name, what it runs (or the problem), plain CPU, memory and disk, and CPU over 24 h. */
export function ServerList({ servers, allCards }: { servers: DeviceCardData[]; allCards: DeviceCardData[] }) {
  const t = useTranslations("dashboard.servers");
  const history = useCpuHistory(servers.map((s) => s.asset.id));
  const guests = useMemo(() => {
    const count = new Map<string, number>();
    for (const c of allCards) if (c.hostedOn) count.set(c.hostedOn.id, (count.get(c.hostedOn.id) ?? 0) + 1);
    return count;
  }, [allCards]);

  return (
    <div className="db-list db-srv">
      <div className="db-row db-cols" aria-hidden="true">
        <span /><span /><span>{t("cpu")}</span><span>{t("memory")}</span><span>{t("disk")}</span><span>{t("cpu24h")}</span>
      </div>
      {servers.map((card) => {
        const { asset } = card;
        const meta = asset.metadata ?? {};
        const n = guests.get(asset.id) ?? 0;
        const problem = needsAttention(card);
        const sub = problem
          ? <span className={card.freshness === "offline" ? "bad" : "warn"}>{t(card.freshness === "offline" ? "offline" : "notResponding")}</span>
          : [meta.os_pretty_name || meta.os_name || friendlySourceLabel(asset.source), n ? t("guests", { count: n }) : ""].filter(Boolean).join(" · ");
        return (
          <Link key={asset.id} href={`/nodes/${asset.id}`} className="db-row db-srv-row">
            <DeviceTile card={card} />
            <span className="db-txt"><span className="db-nm">{asset.name}</span><span className="db-sub">{sub}</span></span>
            {pct(card, card.cpu)}{pct(card, card.mem)}{pct(card, card.disk)}
            <CpuCurve values={history[asset.id] ?? []} label={t("cpuLabel", { name: asset.name })} />
          </Link>
        );
      })}
    </div>
  );
}
