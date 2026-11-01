"use client";

// Datasets: full name, mount point and settings as grey text, a meter of the dataset against its pool, used and
// available space; sortable by name or size (largest first). Delete sits behind ⋯ and asks first.

import { useState } from "react";
import { useTranslations } from "next-intl";
import { DiCols, DiDots, DiList, DiMain, DiMeter, DiNum, DiRow } from "../proxmox/DiList";
import { DiRowMenu, DiSortHead, useDiConfirm } from "../proxmox/DiControls";
import { percentOf, sizeParts, sizeText } from "../proxmox/diFormat";
import { failureText, sortDatasets, type DatasetSort } from "./truenasModel";
import { TnFailure, TnListState } from "./TrueNASParts";
import type { TrueNASDataset } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASDataset } from "./truenasTypes";

function SizeCell({ bytes, className = "" }: { bytes?: number; className?: string }) {
  const parts = sizeParts(bytes);
  return <DiNum value={parts?.value} unit={parts?.unit} className={className} />;
}

type Props = {
  assetId: string;
  /** Pool sizes by pool name, for the meter. Without them the meter compares used with used + available. */
  poolSizes?: Map<string, number>;
};

export function TrueNASDatasetsTab({ assetId, poolSizes }: Props) {
  const t = useTranslations("devices-truenas.datasets");
  const k = useTranslations("devices-proxmox.kit");
  const { data: datasets, loading, error, refresh } = useTrueNASList<TrueNASDataset>(assetId, "datasets");
  const [sort, setSort] = useState<DatasetSort>({ key: "used", dir: "desc" });
  const action = useTrueNASAction();
  const { ask, dialog } = useDiConfirm();

  const remove = (ds: TrueNASDataset) => {
    ask({
      title: t("deleteTitle", { name: ds.name }),
      message: t("deleteBody"),
      confirmLabel: t("deleteConfirm"),
      onConfirm: () => action.run(ds.name, () => truenasAction(
        `/api/truenas/assets/${encodeURIComponent(assetId)}/datasets/${encodeURIComponent(ds.id)}`,
        "DELETE",
      ), refresh),
    });
  };

  const meter = (ds: TrueNASDataset) => {
    const pool = poolSizes?.get(ds.name.split("/")[0]);
    if (pool) return percentOf(ds.used, pool);
    return percentOf(ds.used, (ds.used ?? 0) + (ds.available ?? 0));
  };

  const grey = (ds: TrueNASDataset) => {
    const quotaPct = ds.quota && ds.quota > 0 ? percentOf(ds.used, ds.quota) : null;
    return (
      <DiDots
        parts={[
          ds.mountpoint ? <span className="di-mono">{ds.mountpoint}</span> : null,
          ds.compression,
          ds.quota && ds.quota > 0
            ? (quotaPct != null ? t("quotaPct", { size: sizeText(ds.quota), pct: Math.round(quotaPct) }) : t("quota", { size: sizeText(ds.quota) }))
            : null,
          ds.readonly ? t("readOnly") : null,
        ]}
      />
    );
  };

  const rows = sortDatasets(datasets, sort);
  return (
    <>
      <TnFailure message={action.error ? t("deleteFailed", { name: action.error.key, reason: failureText(action.error.message, t("noReason")) }) : null} />
      <DiList
        label={t("label")}
        cols="minmax(0, 1fr) minmax(48px, 160px) 72px 80px 28px"
        colsM="minmax(0, 1fr) minmax(48px, 120px) 72px 28px"
        colsS="minmax(0, 1fr) 72px 28px"
      >
        <DiCols>
          <DiSortHead id="name" label={t("colName")} sort={sort} onSort={(next) => setSort(next as DatasetSort)} />
          <span className="di-s" />
          <DiSortHead id="used" label={t("colUsed")} sort={sort} onSort={(next) => setSort(next as DatasetSort)} right />
          <span className="r di-m">{t("colAvailable")}</span>
          <span />
        </DiCols>
        <TnListState loading={loading} error={error} count={datasets.length} empty={t("empty")} onRetry={refresh} failed={(reason) => t("loadFailed", { reason })} />
        {rows.map((ds) => (
          <DiRow key={ds.id}>
            <DiMain name={ds.name} mono grey={grey(ds)} />
            <DiMeter pct={meter(ds)} className="di-s" />
            <SizeCell bytes={ds.used} />
            <SizeCell bytes={ds.available} className="di-m" />
            <DiRowMenu
              label={k("actionsFor", { name: ds.name })}
              items={[{ label: t("delete"), danger: true, disabled: action.busy !== null, onSelect: () => remove(ds) }]}
            />
          </DiRow>
        ))}
      </DiList>
      {dialog}
    </>
  );
}
