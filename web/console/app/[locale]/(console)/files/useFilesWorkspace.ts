"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type MouseEvent } from "react";
import { useTranslations } from "next-intl";
import type { FileEntry } from "./fileOpsClient";
import { applyWorkspaceClipboard, buildWorkspaceClipboardItems } from "./fileWorkspaceClipboard";
import { joinPath } from "./fileWorkspaceUtils";
import { filterEntries, menuActions } from "./filesListModel";
import { toMenuItems, useMenuLabels, type MenuItemSpec } from "./FileMenus";
import { useFileTabsState, sourceFromKey } from "./useFileTabsState";
import { useFileTransfers } from "./useFileTransfers";
import { useFileWorkspaceState, type WorkspaceClipboardMode } from "./useFileWorkspaceState";
import { useFilesSource } from "./useFilesSource";
import { usePathHistory } from "./usePathHistory";
import { useUploadReplace } from "./useUploadReplace";
import { copyText } from "../../../lib/clipboard";

export type FileViewMode = "list" | "grid";

const BAD_NAME = /[/\\\0]/;

/** State and actions of the Files page: tabs, the open folder, picks, clipboard, menus, dialogs and transfers. */
export function useFilesWorkspace() {
  const t = useTranslations("files");
  const label = useMenuLabels();
  const tabs = useFileTabsState();
  const transfers = useFileTransfers();
  const src = useFilesSource(tabs.activeTab);
  const ws = useFileWorkspaceState<FileEntry>();
  const { clearSelection, selectedEntries, clipboard } = ws;

  const [query, setQuery] = useState("");
  const [view, setView] = useState<FileViewMode>("list");
  const [renaming, setRenaming] = useState<{ name: string; value: string } | null>(null);
  const [deleteNames, setDeleteNames] = useState<string[] | null>(null);
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [pendingUpload, setPendingUpload] = useState<{ files: File[]; dir: string } | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const uploadInputRef = useRef<HTMLInputElement>(null);
  const uploadDirRef = useRef("~");
  // Enter, Escape and blur can all end one rename; only the first counts.
  const renameOpen = useRef(false);
  const history = usePathHistory(`${tabs.activeTabId}|${src.key}`, src.currentPath, !src.loading);

  const cur = src.currentPath;
  const visible = useMemo(() => filterEntries(src.entries, query), [src.entries, query]);
  const order = useMemo(() => visible.map((e) => e.name), [visible]);

  // Opening another folder (or tab) clears the picks, the search and a rename.
  useEffect(() => {
    clearSelection();
    setQuery("");
    setRenaming(null);
    renameOpen.current = false;
  }, [cur, src.key, clearSelection]);

  const goBack = useCallback(() => {
    const prev = history.back();
    if (prev) src.goTo(prev);
  }, [history, src]);

  // ----- Clipboard -----
  const toClipboard = useCallback((mode: WorkspaceClipboardMode, names: string[]) => {
    const items = buildWorkspaceClipboardItems({
      entries: src.entries,
      names,
      getName: (e) => e.name,
      getPath: (e) => joinPath(cur, e.name),
      getIsDir: (e) => e.is_dir,
    });
    ws.setClipboardItems(mode, src.key, items);
  }, [cur, src.entries, src.key, ws]);

  /** Same source: copy or move at once. Another source: a copy runs as a transfer; a move is refused. */
  const paste = useCallback(async (dir: string) => {
    if (!clipboard || !src.source) return;
    if (clipboard.ownerID !== src.key) {
      const from = sourceFromKey(clipboard.ownerID);
      if (clipboard.mode === "copy" && from) {
        void transfers.startTransfer(clipboard.items.map((i) => i.path), from, src.source, dir);
      } else {
        src.setError(t("errors.crossMove"));
      }
      return;
    }
    const outcome = await applyWorkspaceClipboard({
      clipboard,
      ownerID: src.key,
      targetDirPath: dir,
      copyItem: src.copyPath,
      moveItem: src.movePath,
    });
    if (outcome.status !== "applied") return;
    src.refresh();
    if (!outcome.failed && outcome.mode === "cut") ws.clearClipboard();
  }, [clipboard, src, t, transfers, ws]);

  // ----- Upload, rename, delete -----
  const pickUpload = useCallback((dir: string) => {
    uploadDirRef.current = dir;
    uploadInputRef.current?.click();
  }, []);

  // An upload over files of the same name waits for Replace, Skip these or Cancel.
  const startUpload = useCallback((files: File[], dir: string) => {
    void src.upload(files, dir);
  }, [src]);
  const uploadUnread = useCallback((dir: string) => {
    src.setError(t("errors.uploadNotChecked", { path: dir }));
  }, [src, t]);
  const replace = useUploadReplace({
    held: { path: cur, entries: src.entries, showHidden: src.showHidden, ready: !src.loading && !src.error },
    list: src.listFolder,
    ignoreCase: src.namesIgnoreCase,
    upload: startUpload,
    onUnread: uploadUnread,
  });
  const startReplaceCheck = replace.start;

  const onUploadInput = useCallback((files: File[]) => {
    startReplaceCheck(files, uploadDirRef.current);
  }, [startReplaceCheck]);

  const startRename = useCallback((name: string) => {
    renameOpen.current = true;
    setRenaming({ name, value: name });
  }, []);
  const cancelRename = useCallback(() => {
    renameOpen.current = false;
    setRenaming(null);
  }, []);
  const commitRename = useCallback(() => {
    if (!renaming || !renameOpen.current) return;
    renameOpen.current = false;
    const next = renaming.value.trim();
    setRenaming(null);
    if (!next || next === renaming.name || BAD_NAME.test(next)) return;
    void src.rename(renaming.name, next);
  }, [renaming, src]);

  const confirmDelete = useCallback(() => {
    if (!deleteNames) return;
    void src.remove(deleteNames);
    clearSelection();
    setDeleteNames(null);
  }, [clearSelection, deleteNames, src]);

  const createFolder = useCallback((name: string) => {
    setNewFolderOpen(false);
    if (name.trim() && !BAD_NAME.test(name.trim())) void src.createDir(name.trim());
  }, [src]);

  const confirmUpload = useCallback(() => {
    if (pendingUpload) void src.upload(pendingUpload.files, pendingUpload.dir);
    setPendingUpload(null);
  }, [pendingUpload, src]);

  // ----- Menus -----
  const canPaste = Boolean(clipboard && clipboard.items.length > 0 && (clipboard.ownerID === src.key || clipboard.mode === "copy"));

  const menuFor = useCallback((entry: FileEntry | null, names: string[], dirPath: string): MenuItemSpec[] => {
    const hasFiles = names.some((n) => src.entries.some((e) => e.name === n && !e.is_dir));
    const slots = menuActions({ entry, count: names.length, hasFiles, canPaste, canWrite: true });
    return toMenuItems(slots, {
      open: entry ? () => src.openFile(entry) : undefined,
      download: () => src.download(names),
      rename: entry ? () => startRename(entry.name) : undefined,
      copy: () => toClipboard("copy", names),
      cut: () => toClipboard("cut", names),
      paste: () => void paste(dirPath),
      copyPath: entry ? () => void copyText(joinPath(cur, entry.name)).catch(() => {}) : undefined,
      delete: () => setDeleteNames(names),
      newFolder: () => setNewFolderOpen(true),
      uploadHere: () => pickUpload(dirPath),
      refresh: src.refresh,
    }, (a) => label(a, { count: names.length, name: entry?.name ?? "", dir: Boolean(entry?.is_dir) }));
  }, [canPaste, cur, label, paste, pickUpload, src, startRename, toClipboard]);

  /** The names an item action works on: the picked rows when the item is one of them, else the item. */
  const targetsFor = useCallback((entry: FileEntry) => (
    selectedEntries.has(entry.name) && selectedEntries.size > 1 ? Array.from(selectedEntries) : [entry.name]
  ), [selectedEntries]);

  const rowMenu = useCallback((entry: FileEntry) => {
    const names = targetsFor(entry);
    return menuFor(entry, names, entry.is_dir && names.length === 1 ? joinPath(cur, entry.name) : cur);
  }, [cur, menuFor, targetsFor]);

  const onContextMenu = useCallback((event: MouseEvent, entry: FileEntry | null) => {
    event.preventDefault();
    event.stopPropagation();
    if (entry && (!selectedEntries.has(entry.name) || selectedEntries.size <= 1)) ws.selectOnly(entry.name);
    const names = entry ? targetsFor(entry) : Array.from(selectedEntries);
    ws.openContextMenu(event.clientX, event.clientY, {
      entry,
      names,
      targetDirPath: entry?.is_dir ? joinPath(cur, entry.name) : cur,
    });
  }, [cur, selectedEntries, targetsFor, ws]);

  const contextItems = ws.contextMenu
    ? menuFor(ws.contextMenu.entry, ws.contextMenu.entry ? ws.contextMenu.names : [], ws.contextMenu.targetDirPath)
    : [];

  // ----- Drag and drop from the computer -----
  const drop = useMemo(() => ({
    onDragOver: (e: DragEvent) => {
      if (!e.dataTransfer.types.includes("Files")) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = "copy";
      setDragOver(true);
    },
    onDragLeave: (e: DragEvent) => {
      if (!(e.relatedTarget instanceof Node) || !e.currentTarget.contains(e.relatedTarget)) setDragOver(false);
    },
    onDrop: (e: DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      const files = Array.from(e.dataTransfer.files ?? []);
      // Several dropped files with no clash keep their own question; a clash asks once, in the Replace dialog.
      startReplaceCheck(files, cur, files.length > 1 ? (list, dir) => setPendingUpload({ files: list, dir }) : undefined);
    },
  }), [cur, startReplaceCheck]);

  return {
    t,
    tabs,
    transfers,
    src,
    ws,
    view,
    setView,
    query,
    setQuery,
    visible,
    order,
    history: { canBack: history.canGoBack, back: goBack },
    clipboard: { value: clipboard, canPaste, paste: () => void paste(cur), clear: ws.clearClipboard },
    rename: renaming
      ? { name: renaming.name, value: renaming.value, onChange: (value: string) => setRenaming({ ...renaming, value }), onCommit: commitRename, onCancel: cancelRename }
      : null,
    startRename,
    rowMenu,
    onContextMenu,
    contextItems,
    targetsFor,
    upload: { inputRef: uploadInputRef, pick: pickUpload, onInput: onUploadInput, pending: pendingUpload, confirm: confirmUpload, cancel: () => setPendingUpload(null), replace },
    del: { names: deleteNames, ask: setDeleteNames, confirm: confirmDelete, cancel: () => setDeleteNames(null) },
    newFolder: { open: newFolderOpen, show: () => setNewFolderOpen(true), create: createFolder, cancel: () => setNewFolderOpen(false) },
    drop,
    dragOver,
  };
}

export type FilesWorkspaceState = ReturnType<typeof useFilesWorkspace>;
