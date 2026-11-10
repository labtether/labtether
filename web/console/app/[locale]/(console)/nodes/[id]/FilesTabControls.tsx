"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { MoreHorizontal, Search, Upload } from "lucide-react";
import { useTranslations } from "next-intl";
import { FilesPathBar, type Place } from "../../files/FilesPathBar";
import { ToolMenu } from "../../files/FileMenus";
import { ClipChip } from "../../files/FilesToolbar";
import type { WorkspaceClipboard } from "../../files/useFileWorkspaceState";

type FilesTabControlsProps = {
  currentPath: string;
  pathInput: string;
  onPathInputChange: (next: string) => void;
  onNavigateToPath: (path: string) => void;
  onNavigateUp: () => void;
  disableNavigateUp: boolean;
  places: Place[];
  canBack: boolean;
  onBack: () => void;
  busy: boolean;
  query: string;
  onQuery: (value: string) => void;
  clipboard: WorkspaceClipboard | null;
  onPaste: () => void;
  onClearClipboard: () => void;
  showHidden: boolean;
  onShowHiddenChange: (next: boolean) => void;
  onRefresh: () => void;
  writeEnabled: boolean;
  onToggleWriteActions: () => void;
  onNewFolder: () => void;
  onUpload: () => void;
};

/**
 * The toolbar of a device's Files tab: Back, Up and the path (type one to jump), search in this folder, the
 * clipboard chip, the ⋯ menu and, once changes are allowed, Upload. Until then the tab is read-only and offers
 * "Allow changes…".
 */
export function FilesTabControls(p: FilesTabControlsProps) {
  const t = useTranslations("files");
  return (
    <div className="fx-bar">
      <FilesPathBar
        path={p.currentPath}
        places={p.places}
        draft={p.pathInput}
        onDraft={p.onPathInputChange}
        onGo={p.onNavigateToPath}
        canUp={!p.disableNavigateUp}
        onUp={p.onNavigateUp}
        canBack={p.canBack}
        onBack={p.onBack}
        busy={p.busy}
        idSuffix="-dev"
      />
      <label className="fx-q">
        <Search aria-hidden="true" width={14} height={14} strokeWidth={2} />
        <input type="search" value={p.query} placeholder={t("bar.search")} aria-label={t("bar.search")} onChange={(e) => p.onQuery(e.target.value)} />
      </label>
      <ClipChip clipboard={p.clipboard} canPaste={p.writeEnabled} onPaste={p.onPaste} onClear={p.onClearClipboard} />
      <ToolMenu
        label={t("bar.more")}
        trigger={
          <button type="button" className="fx-ib" title={t("bar.more")}>
            <MoreHorizontal aria-hidden="true" width={16} height={16} />
          </button>
        }
      >
        {p.writeEnabled ? <Menu.Item className="lt-menu-item" onSelect={p.onNewFolder}>{t("bar.newFolder")}</Menu.Item> : null}
        <Menu.Item className="lt-menu-item" onSelect={() => p.onShowHiddenChange(!p.showHidden)}>
          {p.showHidden ? t("bar.hideHidden") : t("bar.showHidden")}
        </Menu.Item>
        <Menu.Item className="lt-menu-item" onSelect={p.onRefresh}>{t("bar.refresh")}</Menu.Item>
        {p.writeEnabled ? (
          <>
            <Menu.Separator className="lt-menu-sep" />
            <Menu.Item className="lt-menu-item" onSelect={p.onToggleWriteActions}>{t("device.stopChanges")}</Menu.Item>
          </>
        ) : null}
      </ToolMenu>
      {p.writeEnabled ? (
        <button type="button" className="fx-btn fx-up" onClick={p.onUpload} disabled={p.busy}>
          <Upload aria-hidden="true" width={14} height={14} strokeWidth={2} />
          <span>{t("bar.upload")}</span>
        </button>
      ) : (
        <button type="button" className="fx-btn" onClick={p.onToggleWriteActions}>{t("device.allowChanges")}</button>
      )}
    </div>
  );
}
