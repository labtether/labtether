"use client";

// VMs on TrueNAS SCALE: name, whether it starts on boot, the power state, vCPUs and memory. Start sits behind ⋯;
// Restart and Stop ask first. A state that is neither running nor stopped keeps Start and Stop waiting.

import { useTranslations } from "next-intl";
import { DiCols, DiEmpty, DiList, DiMain, DiNum, DiRow } from "../proxmox/DiList";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { sizeParts } from "../proxmox/diFormat";
import { failureText, vmState } from "./truenasModel";
import { TnFailure, TnListState, TnWord } from "./TrueNASParts";
import type { TrueNASVM } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASVM } from "./truenasTypes";

type Verb = "start" | "stop" | "restart";

/** TrueNAS reports VM memory in MiB. */
function memoryParts(mib?: number) {
  return typeof mib === "number" ? sizeParts(mib * 1024 * 1024) : null;
}

export function TrueNASVMsTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.vms");
  const k = useTranslations("devices-proxmox.kit");
  const { data: vms, loading, error, refresh } = useTrueNASList<TrueNASVM>(assetId, "vms");
  const action = useTrueNASAction();
  const { ask, dialog } = useDiConfirm();

  const run = (vm: TrueNASVM, verb: Verb) => action.run(`${verb}:${vm.name}`, () => truenasAction(
    `/api/truenas/assets/${encodeURIComponent(assetId)}/vms/${encodeURIComponent(String(vm.id))}/${verb}`,
    "POST",
  ), refresh);

  const confirm = (vm: TrueNASVM, verb: "stop" | "restart") => ask({
    title: t(`${verb}Title`, { name: vm.name }),
    message: t(`${verb}Body`, { name: vm.name }),
    confirmLabel: t(`${verb}Confirm`),
    danger: verb === "stop",
    onConfirm: () => run(vm, verb),
  });

  const scaleOnly = !!error && /scale|core/i.test(error);
  if (scaleOnly) {
    return <DiList cols="minmax(0, 1fr)"><DiEmpty title={t("scaleOnly")} /></DiList>;
  }

  const failure = action.error
    ? t(`${action.error.key.split(":")[0] as Verb}Failed`, {
        name: action.error.key.split(":").slice(1).join(":"),
        reason: failureText(action.error.message, t("noReason")),
      })
    : null;

  return (
    <>
      <TnFailure message={failure} />
      <DiList
        label={t("label")}
        cols="minmax(0, 1fr) 120px 64px 80px 28px"
        colsM="minmax(0, 1fr) 120px 80px 28px"
        colsS="minmax(0, 1fr) 96px 28px"
      >
        <DiCols>
          <span>{t("colVm")}</span>
          <span>{t("colState")}</span>
          <span className="r di-m">{t("colCpu")}</span>
          <span className="r di-s">{t("colMemory")}</span>
          <span />
        </DiCols>
        <TnListState loading={loading} error={error} count={vms.length} empty={t("empty")} onRetry={refresh} failed={(reason) => t("loadFailed", { reason })} />
        {vms.map((vm) => {
          const state = vmState(vm.status);
          const word = state.word === "other" ? (state.raw ?? "") : t(state.word);
          const memory = memoryParts(vm.memory);
          const busy = action.busy !== null;
          const running = state.word === "running";
          return (
            <DiRow key={String(vm.id)}>
              <DiMain name={vm.name} grey={vm.autostart ? t("onBoot") : undefined} />
              <TnWord tone={state.tone} word={word} />
              <DiNum value={vm.vcpus} className="di-m" />
              <DiNum value={memory?.value} unit={memory?.unit} className="di-s" />
              <DiRowMenu
                label={k("actionsFor", { name: vm.name })}
                items={[
                  { label: t("start"), disabled: busy || state.word !== "stopped", onSelect: () => { void run(vm, "start"); } },
                  { label: t("restart"), disabled: busy, onSelect: () => confirm(vm, "restart") },
                  { separator: true },
                  { label: t("stop"), danger: true, disabled: busy || !running, onSelect: () => confirm(vm, "stop") },
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
