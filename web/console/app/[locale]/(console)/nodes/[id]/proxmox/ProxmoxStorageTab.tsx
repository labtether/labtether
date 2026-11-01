"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import type { ProxmoxDetails } from "../nodeDetailTypes";
import { DiCols, DiDots, DiEmpty, DiFoot, DiList, DiMain, DiNum, DiRow, DiSection, DiTime, DiUsage } from "./DiList";
import { percentOf, sizeParts, sizeText, whenText } from "./diFormat";

type Props = {
  proxmoxDetails: ProxmoxDetails;
};

const CONTENT_PREVIEW = 25;
const CONTENT_KINDS = new Set(["iso", "vztmpl", "backup", "images", "rootdir", "snippets", "import"]);

/** ZFS pools on the node (use, health, fragmentation) and the volumes Proxmox lists in its storage. */
export function ProxmoxStorageTab({ proxmoxDetails }: Props) {
  const t = useTranslations("devices-proxmox.pve.storage");
  const kit = useTranslations("devices-proxmox.kit");
  const locale = useLocale();
  const [showAll, setShowAll] = useState(false);
  const pools = proxmoxDetails.zfs_pools ?? [];
  const content = [...(proxmoxDetails.storage_content ?? [])].sort((left, right) => (right.ctime ?? 0) - (left.ctime ?? 0));
  const shown = showAll ? content : content.slice(0, CONTENT_PREVIEW);

  return (
    <>
      <DiSection title={t("zfsPools")}>
        <DiList cols="minmax(0, 1fr) minmax(48px, 140px) 48px 112px" colsM="minmax(0, 1fr) 48px 112px" colsS="minmax(0, 1fr) 48px" label={t("zfsPools")}>
          <DiCols>
            <span>{t("pool")}</span>
            <span className="di-m">{t("used")}</span>
            <span />
            <span className="di-s" />
          </DiCols>
          {pools.length === 0 ? <DiEmpty title={t("noPools")} /> : null}
          {pools.map((pool, index) => {
            const health = (pool.health ?? "").toUpperCase();
            const bad = health !== "" && health !== "ONLINE";
            return (
              <DiRow key={pool.name || index} tone={bad ? (health === "DEGRADED" ? "warn" : "bad") : ""}>
                <DiMain
                  name={pool.name || "—"}
                  grey={
                    <DiDots
                      parts={[
                        pool.free != null ? t("free", { size: sizeText(pool.free) }) : null,
                        pool.frag != null ? t("frag", { pct: pool.frag }) : null,
                        pool.dedup != null && pool.dedup !== 1 ? t("dedup", { ratio: Number(pool.dedup.toFixed(2)) }) : null,
                      ]}
                    />
                  }
                  problem={bad ? { tone: health === "DEGRADED" ? "warn" : "bad", text: health } : null}
                />
                <DiUsage pct={percentOf(pool.alloc, pool.size)} used={pool.alloc} total={pool.size} />
              </DiRow>
            );
          })}
        </DiList>
      </DiSection>

      <DiSection title={t("content")}>
        <DiList cols="minmax(0, 1fr) 72px 96px" colsS="minmax(0, 1fr) auto" label={t("content")}>
          <DiCols>
            <span>{t("volume")}</span>
            <span className="r">{t("size")}</span>
            <span className="r di-s">{t("created")}</span>
          </DiCols>
          {content.length === 0 ? <DiEmpty title={t("noContent")} /> : null}
          {shown.map((item, index) => {
            const size = sizeParts(item.size);
            return (
              <DiRow key={item.volid || index}>
                <DiMain
                  name={item.volid || "—"}
                  mono
                  grey={
                    <DiDots
                      parts={[
                        item.content ? (CONTENT_KINDS.has(item.content) ? t(`kinds.${item.content}`) : item.content) : null,
                        item.format || null,
                        item.vmid ? t("guest", { vmid: item.vmid }) : null,
                        item.notes || null,
                      ]}
                    />
                  }
                />
                <DiNum value={size?.value} unit={size?.unit} />
                <DiTime className="di-s">{whenText(item.ctime, locale)}</DiTime>
              </DiRow>
            );
          })}
          {content.length > CONTENT_PREVIEW ? (
            <DiFoot
              left={kit("shown", { shown: shown.length, total: content.length })}
              right={<button type="button" onClick={() => setShowAll((value) => !value)}>{showAll ? kit("showFewer") : kit("showAll")}</button>}
            />
          ) : null}
        </DiList>
      </DiSection>
    </>
  );
}
