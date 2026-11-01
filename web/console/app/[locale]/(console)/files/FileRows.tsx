"use client";

import type { KeyboardEvent, MouseEvent, DragEvent, ReactNode } from "react";
import type { FileEntry } from "./fileOpsClient";
import { fileIconComponent } from "./filesPageUtils";
import { formatSize } from "./fileWorkspaceUtils";
import { RowMenu, type MenuItemSpec } from "./FileMenus";
import type { ClickMods } from "./filesListModel";

// One item of the file list, as a row (list view) or a tile (grid view). The row picks on click, opens on double
// click (a folder's name opens on one click), carries a checkbox and a ⋯ menu, and edits its name in place on Rename.

export type RenameState = {
  name: string;
  value: string;
  onChange: (value: string) => void;
  onCommit: () => void;
  onCancel: () => void;
};

export type RowHandlers = {
  onPick: (name: string, mods: ClickMods) => void;
  /** ⌘/Ctrl+A on a row. */
  onSelectAll?: () => void;
  onToggle: (name: string) => void;
  onOpen: (entry: FileEntry) => void;
  onContextMenu: (event: MouseEvent, entry: FileEntry | null) => void;
  onKeyAction?: (action: "delete" | "rename", entry: FileEntry) => void;
  onDragStart?: (event: DragEvent, entry: FileEntry) => void;
  menuItems: (entry: FileEntry) => MenuItemSpec[];
  menuLabel: (name: string) => string;
  selectLabel: (name: string) => string;
  renameLabel: string;
};

type ItemProps = {
  entry: FileEntry;
  picked: boolean;
  modified: string;
  compact: boolean;
  rename: RenameState | null;
  h: RowHandlers;
};

const mods = (e: MouseEvent): ClickMods => ({ toggle: e.metaKey || e.ctrlKey, range: e.shiftKey });

function moveFocus(el: HTMLElement, step: 1 | -1) {
  const next = step === 1 ? el.nextElementSibling : el.previousElementSibling;
  if (next instanceof HTMLElement && next.dataset.fileEntryName !== undefined) next.focus();
}

function onRowKey(e: KeyboardEvent<HTMLElement>, entry: FileEntry, h: RowHandlers) {
  if (e.target !== e.currentTarget) return;
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "a" && h.onSelectAll) { e.preventDefault(); h.onSelectAll(); }
  else if (e.key === "Enter") { e.preventDefault(); h.onOpen(entry); }
  else if (e.key === " ") { e.preventDefault(); h.onToggle(entry.name); }
  else if (e.key === "ArrowDown" || e.key === "ArrowRight") { e.preventDefault(); moveFocus(e.currentTarget, 1); }
  else if (e.key === "ArrowUp" || e.key === "ArrowLeft") { e.preventDefault(); moveFocus(e.currentTarget, -1); }
  else if ((e.key === "Delete" || e.key === "Backspace") && h.onKeyAction) { e.preventDefault(); h.onKeyAction("delete", entry); }
  else if (e.key === "F2" && h.onKeyAction) { e.preventDefault(); h.onKeyAction("rename", entry); }
}

