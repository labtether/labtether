"use client";

import { useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiList, DiMain, DiRow } from "./DiList";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

/** Backup jobs (vzdump schedules): when, where to, which guests, mode and retention; a job that is off says so. */
export function ProxmoxBackupTab({ proxmoxDetails }: Props) {
  const t = useTranslations("devices-proxmox.pve.backup");
  const schedules = proxmoxDetails.backup_schedules ?? [];

  return (
    <DiList cols="minmax(0, 1fr) 64px" label={t("label")}>
      <DiCols>
        <span>{t("job")}</span>
        <span className="r">{t("state")}</span>
      </DiCols>
      {schedules.length === 0 ? <DiEmpty title={t("none")} /> : null}
      {schedules.map((schedule, index) => (
        <DiRow key={schedule.id || index} tone={schedule.enabled ? "" : "off"}>
          <DiMain
            name={schedule.schedule || t("noSchedule")}
            grey={
              <DiDots
                parts={[
                  schedule.storage ? t("to", { storage: schedule.storage }) : null,
                  schedule.vmid ? t("guests", { list: schedule.vmid }) : schedule.pool ? t("pool", { pool: schedule.pool }) : t("allGuests"),
                  schedule.exclude ? t("except", { list: schedule.exclude }) : null,
                  schedule.node ? t("onNode", { node: schedule.node }) : null,
                  schedule.mode ? t("mode", { mode: schedule.mode }) : null,
                  schedule.compress ? t("compress", { compress: schedule.compress }) : null,
                  schedule.maxfiles != null ? t("keep", { count: schedule.maxfiles }) : null,
                  schedule.comment || null,
                ]}
              />
            }
          />
          <span className={`di-num${schedule.enabled ? "" : " none"}`}>{schedule.enabled ? t("on") : t("off")}</span>
        </DiRow>
      ))}
    </DiList>
  );
}
