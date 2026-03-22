"use client";

import React, { useCallback, useRef } from "react";
import { type FileEntry } from "../../files/fileOpsClient";
import { useFileWorkspaceState } from "../../files/useFileWorkspaceState";
import { FilesTabControls } from "./FilesTabControls";
import { FilesEntriesTable } from "./FilesEntriesTable";
import { FilesContextMenu } from "./FilesContextMenu";
import { FilesTabOverlays, FilesTabStatus } from "./FilesTabOverlays";
import { useFilesTabClipboardActions } from "./useFilesTabClipboardActions";
import { useFilesTabContextMenuInteractions } from "./useFilesTabContextMenuInteractions";
import { useFilesTabDirectoryState } from "./useFilesTabDirectoryState";
import { useFilesTabMutations } from "./useFilesTabMutations";
import { useFilesTabWriteAccess } from "./useFilesTabWriteAccess";

export function FilesTab({ nodeId }: { nodeId: string }) {
  const uploadInputRef = useRef<HTMLInputElement | null>(null);

  const {
    actionBusy,
    setActionBusy,
    error,
    setError,
    actionMessage,
    setActionMessage,
    newFolderName,
    setNewFolderName,
    deleteTarget,
    setDeleteTarget,
    renameTarget,
    renameName,
    setRenameName,
    downloadEntry,
    createFolder,
    uploadSelectedFiles,
    renameEntry,
    commitRename,
    cancelRename,
    deleteEntry,
    commitDelete,
  } = useFilesTabMutations({ nodeId });

  const {
    selectedEntries,
    selectionNamesFromEntry,
    selectOnly,
    clipboard,
    setClipboardItems,
    clearClipboard,
    contextMenu,
    openContextMenu,
    closeContextMenu,
  } = useFileWorkspaceState<FileEntry>();

  const {
    entries,
    sortedEntries,
    currentPath,
    pathInput,
    loading,
    showHidden,
    setShowHidden,
    setPathInput,
    listDir,
    navigateUp,
    navigateTo,
    openEntry,
  } = useFilesTabDirectoryState({
    nodeId,
    setError,
  });

  const {
    filesMenuHostRef,
    handleEntryContextMenu,
    handleBackgroundContextMenu,
  } = useFilesTabContextMenuInteractions({
    currentPath,
    selectedEntries,
    selectionNamesFromEntry,
    selectOnly,
    openContextMenu,
    closeContextMenu,
  });

  const {
    writeEnabled,
    enableWriteOpen,
    setEnableWriteOpen,
    uploadDestinationPath,
    enableWriteActions,
    handleConfirmEnableWrite,
    handleUploadHere,
  } = useFilesTabWriteAccess({
    currentPath,
    uploadInputRef,
  });

  const {
    handleCopyEntry,
    handleCutEntry,
    handleCopyNames,
    handleCutNames,
    handlePasteTo,
  } = useFilesTabClipboardActions({
    nodeId,
    entries,
    currentPath,
    clipboard,
    setClipboardItems,
    clearClipboard,
    writeEnabled,
    listDir,
    setActionBusy,
    setError,
    setActionMessage,
  });

  const refreshCurrentDir = useCallback(() => {
    void listDir(currentPath);
  }, [currentPath, listDir]);

  const handleUploadSelectedFiles = useCallback(() => {
    void uploadSelectedFiles({
      currentPath,
      listDir,
      writeEnabled,
      uploadDestinationPath,
      uploadInputRef,
    });
  }, [currentPath, listDir, uploadDestinationPath, uploadSelectedFiles, writeEnabled]);

  const handleCreateFolder = useCallback(() => {
    void createFolder({
      currentPath,
      listDir,
      writeEnabled,
    });
  }, [createFolder, currentPath, listDir, writeEnabled]);

  const handleNavigateToPath = useCallback(() => {
    navigateTo(pathInput);
  }, [navigateTo, pathInput]);

  const handleDownloadEntry = useCallback((entry: FileEntry) => {
    void downloadEntry(entry, currentPath);
  }, [currentPath, downloadEntry]);

  const handleRenameEntry = useCallback((entry: FileEntry) => {
    renameEntry(entry, { currentPath, writeEnabled });
  }, [currentPath, renameEntry, writeEnabled]);

  const handleDeleteEntry = useCallback((entry: FileEntry) => {
    deleteEntry(entry, { currentPath, writeEnabled });
  }, [currentPath, deleteEntry, writeEnabled]);

  const handleCommitDelete = useCallback(() => {
    void commitDelete({ currentPath, listDir });
  }, [commitDelete, currentPath, listDir]);

  const handleCommitRename = useCallback(() => {
    void commitRename({ currentPath, listDir });
  }, [commitRename, currentPath, listDir]);

  return (
    <div
      ref={filesMenuHostRef}
      style={{
        background: "var(--panel)",
        border: "1px solid var(--line)",
        borderRadius: "8px",
        padding: "16px",
        marginBottom: "16px",
        position: "relative",
      }}
      onContextMenu={handleBackgroundContextMenu}
    >
      <FilesTabControls
        showHidden={showHidden}
        onShowHiddenChange={(next) => setShowHidden(next)}
        onRefresh={refreshCurrentDir}
        loading={loading}
        actionBusy={actionBusy}
        writeEnabled={writeEnabled}
        onToggleWriteActions={enableWriteActions}
        currentPath={currentPath}
        clipboard={clipboard}
        onNavigateUp={navigateUp}
        disableNavigateUp={loading || actionBusy || currentPath === "~" || currentPath === "/"}
        pathInput={pathInput}
        onPathInputChange={(next) => setPathInput(next)}
        onNavigateToPath={handleNavigateToPath}
        uploadInputRef={uploadInputRef}
        onUploadSelectedFiles={handleUploadSelectedFiles}
        newFolderName={newFolderName}
        onNewFolderNameChange={(next) => setNewFolderName(next)}
        onCreateFolder={handleCreateFolder}
        disableCreateFolder={newFolderName.trim() === ""}
      />

      <FilesTabStatus
        error={error}
        actionMessage={actionMessage}
      />

      <FilesEntriesTable
        loading={loading}
        actionBusy={actionBusy}
        writeEnabled={writeEnabled}
        entriesCount={entries.length}
        sortedEntries={sortedEntries}
        contextMenuEntryName={contextMenu?.entry?.name ?? null}
        onBackgroundContextMenu={handleBackgroundContextMenu}
        onEntryContextMenu={handleEntryContextMenu}
        onOpenEntry={openEntry}
        onDownloadEntry={handleDownloadEntry}
        onCopyEntry={handleCopyEntry}
        onCutEntry={handleCutEntry}
        onRenameEntry={handleRenameEntry}
        onDeleteEntry={handleDeleteEntry}
      />

      <FilesContextMenu
        contextMenu={contextMenu}
        writeEnabled={writeEnabled}
        clipboard={clipboard}
        onClose={closeContextMenu}
        onOpenEntry={openEntry}
        onDownloadEntry={handleDownloadEntry}
        onCopyNames={handleCopyNames}
        onCutNames={handleCutNames}
        onPasteTo={handlePasteTo}
        onRenameEntry={handleRenameEntry}
        onDeleteEntry={handleDeleteEntry}
        onUploadHere={handleUploadHere}
        onRefresh={refreshCurrentDir}
      />

      <FilesTabOverlays
        enableWriteOpen={enableWriteOpen}
        onCloseEnableWrite={() => {
          setEnableWriteOpen(false);
        }}
        onConfirmEnableWrite={handleConfirmEnableWrite}
        deleteTarget={deleteTarget}
        onCloseDelete={() => {
          setDeleteTarget(null);
        }}
        onConfirmDelete={handleCommitDelete}
        actionBusy={actionBusy}
        renameTarget={renameTarget}
        renameName={renameName}
        onRenameNameChange={setRenameName}
        onCommitRename={handleCommitRename}
        onCancelRename={cancelRename}
      />
    </div>
  );
}