/** The type icon; in a row its slot also holds the row's checkbox, shown while picking or when the slot is hovered. */
function NameCell({ entry, rename, h, iconSize, pick }: { entry: FileEntry; rename: RenameState | null; h: RowHandlers; iconSize: number; pick?: ReactNode }) {
  const Icon = fileIconComponent(entry.name, entry.is_dir);
  const glyph = <Icon aria-hidden="true" width={iconSize} height={iconSize} strokeWidth={1.75} />;
  const icon = pick ? <span className="fx-ic">{glyph}{pick}</span> : glyph;
  if (rename && rename.name === entry.name) {
    return (
      <span className="fx-nm">
        {icon}
        <input
          className="fx-edit"
          aria-label={h.renameLabel}
          value={rename.value}
          autoFocus
          spellCheck={false}
          autoComplete="off"
          onFocus={(e) => {
            const dot = e.target.value.lastIndexOf(".");
            e.target.setSelectionRange(0, dot > 0 && !entry.is_dir ? dot : e.target.value.length);
          }}
          onChange={(e) => rename.onChange(e.target.value)}
          onKeyDown={(e) => {
            e.stopPropagation();
            if (e.key === "Enter") rename.onCommit();
            if (e.key === "Escape") rename.onCancel();
          }}
          onBlur={rename.onCommit}
          onClick={(e) => e.stopPropagation()}
          onDoubleClick={(e) => e.stopPropagation()}
        />
      </span>
    );
  }
  if (entry.is_dir) {
    return (
      <span className="fx-nm">
        {icon}
        <button type="button" className="fx-nm-b" tabIndex={-1} onClick={(e) => { e.stopPropagation(); h.onOpen(entry); }}>
          {entry.name}
        </button>
      </span>
    );
  }
  return <span className="fx-nm">{icon}<span className="fx-nm-t">{entry.name}</span></span>;
}

function Pick({ entry, picked, h }: { entry: FileEntry; picked: boolean; h: RowHandlers }) {
  return (
    <input
      type="checkbox"
      className="fx-cbx"
      tabIndex={-1}
      checked={picked}
      aria-label={h.selectLabel(entry.name)}
      onChange={() => h.onToggle(entry.name)}
      onClick={(e) => e.stopPropagation()}
    />
  );
}

export function FileRow({ entry, picked, modified, compact, rename, h }: ItemProps) {
  return (
    <div
      role="row"
      tabIndex={0}
      aria-selected={picked}
      className={`fx-row${picked ? " sel" : ""}`}
      data-file-entry-name={entry.name}
      draggable={h.onDragStart ? true : undefined}
      onDragStart={h.onDragStart ? (e) => h.onDragStart?.(e, entry) : undefined}
      onClick={(e) => h.onPick(entry.name, mods(e))}
      onDoubleClick={() => h.onOpen(entry)}
      onContextMenu={(e) => h.onContextMenu(e, entry)}
      onKeyDown={(e) => onRowKey(e, entry, h)}
    >
      <span role="gridcell" className="fx-cell-nm">
        <NameCell entry={entry} rename={rename} h={h} iconSize={16} pick={<Pick entry={entry} picked={picked} h={h} />} />
      </span>
      <span role="gridcell" className="fx-num fx-r">{entry.is_dir ? "" : formatSize(entry.size)}</span>
      <span role="gridcell" className="fx-num fx-c3 fx-mod">{modified}</span>
      {compact ? null : <span role="gridcell" className="fx-mode">{entry.mode || ""}</span>}
      <span role="gridcell" className="fx-more">
        <RowMenu label={h.menuLabel(entry.name)} items={() => h.menuItems(entry)} />
      </span>
    </div>
  );
}

export function FileTile({ entry, picked, rename, h }: ItemProps) {
  return (
    <div
      role="row"
      tabIndex={0}
      aria-selected={picked}
      className={`fx-tile${picked ? " sel" : ""}`}
      data-file-entry-name={entry.name}
      title={entry.name}
      draggable={h.onDragStart ? true : undefined}
      onDragStart={h.onDragStart ? (e) => h.onDragStart?.(e, entry) : undefined}
      onClick={(e) => h.onPick(entry.name, mods(e))}
      onDoubleClick={() => h.onOpen(entry)}
      onContextMenu={(e) => h.onContextMenu(e, entry)}
      onKeyDown={(e) => onRowKey(e, entry, h)}
    >
      <span role="gridcell" className="fx-tile-pick"><Pick entry={entry} picked={picked} h={h} /></span>
      <span role="gridcell" className="fx-tile-nm"><NameCell entry={entry} rename={rename} h={h} iconSize={28} /></span>
      <span role="gridcell" className="fx-num fx-c3 fx-tile-sz">{entry.is_dir ? "" : formatSize(entry.size)}</span>
    </div>
  );
}
