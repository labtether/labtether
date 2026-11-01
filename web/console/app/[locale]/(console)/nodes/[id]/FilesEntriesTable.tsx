"use client";

import type React from "react";
import { useTranslations } from "next-intl";
import type { FileEntry } from "../../files/fileOpsClient";
import { FileListView } from "../../files/FileListView";
import type { RenameState } from "../../files/FileRows";
import type { MenuItemSpec } from "../../files/FileMenus";
import type { ClickMods } from "../../files/filesListModel";

type FilesEntriesTableProps = {
  loading: boolean;
  currentPath: string;
  entries: FileEntry[];
  emptyText: string;
  picked: ReadonlySet<string>;
  rename: RenameState | null;
  dropText: string | null;
  drop: React.ComponentProps<typeof FileListView>["drop"];
  onPick: (name: string, mods: ClickMods) => void;
  onToggle: (name: string) => void;
  onSelectAll: () => void;
  onOpenEntry: (entry: FileEntry) => void;
  onContextMenu: (event: React.MouseEvent, entry: FileEntry | null) => void;
  onKeyAction: (action: "delete" | "rename", entry: FileEntry) => void;
  menuFor: (entry: FileEntry) => MenuItemSpec[];
};

/** A device's folder as the 2.0 file list: name, size, modified time and permissions, with the item menu. */
export function FilesEntriesTable(p: FilesEntriesTableProps) {
  const t = useTranslations("files");
  return (
    <FileListView
      entries={p.entries}
      label={t("list.label", { path: p.currentPath })}
      loading={p.loading}
      emptyText={p.emptyText}
      picked={p.picked}
      rename={p.rename}
      dropText={p.dropText}
      drop={p.drop}
      onSelectAll={p.onSelectAll}
      handlers={{
        onPick: p.onPick,
        onToggle: p.onToggle,
        onOpen: p.onOpenEntry,
        onContextMenu: p.onContextMenu,
        onKeyAction: p.onKeyAction,
        menuItems: p.menuFor,
        menuLabel: (name) => t("menu.actionsFor", { name }),
        selectLabel: (name) => t("list.select", { name }),
        renameLabel: t("list.rename"),
      }}
    />
  );
}
