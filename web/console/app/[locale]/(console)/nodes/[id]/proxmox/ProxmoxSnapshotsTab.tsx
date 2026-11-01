"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { DiRowMenu, useDiConfirm, type DiMenuItem } from "./DiControls";
import { DiCols, DiDots, DiEmpty, DiFoot, DiList, DiMain, DiRow, DiTime } from "./DiList";
import { whenText } from "./diFormat";

type Props = {
  proxmoxDetails: ProxmoxDetails;
  effectiveKind: string;
  canRunProxmoxQuickActions: boolean;
  proxmoxActionRunning: boolean;
  onRunProxmoxQuickAction: (actionID: string, params?: Record<string, string>) => void;
};

const PREVIEW = 20;

/** The guest's snapshots, newest first, with Roll back and Delete behind ⋯ (both ask first). */
export function ProxmoxSnapshotsTab({
  proxmoxDetails,
  effectiveKind,
  canRunProxmoxQuickActions,
  proxmoxActionRunning,
  onRunProxmoxQuickAction,
}: Props) {
  const t = useTranslations("devices-proxmox.pve.snapshots");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const { ask, dialog } = useDiConfirm();
  const [showAll, setShowAll] = useState(false);
  const prefix = effectiveKind === "lxc" ? "ct" : "vm";

  const all = [...(proxmoxDetails.snapshots ?? [])]
    .filter((snapshot) => snapshot.name !== "current")
    .sort((left, right) => (right.snaptime ?? 0) - (left.snaptime ?? 0));
  const shown = showAll ? all : all.slice(0, PREVIEW);

  if (all.length === 0) {
    return (
      <DiList cols="minmax(0, 1fr)">
        <DiEmpty title={t("none")} />
      </DiList>
    );
  }

  const menu = (name: string, when: string): DiMenuItem[] => [
    {
      label: t("rollBack"),
      disabled: proxmoxActionRunning,
      onSelect: () => ask({
        title: t("rollBackTitle", { name }),
        message: when ? t("rollBackBody", { when }) : t("rollBackBodyNoTime"),
        confirmLabel: t("rollBackConfirm"),
        danger: true,
        onConfirm: () => onRunProxmoxQuickAction(`${prefix}.snapshot.rollback`, { snapshot_name: name }),
      }),
    },
    { separator: true },
    {
      label: t("delete"),
      danger: true,
      disabled: proxmoxActionRunning,
      onSelect: () => ask({
        title: t("deleteTitle", { name }),
        message: t("deleteBody"),
        confirmLabel: t("deleteConfirm"),
        danger: true,
        onConfirm: () => onRunProxmoxQuickAction(`${prefix}.snapshot.delete`, { snapshot_name: name }),
      }),
    },
  ];

  return (
    <>
      <DiList cols="minmax(0, 1fr) 112px 28px" colsS="minmax(0, 1fr) auto 28px" label={t("label")}>
        <DiCols>
          <span>{t("snapshot")}</span>
          <span className="r">{t("taken")}</span>
          <span />
        </DiCols>
        {shown.map((snapshot, index) => {
          const name = snapshot.name || "";
          const when = whenText(snapshot.snaptime, locale);
          return (
            <DiRow key={`${name}-${snapshot.snaptime ?? index}`}>
              <DiMain
                name={name || t("unnamed")}
                grey={
                  <DiDots
                    parts={[
                      snapshot.vmstate ? t("withRam") : null,
                      snapshot.parent ? t("after", { parent: snapshot.parent }) : null,
                      snapshot.description?.trim() || null,
                    ]}
                  />
                }
              />
              <DiTime>{when}</DiTime>
              {canRunProxmoxQuickActions && name ? (
                <DiRowMenu label={kit("actionsFor", { name })} items={menu(name, when)} />
              ) : (
                <span />
              )}
            </DiRow>
          );
        })}
        {all.length > PREVIEW ? (
          <DiFoot
            left={kit("shown", { shown: shown.length, total: all.length })}
            right={<button type="button" onClick={() => setShowAll((value) => !value)}>{showAll ? kit("showFewer") : kit("showAll")}</button>}
          />
        ) : null}
      </DiList>
      {dialog}
    </>
  );
}
