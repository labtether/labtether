"use client";

import { useTranslations } from "next-intl";
import { Link } from "../../../../../../i18n/navigation";
import { useAuth } from "../../../../../contexts/AuthContext";
import { canAccessHostData } from "../../../../../lib/roles";
import { DiEmpty, DiKV, DiSheet } from "./DiList";

type Props = {
  assetId: string;
  proxmoxNode: string;
  proxmoxVMID: string;
  effectiveKind: string;
};

/** The guest's console: what kind it is and a button that opens it through the hub's desktop session. */
export function ProxmoxConsoleTab({ assetId, proxmoxNode, proxmoxVMID, effectiveKind }: Props) {
  const t = useTranslations("devices-proxmox.pve.console");
  const { user } = useAuth();
  // The console opens a desktop session, which the hub keeps for operators.
  const consoleURL = assetId && canAccessHostData(user?.role)
    ? `/nodes/${encodeURIComponent(assetId)}?panel=desktop`
    : null;
  const type = effectiveKind === "qemu" ? t("vnc") : effectiveKind === "lxc" ? t("lxc") : t("generic");

  if (!proxmoxNode || !proxmoxVMID) {
    return (
      <DiSheet>
        <DiEmpty title={t("unavailable")} line={t("needsIds")} />
      </DiSheet>
    );
  }

  return (
    <DiSheet kind="tight" className="di-pve-console">
      <DiKV items={[[t("type"), type], [t("node"), proxmoxNode], [t("vmid"), proxmoxVMID]]} />
      {consoleURL ? (
        <div className="di-acts di-pve-console-acts">
          <Link href={consoleURL} className="di-btn">{t("open")}</Link>
        </div>
      ) : null}
    </DiSheet>
  );
}
