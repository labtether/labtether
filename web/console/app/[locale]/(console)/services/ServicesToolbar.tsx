"use client";

import * as Menu from "@radix-ui/react-dropdown-menu";
import * as Popover from "@radix-ui/react-popover";
import { Check, ChevronDown, Search, SlidersHorizontal } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import {
  countMenuFilters,
  NO_FILTERS,
  SERVICE_GROUPINGS,
  SERVICE_SORT_KEYS,
  type ServiceFilters,
  type ServiceGrouping,
  type ServiceShow,
  type ServiceSort,
  type ServiceSortKey,
} from "./servicesListModel";

function Select({ id, label, value, onChange, children }: { id: string; label: string; value: string; onChange: (v: string) => void; children: ReactNode }) {
  return (
    <div className="sv-field">
      <label htmlFor={id}>{label}</label>
      <select id={id} className="sv-input" value={value} onChange={(e) => onChange(e.target.value)}>{children}</select>
    </div>
  );
}

type ServicesToolbarProps = {
  filters: ServiceFilters;
  onFilters: (patch: Partial<ServiceFilters>) => void;
  counts: Record<ServiceShow, number>;
  categories: string[];
  hosts: Array<[string, string]>;
  sort: ServiceSort;
  onSortKey: (key: ServiceSortKey) => void;
  grouping: ServiceGrouping;
  onGrouping: (value: ServiceGrouping) => void;
};

/**
 * Filter field, then All / Down / Hidden (Down and Hidden only while they hold something or are on), the Filters
 * menu (category, device, source, status, stability and sort) and the Group menu.
 */
export function ServicesToolbar({ filters, onFilters, counts, categories, hosts, sort, onSortKey, grouping, onGrouping }: ServicesToolbarProps) {
  const t = useTranslations("services.toolbar");
  const show: ServiceShow = filters.show !== "all" ? filters.show : filters.status === "down" ? "down" : "all";
  const segs = (["all", "down", "hidden"] as ServiceShow[]).filter((k) => k === "all" || counts[k] > 0 || k === show);
  const menuCount = countMenuFilters(filters);
  const pickShow = (k: ServiceShow) => onFilters({ show: k === "down" ? "all" : k, status: k === "down" ? "down" : "all" });

  return (
    <div className="sv-tools">
      <label className="sv-filter">
        <Search aria-hidden="true" strokeWidth={2} />
        <input type="search" value={filters.query} onChange={(e) => onFilters({ query: e.target.value })} placeholder={t("filter")} aria-label={t("filter")} />
      </label>
      <span className="lt-sp" />
      {segs.length > 1 && (
        <div className="sv-seg" role="group" aria-label={t("show")}>
          {segs.map((k) => (
            <button key={k} type="button" aria-pressed={show === k} onClick={() => pickShow(k)}>{t(`shows.${k}`)}</button>
          ))}
        </div>
      )}
      <Popover.Root>
        <Popover.Trigger className="sv-chip" aria-label={t("filters")}>
          <SlidersHorizontal aria-hidden="true" strokeWidth={2} />
          <span className="sv-chip-t">{t("filters")}</span>
          {menuCount > 0 && <span className="sv-chip-n">{menuCount}</span>}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content className="sv-pop" align="end" sideOffset={6} aria-label={t("filters")}>
            <Select id="sv-f-cat" label={t("category")} value={filters.category} onChange={(v) => onFilters({ category: v })}>
              <option value="all">{t("anyCategory")}</option>
              {categories.map((c) => <option key={c} value={c}>{c}</option>)}
            </Select>
            {hosts.length > 1 && (
              <Select id="sv-f-host" label={t("device")} value={filters.host} onChange={(v) => onFilters({ host: v })}>
                <option value="all">{t("anyDevice")}</option>
                {hosts.map(([id, name]) => <option key={id} value={id}>{name}</option>)}
              </Select>
            )}
            <Select id="sv-f-src" label={t("source")} value={filters.source} onChange={(v) => onFilters({ source: v })}>
              <option value="all">{t("anySource")}</option>
              {["docker", "proxy", "scan", "manual"].map((s) => <option key={s} value={s}>{t(`sources.${s}`)}</option>)}
            </Select>
            <Select id="sv-f-status" label={t("status")} value={filters.status} onChange={(v) => onFilters({ status: v as ServiceFilters["status"] })}>
              <option value="all">{t("anyStatus")}</option>
              {["up", "down", "unknown"].map((s) => <option key={s} value={s}>{t(`statuses.${s}`)}</option>)}
            </Select>
            <Select id="sv-f-health" label={t("health")} value={filters.health} onChange={(v) => onFilters({ health: v as ServiceFilters["health"] })}>
              <option value="all">{t("anyHealth")}</option>
              <option value="unstable">{t("healths.unstable")}</option>
              <option value="changed_recently">{t("healths.changed_recently")}</option>
            </Select>
            <Select id="sv-f-sort" label={t("sort")} value={sort.key} onChange={(v) => onSortKey(v as ServiceSortKey)}>
              {SERVICE_SORT_KEYS.map((k) => <option key={k} value={k}>{t(`sorts.${k}`)}</option>)}
            </Select>
            {menuCount > 0 && (
              <div className="sv-pop-foot">
                <button type="button" className="sv-chip" onClick={() => onFilters({ ...NO_FILTERS, query: filters.query, show: filters.show, status: filters.status === "down" ? "down" : "all" })}>
                  {t("clear")}
                </button>
              </div>
            )}
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
      <Menu.Root>
        <Menu.Trigger className="sv-chip" aria-label={`${t("group")}: ${t(`groupings.${grouping}`)}`}>
          <span className="sv-chip-k">{t("group")}</span>
          {t(`groupings.${grouping}`)}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu sv-menu" align="end" sideOffset={6}>
            <Menu.RadioGroup value={grouping} onValueChange={(v) => onGrouping(v as ServiceGrouping)}>
              {SERVICE_GROUPINGS.map((g) => (
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
