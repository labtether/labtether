"use client";

import { useMemo, type ReactNode } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { Asset, AssetTelemetryDetails, MetricWindow, TelemetryOverviewAsset } from "../../../../console/models";
import { MetricChart } from "./metricsViewChart";
import { analyzeTelemetryDetails, valueToChartY, type AnalyzedSeries } from "./nodeMetricsModel";
import { deviceState, shortWhen } from "./deviceStatusModel";
import { isWindowsPlatform } from "../../files/remotePaths";

const RANGES: MetricWindow[] = ["1h", "6h", "24h", "7d", "30d"];
const num = (raw: string | undefined): number | null => (raw && /^\d+(?:\.\d+)?$/.test(raw.trim()) ? Number(raw) : null);
const pct = (v: number) => `${Math.round(v)}%`;
const toMbit = (bytesPerSec: number) => (bytesPerSec * 8) / 1e6;
const mbit = (bytesPerSec: number) => {
  const v = toMbit(bytesPerSec);
  return v > 0 && v < 10 ? v.toFixed(1) : String(Math.round(v));
};
const GB = 1024 ** 3;
const TB = 1024 ** 4;
/** "21 of 96 GB", "0.4 of 3.8 TB": both numbers in the larger one's unit. */
function usedOf(used: number, size: number): { used: string; size: string } {
  const [div, unit] = size >= TB ? [TB, "TB"] : size >= GB ? [GB, "GB"] : [1024 ** 2, "MB"];
  const fmt = (v: number) => (v / div >= 10 || v === 0 ? String(Math.round(v / div)) : (v / div).toFixed(1));
  return { used: fmt(used), size: `${fmt(size)} ${unit}` };
}
const sizeText = (bytes: number) => usedOf(0, bytes).size;
/** The two network series in Mbit/s on one scale from 0, so in and out compare. */
function networkScale(rx?: AnalyzedSeries, tx?: AnalyzedSeries): [AnalyzedSeries | undefined, AnalyzedSeries | undefined] {
  const peak = Math.max(rx ? toMbit(rx.max) : 0, tx ? toMbit(tx.max) : 0);
  if (!rx || peak <= 0) return [rx, tx];
  const mag = 10 ** Math.floor(Math.log10(peak * 1.1));
  const top = [1, 2, 5, 10].map((m) => m * mag).find((v) => v >= peak * 1.1) ?? 10 * mag;
  const scale = (s: AnalyzedSeries): AnalyzedSeries => ({
    ...s, unit: "mbit_per_sec", yMin: 0, yMax: top, current: s.current == null ? null : toMbit(s.current), min: toMbit(s.min), max: toMbit(s.max),
    points: s.points.map((p) => ({ ...p, value: toMbit(p.value), y: valueToChartY(toMbit(p.value), 0, top) })),
  });
  return [scale(rx), tx ? scale(tx) : undefined];
}

type Props = {
  asset: Asset;
  telemetry: TelemetryOverviewAsset | null;
  details: AssetTelemetryDetails | null;
  loading: boolean;
  window: MetricWindow;
  onWindowChange: (window: MetricWindow) => void;
};

/** One chart cell: its label, the value now ("—" while the device is quiet), then the chart. */
function Cell({ label, value, series, second }: { label: string; value: ReactNode; series?: AnalyzedSeries; second?: AnalyzedSeries }) {
  return (
    <div className="dv-cell">
      <span className="dv-cell-k">{label}</span>
      <div className="dv-cell-v">{value}</div>
      {series ? <MetricChart series={series} second={second} small /> : null}
    </div>
  );
}

