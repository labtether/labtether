"use client";

import { useTranslations } from "next-intl";
import { useCallback, useState } from "react";
import { usePollingTask } from "../../../../hooks/usePollingTask";
import { formatMem } from "../../../../components/containers/containerModel";
import { ListEmpty, ListError, SectionHead } from "../../../../components/containers/PanelParts";
import { fetchDockerContainerStats } from "../../../../../lib/docker";

type Stats = {
  cpu_percent: number;
  memory_bytes: number;
  memory_limit: number;
  memory_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  block_read_bytes: number;
  block_write_bytes: number;
  pids: number;
};
type Sample = { cpu: number; mem: number };
const KEEP = 60;
const num = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

/** A live line in one hue: the samples taken while this tab is open (one every 5 s), scaled to top. */
function LiveLine({ values, top, label }: { values: number[]; top: number; label: string }) {
  const w = 300;
  const h = 72;
  const max = top > 0 ? top : 1;
  const pts = values.map((v, i) => [values.length < 2 ? w : (i / (KEEP - 1)) * w, h - Math.min(1, v / max) * (h - 2) - 1]);
  const shift = values.length ? w - pts[pts.length - 1][0] : 0;
  const line = pts.map(([x, y]) => `${(x + shift).toFixed(1)},${y.toFixed(1)}`).join(" ");
  return (
    <svg className="ct-chart" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" role="img" aria-label={label}>
      {[0.5, 1].map((f) => <line key={f} x1="0" x2={w} y1={h - f * (h - 2) - 1} y2={h - f * (h - 2) - 1} className="ct-chart-grid" />)}
      {values.length > 1 ? (
        <>
          <polygon className="ct-chart-fill" points={`${(pts[0][0] + shift).toFixed(1)},${h} ${line} ${w},${h}`} />
          <polyline className="ct-chart-line" points={line} />
        </>
      ) : null}
    </svg>
  );
}

/** CPU and memory now and over the last minutes, network and disk totals, and processes. */
export function DockerStatsTab({ containerId }: { containerId: string }) {
  const t = useTranslations("devices-docker.stats");
  const [stats, setStats] = useState<Stats | null>(null);
  const [history, setHistory] = useState<Sample[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const task = useCallback(async (signal: AbortSignal) => {
    try {
      const raw = await fetchDockerContainerStats(containerId);
      if (signal.aborted) return;
      if (typeof raw.cpu_percent !== "number") { setStats(null); setHistory([]); setError(null); return; }
      const s: Stats = {
        cpu_percent: num(raw.cpu_percent), memory_bytes: num(raw.memory_bytes), memory_limit: num(raw.memory_limit),
        memory_percent: num(raw.memory_percent), net_rx_bytes: num(raw.net_rx_bytes), net_tx_bytes: num(raw.net_tx_bytes),
        block_read_bytes: num(raw.block_read_bytes), block_write_bytes: num(raw.block_write_bytes), pids: num(raw.pids),
      };
      setStats(s);
      setHistory((prev) => [...prev, { cpu: s.cpu_percent, mem: s.memory_bytes }].slice(-KEEP));
      setError(null);
    } catch (err) {
      if (signal.aborted) return;
      setError(err instanceof Error ? err.message : "error");
    } finally {
      if (!signal.aborted) setLoading(false);
    }
  }, [containerId]);
  usePollingTask(task, 5_000);

  if (loading && !stats) return <ListEmpty text={t("loading")} />;
  if (error && !stats) return <ListError text={t("loadFailed")} />;
  if (!stats) return <ListEmpty text={t("notRunning")} />;

  const cpuTop = Math.max(10, Math.ceil(Math.max(...history.map((p) => p.cpu), 1) / 10) * 10);
  const memTop = stats.memory_limit || Math.max(...history.map((p) => p.mem), 1) * 1.2;
  const hot = (pct: number) => pct >= 85;
  const split = (text: string) => { const [v, u] = text.split(" "); return <>{v}<span className="u"> {u}</span></>; };

  return (
    <div className="ct-tab">
      <section aria-labelledby="ct-use">
        <SectionHead id="ct-use" title={t("usage")}><span className="ct-quiet">{t("window")}</span></SectionHead>
        <div className="ct-cells two">
          <div className="ct-cell">
            <span className="k">{t("cpu")}</span>
            <b className={`ct-read${hot(stats.cpu_percent) ? " hot" : ""}`}>{stats.cpu_percent.toFixed(1)}<span className="u">%</span></b>
            <LiveLine values={history.map((p) => p.cpu)} top={cpuTop} label={t("cpuChart")} />
          </div>
          <div className="ct-cell">
            <span className="k">{t("memory")}</span>
            <b className={`ct-read${hot(stats.memory_percent) ? " hot" : ""}`}>
              {split(formatMem(stats.memory_bytes))}
              {stats.memory_limit ? <span className="u ct-of">{t("of", { limit: formatMem(stats.memory_limit) })}</span> : null}
            </b>
            <LiveLine values={history.map((p) => p.mem)} top={memTop} label={t("memoryChart")} />
          </div>
        </div>
      </section>
      <section className="ct-sec" aria-labelledby="ct-io">
        <SectionHead id="ct-io" title={t("io")} />
        <div className="ct-cells three">
          <div className="ct-cell"><span className="k">{t("network")}</span><span className="ct-io">{t("rxtx", { rx: formatMem(stats.net_rx_bytes), tx: formatMem(stats.net_tx_bytes) })}</span></div>
          <div className="ct-cell"><span className="k">{t("disk")}</span><span className="ct-io">{t("readWrite", { read: formatMem(stats.block_read_bytes), write: formatMem(stats.block_write_bytes) })}</span></div>
          <div className="ct-cell"><span className="k">{t("processes")}</span><span className="ct-io">{stats.pids}</span></div>
        </div>
      </section>
    </div>
  );
}
