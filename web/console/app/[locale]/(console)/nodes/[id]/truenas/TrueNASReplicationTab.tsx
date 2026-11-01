"use client";

// Replication tasks: the name, then source to target, direction, transport and schedule in grey; the last run as a
// word and a time. Run now sits behind ⋯.

import { useLocale, useTranslations } from "next-intl";
import { DiCols, DiDots, DiList, DiMain, DiRow } from "../proxmox/DiList";
import { DiRowMenu } from "../proxmox/DiControls";
import { whenText } from "../proxmox/diFormat";
import { failureText, replicationState } from "./truenasModel";
import { TnFailure, TnListState, TnWord } from "./TrueNASParts";
import type { TrueNASReplicationTask } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASReplicationTask } from "./truenasTypes";

export function TrueNASReplicationTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.replication");
  const k = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const { data: tasks, loading, error, refresh } = useTrueNASList<TrueNASReplicationTask>(assetId, "replication");
  const action = useTrueNASAction();

  const run = (task: TrueNASReplicationTask) => {
    void action.run(task.name, () => truenasAction(
      `/api/truenas/assets/${encodeURIComponent(assetId)}/replication/${encodeURIComponent(String(task.id))}/run`,
      "POST",
    ), refresh);
  };

  const lastRun = (task: TrueNASReplicationTask) => {
    const state = replicationState(task.last_run_state, task.last_run);
    const word = state.word === "other" ? (state.raw ?? "") : t(`state.${state.word}`);
    return <TnWord tone={state.tone} word={word} time={whenText(task.last_run, locale)} />;
  };

  const grey = (task: TrueNASReplicationTask) => {
    const source = task.source_datasets?.join(", ");
    const route = source && task.target_dataset ? t("route", { from: source, to: task.target_dataset }) : source || task.target_dataset;
    return (
      <DiDots
        parts={[
          task.enabled ? null : t("off"),
          route ? <span className="di-mono">{route}</span> : null,
          task.direction,
          task.transport,
          task.schedule ? <span className="di-mono">{task.schedule}</span> : null,
        ]}
      />
    );
  };

  return (
    <>
      <TnFailure message={action.error ? t("runFailed", { name: action.error.key, reason: failureText(action.error.message, t("noReason")) }) : null} />
      <DiList label={t("label")} cols="minmax(0, 1fr) 160px 28px" colsS="minmax(0, 1fr) 28px">
        <DiCols>
          <span>{t("colTask")}</span>
          <span className="di-s">{t("colLast")}</span>
          <span />
        </DiCols>
        <TnListState loading={loading} error={error} count={tasks.length} empty={t("empty")} onRetry={refresh} failed={(reason) => t("loadFailed", { reason })} />
        {tasks.map((task) => (
          <DiRow key={String(task.id)}>
            <DiMain name={task.name} plain={!task.enabled} grey={grey(task)} />
            <span className="di-s">{lastRun(task)}</span>
            <DiRowMenu
              label={k("actionsFor", { name: task.name })}
              items={[{ label: t("runNow"), disabled: action.busy !== null, onSelect: () => run(task) }]}
            />
          </DiRow>
        ))}
      </DiList>
    </>
  );
}
