"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Link } from "../../../../i18n/navigation";
import type { AlertTab } from "./alertsPageTypes";

const TABS: Array<[AlertTab | "issues", string]> = [
  ["alerts", "/alerts"],
  ["issues", "/incidents"],
  ["rules", "/alerts?tab=rules"],
  ["silences", "/alerts?tab=silences"],
  ["delivery", "/alerts?tab=delivery"],
];

/**
 * The Alerts tab row as in the mockup (Alerts, Issues, Rules), then Silences and Delivery log. Issues is its own route
 * (/incidents); that page shows this row with current="issues". tools (optional) sit at the row's right end on wide
 * pages and under it on a phone.
 */
export function AlertTabs({ current, tools }: { current: AlertTab | "issues"; tools?: ReactNode }) {
  const t = useTranslations("alerts.tabs");
  return (
    <div className="al-tabrow">
      <nav className="lt-tabs al-tabs" aria-label={t("label")}>
        {TABS.map(([key, href]) => (
          <Link key={key} href={href} aria-current={key === current ? "page" : undefined}>
            {t(key)}
          </Link>
        ))}
      </nav>
      {tools ? <div className="al-tabtools">{tools}</div> : null}
    </div>
  );
}
