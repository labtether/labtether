"use client";

import React from "react";
import type { FileEntry } from "../../files/fileOpsClient";
import { formatSize, formatTime } from "../../files/fileWorkspaceUtils";

type FilesEntriesTableProps = {
  loading: boolean;
  actionBusy: boolean;
  writeEnabled: boolean;
  entriesCount: number;
  sortedEntries: FileEntry[];
  contextMenuEntryName: string | null;
  onBackgroundContextMenu: (event: React.MouseEvent) => void;
  onEntryContextMenu: (event: React.MouseEvent, entry: FileEntry) => void;
  onOpenEntry: (entry: FileEntry) => void;
  onDownloadEntry: (entry: FileEntry) => void;
  onCopyEntry: (entry: FileEntry) => void;
  onCutEntry: (entry: FileEntry) => void;
  onRenameEntry: (entry: FileEntry) => void;
  onDeleteEntry: (entry: FileEntry) => void;
};

export function FilesEntriesTable({
  loading,
  actionBusy,
  writeEnabled,
  entriesCount,
  sortedEntries,
  contextMenuEntryName,
  onBackgroundContextMenu,
  onEntryContextMenu,
  onOpenEntry,
  onDownloadEntry,
  onCopyEntry,
  onCutEntry,
  onRenameEntry,
  onDeleteEntry,
}: FilesEntriesTableProps) {
  if (loading && entriesCount === 0) {
    return <p style={{ fontSize: "14px", color: "var(--muted)" }}>Loading files...</p>;
  }

  if (sortedEntries.length === 0) {
    return (
      <div style={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "48px 0", gap: "8px" }}>
        <p style={{ fontSize: "14px", fontWeight: 500, color: "var(--text)" }}>No files returned</p>
        <p style={{ fontSize: "12px", color: "var(--muted)", textAlign: "center", maxWidth: "360px" }}>
          This directory is empty or inaccessible with current permissions.
        </p>
      </div>
    );
  }

  return (
    <div style={{ overflowX: "auto" }} onContextMenu={onBackgroundContextMenu}>
      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "12px" }}>
        <thead>
          <tr style={{ borderBottom: "1px solid var(--line)" }}>
            {["Name", "Size", "Modified", "Mode", "Actions"].map((column) => (
              <th
                key={column}
                style={{
                  padding: "6px 8px",
                  textAlign: "left",
                  fontSize: "10px",
                  fontWeight: 500,
                  color: "var(--muted)",
                  textTransform: "uppercase",
                  letterSpacing: "0.05em",
                  whiteSpace: "nowrap",
                }}
              >
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {sortedEntries.map((entry) => {
            const isContextTarget = contextMenuEntryName === entry.name;
            return (
              <tr
                key={entry.name}
                style={{
                  borderBottom: "1px solid var(--line)",
                  background: isContextTarget ? "var(--surface)" : "transparent",
                }}
                onContextMenu={(event) => onEntryContextMenu(event, entry)}
                onMouseEnter={(event) => {
                  if (!isContextTarget) {
                    (event.currentTarget as HTMLTableRowElement).style.background = "var(--hover)";
                  }
                }}
                onMouseLeave={(event) => {
                  if (!isContextTarget) {
                    (event.currentTarget as HTMLTableRowElement).style.background = "transparent";
                  }
                }}
              >
                <td style={{ padding: "6px 8px", color: "var(--text)", fontWeight: 500, maxWidth: "320px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {entry.name}
                  {entry.is_dir ? "/" : ""}
                </td>
                <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                  {entry.is_dir ? "-" : formatSize(entry.size)}
                </td>
                <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap" }}>
                  {formatTime(entry.mod_time)}
                </td>
                <td style={{ padding: "6px 8px", color: "var(--muted)", whiteSpace: "nowrap", fontFamily: "monospace" }}>
                  {entry.mode || "-"}
                </td>
                <td style={{ padding: "6px 8px" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "6px", flexWrap: "wrap" }}>
                    {entry.is_dir ? (
                      <button
                        onClick={() => onOpenEntry(entry)}
                        disabled={loading || actionBusy}
                        style={{
                          padding: "3px 8px",
                          fontSize: "10px",
                          borderRadius: "4px",
                          border: "1px solid var(--line)",
                          background: "transparent",
                          color: "var(--text)",
                          cursor: loading || actionBusy ? "default" : "pointer",
                          opacity: loading || actionBusy ? 0.5 : 1,
                        }}
                      >
                        Open
                      </button>
                    ) : (
                      <button
                        onClick={() => onDownloadEntry(entry)}
                        disabled={loading || actionBusy}
                        style={{
                          padding: "3px 8px",
                          fontSize: "10px",
                          borderRadius: "4px",
                          border: "1px solid var(--line)",
                          background: "transparent",
                          color: "var(--text)",
                          cursor: loading || actionBusy ? "default" : "pointer",
                          opacity: loading || actionBusy ? 0.5 : 1,
                        }}
                      >
                        Download
                      </button>
                    )}
                    <>
                      <button
                        onClick={() => onCopyEntry(entry)}
                        disabled={loading || actionBusy}
                        style={{
                          padding: "3px 8px",
                          fontSize: "10px",
                          borderRadius: "4px",
                          border: "1px solid var(--line)",
                          background: "transparent",
                          color: "var(--muted)",
                          cursor: loading || actionBusy ? "default" : "pointer",
                          opacity: loading || actionBusy ? 0.5 : 1,
                        }}
                      >
                        Copy
                      </button>
                      <button
                        onClick={() => onCutEntry(entry)}
                        disabled={loading || actionBusy}
                        style={{
                          padding: "3px 8px",
                          fontSize: "10px",
                          borderRadius: "4px",
                          border: "1px solid var(--line)",
                          background: "transparent",
                          color: "var(--muted)",
                          cursor: loading || actionBusy ? "default" : "pointer",
                          opacity: loading || actionBusy ? 0.5 : 1,
                        }}
                      >
                        Cut
                      </button>
                    </>
                    {writeEnabled ? (
                      <>
                        <button
                          onClick={() => onRenameEntry(entry)}
                          disabled={loading || actionBusy}
                          style={{
                            padding: "3px 8px",
                            fontSize: "10px",
                            borderRadius: "4px",
                            border: "1px solid var(--line)",
                            background: "transparent",
                            color: "var(--muted)",
                            cursor: loading || actionBusy ? "default" : "pointer",
                            opacity: loading || actionBusy ? 0.5 : 1,
                          }}
                        >
                          Rename
                        </button>
                        <button
                          onClick={() => onDeleteEntry(entry)}
                          disabled={loading || actionBusy}
                          style={{
                            padding: "3px 8px",
                            fontSize: "10px",
                            borderRadius: "4px",
                            border: "1px solid var(--line)",
                            background: "transparent",
                            color: "var(--bad)",
                            cursor: loading || actionBusy ? "default" : "pointer",
                            opacity: loading || actionBusy ? 0.5 : 1,
                          }}
                        >
                          Delete
                        </button>
                      </>
                    ) : null}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
