"use client";

import { FolderOpen } from "lucide-react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { FileTabBar } from "./FileTabBar";
import { FilesBrowserCard } from "./FilesBrowserCard";
import { FilesContextMenu } from "./FilesContextMenu";
import {
BatchDeleteConfirmOverlay,
DeleteConfirmOverlay,
TextPreviewOverlay,
} from "./FilesOverlays";
import { FilesStatusPanels } from "./FilesStatusPanels";
import { FilesToolbarCard } from "./FilesToolbarCard";
import { NewTabPage } from "./NewTabPage";
import { SplitView } from "./SplitView";
import { TransferProgressBar } from "./TransferProgressBar";
import { joinPath } from "./fileWorkspaceUtils";
import { useFilesWorkspace } from "./useFilesWorkspace";

export default function FilesPage() {
  const t = useTranslations("files");
  const {
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
  } = useFilesWorkspace();


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
