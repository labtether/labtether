"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { FolderOpen } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { useFiles, type FileEntry } from "../../../hooks/useFiles";
import { FilesBrowserCard } from "./FilesBrowserCard";
import { FilesContextMenu } from "./FilesContextMenu";
import {
  BatchDeleteConfirmOverlay,
  DeleteConfirmOverlay,
  TextPreviewOverlay,
} from "./FilesOverlays";
import { FilesStatusPanels } from "./FilesStatusPanels";
import { FilesToolbarCard } from "./FilesToolbarCard";
import { joinPath } from "./fileWorkspaceUtils";
import { useFileWorkspaceState } from "./useFileWorkspaceState";
import { useFilesClipboardActions } from "./useFilesClipboardActions";
import { useFilesContextMenuInteractions } from "./useFilesContextMenuInteractions";
import { useFilesUploadInteractions } from "./useFilesUploadInteractions";

// New tabbed components
import { FileTabBar } from "./FileTabBar";
import { useFileTabsState, type FileSource } from "./useFileTabsState";
import { NewTabPage } from "./NewTabPage";
import { SplitView } from "./SplitView";
import { TransferProgressBar } from "./TransferProgressBar";
import { useFileTransfers } from "./useFileTransfers";
import { useConnectionBrowser } from "./useConnectionBrowser";
import type { UnifiedFileEntry } from "./fileOpsClient";

