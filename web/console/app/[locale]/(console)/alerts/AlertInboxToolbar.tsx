"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { ALERT_SORTS, SEVERITIES, STATES, type AlertSort, type InboxFilter, type SeverityFilter, type StateFilter } from "./alertsPageTypes";

function Radio({ value, children }: { value: string; children: ReactNode }) {
  return (
    <Menu.RadioItem value={value} className="lt-menu-item">
      {children}
      <Menu.ItemIndicator className="lt-menu-check"><Check aria-hidden="true" /></Menu.ItemIndicator>
    </Menu.RadioItem>
  );
}

/** Severity and state filters in one menu, and the order (most severe first by default); shown in the tab row. */
export function AlertInboxToolbar({ filter, onFilter }: { filter: InboxFilter; onFilter: (next: InboxFilter) => void }) {
  const t = useTranslations("alerts.inbox");
  const tSev = useTranslations("alerts.severity");
  const tState = useTranslations("alerts.state");
  const parts = [
    filter.severity !== "all" ? tSev(filter.severity) : "",
    filter.state !== "all" ? tState(filter.state) : "",
  ].filter(Boolean);

  return (
    <>
      <Menu.Root>
        <Menu.Trigger className={`al-chip${parts.length ? " on" : ""}`}>
          <span className="al-chip-k">{t("filter")}</span>
          {parts.length ? parts.join(" · ") : t("filterAll")}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            <Menu.Label className="al-menu-h">{t("filterSeverity")}</Menu.Label>
            <Menu.RadioGroup value={filter.severity} onValueChange={(v) => onFilter({ ...filter, severity: v as SeverityFilter })}>
              <Radio value="all">{t("allSeverities")}</Radio>
              {SEVERITIES.map((s) => <Radio key={s} value={s}>{tSev(s)}</Radio>)}
            </Menu.RadioGroup>
            <Menu.Separator className="lt-menu-sep" />
            <Menu.Label className="al-menu-h">{t("filterState")}</Menu.Label>
            <Menu.RadioGroup value={filter.state} onValueChange={(v) => onFilter({ ...filter, state: v as StateFilter })}>
              <Radio value="all">{t("allStates")}</Radio>
              {STATES.map((s) => <Radio key={s} value={s}>{tState(s)}</Radio>)}
            </Menu.RadioGroup>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
      <Menu.Root>
        <Menu.Trigger className="al-chip">
          <span className="al-chip-k">{t("sort")}</span>
          {t(`sortBy.${filter.sort}`)}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            <Menu.RadioGroup value={filter.sort} onValueChange={(v) => onFilter({ ...filter, sort: v as AlertSort })}>
              {ALERT_SORTS.map((s) => <Radio key={s} value={s}>{t(`sortBy.${s}`)}</Radio>)}
            </Menu.RadioGroup>
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </>
  );
}
