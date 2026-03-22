"use client";

import React from "react";
import type { FileEntry } from "../../files/fileOpsClient";
import type { WorkspaceClipboard, WorkspaceContextMenuState } from "../../files/useFileWorkspaceState";

type FilesContextMenuProps = {
  contextMenu: WorkspaceContextMenuState<FileEntry> | null;
  writeEnabled: boolean;
  clipboard: WorkspaceClipboard | null;
  onClose: () => void;
  onOpenEntry: (entry: FileEntry) => void;
  onDownloadEntry: (entry: FileEntry) => void;
  onCopyNames: (names: string[]) => void;
  onCutNames: (names: string[]) => void;
  onPasteTo: (targetDirPath: string) => void;
  onRenameEntry: (entry: FileEntry) => void;
  onDeleteEntry: (entry: FileEntry) => void;
  onUploadHere: (targetDirPath: string) => void;
  onRefresh: () => void;
};

function menuItemStyle(enabled: boolean, danger = false): React.CSSProperties {
  return {
    width: "100%",
    textAlign: "left",
    fontSize: "12px",
    borderRadius: "6px",
    border: "none",
    padding: "8px 10px",
    background: "transparent",
    color: enabled ? (danger ? "var(--bad)" : "var(--text)") : "var(--muted)",
    cursor: enabled ? "pointer" : "not-allowed",
    opacity: enabled ? 1 : 0.6,
  };
}

export function FilesContextMenu({
  contextMenu,
  writeEnabled,
  clipboard,
  onClose,
  onOpenEntry,
  onDownloadEntry,
  onCopyNames,
  onCutNames,
  onPasteTo,
  onRenameEntry,
  onDeleteEntry,
  onUploadHere,
  onRefresh,
}: FilesContextMenuProps) {
  if (!contextMenu) {
    return null;
  }

  return (
    <div
      style={{
        position: "absolute",
        inset: 0,
        zIndex: 50,
      }}
      onClick={onClose}
      onContextMenu={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <div
        style={{
          position: "absolute",
          top: contextMenu.y,
          left: contextMenu.x,
          minWidth: "220px",
          background: "var(--panel)",
          border: "1px solid var(--line)",
          borderRadius: "8px",
          boxShadow: "0 16px 32px rgba(0, 0, 0, 0.25)",
          padding: "6px",
        }}
        onClick={(event) => event.stopPropagation()}
      >
        {contextMenu.entry ? (
          <button
            type="button"
            style={menuItemStyle(true)}
            onClick={() => {
              const entry = contextMenu.entry;
              if (!entry) return;
              if (entry.is_dir) {
                onOpenEntry(entry);
              } else {
                onDownloadEntry(entry);
              }
              onClose();
            }}
          >
            {contextMenu.entry.is_dir ? "Open" : "Download"}
          </button>
        ) : null}

        {contextMenu.names.length > 0 ? (
          <button
            type="button"
            style={menuItemStyle(true)}
            onClick={() => {
              onCopyNames(contextMenu.names);
              onClose();
            }}
          >
            Copy
          </button>
        ) : null}

        {contextMenu.names.length > 0 ? (
          <button
            type="button"
            style={menuItemStyle(true)}
            onClick={() => {
              onCutNames(contextMenu.names);
              onClose();
            }}
          >
            Cut
          </button>
        ) : null}

        <button
          type="button"
          style={menuItemStyle(writeEnabled && Boolean(clipboard))}
          disabled={!writeEnabled || !clipboard}
          onClick={() => {
            onPasteTo(contextMenu.targetDirPath);
            onClose();
          }}
        >
          Paste
        </button>

        {contextMenu.entry ? (
          <button
            type="button"
            style={menuItemStyle(writeEnabled)}
            disabled={!writeEnabled}
            onClick={() => {
              if (!contextMenu.entry) return;
              onRenameEntry(contextMenu.entry);
              onClose();
            }}
          >
            Rename
          </button>
        ) : null}

        {contextMenu.entry ? (
          <button
            type="button"
            style={menuItemStyle(writeEnabled, true)}
            disabled={!writeEnabled}
            onClick={() => {
              if (!contextMenu.entry) return;
              onDeleteEntry(contextMenu.entry);
              onClose();
            }}
          >
            Delete
          </button>
        ) : null}

        <button
          type="button"
          style={menuItemStyle(writeEnabled)}
          disabled={!writeEnabled}
          onClick={() => {
            onUploadHere(contextMenu.targetDirPath);
            onClose();
          }}
        >
          Upload Here
        </button>

        <button
          type="button"
          style={menuItemStyle(true)}
          onClick={() => {
            onRefresh();
            onClose();
          }}
        >
          Refresh
        </button>
      </div>
    </div>
  );
}
