"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { ChevronDown, ChevronUp, MoreHorizontal, RefreshCw, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Button } from "../../../../components/ui/Button";

/** Shared parts of the System tab's list views: filter, refresh, sortable headers, row ⋯ and list states. */

export type SortDir = "asc" | "desc";
export type SortState<K extends string> = { key: K; dir: SortDir };

/** A hub error worth showing as it is; generic fetch failures and bare status codes return "". */
export function plainError(raw: string | null | undefined): string {
  const text = (raw ?? "").trim();
  if (!text) return "";
  if (/\(\d{3}\)\s*$/.test(text) || /^failed to fetch/i.test(text) || /^\d{3}\b/.test(text)) return "";
  return text;
}

/** Bytes in binary units ("16.8 GB"); "—" when unknown. */
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / Math.pow(1024, index);
  return `${value.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

/** A number with its unit in grey ("612 MB"), right-aligned. */
export function SizeCell({ bytes, className = "", title }: { bytes: number; className?: string; title?: string }) {
  const text = formatSize(bytes);
  const [num, unit] = text.split(" ");
  if (!unit) return <span role="cell" className={`dv-n none ${className}`} title={title}>{text}</span>;
  return <span role="cell" className={`dv-n ${className}`} title={title}>{num} <span className="u">{unit}</span></span>;
}

/** A percent, amber at 85% or more. */
export function PctCell({ value, className = "" }: { value: number | null | undefined; className?: string }) {
  if (value == null || !Number.isFinite(value)) return <span role="cell" className={`dv-n none ${className}`}>—</span>;
  const shown = value > 0 && value < 1 ? value.toFixed(1) : String(Math.round(value));
  return (
    <span role="cell" className={`dv-n${value >= 85 ? " hot" : ""} ${className}`}>
      {shown}<span className="u">%</span>
    </span>
  );
}

/** A time short enough for a column: today "21:12", this week "Wed 06:31", older a date. */
export function shortTime(raw: string | undefined, locale: string): string {
  if (!raw || !raw.trim()) return "—";
  const at = new Date(raw);
  if (Number.isNaN(at.getTime())) return raw;
  const now = new Date();
  const time = new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(at);
  if (at.toDateString() === now.toDateString()) return time;
  const days = Math.abs(now.getTime() - at.getTime()) / 86_400_000;
  if (days < 6) return `${new Intl.DateTimeFormat(locale, { weekday: "short" }).format(at)} ${time}`;
  return new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }).format(at);
}

/** The filter field at the start of a view's bar. */
export function FilterField({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <label className="dsy-filter">
      <Search aria-hidden="true" strokeWidth={2} />
      <input type="search" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label={placeholder} />
    </label>
  );
}

/** Refresh as a quiet icon button; it turns while the list loads. */
export function RefreshButton({ loading, onClick }: { loading: boolean; onClick: () => void }) {
  const t = useTranslations("devices.system");
  return (
    <button type="button" className={`dsy-icon${loading ? " busy" : ""}`} onClick={onClick} disabled={loading} aria-label={t("refresh")} title={t("refresh")}>
      <RefreshCw aria-hidden="true" strokeWidth={2} />
    </button>
  );
}

/** A segmented "show" switch (All / Not running …); nothing with fewer than two choices. */
export function ShowSwitch<K extends string>({ items, value, onChange, label }: { items: Array<[K, string]>; value: K; onChange: (key: K) => void; label: string }) {
  if (items.length < 2) return null;
  return (
    <div className="dv-seg" role="group" aria-label={label}>
      {items.map(([key, text]) => (
        <button key={key} type="button" aria-pressed={key === value} onClick={() => onChange(key)}>{text}</button>
      ))}
    </div>
  );
}

/** A column header that sorts its list; the arrow shows on the sorted column only. */
export function SortHead<K extends string>({ k, label, sort, onSort, right = false, className = "", firstDir = "desc" }: {
  k: K; label: string; sort: SortState<K>; onSort: (next: SortState<K>) => void; right?: boolean; className?: string; firstDir?: SortDir;
}) {
  const on = sort.key === k;
  const Arrow = sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <span role="columnheader" aria-sort={on ? (sort.dir === "asc" ? "ascending" : "descending") : undefined} className={`${right ? "r " : ""}${className}`}>
      <button type="button" className={`dsy-sort${on ? " on" : ""}`} onClick={() => onSort({ key: k, dir: on ? (sort.dir === "asc" ? "desc" : "asc") : firstDir })}>
        {label}
        {on ? <Arrow aria-hidden="true" strokeWidth={2.4} /> : null}
      </button>
    </span>
  );
}

export type RowMenuItem = { label: string; onSelect: () => void; danger?: boolean; disabled?: boolean } | "sep";

/** The row ⋯: shows on hover or focus (always on touch screens). */
export function RowMenu({ name, items }: { name: string; items: RowMenuItem[] }) {
  const t = useTranslations("devices.system");
  return (
    <span role="cell" className="dsy-menu-cell">
      <Menu.Root>
        <Menu.Trigger className="dsy-more" aria-label={t("actionsFor", { name })}>
          <MoreHorizontal aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={4}>
            {items.map((item, i) => item === "sep"
              ? <Menu.Separator key={`sep-${i}`} className="lt-menu-sep" />
              : (
                <Menu.Item key={item.label} className={`lt-menu-item${item.danger ? " dsy-danger" : ""}`} disabled={item.disabled} onSelect={item.onSelect}>
                  {item.label}
                </Menu.Item>
              ))}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </span>
  );
}

/** Copy text to the clipboard; nothing happens where the browser refuses. */
export function copyText(text: string) {
  try { void navigator.clipboard?.writeText(text); } catch { /* clipboard not allowed */ }
}

/** Loading, error and empty states for a list; children render when there are rows. */
export function ListState({ loading, hasRows, error, loadError, empty, emptyHint, onRetry, children }: {
  loading: boolean; hasRows: boolean; error: string | null; loadError: string; empty: string; emptyHint?: string; onRetry: () => void; children: ReactNode;
}) {
  const t = useTranslations("devices.system");
  if (loading && !hasRows) {
    return (
      <div className="dv-list dsy-skel" aria-busy="true" aria-label={t("loading")}>
        {[0, 1, 2, 3].map((i) => <div key={i} className="dv-row"><i /></div>)}
      </div>
    );
  }
  if (error && !hasRows) {
    const detail = plainError(error);
    return (
      <div className="dv-list">
        <div className="dv-empty" role="alert">
          <b>{loadError}</b>
          {detail ? <span>{detail}</span> : null}
          <div className="dv-empty-act"><Button size="sm" onClick={onRetry}>{t("retry")}</Button></div>
        </div>
      </div>
    );
  }
  if (!hasRows) {
    return (
      <div className="dv-list">
        <div className="dv-empty"><b>{empty}</b>{emptyHint ? <span>{emptyHint}</span> : null}</div>
      </div>
    );
  }
  return <>{children}</>;
}

/** "12 of 25" and Clear filter, under a filtered list. */
export function FilterFoot({ shown, total, onClear }: { shown: number; total: number; onClear: () => void }) {
  const t = useTranslations("devices.system");
  if (shown === total) return null;
  return (
    <div className="dv-foot">
      <span className="dsy-num">{t("shown", { shown, total })}</span>
      <button type="button" onClick={onClear}>{t("clearFilter")}</button>
    </div>
  );
}
