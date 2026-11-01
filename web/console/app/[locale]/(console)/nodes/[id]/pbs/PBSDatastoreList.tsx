"use client";

// The datastores of a PBS server: name, what is in it, use against size. A row opens its details; its ⋯ verifies,
// runs garbage collection (after a confirm) and moves it in and out of read-only maintenance.

import { useState } from "react";

import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiKV, DiList, DiNote, DiRow, DiUsage, type DiTone } from "../proxmox/DiList";
import { percentOf, sizeText } from "../proxmox/diFormat";
import { backupStaleness, pbsStatusBadge, type PBSDatastoreSummary } from "../pbsTabModel";
import { pbsAction, usePBSActions } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

type Props = {
  assetId: string;
  datastores: PBSDatastoreSummary[];
  onChanged: () => void;
};

type DatastoreAction = "gc" | "verify" | "maintenance-enable" | "maintenance-disable";

const COLS = "minmax(0, 1fr) minmax(48px, 150px) 48px 104px 28px";
const COLS_M = "minmax(0, 1fr) 48px 104px 28px";
const COLS_S = "minmax(0, 1fr) 48px 28px";

function lastBackupEpoch(store: PBSDatastoreSummary): number | undefined {
  const ms = store.last_backup_at ? new Date(store.last_backup_at).getTime() : NaN;
  return Number.isFinite(ms) ? ms / 1000 : undefined;
}

export function PBSDatastoreList({ assetId, datastores, onChanged }: Props) {
  const { t, tk, when } = usePBSWords();
  const [open, setOpen] = useState<string | null>(null);
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();

  const act = (store: string, action: DatastoreAction) => run(
    `${action}-${store}`,
    () => pbsAction(`/api/pbs/assets/${encodeURIComponent(assetId)}/datastores/${encodeURIComponent(store)}/${action}`, "POST"),
    t(`ds.done.${action}`, { store }),
    t(`ds.failed.${action}`, { store }),
    onChanged,
  );

  const panelID = (store: string) => `pbs-datastore-${assetId}-${store}`.replace(/[^a-zA-Z0-9_-]/g, "-");

  return (
    <>
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={COLS} colsM={COLS_M} colsS={COLS_S} hang label={t("datastores")}>
        <DiCols>
          <span>{t("cols.datastore")}</span>
          <span className="di-m">{t("cols.used")}</span>
          <span />
          <span className="di-s" />
          <span />
        </DiCols>
        {datastores.length === 0 ? <DiEmpty title={t("noDatastores")} /> : null}
        {datastores.map((store) => {
          const expanded = open === store.store;
          const statusTone = pbsStatusBadge(store.status);
          const stale = backupStaleness(lastBackupEpoch(store));
          const tone: DiTone = statusTone === "bad" ? "bad" : store.maintenance_mode ? "warn" : stale === "ok" ? "" : stale;
          const problem = statusTone === "bad"
            ? { tone: "bad" as const, text: store.status }
            : store.maintenance_mode
              ? { tone: "warn" as const, text: t("ds.maintenance", { mode: store.maintenance_mode }) }
              : stale !== "ok"
                ? { tone: stale, text: store.last_backup_at ? t("ds.lastBackup", { time: when(store.last_backup_at) }) : t("ds.noBackups") }
                : null;
          const pct = typeof store.usage_percent === "number" ? store.usage_percent : percentOf(store.used_bytes, store.total_bytes);
          const toggle = () => setOpen(expanded ? null : store.store);
          const working = busy !== null;
          return (
            <DiRow
              key={store.store}
              tone={tone}
              onOpen={toggle}
              selected={expanded}
              after={expanded ? (
                <div id={panelID(store.store)} className="di-pbs-panel">
                  <DiKV
                    columns={2}
                    items={[
                      [t("kv.status"), store.status || "—"],
                      [t("kv.used"), sizeText(store.used_bytes)],
                      [t("kv.available"), sizeText(store.avail_bytes)],
                      [t("kv.total"), sizeText(store.total_bytes)],
                      [t("kv.groups"), String(store.group_count ?? 0)],
                      [t("kv.snapshots"), String(store.snapshot_count ?? 0)],
                      [t("kv.lastBackup"), store.last_backup_at ? when(store.last_backup_at) : tk("never")],
                      [t("kv.maintenance"), store.maintenance_mode || tk("off")],
                      [t("kv.mount"), store.mount_status || ""],
                      [t("kv.comment"), store.comment || ""],
                    ]}
                  />
                </div>
              ) : null}
            >
              <span className="di-main">
                <button
                  type="button"
                  className="di-nm di-pbs-open"
                  aria-expanded={expanded}
                  aria-controls={panelID(store.store)}
                  onClick={toggle}
                >
                  {store.store}
                </button>
                {problem ? (
                  <span className={`di-grey ${problem.tone}`}>{problem.text}</span>
                ) : (
                  <span className="di-grey">
                    <DiDots parts={[
                      store.comment,
                      t("groupsCount", { count: store.group_count ?? 0 }),
                      t("snapshotsCount", { count: store.snapshot_count ?? 0 }),
                    ]} />
                  </span>
                )}
              </span>
              <DiUsage pct={pct ?? null} used={store.used_bytes} total={store.total_bytes} />
              <DiRowMenu
                label={tk("actionsFor", { name: store.store })}
                items={[
                  { label: t("ds.verify"), disabled: working, onSelect: () => void act(store.store, "verify") },
                  {
                    label: t("ds.gc"),
                    disabled: working,
                    onSelect: () => ask({
                      title: t("ds.ask.gcTitle", { store: store.store }),
                      message: t("ds.ask.gcBody"),
                      confirmLabel: t("ds.ask.gcConfirm"),
                      onConfirm: () => act(store.store, "gc"),
                    }),
                  },
                  store.maintenance_mode
                    ? { label: t("ds.maintenanceOff"), disabled: working, onSelect: () => void act(store.store, "maintenance-disable") }
                    : {
                        label: t("ds.maintenanceOn"),
                        disabled: working,
                        onSelect: () => ask({
                          title: t("ds.ask.maintenanceTitle", { store: store.store }),
                          message: t("ds.ask.maintenanceBody"),
                          confirmLabel: t("ds.ask.maintenanceConfirm"),
                          danger: false,
                          onConfirm: () => act(store.store, "maintenance-enable"),
                        }),
                      },
                ]}
              />
            </DiRow>
          );
        })}
      </DiList>
      {dialog}
    </>
  );
}
