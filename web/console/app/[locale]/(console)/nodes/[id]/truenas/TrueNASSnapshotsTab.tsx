"use client";

// ZFS snapshots: name with its dataset in grey, size, referenced data and when it was taken; newest first, filter by
// dataset, pick several to delete together. Roll back and Delete sit behind ⋯ and ask first.

import { useMemo, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "../../../../../components/ui/Button";
import { DiCols, DiList, DiMain, DiNum, DiRow, DiTime } from "../proxmox/DiList";
import { DiPick, DiRowMenu, DiSortHead, useDiConfirm } from "../proxmox/DiControls";
import { sizeParts, whenText } from "../proxmox/diFormat";
import { failureText, sortSnapshots, type SnapshotSort } from "./truenasModel";
import { TnFailure, TnListState } from "./TrueNASParts";
import type { TrueNASSnapshot } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASSnapshot } from "./truenasTypes";

const ALL = "__all__";

function SizeCell({ bytes, className = "" }: { bytes?: number; className?: string }) {
  const parts = sizeParts(bytes);
  return <DiNum value={parts?.value} unit={parts?.unit} className={className} />;
}

export function TrueNASSnapshotsTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.snapshots");
  const k = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const { data: snapshots, loading, error, refresh } = useTrueNASList<TrueNASSnapshot>(assetId, "snapshots");
  const [dataset, setDataset] = useState<string>(ALL);
  const [sort, setSort] = useState<SnapshotSort>({ key: "created", dir: "desc" });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [bulkError, setBulkError] = useState<string | null>(null);
  const action = useTrueNASAction();
  const { ask, dialog } = useDiConfirm();

  const datasets = useMemo(() => [...new Set(snapshots.map((snap) => snap.dataset))].sort(), [snapshots]);
  const shown = useMemo(
    () => sortSnapshots(dataset === ALL ? snapshots : snapshots.filter((snap) => snap.dataset === dataset), sort),
    [snapshots, dataset, sort],
  );
  const picked = shown.filter((snap) => selected.has(snap.id));
  const path = (id: string, suffix = "") => `/api/truenas/assets/${encodeURIComponent(assetId)}/snapshots/${encodeURIComponent(id)}${suffix}`;

  const toggle = (id: string) => setSelected((prev) => {
    const next = new Set(prev);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    return next;
  });

  const rollback = (snap: TrueNASSnapshot) => ask({
    title: t("rollbackTitle", { dataset: snap.dataset, name: snap.snapshot_name }),
    message: snap.created ? t("rollbackBody", { dataset: snap.dataset, when: whenText(snap.created, locale) }) : t("rollbackBodyNoTime", { dataset: snap.dataset }),
    confirmLabel: t("rollbackConfirm"),
    onConfirm: () => action.run(`rollback:${snap.id}`, () => truenasAction(path(snap.id, "/rollback"), "POST"), refresh),
  });

  const remove = (snap: TrueNASSnapshot) => ask({
    title: t("deleteTitle", { name: snap.snapshot_name }),
    message: snap.dataset,
    confirmLabel: t("deleteConfirm"),
    onConfirm: () => action.run(`delete:${snap.id}`, () => truenasAction(path(snap.id), "DELETE"), () => {
      setSelected((prev) => { const next = new Set(prev); next.delete(snap.id); return next; });
      refresh();
    }),
  });

  const removePicked = () => ask({
    title: t("bulkTitle", { count: picked.length }),
    message: picked.slice(0, 6).map((snap) => snap.id).join(", ") + (picked.length > 6 ? " …" : ""),
    confirmLabel: t("deleteConfirm"),
    onConfirm: async () => {
      setBulkError(null);
      const failed: string[] = [];
      for (const snap of picked) {
        try {
          await truenasAction(path(snap.id), "DELETE");
        } catch (err) {
          failed.push(failureText(err instanceof Error ? err.message : "", snap.id));
        }
      }
      setSelected(new Set());
      if (failed.length > 0) setBulkError(t("bulkFailed", { count: failed.length, reason: failed.join(", ") }));
      refresh();
    },
  });

  const failure = action.error
    ? t(action.error.key.startsWith("rollback:") ? "rollbackFailed" : "deleteFailed", {
        name: action.error.key.split(":").slice(1).join(":"),
        reason: failureText(action.error.message, t("noReason")),
      })
    : bulkError;

  return (
    <>
      {datasets.length > 1 || picked.length > 0 ? (
        <div className="di-tools">
          {datasets.length > 1 ? (
            <DiPick
              label={t("datasetFilter")}
              value={dataset}
              onChange={(next) => { setDataset(next); setSelected(new Set()); }}
              options={[{ id: ALL, label: t("allDatasets") }, ...datasets.map((name) => ({ id: name, label: name }))]}
            />
          ) : null}
          {picked.length > 0 ? (
            <>
              <span className="di-word">{t("selected", { count: picked.length })}</span>
              <Button size="sm" variant="danger" disabled={action.busy !== null} onClick={removePicked}>
                {t("deleteSelected", { count: picked.length })}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setSelected(new Set())}>{t("clear")}</Button>
            </>
          ) : null}
        </div>
      ) : null}
      <TnFailure message={failure} />
      <DiList
        label={t("label")}
        cols="16px minmax(0, 1fr) 72px 80px 96px 28px"
        colsM="16px minmax(0, 1fr) 72px 96px 28px"
        colsS="16px minmax(0, 1fr) 72px 28px"
      >
        <DiCols>
          <span />
          <DiSortHead id="name" label={t("colName")} sort={sort} onSort={(next) => setSort(next as SnapshotSort)} />
          <DiSortHead id="used" label={t("colSize")} sort={sort} onSort={(next) => setSort(next as SnapshotSort)} right />
          <span className="r di-m">{t("colReferenced")}</span>
          <DiSortHead id="created" label={t("colTaken")} sort={sort} onSort={(next) => setSort(next as SnapshotSort)} right className="di-s" />
          <span />
        </DiCols>
        <TnListState
          loading={loading}
          error={error}
          count={shown.length}
          empty={dataset === ALL ? t("empty") : t("emptyFiltered", { dataset })}
          onRetry={refresh}
          failed={(reason) => t("loadFailed", { reason })}
        />
        {shown.map((snap) => (
          <DiRow key={snap.id} selected={selected.has(snap.id)}>
            <input
              type="checkbox"
              className="di-tn-check"
              checked={selected.has(snap.id)}
              onChange={() => toggle(snap.id)}
              aria-label={t("select", { name: snap.id })}
            />
            <DiMain name={snap.snapshot_name} grey={<span className="di-mono">{snap.dataset}</span>} />
            <SizeCell bytes={snap.used} />
            <SizeCell bytes={snap.referenced} className="di-m" />
            <DiTime className="di-s">{whenText(snap.created, locale)}</DiTime>
            <DiRowMenu
              label={k("actionsFor", { name: snap.snapshot_name })}
              items={[
                { label: t("rollback"), disabled: action.busy !== null, onSelect: () => rollback(snap) },
                { separator: true },
                { label: t("delete"), danger: true, disabled: action.busy !== null, onSelect: () => remove(snap) },
              ]}
            />
          </DiRow>
        ))}
      </DiList>
      {dialog}
    </>
  );
}
