"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { GROUPINGS, type Grouping, type Show } from "./containerModel";

/** A segmented switch: a sunk track with a raised thumb on the chosen item. */
export function Seg<T extends string>({ value, items, onChange, label, small = false }: {
  value: T;
  items: Array<[T, string]>;
  onChange: (v: T) => void;
  label: string;
  small?: boolean;
}) {
  return (
    <div className={`ct-seg${small ? " sm" : ""}`} role="radiogroup" aria-label={label}>
      {items.map(([k, text]) => (
        <button key={k} type="button" role="radio" aria-checked={k === value} className={k === value ? "on" : ""} onClick={() => onChange(k)}>
          {text}
        </button>
      ))}
    </div>
  );
}

/** A filter field with a search glyph. */
export function FilterField({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <label className="ct-filter">
      <Search aria-hidden="true" strokeWidth={2} />
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label={placeholder} />
    </label>
  );
}

/** Filter field, All / Running / Stopped, and the Group menu (Host, Stack, None). */
export function ContainersToolbar({ query, onQuery, show, onShow, shows, grouping, onGrouping }: {
  query: string;
  onQuery: (v: string) => void;
  show: Show;
  onShow: (v: Show) => void;
  /** Which states have containers; the others are left out unless chosen. */
  shows: Show[];
  grouping: Grouping;
  onGrouping: (v: Grouping) => void;
}) {
  const t = useTranslations("containers");
  return (
    <div className="ct-tools">
      <FilterField value={query} onChange={onQuery} placeholder={t("filter")} />
      <span className="lt-sp" />
      {shows.length > 1 ? <Seg value={show} items={shows.map((s) => [s, t(`show.${s}`)])} onChange={onShow} label={t("show.label")} /> : null}
      <Menu.Root modal={false}>
        <Menu.Trigger className="ct-chip">
          <span className="ct-chip-k">{t("groupBy")}</span>
          {t(`groupings.${grouping}`)}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            <Menu.RadioGroup value={grouping} onValueChange={(v) => onGrouping(v as Grouping)}>
              {GROUPINGS.map((g) => (
                <Menu.RadioItem key={g} value={g} className="lt-menu-item">
                  {t(`groupings.${g}`)}
                  <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
                </Menu.RadioItem>
              ))}
            </Menu.RadioGroup>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
