"use client";

import { useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiKV, DiList, DiMain, DiMeter, DiNum, DiRow, DiSection, DiSheet, DiStatus, type DiTone } from "./DiList";
import { ofParts, percentOf, sizeText, usageTone } from "./diFormat";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

function healthTone(status: string): DiTone {
  if (status === "HEALTH_OK") return "ok";
  if (status === "HEALTH_WARN") return "warn";
  return "bad";
}

function pgTone(state: string): DiTone {
  if (state.includes("active+clean")) return "";
  if (state.includes("degraded") || state.includes("recovering") || state.includes("backfill") || state.includes("peering")) return "warn";
  if (state.includes("down") || state.includes("incomplete") || state.includes("stale") || state.includes("inconsistent")) return "bad";
  return "";
}

/** Ceph health, space and monitors, placement group states and the OSDs with host, class and weight. */
export function ProxmoxCephTab({ proxmoxDetails }: Props) {
  const t = useTranslations("devices-proxmox.pve.ceph");
  const kit = useTranslations("devices-proxmox.kit");
  const status = proxmoxDetails.ceph_status;
  const health = status?.health?.status ?? "";
  const pg = status?.pgmap;
  const pct = percentOf(pg?.bytes_used, pg?.bytes_total);
  const of = ofParts(pg?.bytes_used, pg?.bytes_total);
  const mons = status?.monmap?.mons ?? [];
  const osds = proxmoxDetails.ceph_osds ?? [];
  const pgs = pg?.pgs_by_state ?? [];
  const healthWord = health === "HEALTH_OK" ? t("ok") : health === "HEALTH_WARN" ? t("warn") : health === "HEALTH_ERR" ? t("err") : health;

  return (
    <>
      {health ? (
        <DiSection title={t("cluster")}>
          <DiSheet kind="tight">
            <DiKV
              columns={2}
              items={[
                [t("health"), <DiStatus key="h" tone={healthTone(health)} word={healthWord} />],
                [t("used"), of ? (
                  <span key="u" className="di-pve-use">
                    <DiMeter pct={pct} hot={usageTone(pct) === "warn"} />
                    <span>{kit("of", of)}</span>
                  </span>
                ) : null],
                [t("available"), sizeText(pg?.bytes_avail) || null],
                [t("stored"), sizeText(pg?.data_bytes) || null],
                [t("monitors"), mons.length ? mons.map((mon) => mon.name).join(", ") : null],
              ]}
            />
          </DiSheet>
        </DiSection>
      ) : null}

      {pgs.length > 0 ? (
        <DiSection title={t("placementGroups")}>
          <DiList cols="minmax(0, 1fr) 72px" label={t("placementGroups")}>
            <DiCols>
              <span>{t("pgState")}</span>
              <span className="r">{t("count")}</span>
            </DiCols>
            {pgs.map((entry, index) => (
              <DiRow key={`${entry.state_name}-${index}`} tone={pgTone(entry.state_name)}>
                <DiMain name={entry.state_name} mono />
                <DiNum value={entry.count} />
              </DiRow>
            ))}
          </DiList>
        </DiSection>
      ) : null}

      <DiSection title={t("osds")}>
        <DiList cols="minmax(0, 1fr) 72px 64px" colsS="minmax(0, 1fr) auto" label={t("osds")}>
          <DiCols>
            <span>{t("osd")}</span>
            <span className="r di-s">{t("weight")}</span>
            <span className="r">{t("state")}</span>
          </DiCols>
          {osds.length === 0 ? <DiEmpty title={t("noOsds")} /> : null}
          {osds.map((osd, index) => {
            const up = osd.status === "up";
            return (
              <DiRow key={osd.id ?? index} tone={osd.status && !up ? "bad" : ""}>
                <DiMain name={osd.name || `osd.${osd.id ?? index}`} grey={<DiDots parts={[osd.host || null, osd.device_class || null]} />} />
                <DiNum className="di-s" value={osd.crush_weight != null ? osd.crush_weight.toFixed(4) : undefined} />
                {up ? <span className="di-num">{t("up")}</span> : <DiStatus tone="bad" word={osd.status || t("unknown")} />}
              </DiRow>
            );
          })}
        </DiList>
      </DiSection>
    </>
  );
}
