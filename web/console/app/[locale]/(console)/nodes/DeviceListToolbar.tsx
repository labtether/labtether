"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, Search } from "lucide-react";
import { useTranslations } from "next-intl";
import { GROUPINGS, type Grouping } from "./deviceListModel";

/** Filter field, a "Needs attention" chip (only while something needs attention, or while it is on) and the Group menu. */
export function DeviceListToolbar({
  query,
  onQuery,
  problemsOnly,
  onProblemsOnly,
  showProblemsChip,
  grouping,
  onGrouping,
}: {
  query: string;
  onQuery: (value: string) => void;
  problemsOnly: boolean;
  onProblemsOnly: (value: boolean) => void;
  showProblemsChip: boolean;
  grouping: Grouping;
  onGrouping: (value: Grouping) => void;
}) {
  const t = useTranslations("devices.list");
  return (
    <div className="ld-tools">
      <label className="ld-filter">
        <Search aria-hidden="true" strokeWidth={2} />
        <input value={query} onChange={(e) => onQuery(e.target.value)} placeholder={t("filter")} aria-label={t("filter")} />
      </label>
      <span className="lt-sp" />
      {(showProblemsChip || problemsOnly) && (
        <button type="button" className={`ld-chip${problemsOnly ? " on" : ""}`} aria-pressed={problemsOnly} onClick={() => onProblemsOnly(!problemsOnly)}>
          {t("sections.attention")}
        </button>
      )}
      <Menu.Root>
        <Menu.Trigger className="ld-chip">
          <span className="ld-chip-k">{t("groupBy")}</span>
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
