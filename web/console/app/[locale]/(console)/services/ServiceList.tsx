"use client";

import { ChevronDown, ChevronUp } from "lucide-react";
import { useTranslations } from "next-intl";
import { useCallback, useEffect, useRef, useState } from "react";
import type { WebService } from "../../../hooks/useWebServices";
import { ServiceRow, type ServiceRowActions } from "./ServiceRow";
import { serviceLayoutKey } from "./servicesLayoutHelpers";
import type { ServiceGroup, ServiceGrouping, ServiceSort, ServiceSortKey } from "./servicesListModel";

const CLOSED_KEY = "labtether.services.closedGroups";

/** Group headings someone folded, kept in this browser. */
function useClosedGroups() {
  const [closed, setClosed] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(CLOSED_KEY);
      if (raw) setClosed(new Set(JSON.parse(raw) as string[]));
    } catch { /* private window or a bad value: start open */ }
  }, []);
  const flip = useCallback((key: string) => {
    setClosed((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      try { window.localStorage.setItem(CLOSED_KEY, JSON.stringify([...next])); } catch { /* not kept */ }
      return next;
    });
  }, []);
  return [closed, flip] as const;
}

function SortButton({ k, label, sort, onSort, right = false }: {
  k: ServiceSortKey; label: string; sort: ServiceSort; onSort: (key: ServiceSortKey) => void; right?: boolean;
}) {
  const on = sort.key === k;
  const Arrow = sort.dir === "asc" ? ChevronUp : ChevronDown;
  return (
    <button type="button" className={`sv-sort${on ? " on" : ""}${right ? " sv-r" : ""}`} onClick={() => onSort(k)}
      aria-sort={on ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}>
      {label}
      {on && <Arrow aria-hidden="true" strokeWidth={2.4} />}
    </button>
  );
}

/** Select-all for what the list shows: checked when all are picked, mixed when some are. */
function CheckAll({ picked, total, onAll, label }: { picked: number; total: number; onAll: (on: boolean) => void; label: string }) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = picked > 0 && picked < total;
  }, [picked, total]);
  return <input ref={ref} type="checkbox" className="sv-cbx" checked={total > 0 && picked === total} onChange={() => onAll(picked < total)} aria-label={label} />;
}

type ServiceListProps = {
  groups: ServiceGroup[];
  grouping: ServiceGrouping;
  hostNames: Map<string, string>;
  shown: number;
  total: number;
  filtering: boolean;
  emptyText: string;
  sort: ServiceSort;
  onSort: (key: ServiceSortKey) => void;
  selected: Set<string>;
  onSelectAll: (on: boolean) => void;
  openID: string | null;
  withDetails: (service: WebService) => WebService;
  arranging: boolean;
  dragKey: string | null;
  dragOverKey: string | null;
  onClearFilters: () => void;
  actions: ServiceRowActions;
};

/** Every shown service in one sheet under one column header, cut into groups when a grouping is chosen. */
export function ServiceList(props: ServiceListProps) {
  const { groups, grouping, hostNames, sort, selected, arranging, actions } = props;
  const t = useTranslations("services.list");
  const [closed, flip] = useClosedGroups();
  const visible: WebService[] = groups.flatMap((g) => g.services);
  const picked = visible.filter((s) => selected.has(serviceLayoutKey(s))).length;

  return (
    <div className={`sv-list${selected.size > 0 ? " picking" : ""}`} role="region" aria-label={t("label")}>
      <div className="sv-row sv-cols">
        <span className="sv-lead">
          {!arranging && <CheckAll picked={picked} total={visible.length} onAll={props.onSelectAll} label={t("selectAll")} />}
        </span>
        <span />
        <SortButton k="name" label={t("columns.service")} sort={sort} onSort={props.onSort} />
        <SortButton k="latency" label={t("columns.latency")} sort={sort} onSort={props.onSort} right />
        <span />
      </div>
      {groups.map((group) => {
        const isClosed = grouping !== "none" && closed.has(group.key);
        return (
          <section key={group.key} aria-label={grouping === "none" ? undefined : group.title || t("noDevice")}>
            {grouping !== "none" && (
              <button type="button" className={`sv-gh${isClosed ? " closed" : ""}`} aria-expanded={!isClosed} onClick={() => flip(group.key)}>
                <ChevronDown aria-hidden="true" strokeWidth={2.4} />
                {group.title || t("noDevice")}
                <span className="n">{group.services.length}</span>
              </button>
            )}
            {!isClosed && group.services.map((service) => {
              const key = serviceLayoutKey(service);
              return (
                <ServiceRow
                  key={key}
                  service={service}
                  hostName={hostNames.get(service.host_asset_id) ?? service.host_asset_id}
                  selected={selected.has(key)}
                  open={props.openID === service.id}
                  detail={props.openID === service.id ? props.withDetails(service) : undefined}
                  arranging={arranging}
                  dragging={props.dragKey === key}
                  dragOver={props.dragOverKey === key}
                  actions={actions}
                />
              );
            })}
          </section>
        );
      })}
      {visible.length === 0 && <p className="sv-none">{props.emptyText}</p>}
      {props.filtering && (
        <div className="sv-lfoot">
          <span>{t("shownOf", { shown: props.shown, total: props.total })}</span>
          <button type="button" onClick={props.onClearFilters}>{t("clearFilter")}</button>
        </div>
      )}
    </div>
  );
}
