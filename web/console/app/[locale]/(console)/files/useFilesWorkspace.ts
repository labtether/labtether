"use client";

import { useCallback,useEffect,useMemo,useRef,useState } from "react";
import { useFiles,type FileEntry } from "../../../hooks/useFiles";
import type { UnifiedFileEntry } from "./fileOpsClient";
import { joinPath } from "./fileWorkspaceUtils";
import { useConnectionBrowser } from "./useConnectionBrowser";
import { useFileTabsState } from "./useFileTabsState";
import { useFileTransfers } from "./useFileTransfers";
import { useFileWorkspaceState } from "./useFileWorkspaceState";
import { useFilesClipboardActions } from "./useFilesClipboardActions";
import { useFilesContextMenuInteractions } from "./useFilesContextMenuInteractions";
import { useFilesUploadInteractions } from "./useFilesUploadInteractions";

export function useFilesWorkspace() {

  // -------------------------------------------------------------------------
  // Tab state + transfer state
  // -------------------------------------------------------------------------

  const tabs = useFileTabsState();
  const transfers = useFileTransfers();

  // -------------------------------------------------------------------------
  // Existing file browser state (agent-based)
  // -------------------------------------------------------------------------

  const {
    target,
    setTarget,
    connectedAgentIds,
    currentPath,
    rootPath,
    entries,
    loading,
    error,
    showHidden,
    setShowHidden,
    sortField,
    sortDir,
    toggleSort,
    listDir,
    navigate,
    navigateUp,
    navigateToPath,
    downloadFile,
    uploadFile,
    createDir,
    deleteEntry,
    renameEntry,
    copyEntry,
    uploadProgress,
    cancelUpload,
    downloadProgress,
    cancelDownload,
    deleteSelected,
    isPreviewable,
    previewFile,
    previewContent,
    closePreview,
    setErrorMessage,
  } = useFiles();

  // -------------------------------------------------------------------------
  // Derive active tab/source early (needed by hooks below)
  // -------------------------------------------------------------------------

  const activeTab = tabs.activeTab;
  const activeSource = tabs.activeSource;
  const isAgentTab = activeTab?.type === "agent";
  const isConnectionTab = activeTab?.type === "connection";
  const isBrowsingTab = isAgentTab || isConnectionTab;
  const isNewTab = activeTab?.type === "new";
  const inSplitMode = tabs.splitMode && isBrowsingTab;

  // -------------------------------------------------------------------------
  // Connection browser state (used when active tab is a connection)
  // -------------------------------------------------------------------------

  const connBrowser = useConnectionBrowser(
    activeTab?.type === "connection" && activeSource ? activeSource : null,
  );

  // Map UnifiedFileEntry → FileEntry for FilesBrowserCard compatibility
  const connMappedEntries: FileEntry[] = useMemo(
    () =>
      connBrowser.entries.map((e: UnifiedFileEntry) => ({
        name: e.name,
        is_dir: e.is_dir,
        size: e.size,
        mod_time: e.mod_time ?? "",
        mode: e.mode ?? "",
      })),
    [connBrowser.entries],
  );

  // Connection file input ref for uploads
  const connFileInputRef = useRef<HTMLInputElement>(null);

  // Connection clipboard (copy/cut/paste within a connection)
  const [connClipboardState, setConnClipboardState] = useState<{
    mode: "copy" | "cut";
    names: string[];
    basePath: string;
  } | null>(null);

  const connClipboard = useMemo(() => ({
    canPaste: connClipboardState != null && connClipboardState.names.length > 0,
    setCopy: (names: string[]) =>
      setConnClipboardState({ mode: "copy", names, basePath: connBrowser.currentPath }),
    setCut: (names: string[]) =>
      setConnClipboardState({ mode: "cut", names, basePath: connBrowser.currentPath }),
    paste: async (targetDirPath: string) => {
      if (!connClipboardState) return;
      const { mode, names, basePath } = connClipboardState;
      for (const name of names) {
        const srcPath = joinPath(basePath, name);
        const dstPath = joinPath(targetDirPath, name);
        if (mode === "copy") {
          await connBrowser.copyEntry(srcPath, dstPath);
        } else {
          await connBrowser.renameEntry(srcPath, dstPath);
        }
      }
      if (mode === "cut") setConnClipboardState(null);
      connBrowser.refresh();
    },
  }), [connClipboardState, connBrowser]);

  const [viewMode, setViewMode] = useState<"list" | "grid">("list");
  const [mkdirName, setMkdirName] = useState("");
  const [showMkdir, setShowMkdir] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const [confirmBatchDelete, setConfirmBatchDelete] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [renamingEntry, setRenamingEntry] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const renameInputRef = useRef<HTMLInputElement>(null);
  const targetHasAgent = connectedAgentIds.has(target);

  const {
    selectedEntries,
    toggleSelected,
    toggleSelectAll,
    clearSelection,
    selectOnly,
    setSelectedNames,
    selectionNamesFromEntry,
    clipboard,
    setClipboardItems,
    clearClipboard,
    contextMenu,
    openContextMenu,
    closeContextMenu,
  } = useFileWorkspaceState<FileEntry>();

  // -------------------------------------------------------------------------
  // Sync tab selection -> useFiles target
  // -------------------------------------------------------------------------

  // Track the previous active tab ID so we only react to actual tab changes.
  const prevActiveTabIdRef = useRef<string | null>(null);

  useEffect(() => {
    const activeTab = tabs.activeTab;
    if (!activeTab) return;

    // Only sync when the active tab actually changes.
    if (prevActiveTabIdRef.current === activeTab.id) return;
    prevActiveTabIdRef.current = activeTab.id;

    if (activeTab.type === "agent" && activeTab.sourceId) {
      setTarget(activeTab.sourceId);
    } else if (activeTab.type === "connection") {
      // Connection tabs don't use the legacy useFiles target.
      // Clear target so agent-specific UI doesn't show.
      setTarget("");
    } else if (activeTab.type === "new") {
      setTarget("");
    }
  }, [tabs.activeTab, setTarget]);

  // Auto-list when target changes or when the selected target becomes connected.
  useEffect(() => {
    if (target && targetHasAgent) {
      void listDir(target, "~");
    }
  }, [target, targetHasAgent, listDir]);

  // Clear selection when directory changes.
  useEffect(() => {
    clearSelection();
  }, [currentPath, clearSelection]);

  // Keyboard shortcuts.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setConfirmDelete(null);
        setConfirmBatchDelete(false);
        setRenamingEntry(null);
        setShowMkdir(false);
        closeContextMenu();
        closePreview();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [closeContextMenu, closePreview]);

  // Focus rename input when it appears.
  useEffect(() => {
    if (renamingEntry && renameInputRef.current) {
      renameInputRef.current.focus();
      const dotIdx = renameValue.lastIndexOf(".");
      if (dotIdx > 0) {
        renameInputRef.current.setSelectionRange(0, dotIdx);
      } else {
        renameInputRef.current.select();
      }
    }
  }, [renamingEntry, renameValue]);

  const fullPathForName = useCallback((name: string) => {
    return joinPath(currentPath, name);
  }, [currentPath]);

  const {
    fileInputRef,
    handleDrop,
    handleFileInput,
    handleToolbarUpload,
    handleContextUploadHere,
  } = useFilesUploadInteractions({
    currentPath,
    uploadFile,
    setDragOver,
  });

  const {
    workspaceMenuHostRef,
    handleEntryContextMenu,
    handleBackgroundContextMenu,
  } = useFilesContextMenuInteractions({
    currentPath,
    selectedEntries,
    selectionNamesFromEntry,
    selectOnly,
    fullPathForName,
    openContextMenu,
    closeContextMenu,
  });

  const { setClipboardFromNames, pasteClipboardToDir } = useFilesClipboardActions({
    target,
    entries,
    fullPathForName,
    setClipboardItems,
    clipboard,
    copyEntry,
    renameEntry,
    listDir,
    currentPath,
    clearClipboard,
    setErrorMessage,
  });

  const handleEntryClick = useCallback(
    (entry: FileEntry) => {
      if (renamingEntry) return;
      if (entry.is_dir) {
        navigate(entry.name);
      } else {
        const fullPath = joinPath(currentPath, entry.name);
        if (isPreviewable(entry.name, entry.size)) {
          void previewFile(fullPath, entry.name);
        } else {
          downloadFile(fullPath);
        }
      }
    },
    [navigate, currentPath, downloadFile, renamingEntry, isPreviewable, previewFile]
  );

  const handleCreateDir = useCallback(() => {
    const name = mkdirName.trim();
    if (name) {
      void createDir(name);
      setMkdirName("");
      setShowMkdir(false);
    }
  }, [mkdirName, createDir]);

  const handleDeleteConfirm = useCallback(() => {
    if (!confirmDelete) return;
    if (isConnectionTab) {
      void connBrowser.deleteEntry(joinPath(connBrowser.currentPath, confirmDelete));
    } else {
      const fullPath = joinPath(currentPath, confirmDelete);
      void deleteEntry(fullPath);
    }
    setConfirmDelete(null);
  }, [confirmDelete, currentPath, deleteEntry, isConnectionTab, connBrowser]);

  const handleBatchDeleteConfirm = useCallback(() => {
    if (isConnectionTab) {
      void connBrowser.deleteSelected(Array.from(selectedEntries));
    } else {
      void deleteSelected(Array.from(selectedEntries));
    }
    clearSelection();
    setConfirmBatchDelete(false);
  }, [clearSelection, deleteSelected, selectedEntries, isConnectionTab, connBrowser]);

  const startRename = useCallback((entryName: string) => {
    setRenamingEntry(entryName);
    setRenameValue(entryName);
  }, []);

  const commitRename = useCallback(() => {
    if (!renamingEntry || !renameValue.trim() || renameValue.trim() === renamingEntry) {
      setRenamingEntry(null);
      return;
    }
    const oldPath = joinPath(currentPath, renamingEntry);
    const newPath = joinPath(currentPath, renameValue.trim());
    void renameEntry(oldPath, newPath);
    setRenamingEntry(null);
  }, [renamingEntry, renameValue, currentPath, renameEntry]);

  const handleContextDelete = useCallback((names: string[]) => {
    if (names.length === 0) return;
    setSelectedNames(names);
    if (names.length === 1) {
      setConfirmDelete(names[0]);
      return;
    }
    setConfirmBatchDelete(true);
  }, [setSelectedNames]);

  const handleContextOpen = useCallback((entry: FileEntry) => {
    if (entry.is_dir) {
      navigate(entry.name);
      return;
    }
    const fullPath = fullPathForName(entry.name);
    if (isPreviewable(entry.name, entry.size)) {
      void previewFile(fullPath, entry.name);
    } else {
      downloadFile(fullPath);
    }
  }, [downloadFile, fullPathForName, isPreviewable, navigate, previewFile]);

  const handleEntryDoubleClick = useCallback((name: string) => {
    const entry = entries.find((item) => item.name === name);
    if (!entry) return;
    handleContextOpen(entry);
  }, [entries, handleContextOpen]);

  const handleShowHiddenChange = useCallback((nextShowHidden: boolean) => {
    setShowHidden(nextShowHidden);
    if (target) {
      void listDir(target, currentPath, nextShowHidden);
    }
  }, [currentPath, listDir, setShowHidden, target]);

  const handleToolbarRefresh = useCallback(() => {
    if (!target) return;
    void listDir(target, currentPath);
  }, [currentPath, listDir, target]);

  const handleDownloadEntry = useCallback((entry: FileEntry) => {
    downloadFile(fullPathForName(entry.name));
  }, [downloadFile, fullPathForName]);

  const handleContextDownload = useCallback((entry: FileEntry) => {
    downloadFile(fullPathForName(entry.name));
  }, [downloadFile, fullPathForName]);

  const handleDownloadSelected = useCallback(() => {
    if (isConnectionTab) {
      for (const name of selectedEntries) {
        const entry = connMappedEntries.find((e) => e.name === name);
        if (entry && !entry.is_dir) {
          connBrowser.downloadFile(joinPath(connBrowser.currentPath, name));
        }
      }
    } else {
      for (const name of selectedEntries) {
        const entry = entries.find((e) => e.name === name);
        if (entry && !entry.is_dir) {
          downloadFile(fullPathForName(name));
        }
      }
    }
  }, [isConnectionTab, selectedEntries, entries, connMappedEntries, downloadFile, fullPathForName, connBrowser]);

  const selectionCount = selectedEntries.size;
  const allSelected = entries.length > 0 && selectionCount === entries.length;
  const clipboardCount = clipboard?.items.length ?? 0;
  const canPaste = Boolean(
    target
      && targetHasAgent
      && contextMenu
      && clipboard
      && clipboardCount > 0
      && clipboard.ownerID === target,
  );

  // -------------------------------------------------------------------------
  // Tab callbacks
  // -------------------------------------------------------------------------

  const handleOpenAgent = useCallback((assetId: string, name: string) => {
    tabs.addTab({ type: "agent", sourceId: assetId, label: name, protocol: "agent" });
  }, [tabs]);

  const handleOpenConnection = useCallback((connId: string, name: string, protocol: string) => {
    tabs.addTab({ type: "connection", sourceId: connId, label: name, protocol });
  }, [tabs]);

  const handleNewConnection = useCallback((_protocol: string) => {
    // The NewTabPage handles this internally by showing the ConnectionForm
  }, []);
  return {
    tabs,
    isNewTab,
    handleOpenAgent,
    handleOpenConnection,
    handleNewConnection,
    isAgentTab,
    inSplitMode,
    workspaceMenuHostRef,
    activeSource,
    activeTab,
    showHidden,
    clipboardCount,
    clipboard,
    selectionCount,
    currentPath,
    rootPath,
    handleShowHiddenChange,
    setConfirmBatchDelete,
    handleDownloadSelected,
    handleToolbarUpload,
    setShowMkdir,
    handleToolbarRefresh,
    navigateToPath,
    navigateUp,
    clearSelection,
    viewMode,
    setViewMode,
    fileInputRef,
    handleFileInput,
    showMkdir,
    mkdirName,
    setMkdirName,
    handleCreateDir,
    uploadProgress,
    cancelUpload,
    downloadProgress,
    cancelDownload,
    error,
    target,
    targetHasAgent,
    entries,
    loading,
    dragOver,
    sortField,
    sortDir,
    selectedEntries,
    allSelected,
    renamingEntry,
    renameValue,
    renameInputRef,
    toggleSort,
    toggleSelectAll,
    toggleSelected,
    handleEntryClick,
    handleEntryDoubleClick,
    handleDownloadEntry,
    startRename,
    setRenameValue,
    commitRename,
    setRenamingEntry,
    setConfirmDelete,
    handleBackgroundContextMenu,
    handleEntryContextMenu,
    setDragOver,
    handleDrop,
    contextMenu,
    canPaste,
    closeContextMenu,
    handleContextOpen,
    handleContextDownload,
    setClipboardFromNames,
    pasteClipboardToDir,
    handleContextDelete,
    handleContextUploadHere,
    isConnectionTab,
    connBrowser,
    connMappedEntries,
    connFileInputRef,
    connClipboard,
    transfers,
    isBrowsingTab,
    confirmDelete,
    handleDeleteConfirm,
    confirmBatchDelete,
    handleBatchDeleteConfirm,
    previewContent,
    closePreview,
    downloadFile,
  };
}
