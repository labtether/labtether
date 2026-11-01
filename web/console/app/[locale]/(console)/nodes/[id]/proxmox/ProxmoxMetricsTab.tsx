"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { withCollector } from "../proxmoxFormatters";
import { DiSeg } from "./DiControls";
import { DiCols, DiEmpty, DiList, DiNote, DiNum, DiRow, DiSection, DiTime } from "./DiList";
import { sizeParts, whenText } from "./diFormat";
import { useProxmoxFetch } from "./useProxmoxData";

type MetricPoint = {
  time?: number;
  cpu?: number;
  mem?: number;
  maxmem?: number;
  disk?: number;
  maxdisk?: number;
  netin?: number;
  netout?: number;
  diskread?: number;
  diskwrite?: number;
};

type MetricsResponse = {
  data?: MetricPoint[];
  timeframe?: string;
};

const WINDOWS = ["hour", "day", "week", "month"] as const;
type Window = (typeof WINDOWS)[number];

type Props = {
  assetId: string;
  proxmoxCollectorID: string;
};

function Rate({ bytes, className = "" }: { bytes?: number; className?: string }) {
  const size = sizeParts(bytes);
  return <DiNum value={size?.value} unit={size ? `${size.unit}/s` : undefined} className={className} />;
}

function Size({ bytes, className = "" }: { bytes?: number; className?: string }) {
  const size = sizeParts(bytes);
  return <DiNum value={size?.value} unit={size?.unit} className={className} />;
}

/** Proxmox's own samples (RRD) for the resource: CPU, memory, network and disk rates, newest first. */
export function ProxmoxMetricsTab({ assetId, proxmoxCollectorID }: Props) {
  const t = useTranslations("devices-proxmox.pve.metrics");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [range, setRange] = useState<Window>("hour");
  const path = withCollector(`/api/proxmox/assets/${encodeURIComponent(assetId)}/metrics?timeframe=${range}`, proxmoxCollectorID);
  const { data, loading, error, refresh } = useProxmoxFetch<MetricsResponse>(path);
  const points: MetricPoint[] = data?.data ?? (Array.isArray(data) ? (data as MetricPoint[]) : []);
  const shown = points.slice(-50).reverse();

  return (
    <DiSection
      title={t("title")}
      right={
        <>
          <DiSeg label={t("rangeLabel")} options={WINDOWS.map((id) => ({ id, label: t(`range.${id}`) }))} value={range} onChange={setRange} />
          <Button size="sm" onClick={refresh} disabled={loading}>{loading ? kit("loading") : kit("refresh")}</Button>
        </>
      }
    >
      {error ? <DiNote tone="bad">{t("loadFailed", { error })}</DiNote> : null}
      <DiList
        cols="96px repeat(6, minmax(0, 1fr))"
        colsM="80px repeat(4, minmax(0, 1fr))"
        colsS="72px repeat(2, minmax(0, 1fr))"
        label={t("title")}
      >
        <DiCols>
          <span>{t("time")}</span>
          <span className="r">{t("cpu")}</span>
          <span className="r">{t("memory")}</span>
          <span className="r di-s">{t("netIn")}</span>
          <span className="r di-s">{t("netOut")}</span>
          <span className="r di-m">{t("diskRead")}</span>
          <span className="r di-m">{t("diskWrite")}</span>
        </DiCols>
        {shown.length === 0 ? <DiEmpty title={loading ? kit("loading") : t("none")} /> : null}
        {shown.map((point, index) => (
          <DiRow key={point.time ?? index}>
            <DiTime className="di-left">{whenText(point.time, locale)}</DiTime>
            <DiNum value={point.cpu != null ? (point.cpu * 100).toFixed(1) : undefined} unit="%" hot={point.cpu != null && point.cpu >= 0.85} />
            <Size bytes={point.mem} />
            <Rate bytes={point.netin} className="di-s" />
            <Rate bytes={point.netout} className="di-s" />
            <Rate bytes={point.diskread} className="di-m" />
            <Rate bytes={point.diskwrite} className="di-m" />
          </DiRow>
        ))}
      </DiList>
    </DiSection>
  );
}
