"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronRight, LayoutGrid, List, MoreHorizontal, Search, Upload, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { FilesPathBar, type Place } from "./FilesPathBar";
import { ToolMenu } from "./FileMenus";
import type { WorkspaceClipboard } from "./useFileWorkspaceState";

// The toolbar over a folder: Back, Up and the path; search in this folder; the clipboard chip; List or Grid; the ⋯
// menu (new folder, hidden files, refresh, the second pane); Upload, the page's main button.

export type SplitChoice = { key: string; label: string; onSelect: () => void };

type FilesToolbarProps = {
  path: string;
  /** The folder a device shares; see FilesPathBar. */
  root?: string;
  places: Place[];
  busy: boolean;
  canUp: boolean;
  onUp: () => void;
  canBack: boolean;
  onBack: () => void;
  onGo: (path: string) => void;
  query?: string;
  onQuery?: (value: string) => void;
  clipboard: WorkspaceClipboard | null;
  canPaste: boolean;
  onPaste: () => void;
  onClearClipboard: () => void;
  view: "list" | "grid";
  onView: (view: "list" | "grid") => void;
  showHidden: boolean;
  onShowHidden: (next: boolean) => void;
  onNewFolder: () => void;
  onRefresh: () => void;
  onUpload: () => void;
  split?: { open: boolean; choices: SplitChoice[]; onClose: () => void };
};

export function ClipChip({ clipboard, canPaste, onPaste, onClear }: { clipboard: WorkspaceClipboard | null; canPaste: boolean; onPaste: () => void; onClear: () => void }) {
  const t = useTranslations("files.clip");
  if (!clipboard || clipboard.items.length === 0) return null;
  const count = clipboard.items.length;
  const text = clipboard.mode === "cut" ? t("cut", { count }) : t("copy", { count });
  return (
    <span className="fx-chip">
      <button type="button" onClick={onPaste} disabled={!canPaste} title={t("paste")}>{text}</button>
      <button type="button" className="fx-chip-x" onClick={onClear} aria-label={t("clear")} title={t("clear")}>
        <X aria-hidden="true" width={12} height={12} strokeWidth={2} />
      </button>
    </span>
  );
}

/** The ⋯ menu: new folder, hidden files, refresh and the second pane; items without a handler are left out. */
export function FilesMoreMenu({ showHidden, onShowHidden, onNewFolder, onRefresh, split }: {
  showHidden?: boolean;
  onShowHidden?: (next: boolean) => void;
  onNewFolder?: () => void;
  onRefresh?: () => void;
  split?: { open: boolean; choices: SplitChoice[]; onClose: () => void };
}) {
  const t = useTranslations("files.bar");
  return (
    <ToolMenu
      label={t("more")}
      trigger={
        <button type="button" className="fx-ib" title={t("more")}>
          <MoreHorizontal aria-hidden="true" width={16} height={16} />
        </button>
      }
    >
      {onNewFolder ? <Menu.Item className="lt-menu-item" onSelect={onNewFolder}>{t("newFolder")}</Menu.Item> : null}
      {onNewFolder && (onShowHidden || onRefresh) ? <Menu.Separator className="lt-menu-sep" /> : null}
      {onShowHidden ? (
        <Menu.Item className="lt-menu-item" onSelect={() => onShowHidden(!showHidden)}>
          {showHidden ? t("hideHidden") : t("showHidden")}
        </Menu.Item>
      ) : null}
      {onRefresh ? <Menu.Item className="lt-menu-item" onSelect={onRefresh}>{t("refresh")}</Menu.Item> : null}
      {split && (onNewFolder || onShowHidden || onRefresh) ? <Menu.Separator className="lt-menu-sep" /> : null}
      {split?.open ? <Menu.Item className="lt-menu-item" onSelect={split.onClose}>{t("closeSplit")}</Menu.Item> : null}
      {split && !split.open ? (
        <Menu.Sub>
          <Menu.SubTrigger className="lt-menu-item">
            {t("openSplit")}
            <ChevronRight aria-hidden="true" className="fx-sub-chev" width={14} height={14} />
          </Menu.SubTrigger>
          <Menu.Portal>
            <Menu.SubContent className="lt-menu fx-menu fx-menu-scroll" sideOffset={6}>
              {split.choices.map((c) => (
                <Menu.Item key={c.key} className="lt-menu-item" onSelect={c.onSelect}>{c.label}</Menu.Item>
              ))}
            </Menu.SubContent>
          </Menu.Portal>
        </Menu.Sub>
      ) : null}
    </ToolMenu>
  );
}

export function FilesToolbar(p: FilesToolbarProps) {
  const t = useTranslations("files.bar");
  return (
    <div className="fx-bar">
      <FilesPathBar path={p.path} root={p.root} places={p.places} busy={p.busy} canUp={p.canUp} onUp={p.onUp} canBack={p.canBack} onBack={p.onBack} onGo={p.onGo} />
      {p.onQuery ? (
        <label className="fx-q">
          <Search aria-hidden="true" width={14} height={14} strokeWidth={2} />
          <input type="search" value={p.query ?? ""} placeholder={t("search")} aria-label={t("search")} onChange={(e) => p.onQuery?.(e.target.value)} />
        </label>
      ) : null}
      <ClipChip clipboard={p.clipboard} canPaste={p.canPaste} onPaste={p.onPaste} onClear={p.onClearClipboard} />
      <div className="fx-seg" role="group" aria-label={t("view")}>
        {(["list", "grid"] as const).map((v) => {
          const Icon = v === "list" ? List : LayoutGrid;
          return (
            <button key={v} type="button" className={p.view === v ? "on" : undefined} aria-pressed={p.view === v} aria-label={t(v)} title={t(v)} onClick={() => p.onView(v)}>
              <Icon aria-hidden="true" width={14} height={14} strokeWidth={2} />
            </button>
          );
        })}
      </div>
      <FilesMoreMenu
        showHidden={p.showHidden}
        onShowHidden={p.onShowHidden}
        onNewFolder={p.onNewFolder}
        onRefresh={p.onRefresh}
        split={p.split}
      />
      <button type="button" className="fx-btn primary fx-up" onClick={p.onUpload}>
        <Upload aria-hidden="true" width={14} height={14} strokeWidth={2} />
        <span>{t("upload")}</span>
      </button>
    </div>
  );
}
