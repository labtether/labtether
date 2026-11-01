"use client";

import { useEffect, useState } from "react";
import type { AssetTelemetryDetails } from "../../../console/models";
import type { MapWords } from "./useMapWords";

/** CPU over the last 24 hours for the Inspector. Draws nothing until the hub returns a series for the device. */
export function MapCpu({ id, words }: { id: string; words: MapWords }) {
  const { t } = words;
  const [values, setValues] = useState<number[]>([]);
  useEffect(() => {
    const controller = new AbortController();
    setValues([]);
    fetch(`/api/metrics/assets/${encodeURIComponent(id)}?window=24h`, { cache: "no-store", signal: controller.signal })
      .then((r) => (r.ok ? (r.json() as Promise<AssetTelemetryDetails>) : null))
      .then((data) => {
        const cpu = data?.series?.find((s) => s.metric === "cpu_used_percent");
        if (!controller.signal.aborted) setValues((cpu?.points ?? []).map((p) => p.value).filter((v) => Number.isFinite(v)));
      })
      .catch(() => { /* no chart without data */ });
    return () => controller.abort();
  }, [id]);

  if (values.length < 2) return null;
  const step = Math.ceil(values.length / 96);
  const v = values.filter((_, i) => i % step === 0);
  const now = Math.round(v[v.length - 1]);
  const pts = v.map((y, i) => `${((i / (v.length - 1)) * 280).toFixed(1)},${(62 - (Math.min(100, Math.max(0, y)) / 100) * 60).toFixed(1)}`);
  return (
    <section className="mp-sec" aria-labelledby="mp-i-cpu">
      <h3 id="mp-i-cpu" className="mp-sh">{t("inspector.cpu")}<span className="mp-cpu-now">{t("inspector.cpuNow", { value: now })}</span></h3>
      <svg className="mp-cpu" viewBox="0 0 280 64" preserveAspectRatio="none" role="img" aria-label={t("inspector.cpuLabel", { value: now })}>
        <polygon points={`0,64 ${pts.join(" ")} 280,64`} className="mp-cpu-fill" />
        <polyline points={pts.join(" ")} className="mp-cpu-line" />
      </svg>
    </section>
  );
}
