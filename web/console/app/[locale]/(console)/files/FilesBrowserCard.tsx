"use client";

import { type RefObject } from "react";
import { Card } from "../../../components/ui/Card";
import type { FileEntry, SortField } from "../../../hooks/useFiles";
import { fileIconComponent, formatDate, formatSize } from "./filesPageUtils";
import {
  ArrowUpDown,
  ChevronUp,
  ChevronDown,
  Download,
  Pencil,
  Trash2,
  FolderOpen,
} from "lucide-react";

type FilesBrowserCardProps = {
  viewMode: "list" | "grid";
  entries: FileEntry[];
  loading: boolean;
  dragOver: boolean;
  sortField: SortField;
  sortDir: string;
  selectedEntries: Set<string>;
  allSelected: boolean;
  renamingEntry: string | null;
  renameValue: string;
  renameInputRef: RefObject<HTMLInputElement | null>;
  onToggleSort: (field: SortField) => void;
  onToggleSelectAll: () => void;
  onToggleSelected: (name: string) => void;
  onEntryClick: (entry: FileEntry) => void;
  onEntryDoubleClick: (name: string) => void;
  onDownloadEntry: (entry: FileEntry) => void;
  onStartRename: (name: string) => void;
  onRenameValueChange: (value: string) => void;
  onCommitRename: () => void;
  onCancelRename: () => void;
  onDeleteEntry: (name: string) => void;
  onBackgroundContextMenu: (event: React.MouseEvent) => void;
  onEntryContextMenu: (event: React.MouseEvent, entry: FileEntry) => void;
  onDragOver: (event: React.DragEvent) => void;
  onDragLeave: () => void;
  onDrop: (event: React.DragEvent) => void;
};

function SortIndicator({ field, activeField, dir }: { field: SortField; activeField: SortField; dir: string }) {
  if (field !== activeField) return <ArrowUpDown className="w-3 h-3 text-[var(--muted)] opacity-0 group-hover:opacity-100 transition-opacity" />;
  return dir === "asc"
    ? <ChevronUp className="w-3 h-3 text-[var(--accent)]" />
    : <ChevronDown className="w-3 h-3 text-[var(--accent)]" />;
}

