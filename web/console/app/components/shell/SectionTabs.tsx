"use client";

import { useTranslations } from "next-intl";
import { Link, usePathname } from "../../../i18n/navigation";
import { useAuth } from "../../contexts/AuthContext";
import { currentTabHref, sectionFor, sectionTabs } from "../../lib/navigation";

/**
 * The tab row across a sidebar entry's pages: Automations (Actions, Schedules, Webhooks) and Settings (General,
 * Notifications, Users, Groups, Security, Agents, Health, Audit log). Nothing on other pages, or when the role may open only one of them.
 */
export function SectionTabs() {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const { user } = useAuth();
  const section = sectionFor(pathname);
  if (!section) return null;
  const tabs = sectionTabs(section, user?.role);
  if (tabs.length < 2) return null;
  const currentHref = currentTabHref(pathname, tabs);

  return (
    <nav className="lt-tabs" aria-label={t(section.translationKey)}>
      {tabs.map((tab) => {
        const current = tab.href === currentHref;
        return (
          <Link key={tab.href} href={tab.href} aria-current={current ? "page" : undefined}>
            {t(tab.translationKey)}
          </Link>
        );
      })}
    </nav>
  );
}
