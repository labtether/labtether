"use client";

// Jobs: verification, prune, garbage collection and sync, one section each. Refresh reads them all again.

import { useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiNote, DiRow, DiSection } from "../proxmox/DiList";
import { ofParts } from "../proxmox/diFormat";
import type { PBSAssetDetailsResponse } from "../pbsTabModel";
import { JOB_COLS, JOB_COLS_S, PBSLastRun } from "./PBSLastRun";
import { PBSPruneJobs } from "./PBSPruneJobs";
import { PBSSyncJobs } from "./PBSSyncJobs";
import { PBSVerifyJobs } from "./PBSVerifyJobs";
import { newestTask, pbsStores, taskState } from "./pbsViewModel";
import { pbsAction, usePBSActions } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

type Props = {
  assetId: string;
  details: PBSAssetDetailsResponse | null;
  onRefreshDetails: () => void;
};

export function PBSJobsView({ assetId, details, onRefreshDetails }: Props) {
  const { tk } = usePBSWords();
  const [reloadKey, setReloadKey] = useState(0);
  const refresh = () => {
    setReloadKey((key) => key + 1);
    onRefreshDetails();
  };
  return (
    <>
      <PBSVerifyJobs
        assetId={assetId}
        reloadKey={reloadKey}
        right={<Button size="sm" variant="ghost" onClick={refresh}>{tk("refresh")}</Button>}
      />
      <PBSPruneJobs assetId={assetId} reloadKey={reloadKey} tasks={details?.tasks ?? []} />
      <PBSGcList assetId={assetId} details={details} onChanged={onRefreshDetails} />
      <PBSSyncJobs assetId={assetId} reloadKey={reloadKey} tasks={details?.tasks ?? []} />
    </>
  );
}

/** Garbage collection per datastore: use, the last run from the task list, and Run now after a confirm. */
function PBSGcList({ assetId, details, onChanged }: { assetId: string; details: PBSAssetDetailsResponse | null; onChanged: () => void }) {
  const { t, tk } = usePBSWords();
  const { busy, note, run } = usePBSActions();
  const { ask, dialog } = useDiConfirm();
  const stores = pbsStores(details);
  const tasks = details?.tasks ?? [];

  const runGc = (store: string) => run(
    `gc-${store}`,
    () => pbsAction(`/api/pbs/assets/${encodeURIComponent(assetId)}/datastores/${encodeURIComponent(store)}/gc`, "POST"),
    t("ds.done.gc", { store }),
    t("ds.failed.gc", { store }),
    onChanged,
  );

  return (
    <DiSection id="pbs-gc" title={t("jobs.gc")}>
      {note ? <DiNote tone={note.tone}>{note.text}</DiNote> : null}
      <DiList cols={JOB_COLS} colsS={JOB_COLS_S} hang label={t("jobs.gc")}>
        <DiCols>
          <span>{t("cols.datastore")}</span>
          <span className="r di-s">{t("cols.lastRun")}</span>
          <span />
        </DiCols>
        {stores.length === 0 ? <DiEmpty title={details ? t("noDatastores") : tk("loading")} /> : null}
        {stores.map((store) => {
          const last = newestTask(tasks, "gc", store.store);
          const state = last ? taskState(last) : null;
          const of = ofParts(store.used_bytes, store.total_bytes);
          return (
            <DiRow key={store.store} tone={state === "failed" ? "bad" : ""}>
              <DiMain
                name={store.store}
                grey={<DiDots parts={[of ? tk("of", of) : null]} />}
              />
              <PBSLastRun task={last} className="di-s" />
              <DiRowMenu
                label={tk("actionsFor", { name: store.store })}
                items={[{
                  label: t("jobs.runNowAsk"),
                  disabled: busy !== null || state === "running",
                  onSelect: () => ask({
                    title: t("ds.ask.gcTitle", { store: store.store }),
                    message: t("ds.ask.gcBody"),
                    confirmLabel: t("ds.ask.gcConfirm"),
                    onConfirm: () => runGc(store.store),
                  }),
                }]}
              />
            </DiRow>
          );
        })}
      </DiList>
      {dialog}
    </DiSection>
  );
}
