"use client";

import type { ReactNode } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, Search } from "lucide-react";

/** The list filter field: a search glyph and one input (Logs, Audit log, Recordings). */
export function FilterField({ value, onChange, placeholder }: { value: string; onChange: (value: string) => void; placeholder: string }) {
  return (
    <label className="lg-filter">
      <Search aria-hidden="true" strokeWidth={2} />
      <input type="search" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label={placeholder} />
    </label>
  );
}

export type PickOption = { value: string; label: string };

/** A menu button that reads its current choice ("All devices", "Last hour"); the menu ticks the one in use. */
export function PickMenu({
  label,
  value,
  options,
  onChange,
  current,
  children,
}: {
  label: string;
  value: string;
  options: PickOption[];
  onChange: (value: string) => void;
  current?: string;
  children?: ReactNode;
}) {
  const shown = current ?? options.find((o) => o.value === value)?.label ?? value;
  return (
    <Menu.Root>
      <Menu.Trigger className={`lg-pick${value !== options[0]?.value ? " on" : ""}`} aria-label={`${label}: ${shown}`}>
        <span className="lg-pick-t">{shown}</span>
        <ChevronDown aria-hidden="true" strokeWidth={2} />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content className="lt-menu lg-menu" align="start" sideOffset={6}>
          <Menu.RadioGroup value={value} onValueChange={onChange}>
            {options.map((o) => (
              <Menu.RadioItem key={o.value} value={o.value} className="lt-menu-item">
                <span className="lg-menu-t">{o.label}</span>
                <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
              </Menu.RadioItem>
            ))}
          </Menu.RadioGroup>
          {children}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  );
}

/** One row of chips where one is on (All · a person · …); scrolls sideways when it runs out of room. */
export function ChipGroup({ label, value, options, onChange }: { label: string; value: string; options: PickOption[]; onChange: (value: string) => void }) {
  return (
    <div className="lg-chips" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" className={o.value === value ? "on" : ""} aria-pressed={o.value === value} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** A square tick box for picking rows. */
export function RowCheck({ checked, mixed, label, onChange }: { checked: boolean; mixed?: boolean; label: string; onChange: () => void }) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={mixed ? "mixed" : checked}
      aria-label={label}
      className={`lg-cbx${checked || mixed ? " on" : ""}`}
      onClick={(e) => { e.stopPropagation(); onChange(); }}
    >
      {checked ? <Check aria-hidden="true" strokeWidth={3} /> : mixed ? <span className="lg-cbx-m" aria-hidden="true" /> : null}
    </button>
  );
}

/** The mascot, one plain line and the page's one main button: only for "nothing here yet", never for a filter. */
export function EmptyLine({ line, children }: { line: string; children?: ReactNode }) {
  return (
    <div className="lg-empty-first">
      <img src="/logo.svg" alt="" width={64} height={64} aria-hidden="true" />
      <b>{line}</b>
      {children}
    </div>
  );
}
