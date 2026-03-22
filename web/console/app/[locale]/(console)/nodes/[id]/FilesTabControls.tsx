"use client";

import React from "react";
import type { WorkspaceClipboard } from "../../files/useFileWorkspaceState";

type FilesTabControlsProps = {
  showHidden: boolean;
  onShowHiddenChange: (next: boolean) => void;
  onRefresh: () => void;
  loading: boolean;
  actionBusy: boolean;
  writeEnabled: boolean;
  onToggleWriteActions: () => void;
  currentPath: string;
  clipboard: WorkspaceClipboard | null;
  onNavigateUp: () => void;
  disableNavigateUp: boolean;
  pathInput: string;
  onPathInputChange: (next: string) => void;
  onNavigateToPath: () => void;
  uploadInputRef: React.RefObject<HTMLInputElement | null>;
  onUploadSelectedFiles: () => void;
  newFolderName: string;
  onNewFolderNameChange: (next: string) => void;
  onCreateFolder: () => void;
  disableCreateFolder: boolean;
};

export function FilesTabControls({
  showHidden,
  onShowHiddenChange,
  onRefresh,
  loading,
  actionBusy,
  writeEnabled,
  onToggleWriteActions,
  currentPath,
  clipboard,
  onNavigateUp,
  disableNavigateUp,
  pathInput,
  onPathInputChange,
  onNavigateToPath,
  uploadInputRef,
  onUploadSelectedFiles,
  newFolderName,
  onNewFolderNameChange,
  onCreateFolder,
  disableCreateFolder,
}: FilesTabControlsProps) {
  return (
    <>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "12px", marginBottom: "12px", flexWrap: "wrap" }}>
        <h2 style={{ margin: 0, fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>Files</h2>
        <div style={{ display: "flex", alignItems: "center", gap: "8px", flexWrap: "wrap" }}>
          <label style={{ display: "flex", alignItems: "center", gap: "6px", fontSize: "12px", color: "var(--muted)" }}>
            <input
              type="checkbox"
              checked={showHidden}
              onChange={(event) => onShowHiddenChange(event.target.checked)}
            />
            Show hidden
          </label>
          <button
            onClick={onRefresh}
            disabled={loading || actionBusy}
            style={{
              padding: "5px 10px",
              fontSize: "11px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: loading || actionBusy ? "default" : "pointer",
              background: "transparent",
              color: "var(--muted)",
              opacity: loading || actionBusy ? 0.5 : 1,
            }}
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
          <button
            onClick={onToggleWriteActions}
            style={{
              padding: "5px 10px",
              fontSize: "11px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: "pointer",
              background: writeEnabled ? "var(--warn-glow)" : "transparent",
              color: writeEnabled ? "var(--warn)" : "var(--muted)",
            }}
          >
            {writeEnabled ? "Disable Write Actions" : "Enable Write Actions"}
          </button>
        </div>
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: "8px", marginBottom: "12px", flexWrap: "wrap" }}>
        <span style={{ fontSize: "11px", color: "var(--muted)" }}>Path:</span>
        <code style={{ fontSize: "11px", color: "var(--text)", padding: "2px 6px", borderRadius: "4px", background: "var(--surface)" }}>{currentPath}</code>
        {clipboard ? (
          <span style={{ fontSize: "11px", color: "var(--muted)" }}>
            Clipboard: {clipboard.items[0]?.name ?? ""} ({clipboard.mode})
          </span>
        ) : null}
        <button
          onClick={onNavigateUp}
          disabled={disableNavigateUp}
          style={{
            padding: "4px 8px",
            fontSize: "11px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            cursor: loading || actionBusy ? "default" : "pointer",
            background: "transparent",
            color: "var(--muted)",
            opacity: loading || actionBusy ? 0.5 : 1,
          }}
        >
          Up
        </button>
        <input
          type="text"
          value={pathInput}
          onChange={(event) => onPathInputChange(event.target.value)}
          placeholder="Enter path"
          style={{
            minWidth: "220px",
            padding: "5px 10px",
            fontSize: "12px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            background: "transparent",
            color: "var(--text)",
            outline: "none",
          }}
        />
        <button
          onClick={onNavigateToPath}
          disabled={loading || actionBusy}
          style={{
            padding: "4px 8px",
            fontSize: "11px",
            borderRadius: "6px",
            border: "1px solid var(--line)",
            cursor: loading || actionBusy ? "default" : "pointer",
            background: "transparent",
            color: "var(--muted)",
            opacity: loading || actionBusy ? 0.5 : 1,
          }}
        >
          Go
        </button>
      </div>

      {writeEnabled ? (
        <div style={{ display: "grid", gridTemplateColumns: "minmax(180px, 1fr) auto auto minmax(140px, auto) auto", gap: "8px", alignItems: "center", marginBottom: "12px" }}>
          <input
            ref={uploadInputRef}
            type="file"
            multiple
            style={{
              fontSize: "12px",
              color: "var(--muted)",
            }}
          />
          <button
            onClick={() => {
              onUploadSelectedFiles();
            }}
            disabled={loading || actionBusy}
            style={{
              padding: "6px 10px",
              fontSize: "11px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: loading || actionBusy ? "default" : "pointer",
              background: "transparent",
              color: "var(--text)",
              opacity: loading || actionBusy ? 0.5 : 1,
              whiteSpace: "nowrap",
            }}
          >
            Upload
          </button>
          <input
            type="text"
            value={newFolderName}
            onChange={(event) => onNewFolderNameChange(event.target.value)}
            placeholder="New folder name"
            style={{
              padding: "6px 10px",
              fontSize: "12px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              background: "transparent",
              color: "var(--text)",
              outline: "none",
            }}
          />
          <button
            onClick={onCreateFolder}
            disabled={loading || actionBusy || disableCreateFolder}
            style={{
              padding: "6px 10px",
              fontSize: "11px",
              borderRadius: "6px",
              border: "1px solid var(--line)",
              cursor: loading || actionBusy ? "default" : "pointer",
              background: "transparent",
              color: "var(--text)",
              opacity: loading || actionBusy ? 0.5 : 1,
              whiteSpace: "nowrap",
            }}
          >
            Create Folder
          </button>
          <span style={{ fontSize: "11px", color: "var(--muted)", textAlign: "right" }}>
            Restricted to agent home directory.
          </span>
        </div>
      ) : (
        <p style={{ fontSize: "11px", color: "var(--muted)", marginTop: 0, marginBottom: "12px" }}>
          Read-only mode is active. Enable write actions to upload, create, paste, rename, move, or delete.
        </p>
      )}
    </>
  );
}
