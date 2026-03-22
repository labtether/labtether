"use client";

import type { LucideIcon } from "lucide-react";
import {
  FolderOpen,
  Download,
  Copy,
  Scissors,
  ClipboardPaste,
  Pencil,
  Trash2,
  Upload,
  RefreshCw,
  FolderOutput,
} from "lucide-react";
import type { FileEntry } from "../../../hooks/useFiles";
import type { WorkspaceContextMenuState } from "./useFileWorkspaceState";

type FilesContextMenuProps = {
  contextMenu: WorkspaceContextMenuState<FileEntry>;
  canPaste: boolean;
  onClose: () => void;
  onOpenEntry: (entry: FileEntry) => void;
  onDownloadEntry: (entry: FileEntry) => void;
  onCopyNames: (names: string[]) => void;
  onCutNames: (names: string[]) => void;
  onPasteToDir: (targetDirPath: string) => void;
  onRenameEntry: (entry: FileEntry) => void;
  onDeleteNames: (names: string[]) => void;
  onUploadHere: (targetDirPath: string) => void;
  onRefresh: () => void;
  /** Called with selected file paths when the user picks "Copy to...". */
  onCopyTo?: (files: string[]) => void;
};

function MenuItem({
  icon: Icon,
  label,
  danger,
  disabled,
  onClick,
}: {
  icon: LucideIcon;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  onClick: () => void;
}) {
  const base = "w-full flex items-center gap-2.5 px-3 py-1.5 text-xs rounded-md transition-colors duration-[var(--dur-instant)]";
  const enabled = danger
    ? `${base} text-[var(--bad)] hover:bg-[var(--bad-glow)] cursor-pointer`
    : `${base} text-[var(--text)] hover:bg-[var(--hover)] cursor-pointer`;
  const disabledStyle = `${base} text-[var(--muted)] cursor-not-allowed opacity-50`;

  return (
    <button
      type="button"
      className={disabled ? disabledStyle : enabled}
      disabled={disabled}
      onClick={onClick}
    >
      <Icon className="w-3.5 h-3.5 flex-shrink-0" strokeWidth={1.5} />
      <span>{label}</span>
    </button>
  );
}

function MenuDivider() {
  return <div className="my-1 border-t border-[var(--line)]" />;
}

export function FilesContextMenu({
  contextMenu,
  canPaste,
  onClose,
  onOpenEntry,
  onDownloadEntry,
  onCopyNames,
  onCutNames,
  onPasteToDir,
  onRenameEntry,
  onDeleteNames,
  onUploadHere,
  onRefresh,
  onCopyTo,
}: FilesContextMenuProps) {
  const selectedNames = contextMenu.names;
  const entry = contextMenu.entry;

  return (
    <div
      className="absolute inset-0 z-40"
      onClick={onClose}
      onContextMenu={(event) => {
        event.preventDefault();
        onClose();
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") onClose();
        if (event.key === "ArrowDown" || event.key === "ArrowUp") {
          event.preventDefault();
          const menu = event.currentTarget.querySelector("[role='menu']");
          const items = menu?.querySelectorAll("button:not([disabled])");
          if (!items?.length) return;
          const active = document.activeElement;
          const idx = Array.from(items).indexOf(active as Element);
          const next = event.key === "ArrowDown"
            ? items[(idx + 1) % items.length]
            : items[(idx - 1 + items.length) % items.length];
          (next as HTMLElement).focus();
        }
      }}
    >
      <div
        className="absolute min-w-[200px] rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] shadow-xl p-1"
        role="menu"
        aria-label="Files context menu"
        ref={(el) => {
          // Auto-focus first item for keyboard accessibility
          if (el) {
            const first = el.querySelector("button:not([disabled])") as HTMLElement | null;
            first?.focus();
          }
        }}
        style={{
          left: contextMenu.x,
          top: contextMenu.y,
          backdropFilter: "blur(16px) saturate(1.5)",
          WebkitBackdropFilter: "blur(16px) saturate(1.5)",
        }}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="px-3 py-1.5 text-[10px] uppercase tracking-wider text-[var(--muted)] truncate font-mono" title={entry ? entry.name : contextMenu.targetDirPath}>
          {entry ? entry.name : contextMenu.targetDirPath}
        </div>
        <MenuDivider />

        {entry && (
          <MenuItem
            icon={FolderOpen}
            label="Open"
            onClick={() => { onOpenEntry(entry); onClose(); }}
          />
        )}
        {entry && !entry.is_dir && (
          <MenuItem
            icon={Download}
            label="Download"
            onClick={() => { onDownloadEntry(entry); onClose(); }}
          />
        )}

        {selectedNames.length > 0 && (
          <>
            <MenuDivider />
            <MenuItem
              icon={Copy}
              label={selectedNames.length > 1 ? `Copy ${selectedNames.length} Items` : "Copy"}
              onClick={() => { onCopyNames(selectedNames); onClose(); }}
            />
            <MenuItem
              icon={Scissors}
              label={selectedNames.length > 1 ? `Cut ${selectedNames.length} Items` : "Cut"}
              onClick={() => { onCutNames(selectedNames); onClose(); }}
            />
            {onCopyTo && (
              <MenuItem
                icon={FolderOutput}
                label={selectedNames.length > 1 ? `Copy ${selectedNames.length} to...` : "Copy to..."}
                onClick={() => { onCopyTo(selectedNames); onClose(); }}
              />
            )}
          </>
        )}

        <MenuItem
          icon={ClipboardPaste}
          label="Paste"
          disabled={!canPaste}
          onClick={() => { onPasteToDir(contextMenu.targetDirPath); onClose(); }}
        />

        {entry && (
          <>
            <MenuDivider />
            <MenuItem
              icon={Pencil}
              label="Rename"
              onClick={() => { onRenameEntry(entry); onClose(); }}
            />
            <MenuItem
              icon={Trash2}
              label="Delete"
              danger
              onClick={() => { onDeleteNames(selectedNames.length > 0 ? selectedNames : [entry.name]); onClose(); }}
            />
          </>
        )}

        <MenuDivider />
        <MenuItem
          icon={Upload}
          label="Upload Here"
          onClick={() => { onUploadHere(contextMenu.targetDirPath); onClose(); }}
        />
        <MenuItem
          icon={RefreshCw}
          label="Refresh"
          onClick={() => { onRefresh(); onClose(); }}
        />
      </div>
    </div>
  );
}
