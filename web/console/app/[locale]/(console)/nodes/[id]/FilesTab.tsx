"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { type FileEntry } from "../../files/fileOpsClient";
import { joinPath } from "../../files/fileWorkspaceUtils";
import { filterEntries, pickedBytes } from "../../files/filesListModel";
import { FilesStatusLine } from "../../files/FilesStatusLine";
import { useFileWorkspaceState } from "../../files/useFileWorkspaceState";
import { usePathHistory } from "../../files/usePathHistory";
import { FilesTabControls } from "./FilesTabControls";
import { FilesEntriesTable } from "./FilesEntriesTable";
import { FilesContextMenu, useFilesTabMenu, type FilesTabMenuHandlers } from "./FilesContextMenu";
import { FilesTabOverlays, FilesTabStatus } from "./FilesTabOverlays";
import { useFilesTabClipboardActions } from "./useFilesTabClipboardActions";
import { useFilesTabContextMenuInteractions } from "./useFilesTabContextMenuInteractions";
import { useFilesTabDirectoryState } from "./useFilesTabDirectoryState";
import { useFilesTabMutations } from "./useFilesTabMutations";
import { useFilesTabWriteAccess } from "./useFilesTabWriteAccess";

/** A device's Files tab: the 2.0 file list on the device's agent. It opens read-only; "Allow changes" turns on writes. */
export function FilesTab({ nodeId }: { nodeId: string }) {
  const t = useTranslations("files");
  const uploadInputRef = useRef<HTMLInputElement | null>(null);
  const m = useFilesTabMutations({ nodeId });
  const ws = useFileWorkspaceState<FileEntry>();
  const dir = useFilesTabDirectoryState({ nodeId, setError: m.setError });
  const cur = dir.currentPath;
  const { listDir } = dir;
  const access = useFilesTabWriteAccess({ currentPath: cur, uploadInputRef });
  const write = access.writeEnabled;
  const clip = useFilesTabClipboardActions({
    nodeId,
    entries: dir.entries,
    currentPath: cur,
    clipboard: ws.clipboard,
    setClipboardItems: ws.setClipboardItems,
    clearClipboard: ws.clearClipboard,
    writeEnabled: write,
    listDir,
    setActionBusy: m.setActionBusy,
    setError: m.setError,
    setActionMessage: m.setActionMessage,
  });
  const { handleContextMenu } = useFilesTabContextMenuInteractions({
    currentPath: cur,
    selectedEntries: ws.selectedEntries,
    selectionNamesFromEntry: ws.selectionNamesFromEntry,
    selectOnly: ws.selectOnly,
    openContextMenu: ws.openContextMenu,
  });
  const history = usePathHistory(nodeId, cur, !dir.loading);
  const [query, setQuery] = useState("");
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  // Enter, Escape and blur can all end one rename; only the first counts.
  const renameLive = useRef(false);
  const { clearSelection } = ws;

  const visible = useMemo(() => filterEntries(dir.sortedEntries, query), [dir.sortedEntries, query]);
  const order = useMemo(() => visible.map((e) => e.name), [visible]);

  useEffect(() => {
    clearSelection();
    setQuery("");
  }, [cur, clearSelection]);

  const refresh = useCallback(() => {
    void listDir(cur);
  }, [cur, listDir]);

  const downloadAll = async (entries: FileEntry[]) => {
    for (const entry of entries) {
      if (!entry.is_dir) await m.downloadEntry(entry, cur);
    }
  };
  const startRename = (entry: FileEntry) => {
    if (!write) return;
    renameLive.current = true;
    m.renameEntry(entry, { currentPath: cur, writeEnabled: write });
  };
  const remove = (entries: FileEntry[]) => m.deleteEntries(entries, { currentPath: cur, writeEnabled: write });

  const handlers: FilesTabMenuHandlers = {
    open: (entry) => (entry.is_dir ? dir.openEntry(entry) : void m.downloadEntry(entry, cur)),
    download: (entries) => void downloadAll(entries),
    rename: startRename,
    copyNames: clip.handleCopyNames,
    cutNames: clip.handleCutNames,
    pasteTo: clip.handlePasteTo,
    remove,
    newFolder: () => setNewFolderOpen(true),
    uploadHere: access.handleUploadHere,
    refresh,
  };
  const menuFor = useFilesTabMenu({ entries: dir.entries, currentPath: cur, writeEnabled: write, clipboard: ws.clipboard, h: handlers });
  const targets = (entry: FileEntry) => (
    ws.selectedEntries.has(entry.name) && ws.selectedEntries.size > 1 ? Array.from(ws.selectedEntries) : [entry.name]
  );
  const picked = ws.selectedEntries;
  const pickedList = dir.entries.filter((e) => picked.has(e.name));

  const rename = m.renameTarget
    ? {
      name: m.renameTarget.name,
      value: m.renameName,
      onChange: m.setRenameName,
      onCommit: () => {
        if (!renameLive.current) return;
        renameLive.current = false;
        void m.commitRename({ currentPath: cur, listDir });
      },
      onCancel: () => {
        renameLive.current = false;
        m.cancelRename();
      },
    }
    : null;

  return (
    <div className="fx-dev">
      <FilesTabControls
        currentPath={cur}
        pathInput={dir.pathInput}
        onPathInputChange={dir.setPathInput}
        onNavigateToPath={dir.navigateTo}
        onNavigateUp={dir.navigateUp}
        disableNavigateUp={dir.loading || m.actionBusy || cur === "~" || cur === "/"}
        canBack={history.canGoBack}
        onBack={() => {
          const prev = history.back();
          if (prev) dir.navigateTo(prev);
        }}
        busy={dir.loading || m.actionBusy}
        query={query}
        onQuery={setQuery}
        clipboard={ws.clipboard}
        onPaste={() => clip.handlePasteTo(cur)}
        onClearClipboard={ws.clearClipboard}
        showHidden={dir.showHidden}
        onShowHiddenChange={dir.setShowHidden}
        onRefresh={refresh}
        writeEnabled={write}
        onToggleWriteActions={access.enableWriteActions}
        onNewFolder={() => setNewFolderOpen(true)}
        onUpload={() => access.handleUploadHere(cur)}
      />

      <FilesTabStatus error={m.error} actionMessage={m.actionMessage} />

      <div className="fx-sheet">
        <FilesEntriesTable
          loading={dir.loading}
          currentPath={cur}
          entries={visible}
          emptyText={query ? t("list.noMatch", { query }) : t("list.empty")}
          picked={picked}
          rename={rename}
          dropText={null}
          drop={undefined}
          onPick={(name, mods) => ws.pickClick(name, order, mods)}
          onToggle={ws.toggleSelected}
          onSelectAll={() => ws.toggleSelectAll(order)}
          onOpenEntry={handlers.open}
          onContextMenu={handleContextMenu}
          onKeyAction={(action, entry) => {
            if (action === "rename") startRename(entry);
            else remove(dir.entries.filter((e) => targets(entry).includes(e.name)));
          }}
          menuFor={(entry) => {
            const names = targets(entry);
            return menuFor(entry, names, entry.is_dir && names.length === 1 ? joinPath(cur, entry.name) : cur);
          }}
        />
        <FilesStatusLine
          shown={visible.length}
          total={dir.entries.length}
          searching={query.trim() !== ""}
          picked={picked.size}
          pickedBytes={pickedBytes(dir.entries, picked)}
          onDownload={pickedList.some((e) => !e.is_dir) ? () => void downloadAll(pickedList) : undefined}
          onDelete={write ? () => remove(pickedList) : undefined}
          onClear={clearSelection}
          extra={write ? null : <span className="fx-c3">{t("device.readOnly")}</span>}
        />
      </div>

      <input
        ref={uploadInputRef}
        type="file"
        multiple
        hidden
        onChange={() => {
          void m.uploadSelectedFiles({ currentPath: cur, listDir, writeEnabled: write, uploadDestinationPath: access.uploadDestinationPath, uploadInputRef });
        }}
      />

      <FilesContextMenu contextMenu={ws.contextMenu} menuFor={menuFor} onClose={ws.closeContextMenu} />

      <FilesTabOverlays
        where={cur}
        enableWriteOpen={access.enableWriteOpen}
        onCloseEnableWrite={() => access.setEnableWriteOpen(false)}
        onConfirmEnableWrite={access.handleConfirmEnableWrite}
        deleteTargets={m.deleteTargets}
        onCloseDelete={() => m.setDeleteTargets(null)}
        onConfirmDelete={() => void m.commitDelete({ currentPath: cur, listDir })}
        actionBusy={m.actionBusy}
        newFolderOpen={newFolderOpen}
        onCloseNewFolder={() => setNewFolderOpen(false)}
        onCreateFolder={(name) => {
          setNewFolderOpen(false);
          void m.createFolder({ currentPath: cur, listDir, writeEnabled: write, name });
        }}
      />
    </div>
  );
}
