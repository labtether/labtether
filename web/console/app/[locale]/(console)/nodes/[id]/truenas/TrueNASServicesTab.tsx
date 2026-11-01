"use client";

// TrueNAS services: file sharing and remote access first. The state reads Running or Stopped (amber when the service
// should start on boot); a state TrueNAS reports that is neither shows as it came, and then Start and Stop wait.
// Start and Restart sit behind ⋯; Stop asks first.

import { useTranslations } from "next-intl";
import { DiCols, DiDots, DiList, DiMain, DiRow } from "../proxmox/DiList";
import { DiRowMenu, useDiConfirm } from "../proxmox/DiControls";
import { failureText, serviceState, sortServices } from "./truenasModel";
import { TnFailure, TnListState, TnWord } from "./TrueNASParts";
import type { TrueNASService } from "./truenasTypes";
import { useTrueNASAction } from "./useTrueNASAction";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type { TrueNASService } from "./truenasTypes";

type Verb = "start" | "stop" | "restart";

export function TrueNASServicesTab({ assetId }: { assetId: string }) {
  const t = useTranslations("devices-truenas.services");
  const k = useTranslations("devices-proxmox.kit");
  const { data: services, loading, error, refresh } = useTrueNASList<TrueNASService>(assetId, "services");
  const action = useTrueNASAction();
  const { ask, dialog } = useDiConfirm();

  const run = (svc: TrueNASService, verb: Verb) => action.run(`${verb}:${svc.name}`, () => truenasAction(
    `/api/truenas/assets/${encodeURIComponent(assetId)}/services/${encodeURIComponent(svc.name)}/${verb}`,
    "POST",
  ), refresh);

  const stop = (svc: TrueNASService) => ask({
    title: t("stopTitle", { name: svc.name }),
    message: t("stopBody", { name: svc.name }),
    confirmLabel: t("stopConfirm"),
    onConfirm: () => run(svc, "stop"),
  });

  const failure = action.error
    ? t(`${action.error.key.split(":")[0] as Verb}Failed`, {
        name: action.error.key.split(":").slice(1).join(":"),
        reason: failureText(action.error.message, t("noReason")),
      })
    : null;

  return (
    <>
      <TnFailure message={failure} />
      <DiList label={t("label")} cols="minmax(0, 1fr) 120px 28px" colsS="minmax(0, 1fr) 96px 28px">
        <DiCols>
          <span>{t("colService")}</span>
          <span>{t("colState")}</span>
          <span />
        </DiCols>
        <TnListState loading={loading} error={error} count={services.length} empty={t("empty")} onRetry={refresh} failed={(reason) => t("loadFailed", { reason })} />
        {sortServices(services).map((svc) => {
          const state = serviceState(svc);
          const word = state.word === "other" ? (state.raw ?? "") : t(state.word);
          const busy = action.busy !== null;
          return (
            <DiRow key={svc.name}>
              <DiMain name={svc.name} grey={<DiDots parts={[svc.enabled ? t("onBoot") : null, svc.uptime]} />} />
              <TnWord tone={state.tone} word={word} />
              <DiRowMenu
                label={k("actionsFor", { name: svc.name })}
                items={[
                  { label: t("start"), disabled: busy || svc.running !== false, onSelect: () => { void run(svc, "start"); } },
                  { label: t("restart"), disabled: busy, onSelect: () => { void run(svc, "restart"); } },
                  { separator: true },
                  { label: t("stop"), danger: true, disabled: busy || svc.running !== true, onSelect: () => stop(svc) },
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
