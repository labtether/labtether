"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { ConfirmDialog } from "../../../../components/ui/ConfirmDialog";
import type { Asset } from "../../../../console/models";
import { useAuth } from "../../../../contexts/AuthContext";
import { useSlowStatus, useStatusControls } from "../../../../contexts/StatusContext";
import { canAccessHostData, hasWriteRole } from "../../../../lib/roles";
import { NodeDeleteConfirmModal } from "./NodeDeleteConfirmModal";
import { NodeEditModal } from "./NodeEditModal";
import { deviceAddress } from "./deviceStatusModel";
import { panelHref } from "./deviceTabsModel";
import { useNodeDeleteFlow } from "./useNodeDeleteFlow";
import { copyText } from "../../../../lib/clipboard";

export type DeviceOffers = { terminal: boolean; files: boolean; desktop: boolean };
type Power = "restart" | "shutdown";

type DeviceMenuProps = {
  asset: Asset;
  offers: DeviceOffers;
  /** Connect views already shown as header buttons, left out of the menu. */
  skip?: Array<keyof DeviceOffers>;
  /** A list row adds Open; the header does not. */
  variant?: "head" | "row";
  /** Restart and shut down through the agent (the header passes these for agent devices). */
  onPower?: (action: Power) => void;
  className?: string;
};

/**
 * The device ⋯: connect views not on the header, Copy address, Edit…, Add connection…, Restart… and Shut down…
 * through the agent, and Remove…. Edit and Remove open their dialogs here, so the Devices list and the device
 * page share one menu.
 */
export function DeviceMenu({ asset, offers, skip = [], variant = "head", onPower, className }: DeviceMenuProps) {
  const t = useTranslations("devices.page.menu");
  const { user } = useAuth();
  const slowStatus = useSlowStatus();
  const { fetchStatus } = useStatusControls();
  const [editing, setEditing] = useState(false);
  const [power, setPower] = useState<Power | null>(null);
  const del = useNodeDeleteFlow({ nodeId: asset.id });

  const canWrite = hasWriteRole(user?.role);
  const hostData = canAccessHostData(user?.role);
  const address = deviceAddress(asset);
  const connect = (["terminal", "files", "desktop"] as const).filter((k) => offers[k] && hostData && !skip.includes(k));
  const label = variant === "row" ? t("rowLabel", { name: asset.name }) : t("label");
  // A viewer on a device with nothing to open or copy gets no empty menu.
  if (variant === "head" && !connect.length && !address && !canWrite) return null;

  // Menus and dialogs render in portals, but their clicks still bubble through React to a list row: stop them here.
  return (
    <span className="dv-menu-wrap" onClick={(e) => e.stopPropagation()}>
      <Menu.Root modal={false}>
        <Menu.Trigger className={className ?? "lt-icon-btn"} aria-label={label}>
          <MoreHorizontal aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            {variant === "row" ? (
              <Menu.Item className="lt-menu-item" asChild>
                <Link href={panelHref(asset.id, null)}>{t("open")}</Link>
              </Menu.Item>
            ) : null}
            {connect.map((k) => (
              <Menu.Item key={k} className="lt-menu-item" asChild>
                <Link href={panelHref(asset.id, k)}>{t(k)}</Link>
              </Menu.Item>
            ))}
            {address ? (
              <Menu.Item className="lt-menu-item" onSelect={() => void copyText(address).catch(() => {})}>
                {t("copyAddress")}
              </Menu.Item>
            ) : null}
            {canWrite ? (
              <>
                <Menu.Item className="lt-menu-item" onSelect={() => setEditing(true)}>{t("edit")}</Menu.Item>
                <Menu.Item className="lt-menu-item" asChild>
                  <Link href={`${panelHref(asset.id, "connect")}&adding=true`}>{t("addConnection")}</Link>
                </Menu.Item>
              </>
            ) : null}
            {canWrite && onPower ? (
              <>
                <Menu.Separator className="lt-menu-sep" />
                <Menu.Item className="lt-menu-item" onSelect={() => setPower("restart")}>{t("restart")}</Menu.Item>
                <Menu.Item className="lt-menu-item" onSelect={() => setPower("shutdown")}>{t("shutdown")}</Menu.Item>
              </>
            ) : null}
            {canWrite ? (
              <>
                <Menu.Separator className="lt-menu-sep" />
                <Menu.Item className="lt-menu-item dv-menu-danger" onSelect={del.openDeleteConfirm}>{t("remove")}</Menu.Item>
              </>
            ) : null}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>

      {editing ? (
        <NodeEditModal asset={asset} groups={slowStatus?.groups ?? []} onClose={() => setEditing(false)} onSave={() => void fetchStatus()} />
      ) : null}
      <NodeDeleteConfirmModal
        open={del.showDeleteConfirm}
        assetName={asset.name}
        confirmInput={del.deleteConfirmInput}
        deleting={del.deleting}
        error={del.deleteError}
        onConfirmInputChange={del.setDeleteConfirmInput}
        onCancel={del.cancelDeleteConfirm}
        onConfirm={() => void del.confirmDelete()}
      />
      {onPower ? (
        <ConfirmDialog
          open={power !== null}
          onClose={() => setPower(null)}
          onConfirm={() => {
            if (power) onPower(power);
            setPower(null);
          }}
          title={t(power === "shutdown" ? "shutdownTitle" : "restartTitle", { name: asset.name })}
          message={t(power === "shutdown" ? "shutdownBody" : "restartBody", { name: asset.name })}
          confirmLabel={t(power === "shutdown" ? "shutdownConfirm" : "restartConfirm")}
          danger
        />
      ) : null}
    </span>
  );
}