export function FilesBrowserCard({
  viewMode,
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
  onToggleSort,
  onToggleSelectAll,
  onToggleSelected,
  onEntryClick,
  onEntryDoubleClick,
  onDownloadEntry,
  onStartRename,
  onRenameValueChange,
  onCommitRename,
  onCancelRename,
  onDeleteEntry,
  onBackgroundContextMenu,
  onEntryContextMenu,
  onDragOver,
  onDragLeave,
  onDrop,
}: FilesBrowserCardProps) {
  return (
    <div
      onContextMenu={onBackgroundContextMenu}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      <Card
        variant="flush"
        className={`relative${dragOver ? " ring-2 ring-[var(--accent)]" : ""}`}
      >
        {/* Loading skeleton */}
        {loading ? (
          viewMode === "grid" ? (
            <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2 p-3">
              {[...Array(12)].map((_, i) => (
                <div key={i} className="flex flex-col items-center gap-2 p-4 rounded-lg border border-[var(--line)]">
                  <div className="w-8 h-8 rounded bg-[var(--surface)] animate-pulse" />
                  <div className="h-3 w-16 rounded bg-[var(--surface)] animate-pulse" />
                  <div className="h-2.5 w-10 rounded bg-[var(--surface)] animate-pulse" />
                </div>
              ))}
            </div>
          ) : (
            <div className="flex flex-col">
              {/* List header */}
              <div className="flex items-center gap-2 px-3 py-2 border-b border-[var(--line)] text-xs font-medium text-[var(--muted)]">
                <div className="w-7 flex-shrink-0" />
                <div className="flex-1">Name</div>
                <div className="w-20 flex-shrink-0 text-right">Size</div>
                <div className="w-36 flex-shrink-0">Modified</div>
                <div className="w-20 flex-shrink-0" />
              </div>
              {[...Array(6)].map((_, i) => (
                <div key={i} className="flex items-center gap-2 px-3 py-2.5 border-b border-[var(--line)] last:border-b-0">
                  <div className="w-7 flex-shrink-0" />
                  <div className="flex items-center gap-2.5 flex-1">
                    <div className="w-4 h-4 rounded bg-[var(--surface)] animate-pulse" />
                    <div className="h-3 rounded bg-[var(--surface)] animate-pulse" style={{ width: `${100 + i * 20}px` }} />
                  </div>
                  <div className="w-20 flex-shrink-0">
                    <div className="h-3 w-12 rounded bg-[var(--surface)] animate-pulse ml-auto" />
                  </div>
                  <div className="w-36 flex-shrink-0">
                    <div className="h-3 w-24 rounded bg-[var(--surface)] animate-pulse" />
                  </div>
                  <div className="w-20 flex-shrink-0" />
                </div>
              ))}
            </div>
          )
        ) : entries.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-12 px-6 text-center">
            <FolderOpen className="w-10 h-10 text-[var(--muted)]" strokeWidth={1.5} />
            <p className="text-sm text-[var(--muted)]">This folder is empty</p>
          </div>
        ) : viewMode === "grid" ? (
          /* Grid view */
          <div className="p-3">
            <div className="flex items-center gap-2 pb-2 mb-1">
              <div className="flex items-center">
                <input
                  type="checkbox"
                  className="accent-[var(--accent)]"
                  checked={allSelected}
                  onChange={onToggleSelectAll}
                  title="Select all"
                />
              </div>
              <span className="text-xs text-[var(--muted)]">{entries.length} items</span>
            </div>
            <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2">
              {entries.map((entry) => {
                const Icon = fileIconComponent(entry.name, entry.is_dir);
                const isSelected = selectedEntries.has(entry.name);

                return (
                  <div
                    key={entry.name}
                    data-file-entry-name={entry.name}
                    className={`group relative flex flex-col items-center gap-1.5 p-3 rounded-lg border cursor-pointer transition-[border-color,background-color] duration-[var(--dur-instant)] ${
                      isSelected
                        ? "border-[var(--accent)] bg-[rgba(var(--accent-rgb),0.06)]"
                        : "border-[var(--line)] hover:border-[var(--panel-border)] hover:bg-[var(--hover)]"
                    }`}
                    onClick={() => onEntryClick(entry)}
                    onContextMenu={(event) => onEntryContextMenu(event, entry)}
                    onDoubleClick={(event) => {
                      event.stopPropagation();
                      onEntryDoubleClick(entry.name);
                    }}
                  >
                    {/* Selection checkbox */}
                    <div
                      className={`absolute top-1.5 left-1.5 ${isSelected ? "opacity-100" : "opacity-0 group-hover:opacity-100"} transition-opacity`}
                      onClick={(event) => event.stopPropagation()}
                    >
                      <input
                        type="checkbox"
                        className="accent-[var(--accent)]"
                        checked={isSelected}
                        onChange={() => onToggleSelected(entry.name)}
                      />
                    </div>
                    {/* Hover actions */}
                    <div className="absolute top-1.5 right-1.5 flex gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        className="p-0.5 rounded text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors cursor-pointer bg-transparent border-none"
                        onClick={(event) => {
                          event.stopPropagation();
                          onDeleteEntry(entry.name);
                        }}
                        title="Delete"
                      >
                        <Trash2 className="w-3 h-3" />
                      </button>
                    </div>
                    {/* Icon */}
                    <Icon
                      className={`w-8 h-8 ${
                        entry.is_dir ? "text-[var(--accent)]" : "text-[var(--muted)]"
                      }`}
                      strokeWidth={1.25}
                    />
                    {/* Name */}
                    <span className="text-xs text-[var(--text)] text-center truncate w-full" title={entry.name}>
                      {entry.name}
                    </span>
                    {/* Size */}
                    <span className="text-[10px] text-[var(--muted)] tabular-nums">
                      {entry.is_dir ? "Folder" : formatSize(entry.size)}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        ) : (
          /* List view */
          <>
            {/* Column header */}
            <div className="flex items-center gap-2 px-3 py-2 border-b border-[var(--line)] text-xs font-medium text-[var(--muted)]">
              <div className="w-7 flex-shrink-0 flex items-center justify-center">
                <input
                  type="checkbox"
                  className="accent-[var(--accent)]"
                  checked={allSelected}
                  onChange={onToggleSelectAll}
                  title="Select all"
                />
              </div>
              <button
                className="group flex items-center gap-1 flex-1 min-w-0 cursor-pointer bg-transparent border-none text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] select-none transition-colors"
                onClick={() => onToggleSort("name")}
              >
                Name <SortIndicator field="name" activeField={sortField} dir={sortDir} />
              </button>
              <button
                className="group flex items-center gap-1 w-20 justify-end flex-shrink-0 cursor-pointer bg-transparent border-none text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] select-none transition-colors"
                onClick={() => onToggleSort("size")}
              >
                Size <SortIndicator field="size" activeField={sortField} dir={sortDir} />
              </button>
              <button
                className="group flex items-center gap-1 w-36 flex-shrink-0 cursor-pointer bg-transparent border-none text-xs font-medium text-[var(--muted)] hover:text-[var(--text)] select-none transition-colors"
                onClick={() => onToggleSort("mod_time")}
              >
                Modified <SortIndicator field="mod_time" activeField={sortField} dir={sortDir} />
              </button>
              <div className="w-20 flex-shrink-0" />
            </div>
            <div className="flex flex-col">
              {entries.map((entry) => {
                const Icon = fileIconComponent(entry.name, entry.is_dir);
                const isSelected = selectedEntries.has(entry.name);
                const isRenaming = renamingEntry === entry.name;

                return (
                  <div
                    key={entry.name}
                    data-file-entry-name={entry.name}
                    className={`group flex items-center gap-2 px-3 py-2 border-b border-[var(--line)] last:border-b-0 cursor-pointer transition-colors duration-[var(--dur-instant)] ${
                      isSelected
                        ? "bg-[rgba(var(--accent-rgb),0.06)]"
                        : "hover:bg-[var(--hover)]"
                    }`}
                    onClick={() => onEntryClick(entry)}
                    onContextMenu={(event) => onEntryContextMenu(event, entry)}
                    onDoubleClick={(event) => {
                      event.stopPropagation();
                      onEntryDoubleClick(entry.name);
                    }}
                  >
                    <div
                      className="w-7 flex-shrink-0 flex items-center justify-center"
                      onClick={(event) => event.stopPropagation()}
                    >
                      <input
                        type="checkbox"
                        className="accent-[var(--accent)]"
                        checked={isSelected}
                        onChange={() => onToggleSelected(entry.name)}
                      />
                    </div>

                    <div className="flex items-center gap-2.5 flex-1 min-w-0">
                      <Icon
                        className={`w-4 h-4 flex-shrink-0 ${
                          entry.is_dir ? "text-[var(--accent)]" : "text-[var(--muted)]"
                        }`}
                        strokeWidth={1.5}
                      />
                      {isRenaming ? (
                        <input
                          ref={renameInputRef}
                          type="text"
                          className="flex-1 min-w-0 bg-transparent border border-[var(--line)] rounded-md px-2 py-0.5 text-sm text-[var(--text)] focus:outline-none focus:border-[var(--accent)] transition-colors"
                          value={renameValue}
                          onChange={(event) => onRenameValueChange(event.target.value)}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") onCommitRename();
                            if (event.key === "Escape") onCancelRename();
                          }}
                          onBlur={onCommitRename}
                          onClick={(event) => event.stopPropagation()}
                        />
                      ) : (
                        <span className="text-sm text-[var(--text)] truncate">
                          {entry.name}
                        </span>
                      )}
                    </div>

                    <div className="w-20 flex-shrink-0 text-right text-xs text-[var(--muted)] tabular-nums">
                      {entry.is_dir ? "\u2014" : formatSize(entry.size)}
                    </div>

                    <div className="w-36 flex-shrink-0 text-xs text-[var(--muted)]">
                      {formatDate(entry.mod_time)}
                    </div>

                    <div className="w-20 flex-shrink-0 flex items-center justify-end gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity duration-[var(--dur-instant)]">
                      {!entry.is_dir && (
                        <button
                          className="p-1 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--surface)] transition-colors cursor-pointer bg-transparent border-none"
                          onClick={(event) => {
                            event.stopPropagation();
                            onDownloadEntry(entry);
                          }}
                          title="Download"
                        >
                          <Download className="w-3.5 h-3.5" />
                        </button>
                      )}
                      <button
                        className="p-1 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--surface)] transition-colors cursor-pointer bg-transparent border-none"
                        onClick={(event) => {
                          event.stopPropagation();
                          onStartRename(entry.name);
                        }}
                        title="Rename"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        className="p-1 rounded-md text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors cursor-pointer bg-transparent border-none"
                        onClick={(event) => {
                          event.stopPropagation();
                          onDeleteEntry(entry.name);
                        }}
                        title="Delete"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          </>
        )}

        {dragOver && (
          <div className="absolute inset-0 flex items-center justify-center bg-[rgba(var(--accent-rgb),0.08)] rounded-lg border-2 border-dashed border-[var(--accent)]">
            <p className="text-sm font-medium text-[var(--accent)]">Drop files here to upload</p>
          </div>
        )}
      </Card>
    </div>
  );
}