/** CPU, memory and network over the chosen range, and root disk use as a bar. Offline devices keep their history. */
export function DeviceUsage({ asset, telemetry, details, loading, window, onWindowChange }: Props) {
  const t = useTranslations("devices.page.usage");
  const locale = useLocale();
  const analyzed = useMemo(() => analyzeTelemetryDetails(details), [details]);
  const bySeries = (metric: string) => analyzed.find((s) => s.metric === metric);
  const metaOf = asset.metadata ?? {};
  const state = deviceState(asset);
  const quiet = state.tone === "bad";
  const m: TelemetryOverviewAsset["metrics"] = telemetry?.metrics ?? {};
  const [netIn, netOut] = networkScale(bySeries("network_rx_bytes_per_sec"), bySeries("network_tx_bytes_per_sec"));
  const now = (metric: keyof typeof m) => (quiet ? null : bySeries(metric)?.current ?? m[metric] ?? null);

  const cpu = now("cpu_used_percent");
  const mem = now("memory_used_percent");
  const rx = now("network_rx_bytes_per_sec");
  const tx = now("network_tx_bytes_per_sec");
  const disk = quiet ? null : m.disk_used_percent ?? bySeries("disk_used_percent")?.current ?? null;
  const hasAny = analyzed.length > 0 || [cpu, mem, rx, tx, disk].some((v) => v != null);

  const threads = num(metaOf.cpu_threads_logical) ?? num(metaOf.cpu_cores_physical);
  const memTotal = num(metaOf.memory_total_bytes) ?? num(metaOf.physmem);
  const diskTotal = num(metaOf.disk_root_total_bytes);
  const diskFree = num(metaOf.disk_root_available_bytes);
  // The Hub's overview has one disk reading and no mount name. The agent takes it from C:\ on Windows, / elsewhere.
  const diskMount = isWindowsPlatform(asset.platform || metaOf.platform) ? "C:" : "/";

  const note = quiet ? (state.since ? t("quietSince", { time: shortWhen(state.since, locale) }) : t("quiet")) : !hasAny && !loading ? t("none") : "";
  const big = (text: string, hot = false) => <b className={`dv-readout${hot ? " hot" : ""}`}>{text}</b>;
  const none = <b className="dv-readout none">—</b>;

  return (
    <section className="dv-sec" aria-labelledby="dv-use">
      <div className="dv-sh">
        <h2 id="dv-use">
          {t("title")}
          {note ? <span className={`dv-quiet${quiet ? " bad" : ""}`}>{note}</span> : null}
        </h2>
        {hasAny || quiet ? (
          <div className="dv-seg" role="group" aria-label={t("range")}>
            {RANGES.map((r) => (
              <button key={r} type="button" aria-pressed={r === window} onClick={() => onWindowChange(r)}>{t(`ranges.${r}`)}</button>
            ))}
          </div>
        ) : null}
      </div>
      {hasAny || quiet ? (
        <div className="dv-cells">
          <Cell
            label={t("cpu")}
            series={bySeries("cpu_used_percent")}
            value={cpu == null ? none : <>{big(pct(cpu), cpu >= 85)}{threads ? <span className="u">{t("ofThreads", { count: threads })}</span> : null}</>}
          />
          <Cell
            label={t("memory")}
            series={bySeries("memory_used_percent")}
            value={mem == null ? none : <>{big(pct(mem), mem >= 85)}{memTotal ? <span className="u">{t("of", { size: sizeText(memTotal) })}</span> : null}</>}
          />
          <Cell
            label={t("network")}
            series={netIn}
            second={netOut}
            value={rx == null && tx == null ? none : (
              <>
                <i className="dv-sw" aria-hidden="true" />{big(mbit(rx ?? 0))}<span className="u">{t("in")}</span>
                <i className="dv-sw dash" aria-hidden="true" />{big(mbit(tx ?? 0))}<span className="u">{t("outMbit")}</span>
              </>
            )}
          />
          <div className="dv-cell">
            <span className="dv-cell-k">{t("disk")}</span>
            {disk == null ? <div className="dv-cell-v">{quiet ? none : <span className="dv-hint">{t("noDisk")}</span>}</div> : (
              <div className="dv-mount">
                <span className="dv-mono">{diskMount}</span>
                <span className={`dv-meter${disk >= 85 ? " hot" : ""}`} aria-hidden="true"><i style={{ width: `${Math.min(100, Math.max(0, disk))}%` }} /></span>
                <span className={`dv-n${disk >= 85 ? " hot" : ""}`}>{pct(disk)}</span>
                <span className="dv-n">{diskTotal ? t("ofSize", usedOf(diskFree != null ? diskTotal - diskFree : (diskTotal * disk) / 100, diskTotal)) : ""}</span>
              </div>
            )}
          </div>
        </div>
      ) : null}
    </section>
  );
}
