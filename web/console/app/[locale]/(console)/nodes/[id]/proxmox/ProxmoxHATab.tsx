"use client";

import { useTranslations } from "next-intl";
import type { ProxmoxDetails, ProxmoxHAResource } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiKV, DiList, DiMain, DiRow, DiSection, DiSheet, DiStatus, type DiTone } from "./DiList";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

/** A HA state's tone: errors and fences are problems, a stopped or disabled resource is off. */
function haTone(state?: string): DiTone {
  const value = (state ?? "").toLowerCase();
  if (value.includes("error") || value.includes("fence")) return "bad";
  if (value.includes("recover") || value.includes("migrate") || value.includes("relocate") || value.includes("freeze")) return "warn";
  if (value === "stopped" || value === "disabled" || value === "ignored") return "off";
  return "";
}

function HAState({ state }: { state?: string }) {
  if (!state) return <span className="di-num none">—</span>;
  const tone = haTone(state);
  if (tone === "bad" || tone === "warn") return <DiStatus tone={tone} word={state} />;
  return <span className={`di-num${tone === "off" ? " none" : ""}`}>{state}</span>;
}

/** This resource's HA settings, then every HA resource in the cluster with its group, node and state. */
export function ProxmoxHATab({ proxmoxDetails }: Props) {
  const t = useTranslations("devices-proxmox.pve.ha");
  const match = proxmoxDetails.ha?.match;
  const resources: ProxmoxHAResource[] = proxmoxDetails.ha?.resources ?? [];

  return (
    <>
      <DiSection title={t("thisResource")}>
        <DiSheet kind="tight">
          {match ? (
            <DiKV
              columns={2}
              items={[
                [t("resource"), match.sid ? <span className="di-mono">{match.sid}</span> : null],
                [t("state"), match.state ? <HAState state={match.state} /> : null],
                [t("status"), match.status || null],
                [t("group"), match.group || t("noGroup")],
                [t("node"), match.node || null],
                [t("maxRestart"), match.max_restart != null ? String(match.max_restart) : null],
                [t("maxRelocate"), match.max_relocate != null ? String(match.max_relocate) : null],
                [t("comment"), match.comment || null],
              ]}
            />
          ) : (
            <DiEmpty title={t("notManaged")} />
          )}
        </DiSheet>
      </DiSection>

      <DiSection title={t("resources")}>
        <DiList cols="minmax(0, 1fr) 120px" label={t("resources")}>
          <DiCols>
            <span>{t("resource")}</span>
            <span className="r">{t("state")}</span>
          </DiCols>
          {resources.length === 0 ? <DiEmpty title={t("none")} /> : null}
          {resources.map((resource, index) => (
            <DiRow key={resource.sid ?? index}>
              <DiMain
                name={resource.sid || "—"}
                mono
                grey={<DiDots parts={[resource.node ? t("on", { node: resource.node }) : null, resource.group || t("noGroup"), resource.status || null]} />}
              />
              <HAState state={resource.state} />
            </DiRow>
          ))}
        </DiList>
      </DiSection>
    </>
  );
}
