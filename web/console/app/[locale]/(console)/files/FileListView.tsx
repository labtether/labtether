"use client";

import { ChevronDown, ChevronUp, Upload } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";
import type { DragEvent, MouseEvent, ReactNode } from "react";
import type { SortField } from "../../../hooks/useFiles";
import type { FileEntry } from "./fileOpsClient";
import { formatModified } from "./filesListModel";
import { FileRow, FileTile, type RenameState, type RowHandlers } from "./FileRows";

// The 2.0 file list (Files page, its second pane and a device's Files tab): a column header that sorts, plain rows
// on the sheet, a grid of tiles, the empty and loading states, and the drop target shown while files are dragged in.

export type ListSort = { field: SortField; dir: "asc" | "desc"; onSort: (field: SortField) => void };

export type FileListViewProps = {
  entries: FileEntry[];
  view?: "list" | "grid";
  compact?: boolean;
  label: string;
  loading: boolean;
  emptyText: string;
  picked: ReadonlySet<string>;
  sort?: ListSort;
  rename?: RenameState | null;
  /** Text of the drop target while something is dragged over the list; null hides it. */
  dropText?: string | null;
  onSelectAll?: () => void;
  handlers: RowHandlers;
  drop?: {
    onDragEnter?: (event: DragEvent) => void;
    onDragOver: (event: DragEvent) => void;
    onDragLeave: (event: DragEvent) => void;
    onDrop: (event: DragEvent) => void;
  };
};

function SortHead({ field, label, sort, right, prefix }: { field: SortField; label: string; sort?: ListSort; right?: boolean; prefix?: ReactNode }) {
  const cls = right ? "fx-r" : prefix ? "fx-cols-nm" : undefined;
  if (!sort) return <span role="columnheader" className={cls}>{prefix}{label}</span>;
  const on = sort.field === field;
  const Arrow = sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <span role="columnheader" className={cls} aria-sort={on ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}>
      {prefix}
      <button type="button" className={`fx-sort${on ? " on" : ""}`} onClick={() => sort.onSort(field)}>
        {label}
        {on ? <Arrow aria-hidden="true" width={12} height={12} strokeWidth={2.4} /> : null}
      </button>
    </span>
  );
}

function Skeleton({ grid }: { grid: boolean }) {
  const widths = [120, 148, 104, 136, 112, 156];
  return (
    <div className={grid ? "fx-grid" : undefined} aria-hidden="true">
      {widths.map((w) => (
        <div key={w} className={grid ? "fx-tile fx-skel-tile" : "fx-row fx-skel-row"}>
          <i className="fx-skel" style={{ width: grid ? 64 : w }} />
        </div>
      ))}
    </div>
  );
}

export function FileListView(props: FileListViewProps) {
  const { entries, view = "list", compact = false, label, loading, emptyText, picked, sort, rename = null, dropText = null, onSelectAll, drop } = props;
  const handlers = onSelectAll && !props.handlers.onSelectAll ? { ...props.handlers, onSelectAll } : props.handlers;
  const t = useTranslations("files.list");
  const locale = useLocale();
  const now = new Date();
  const grid = view === "grid" && !compact;
  const allPicked = entries.length > 0 && entries.every((e) => picked.has(e.name));
  const onBackground = (e: MouseEvent) => handlers.onContextMenu(e, null);

  const body = loading && entries.length === 0
    ? <Skeleton grid={grid} />
    : entries.length === 0
      ? <p className="fx-empty">{emptyText}</p>
      : grid
        ? (
          <div className="fx-grid">
            {entries.map((e) => (
              <FileTile key={e.name} entry={e} picked={picked.has(e.name)} modified="" compact rename={rename} h={handlers} />
            ))}
          </div>
        )
        : entries.map((e) => (
          <FileRow
            key={e.name}
            entry={e}
            picked={picked.has(e.name)}
            modified={formatModified(e.mod_time, locale, now)}
            compact={compact}
            rename={rename}
            h={handlers}
          />
        ));

  return (
    <div
      role="grid"
      aria-label={label}
      aria-multiselectable="true"
      aria-busy={loading}
      className={`fx-list${compact ? " compact" : ""}${picked.size ? " picking" : ""}${dropText ? " dropping" : ""}`}
      onContextMenu={onBackground}
      onDragEnter={drop?.onDragEnter}
      onDragOver={drop?.onDragOver}
      onDragLeave={drop?.onDragLeave}
      onDrop={drop?.onDrop}
    >
      {grid ? null : (
        <div role="row" className="fx-row fx-cols">
          <SortHead
            field="name"
            label={t("name")}
            sort={sort}
            prefix={onSelectAll ? (
              <input type="checkbox" className="fx-cbx fx-all" checked={allPicked} aria-label={t("selectAll")} onChange={onSelectAll} disabled={entries.length === 0} />
            ) : null}
          />
          <SortHead field="size" label={t("size")} sort={sort} right />
          <SortHead field="mod_time" label={t("modified")} sort={sort} />
          {compact ? null : <span role="columnheader" className="fx-mode">{t("permissions")}</span>}
          <span role="columnheader" aria-hidden="true" />
        </div>
      )}
      {body}
      {dropText ? (
        <div className="fx-drop" aria-hidden="true">
          <Upload width={16} height={16} strokeWidth={2} />
          {dropText}
        </div>
      ) : null}
    </div>
  );
}
