"use client";

import { Monitor } from "lucide-react";
import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { Link, usePathname } from "../../../i18n/navigation";
import { useAuth } from "../../contexts/AuthContext";
import { useDesktopSession } from "../../contexts/DesktopSessionContext";
import { isActiveHref, navMain, navTools, sectionFor, sectionTabs, settingsSection, type NavItem } from "../../lib/navigation";
import { meetsMinimumRole } from "../../lib/roles";
import { AccountMenu } from "./AccountMenu";

/**
 * The 2.0 sidebar contents, shared by the desktop panel and the phone drawer: the wordmark, one flat list of pages,
 * the per-device tools a little apart, then Settings and the account at the bottom. An entry that stands for several
 * pages (Automations, Settings) is current on any of them.
 */
export function SidebarNav({ onNavigate, close }: { onNavigate?: () => void; close?: ReactNode }) {
  const t = useTranslations("nav");
  const pathname = usePathname();
  const { user } = useAuth();
  const { activeSession } = useDesktopSession();
  const section = sectionFor(pathname);
  const settingsTabs = sectionTabs(settingsSection, user?.role);

  const link = (item: NavItem, href = item.href) => {
    const current = section ? section.href === item.href : isActiveHref(pathname, item.href);
    return (
      <Link key={item.href} href={href} onClick={onNavigate} aria-current={current ? "page" : undefined}>
        <item.icon aria-hidden="true" strokeWidth={1.75} />
        <span>{t(item.translationKey)}</span>
      </Link>
    );
  };
  const allowed = (items: NavItem[]) => items.filter((item) => meetsMinimumRole(user?.role, item.minimumRole));
  const tools = allowed(navTools);

  return (
    <>
      <div className="lt-brand">
        <Link href="/" onClick={onNavigate} className="lt-brand-link">
          <img src="/logo.svg" alt="" width={28} height={28} aria-hidden="true" />
          {t("brand")}
        </Link>
        {close}
      </div>
      <div className="lt-nav">{allowed(navMain).map((item) => link(item))}</div>
      {tools.length > 0 && <div className="lt-nav lt-nav-tools">{tools.map((item) => link(item))}</div>}
      <div className="lt-side-foot">
        {activeSession && (
          <Link href={`/nodes/${activeSession.nodeId}`} onClick={onNavigate} className="lt-session">
            <Monitor aria-hidden="true" strokeWidth={1.75} />
            <span>{activeSession.nodeName}</span>
            <i className="lt-live" aria-hidden="true" />
          </Link>
        )}
        {settingsTabs.length > 0 && <div className="lt-nav">{link(settingsSection, settingsTabs[0].href)}</div>}
        <AccountMenu />
      </div>
    </>
  );
}
