"use client";

import { useTranslations } from "next-intl";
import { Link } from "../../../../../i18n/navigation";
import { panelHref, type DeviceTab } from "./deviceTabsModel";

/** The device's tab row. Each tab is a link, so the open tab lives in the address and Back returns to it. */
export function DeviceTabBar({ nodeId, tabs, current }: { nodeId: string; tabs: DeviceTab[]; current: string }) {
  const t = useTranslations("devices.page");
  return (
    <nav className="dv-tabs" aria-label={t("tabsLabel")}>
      {tabs.map((tab) => (
        <Link key={tab.key} href={panelHref(nodeId, tab.panel)} aria-current={tab.key === current ? "page" : undefined}>
          {tab.labelKey ? t(`tabs.${tab.labelKey}`) : tab.label}
        </Link>
      ))}
    </nav>
  );
}

/** The switch inside a tab (System, Logs): one link per view, the open one raised. Nothing for a single view. */
export function DeviceViewSwitch({ nodeId, views, current, label }: { nodeId: string; views: string[]; current: string; label: string }) {
  const t = useTranslations("devices.page.views");
  if (views.length < 2) return null;
  return (
    <nav className="dv-seg" aria-label={label}>
      {views.map((view) => (
        <Link key={view} href={panelHref(nodeId, view)} replace scroll={false} aria-current={view === current ? "page" : undefined}>
          {t(view)}
        </Link>
      ))}
    </nav>
  );
}
