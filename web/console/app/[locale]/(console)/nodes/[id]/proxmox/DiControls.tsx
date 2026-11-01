"use client";

// Controls for the integration tabs: the view switch (with a More menu for long sets), the row ⋯ menu, a menu
// button that picks one option, the filter field, sortable column labels and a confirm dialog for destructive work.

import { useState, type ReactNode } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { useTranslations } from "next-intl";
import { ArrowDown, ArrowUp, Check, ChevronDown, MoreHorizontal, Search } from "lucide-react";
import { ConfirmDialog } from "../../../../../components/ui/ConfirmDialog";

export type DiOption<T extends string = string> = { id: T; label: string };

/**
 * The switch between a tab's views. The first `inline` options show as segments; the rest go in a More menu whose
 * segment shows the chosen view's name while one of them is open.
 */
export function DiSeg<T extends string>({ options, value, onChange, label, inline = 6 }: {
  options: ReadonlyArray<DiOption<T>>;
  value: T;
  onChange: (value: T) => void;
  label: string;
  inline?: number;
}) {
  const t = useTranslations("devices-proxmox.kit");
  if (options.length < 2) return null;
  const shown = options.length > inline + 1 ? options.slice(0, inline) : options;
  const rest = options.slice(shown.length);
  const restChosen = rest.find((option) => option.id === value);
  return (
    <div className="di-seg" role="group" aria-label={label}>
      {shown.map((option) => (
        <button key={option.id} type="button" aria-pressed={option.id === value} onClick={() => onChange(option.id)}>
          {option.label}
        </button>
      ))}
      {rest.length > 0 ? (
        <Menu.Root modal={false}>
          <Menu.Trigger asChild>
            <button type="button" aria-pressed={!!restChosen} className="di-seg-more">
              {restChosen ? restChosen.label : t("more")}
              <ChevronDown aria-hidden="true" />
            </button>
          </Menu.Trigger>
          <Menu.Portal>
            <Menu.Content className="lt-menu di-menu" align="end" sideOffset={6}>
              {rest.map((option) => (
                <Menu.Item key={option.id} className="lt-menu-item" onSelect={() => onChange(option.id)}>
                  {option.label}
                  {option.id === value ? <Check className="lt-menu-check" aria-hidden="true" /> : null}
                </Menu.Item>
              ))}
            </Menu.Content>
          </Menu.Portal>
        </Menu.Root>
      ) : null}
    </div>
  );
}

export type DiMenuItem =
  | { label: string; onSelect: () => void; danger?: boolean; disabled?: boolean; hint?: string }
  | { separator: true };

/** The row's ⋯ menu. It shows on hover, focus or while open; items marked danger read in the problem colour. */
export function DiRowMenu({ label, items }: { label: string; items: DiMenuItem[] }) {
  const visible = items.filter((item, index) => !("separator" in item) || (index > 0 && index < items.length - 1));
  if (!visible.some((item) => !("separator" in item))) return <span className="di-more-slot" />;
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger asChild>
        <button type="button" className="di-more" aria-label={label}>
          <MoreHorizontal aria-hidden="true" />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu di-menu" align="end" sideOffset={4}>
          {visible.map((item, index) =>
            "separator" in item ? (
              <Menu.Separator key={`sep-${index}`} className="lt-menu-sep" />
            ) : (
              <Menu.Item
                key={`${item.label}-${index}`}
                className={`lt-menu-item${item.danger ? " di-danger" : ""}`}
                disabled={item.disabled}
                onSelect={item.onSelect}
              >
                {item.label}
                {item.hint ? <span className="lt-menu-hint">{item.hint}</span> : null}
              </Menu.Item>
            ),
          )}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** A secondary button that opens a menu of options and marks the chosen one ("All domains ▾"). */
export function DiPick<T extends string>({ options, value, onChange, label }: {
  options: ReadonlyArray<DiOption<T>>;
  value: T;
  onChange: (value: T) => void;
  label: string;
}) {
  const chosen = options.find((option) => option.id === value) ?? options[0];
  return (
    <Menu.Root modal={false}>
      <Menu.Trigger asChild>
        <button type="button" className="di-pick" aria-label={`${label}: ${chosen?.label ?? ""}`}>
          {chosen?.label}
          <ChevronDown aria-hidden="true" />
        </button>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu di-menu" align="end" sideOffset={6}>
          <Menu.RadioGroup value={value} onValueChange={(next) => onChange(next as T)}>
            {options.map((option) => (
              <Menu.RadioItem key={option.id} value={option.id} className="lt-menu-item">
                {option.label}
                <Menu.ItemIndicator className="lt-menu-check">
                  <Check aria-hidden="true" />
                </Menu.ItemIndicator>
              </Menu.RadioItem>
            ))}
          </Menu.RadioGroup>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** The filter field of a toolbar. */
export function DiFilter({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <label className="di-filter">
      <Search aria-hidden="true" />
      <input type="search" value={value} placeholder={placeholder} aria-label={placeholder} onChange={(event) => onChange(event.target.value)} />
    </label>
  );
}

export type DiSortState<K extends string> = { key: K; dir: "asc" | "desc" };

/** A column label that sorts the list; the arrow shows only on the active column. */
export function DiSortHead<K extends string>({ id, label, sort, onSort, right = false, className = "" }: {
  id: K;
  label: string;
  sort: DiSortState<K>;
  onSort: (next: DiSortState<K>) => void;
  right?: boolean;
  className?: string;
}) {
  const active = sort.key === id;
  const next: DiSortState<K> = { key: id, dir: active && sort.dir === "asc" ? "desc" : "asc" };
  return (
    <span className={`${right ? "r" : ""} ${className}`} aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : undefined}>
      <button type="button" className={`di-sort${active ? " on" : ""}`} onClick={() => onSort(next)}>
        {label}
        {active ? (sort.dir === "asc" ? <ArrowUp aria-hidden="true" /> : <ArrowDown aria-hidden="true" />) : null}
      </button>
    </span>
  );
}

export type DiAsk = {
  title: string;
  message: string;
  confirmLabel: string;
  danger?: boolean;
  onConfirm: () => Promise<void> | void;
};

/** One confirm dialog for a tab: ask(...) opens it; the dialog closes after the confirmed work settles. */
export function useDiConfirm(): { ask: (request: DiAsk) => void; dialog: ReactNode } {
  const [request, setRequest] = useState<DiAsk | null>(null);
  const [busy, setBusy] = useState(false);
  const dialog = (
    <ConfirmDialog
      open={request !== null}
      onClose={() => { if (!busy) setRequest(null); }}
      onConfirm={() => {
        if (!request) return;
        setBusy(true);
        void Promise.resolve()
          .then(() => request.onConfirm())
          .catch(() => undefined)
          .finally(() => {
            setBusy(false);
            setRequest(null);
          });
      }}
      title={request?.title ?? ""}
      message={request?.message ?? ""}
      confirmLabel={request?.confirmLabel}
      danger={request?.danger ?? true}
      loading={busy}
    />
  );
  return { ask: setRequest, dialog };
}
