"use client";

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";
import type { StorageRow } from "./storageOperationsModel";
import { isHealthyHealth } from "./storageOperationsUtils";
import { DiRowMenu, type DiMenuItem } from "./proxmox/DiControls";
import { DiDots, DiMain, DiRow, DiUsage, type DiTone } from "./proxmox/DiList";
import { sizeText } from "./proxmox/diFormat";
import { useDaysWords } from "./StoragePoolExpandedDetails";

type StorageOperationsPoolRowProps = {
  row: StorageRow;
  isExpanded: boolean;
  onToggleExpanded: () => void;
  menu: DiMenuItem[];
  details: ReactNode;
};

/** Proxmox storage types by plugin name; "dir" is the only one with a word of its own. */
const TYPE_NAMES: Record<string, string> = {
  lvmthin: "LVM-Thin", lvm: "LVM", zfspool: "ZFS", zfs: "ZFS over iSCSI", nfs: "NFS", cifs: "SMB/CIFS", pbs: "Proxmox Backup Server",
  rbd: "Ceph RBD", cephfs: "CephFS", iscsi: "iSCSI", iscsidirect: "iSCSI", glusterfs: "GlusterFS", btrfs: "BTRFS", esxi: "ESXi",
};

/** A pool's tone: critical risk is a problem, action is a warning, watch and healthy show no dot. */
export function poolTone(row: StorageRow): DiTone {
  if (row.riskState === "critical") return "bad";
  if (row.riskState === "action") return "warn";
  return "";
}

/** One pool: name, type and the risk in words (in its tone), the meter, percent and "used of total". */
export function StorageOperationsPoolRow({ row, isExpanded, onToggleExpanded, menu, details }: StorageOperationsPoolRowProps) {
  const t = useTranslations("devices-proxmox.pools");
  const kit = useTranslations("devices-proxmox.kit");
  const daysWords = useDaysWords();
  const tone = poolTone(row);
  const total = row.allocBytes != null && row.freeBytes != null ? row.allocBytes + row.freeBytes : null;
  const reason = row.reason || (row.reasonKey && row.reasonKey !== "healthy" ? t(`reasons.${row.reasonKey}`, { health: row.health }) : "");
  const full = daysWords(row.daysToFull);
  const problem = tone && reason ? { tone: tone as "bad" | "warn", text: reason } : !isHealthyHealth(row.health) ? { tone: "bad" as const, text: row.health } : null;

  return (
    <DiRow tone={tone} onOpen={onToggleExpanded} selected={isExpanded} after={isExpanded ? details : null}>
      <DiMain
        name={row.poolName}
        problem={problem}
        grey={
          <DiDots
            parts={[
              row.typeLabel === "dir" ? t("directory") : TYPE_NAMES[row.typeLabel] ?? (row.typeLabel === "storage" ? null : row.typeLabel),
              row.freeBytes != null ? t("freeValue", { size: sizeText(row.freeBytes) }) : null,
              full ? t("fullIn", { days: full }) : null,
              row.stale ? t("staleData") : null,
            ]}
          />
        }
      />
      <DiUsage pct={row.usedPercent} used={row.allocBytes} total={total} />
      <DiRowMenu label={kit("actionsFor", { name: row.poolName })} items={menu} />
    </DiRow>
  );
}
