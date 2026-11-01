"use client";

import { ChevronDown } from "lucide-react";
import { useTranslations } from "next-intl";
import { Fragment, useCallback, useEffect, useState } from "react";
import type { ListSection } from "./deviceListModel";
import { DeviceRow } from "./DeviceRow";

/** A set of ids kept in localStorage (open hosts, closed sections). */
function useStoredSet(key: string) {
  const [set, setSet] = useState<Set<string>>(() => new Set());
  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(key);
      if (raw) setSet(new Set(JSON.parse(raw) as string[]));
    } catch { /* private window or bad value: start empty */ }
  }, [key]);
  const flip = useCallback((id: string) => {
    setSet((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      try { window.localStorage.setItem(key, JSON.stringify([...next])); } catch { /* not kept */ }
      return next;
    });
  }, [key]);
  return [set, flip] as const;
}

/**
 * Every device in one list under one column header, cut into sections (by type, status or group). In the Type view a
 * host's VMs fold under it on tree lines, opened by the guest count in its second line.
 */
export function DeviceList({ sections, emptyText }: { sections: ListSection[]; emptyText: string }) {
  const t = useTranslations("devices.list");
  const [openHosts, flipHost] = useStoredSet("labtether.devices.openHosts");
  const [closed, flipSection] = useStoredSet("labtether.devices.closedSections");

  if (!sections.length) return <div className="ld-list"><p className="ld-empty">{emptyText}</p></div>;

  // ld-more: each row ends with its ⋯ (device.css adds that column).
  return (
    <div className="ld-list ld-more">
      <div className="ld-row ld-cols" aria-hidden="true">
        <span />
        <span>{t("columns.device")}</span>
        <span className="ld-ip">{t("columns.address")}</span>
        <span>{t("columns.cpu")}</span>
        <span>{t("columns.memory")}</span>
        <span>{t("columns.disk")}</span>
        <span />
      </div>
      {sections.map((section) => {
        const isClosed = closed.has(section.key);
        const title = section.labelIsName ? section.label : section.label ? t(`sections.${section.label}`) : "";
        return (
          <section key={section.key} className="ld-sec" aria-label={title || undefined}>
            {title && (
              <button type="button" className={`ld-sec-h${isClosed ? " closed" : ""}`} aria-expanded={!isClosed} onClick={() => flipSection(section.key)}>
                <ChevronDown aria-hidden="true" strokeWidth={2.4} />
                {title}
              </button>
            )}
            {!isClosed && section.rows.map(({ card, guests }) => {
              const open = openHosts.has(card.asset.id);
              return (
                <Fragment key={card.asset.id}>
                  <DeviceRow card={card} guests={guests.length} open={open} onToggleGuests={() => flipHost(card.asset.id)} />
                  {open && guests.map((guest, i) => (
                    <DeviceRow key={guest.asset.id} card={guest} nested tree={i === guests.length - 1 ? "last" : "mid"} />
                  ))}
                </Fragment>
              );
            })}
          </section>
        );
      })}
    </div>
  );
}
