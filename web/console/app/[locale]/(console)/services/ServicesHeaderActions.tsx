"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal, Plus, RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../components/ui/Button";
import { useRouter } from "../../../../i18n/navigation";
import { useCanWrite } from "../../../hooks/useCanWrite";

interface ServicesHeaderActionsProps {
  adding: boolean;
  arranging: boolean;
  editCount: number;
  bulkSaving: boolean;
  syncing: boolean;
  pullingImages: boolean;
  pullCount: number;
  onAdd: () => void;
  onEditShown: () => void;
  onToggleArrange: () => void;
  onRefresh: () => void | Promise<void>;
  onPullImages: () => void;
}

/** Refresh, Add service (the page's one rose button) and ⋯ with editing, image pulls, arranging and settings. */
export function ServicesHeaderActions({
  adding,
  arranging,
  editCount,
  bulkSaving,
  syncing,
  pullingImages,
  pullCount,
  onAdd,
  onEditShown,
  onToggleArrange,
  onRefresh,
  onPullImages,
}: ServicesHeaderActionsProps) {
  const t = useTranslations("services.actions");
  const router = useRouter();
  // A viewer keeps Refresh and the links to settings; adding, editing, pulling and arranging are changes.
  const canWrite = useCanWrite();
  return (
    <div className="flex items-center gap-2">
      <button type="button" className="lt-icon-btn" onClick={() => void onRefresh()} disabled={syncing} aria-label={t("refresh")} title={t("refresh")}>
        <RefreshCw aria-hidden="true" strokeWidth={2} className={syncing ? "animate-spin" : undefined} />
      </button>
      {canWrite ? (
        <Button variant={adding ? "secondary" : "primary"} onClick={onAdd} aria-expanded={adding}>
          <Plus size={14} aria-hidden="true" />
          {t("add")}
        </Button>
      ) : null}
      <Menu.Root>
        <Menu.Trigger className="lt-icon-btn" aria-label={t("more")}>
          <MoreHorizontal aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu sv-menu" align="end" sideOffset={6}>
            {canWrite ? (
              <>
                <Menu.Item className="lt-menu-item" disabled={editCount === 0 || bulkSaving} onSelect={onEditShown}>
                  {t("editShown", { count: editCount })}
                </Menu.Item>
                <Menu.Item className="lt-menu-item" disabled={pullCount === 0 || pullingImages} onSelect={onPullImages}>
                  {pullingImages ? t("pulling") : pullCount > 0 ? t("pullImages", { count: pullCount }) : t("noImages")}
                </Menu.Item>
                <Menu.Item className="lt-menu-item" onSelect={onToggleArrange}>
                  {arranging ? t("doneArranging") : t("arrange")}
                </Menu.Item>
                <Menu.Separator className="lt-menu-sep" />
              </>
            ) : null}
            <Menu.Item className="lt-menu-item" onSelect={() => router.push("/services/config")}>{t("settings")}</Menu.Item>
            <Menu.Item className="lt-menu-item" onSelect={() => router.push("/services/config?tab=discovery")}>{t("discovery")}</Menu.Item>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