export default function FilesPage() {
  const t = useTranslations('files');

  // -------------------------------------------------------------------------
  // Tab state + transfer state
  // -------------------------------------------------------------------------

  const tabs = useFileTabsState();
  const transfers = useFileTransfers();

  // -------------------------------------------------------------------------
  // Existing file browser state (agent-based)
  // -------------------------------------------------------------------------

  const {
    assets,
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

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        title={t('title')}
        subtitle={t('subtitle')}
      />

      {/* Tab bar */}
      <FileTabBar
        tabs={tabs.tabs}
        activeTabId={tabs.activeTabId}
        splitMode={tabs.splitMode}
        onAddTab={() => tabs.addTab({ type: "new", label: "New Tab" })}
        onRemoveTab={tabs.removeTab}
        onSetActiveTab={tabs.setActiveTab}
        onToggleSplit={tabs.toggleSplit}
      />

      {/* Content area */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {/* ---- New Tab Page ---- */}
        {isNewTab && (
          <NewTabPage
            onOpenAgent={handleOpenAgent}
            onOpenConnection={handleOpenConnection}
            onNewConnection={handleNewConnection}
          />
        )}

        {/* ---- Agent file browser (non-split) ---- */}
        {isAgentTab && !inSplitMode && (
          <div
            ref={workspaceMenuHostRef}
            className="relative flex-1 flex flex-col gap-3 p-4 md:p-6 overflow-y-auto"
          >
            <FilesToolbarCard
              source={activeSource}
              sourceLabel={activeTab?.label}
              sourceProtocol={activeTab?.protocol}
              showHidden={showHidden}
              clipboardCount={clipboardCount}
              clipboardMode={clipboard?.mode ?? null}
              selectionCount={selectionCount}
              currentPath={currentPath}
              rootPath={rootPath}
              onShowHiddenChange={handleShowHiddenChange}
              onDeleteSelected={() => setConfirmBatchDelete(true)}
              onDownloadSelected={handleDownloadSelected}
              onUpload={handleToolbarUpload}
              onNewFolder={() => setShowMkdir(true)}
              onRefresh={handleToolbarRefresh}
              onNavigateToPath={navigateToPath}
              onNavigateUp={navigateUp}
              onClearSelection={clearSelection}
              viewMode={viewMode}
              onViewModeChange={setViewMode}
            />
            <input
              ref={fileInputRef}
              type="file"
              multiple
              style={{ display: "none" }}
              onChange={handleFileInput}
            />
            <FilesStatusPanels
              showMkdir={showMkdir}
              mkdirName={mkdirName}
              onMkdirNameChange={setMkdirName}
              onCreateDir={handleCreateDir}
              onCancelMkdir={() => {
                setShowMkdir(false);
                setMkdirName("");
              }}
              uploadProgress={uploadProgress}
              onCancelUpload={cancelUpload}
              downloadProgress={downloadProgress}
              onCancelDownload={cancelDownload}
              error={error}
              showNoAgentWarning={Boolean(target && !targetHasAgent)}
            />

            {/* File browser */}
            {target && targetHasAgent && (
              <FilesBrowserCard
                viewMode={viewMode}
                entries={entries}
                loading={loading}
                dragOver={dragOver}
                sortField={sortField}
                sortDir={sortDir}
                selectedEntries={selectedEntries}
                allSelected={allSelected}
                renamingEntry={renamingEntry}
                renameValue={renameValue}
                renameInputRef={renameInputRef}
                onToggleSort={toggleSort}
                onToggleSelectAll={() => toggleSelectAll(entries.map((entry) => entry.name))}
                onToggleSelected={toggleSelected}
                onEntryClick={handleEntryClick}
                onEntryDoubleClick={handleEntryDoubleClick}
                onDownloadEntry={handleDownloadEntry}
                onStartRename={startRename}
                onRenameValueChange={setRenameValue}
                onCommitRename={commitRename}
                onCancelRename={() => { setRenamingEntry(null); }}
                onDeleteEntry={setConfirmDelete}
                onBackgroundContextMenu={handleBackgroundContextMenu}
                onEntryContextMenu={handleEntryContextMenu}
                onDragOver={(event) => {
                  event.preventDefault();
                  setDragOver(true);
                }}
                onDragLeave={() => { setDragOver(false); }}
                onDrop={handleDrop}
              />
            )}

            {/* Loading / waiting for agent */}
            {target && !targetHasAgent && !error && (
              <Card>
                <EmptyState
                  icon={FolderOpen}
                  title="Waiting for agent"
                  description="The agent on this device is not connected."
                />
              </Card>
            )}

            {contextMenu && target && targetHasAgent && (
              <FilesContextMenu
                contextMenu={contextMenu}
                canPaste={canPaste}
                onClose={closeContextMenu}
                onOpenEntry={handleContextOpen}
                onDownloadEntry={handleContextDownload}
                onCopyNames={(names) => setClipboardFromNames("copy", names)}
                onCutNames={(names) => setClipboardFromNames("cut", names)}
                onPasteToDir={(targetDirPath) => {
                  void pasteClipboardToDir(targetDirPath);
                }}
                onRenameEntry={(entry) => startRename(entry.name)}
                onDeleteNames={handleContextDelete}
                onUploadHere={handleContextUploadHere}
                onRefresh={handleToolbarRefresh}
              />
            )}
          </div>
        )}

        {/* ---- Connection file browser (non-split) ---- */}
        {isConnectionTab && !inSplitMode && activeSource && (
          <div
            ref={workspaceMenuHostRef}
            className="relative flex-1 flex flex-col gap-3 p-4 md:p-6 overflow-y-auto"
          >
            <FilesToolbarCard
              source={activeSource}
              sourceLabel={activeTab?.label}
              sourceProtocol={activeTab?.protocol}
              showHidden={connBrowser.showHidden}
              clipboardCount={clipboardCount}
              clipboardMode={clipboard?.mode ?? null}
              selectionCount={selectionCount}
              currentPath={connBrowser.currentPath}
              rootPath={connBrowser.rootPath}
              onShowHiddenChange={connBrowser.setShowHidden}
              onDeleteSelected={() => setConfirmBatchDelete(true)}
              onDownloadSelected={() => {
                for (const name of selectedEntries) {
                  const entry = connMappedEntries.find((e) => e.name === name);
                  if (entry && !entry.is_dir) {
                    connBrowser.downloadFile(joinPath(connBrowser.currentPath, name));
                  }
                }
              }}
              onUpload={() => {
                connFileInputRef.current?.click();
              }}
              onNewFolder={() => setShowMkdir(true)}
              onRefresh={connBrowser.refresh}
              onNavigateToPath={connBrowser.navigateToPath}
              onNavigateUp={connBrowser.navigateUp}
              onClearSelection={clearSelection}
              viewMode={viewMode}
              onViewModeChange={setViewMode}
            />
            <input
              ref={connFileInputRef}
              type="file"
              multiple
              style={{ display: "none" }}
              onChange={(e) => {
                const files = Array.from(e.target.files ?? []);
                for (const file of files) {
                  void connBrowser.uploadFile(
                    joinPath(connBrowser.currentPath, file.name),
                    file,
                  );
                }
                e.target.value = "";
              }}
            />
            <FilesStatusPanels
              showMkdir={showMkdir}
              mkdirName={mkdirName}
              onMkdirNameChange={setMkdirName}
              onCreateDir={() => {
                const name = mkdirName.trim();
                if (name) {
                  void connBrowser.createDir(name);
                  setMkdirName("");
                  setShowMkdir(false);
                }
              }}
              onCancelMkdir={() => {
                setShowMkdir(false);
                setMkdirName("");
              }}
              uploadProgress={uploadProgress}
              onCancelUpload={cancelUpload}
              downloadProgress={downloadProgress}
              onCancelDownload={cancelDownload}
              error={connBrowser.error}
              showNoAgentWarning={false}
            />

            {/* File browser */}
            <FilesBrowserCard
              viewMode={viewMode}
              entries={connMappedEntries}
              loading={connBrowser.loading}
              dragOver={dragOver}
              sortField={connBrowser.sortField}
              sortDir={connBrowser.sortDir}
              selectedEntries={selectedEntries}
              allSelected={connMappedEntries.length > 0 && selectionCount === connMappedEntries.length}
              renamingEntry={renamingEntry}
              renameValue={renameValue}
              renameInputRef={renameInputRef}
              onToggleSort={connBrowser.toggleSort}
              onToggleSelectAll={() => toggleSelectAll(connMappedEntries.map((e) => e.name))}
              onToggleSelected={toggleSelected}
              onEntryClick={(entry) => {
                if (renamingEntry) return;
                if (entry.is_dir) {
                  connBrowser.navigate(entry.name);
                } else {
                  connBrowser.downloadFile(joinPath(connBrowser.currentPath, entry.name));
                }
              }}
              onEntryDoubleClick={(name) => {
                const entry = connMappedEntries.find((e) => e.name === name);
                if (entry?.is_dir) connBrowser.navigate(entry.name);
              }}
              onDownloadEntry={(entry) => {
                connBrowser.downloadFile(joinPath(connBrowser.currentPath, entry.name));
              }}
              onStartRename={startRename}
              onRenameValueChange={setRenameValue}
              onCommitRename={() => {
                if (!renamingEntry || !renameValue.trim() || renameValue.trim() === renamingEntry) {
                  setRenamingEntry(null);
                  return;
                }
                void connBrowser.renameEntry(
                  joinPath(connBrowser.currentPath, renamingEntry),
                  joinPath(connBrowser.currentPath, renameValue.trim()),
                );
                setRenamingEntry(null);
              }}
              onCancelRename={() => { setRenamingEntry(null); }}
              onDeleteEntry={setConfirmDelete}
              onBackgroundContextMenu={handleBackgroundContextMenu}
              onEntryContextMenu={handleEntryContextMenu}
              onDragOver={(event) => {
                event.preventDefault();
                setDragOver(true);
              }}
              onDragLeave={() => { setDragOver(false); }}
              onDrop={(event) => {
                event.preventDefault();
                setDragOver(false);
                const files = Array.from(event.dataTransfer?.files ?? []);
                if (files.length > 0 && activeSource) {
                  for (const file of files) {
                    void connBrowser.uploadFile(
                      joinPath(connBrowser.currentPath, file.name),
                      file,
                    );
                  }
                }
              }}
            />

            {contextMenu && (
              <FilesContextMenu
                contextMenu={contextMenu}
                canPaste={connClipboard.canPaste}
                onClose={closeContextMenu}
                onOpenEntry={(entry) => {
                  if (entry.is_dir) {
                    connBrowser.navigate(entry.name);
                  } else {
                    connBrowser.downloadFile(joinPath(connBrowser.currentPath, entry.name));
                  }
                }}
                onDownloadEntry={(entry) => {
                  connBrowser.downloadFile(joinPath(connBrowser.currentPath, entry.name));
                }}
                onCopyNames={(names) => connClipboard.setCopy(names)}
                onCutNames={(names) => connClipboard.setCut(names)}
                onPasteToDir={(targetDirPath) => {
                  void connClipboard.paste(targetDirPath);
                }}
                onRenameEntry={(entry) => startRename(entry.name)}
                onDeleteNames={(names) => {
                  if (names.length === 1) {
                    setConfirmDelete(names[0]);
                  } else {
                    for (const name of names) toggleSelected(name);
                    setConfirmBatchDelete(true);
                  }
                }}
                onUploadHere={() => {
                  fileInputRef.current?.click();
                }}
                onRefresh={connBrowser.refresh}
              />
            )}
          </div>
        )}

        {/* ---- Split view ---- */}
        {inSplitMode && activeSource && (
          <div className="flex-1 flex flex-col p-4 md:p-6 overflow-hidden">
            <SplitView
              leftSource={activeSource}
              leftProtocol={activeTab?.protocol}
              rightSource={tabs.splitSource}
              onTransfer={(files, src, dst, dstPath) =>
                transfers.startTransfer(files, src, dst, dstPath)
              }
              onSelectRightSource={() => {
                // Add a new tab to pick the split target
                tabs.addTab({ type: "new", label: "Pick split target" });
              }}
              onSetSplitTarget={tabs.setSplitTarget}
            />
          </div>
        )}

        {/* ---- Idle state: no tabs with content ---- */}
        {!isNewTab && !isBrowsingTab && (
          <div className="flex-1 flex items-center justify-center p-4">
            <Card>
              <EmptyState
                icon={FolderOpen}
                title={t('empty.title')}
                description={t('empty.description')}
              />
            </Card>
          </div>
        )}
      </div>

      {/* Transfer progress bar */}
      <TransferProgressBar
        transfers={transfers.transfers}
        onCancel={transfers.cancelTransfer}
        onClearCompleted={transfers.clearCompleted}
      />

      {/* Overlays */}
      <DeleteConfirmOverlay
        name={confirmDelete}
        onCancel={() => { setConfirmDelete(null); }}
        onConfirm={handleDeleteConfirm}
      />
      <BatchDeleteConfirmOverlay
        open={confirmBatchDelete}
        selectionCount={selectionCount}
        onCancel={() => { setConfirmBatchDelete(false); }}
        onConfirm={handleBatchDeleteConfirm}
      />
      <TextPreviewOverlay
        previewContent={previewContent}
        currentPath={currentPath}
        onClose={closePreview}
        onDownload={downloadFile}
      />
    </div>
  );
}
