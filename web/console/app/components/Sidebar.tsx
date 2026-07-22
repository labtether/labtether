"use client";

import { memo, type CSSProperties } from "react";
import { Link, usePathname } from "../../i18n/navigation";
import { useTranslations } from "next-intl";
import { useServiceStatusLabel } from "../contexts/StatusContext";
import { useDesktopSession } from "../contexts/DesktopSessionContext";
import { useAuth } from "../contexts/AuthContext";
import { LanguagePicker } from "./LanguagePicker";
import { Monitor } from "lucide-react";
import { meetsMinimumRole } from "../lib/roles";
import { navGroups } from "../lib/navigation";

const SIDEBAR_NAV_STYLE: CSSProperties = {
  background: 'rgba(var(--accent-rgb), 0.01)',
  backdropFilter: 'blur(16px) saturate(1.4)',
  WebkitBackdropFilter: 'blur(16px) saturate(1.4)',
};
const BRAND_GLOW_STYLE: CSSProperties = {
  background: 'var(--accent-glow)',
  filter: 'blur(8px)',
  animation: 'glow-breathe 4s ease-in-out infinite',
  willChange: 'opacity',
  contain: 'strict',
};
const BRAND_ICON_STYLE: CSSProperties = { boxShadow: '0 0 12px var(--accent-glow)' };
const ACTIVE_NAV_GLOW_STYLE: CSSProperties = { background: 'radial-gradient(circle, var(--accent-glow), transparent 70%)' };
const STATUS_DOT_STYLE: CSSProperties = {
  boxShadow: '0 0 4px var(--ok-glow), 0 0 12px var(--ok-glow)',
  animation: 'status-glow 3s ease-in-out infinite',
};

export const Sidebar = memo(function Sidebar() {
  const pathname = usePathname();
  const serviceStatusLabel = useServiceStatusLabel();
  const { activeSession } = useDesktopSession();
  const t = useTranslations('nav');
  const { user } = useAuth();

  return (
    <nav
      aria-label="Primary navigation"
      className="hidden md:flex flex-col fixed top-0 left-0 bottom-0 w-52 border-r border-[var(--line)] z-30"
      style={SIDEBAR_NAV_STYLE}
    >
      {/* Brand */}
      <div className="flex items-center gap-2.5 px-5 h-14 border-b border-[var(--line)]">
        <img src="/logo.svg" alt="" width={32} height={32} className="shrink-0" aria-hidden="true" />
        <div className="flex flex-col">
          <span className="text-sm font-medium text-[var(--text)] font-[family-name:var(--font-heading)]">{t('brand')}</span>
          <span className="text-[10px] font-mono uppercase tracking-widest text-[var(--muted)] -mt-0.5">{t('brandSub')}</span>
        </div>
      </div>

      {/* Navigation */}
      <div className="flex-1 min-h-0 overflow-y-auto py-4 px-3 space-y-5">
        {navGroups.map((group) => (
          <div key={group.category}>
            <span className="block px-2 mb-1.5 text-[10px] font-mono uppercase tracking-[0.06em] text-[var(--muted)]">
              // {t(group.categoryKey)}
            </span>
            <div className="space-y-0.5">
              {group.items
                .filter((item) => meetsMinimumRole(user?.role, item.minimumRole))
                .map((item) => {
                const Icon = item.icon;
                const isActive = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={isActive ? "page" : undefined}
                    className={`group relative flex items-center gap-2.5 px-2 h-[34px] rounded-r-lg text-sm transition-colors duration-[var(--dur-instant)] ${
                      isActive
                        ? "border-l-2 border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--text)] font-medium"
                        : "border-l-2 border-transparent text-[var(--text-secondary)] hover:bg-[var(--hover)]"
                    }`}
                  >
                    {isActive && (
                      <div className="absolute -left-1 top-1/2 -translate-y-1/2 w-8 h-8 rounded-full pointer-events-none"
                        style={ACTIVE_NAV_GLOW_STYLE} />
                    )}
                    <Icon
                      className={`w-4 h-4 transition-[color,opacity,transform] duration-[var(--dur-fast)] ${
                        isActive
                          ? "text-[var(--accent-text)] opacity-100"
                          : "opacity-50 group-hover:opacity-90 group-hover:scale-105"
                      }`}
                      strokeWidth={1.5}
                    />
                    <span>{t(item.translationKey)}</span>
                  </Link>
                );
              })}
            </div>
          </div>
        ))}
      </div>

      {/* Active desktop session indicator */}
      {activeSession && (
        <Link
          href={`/nodes/${activeSession.nodeId}`}
          className="flex items-center gap-2 px-3 py-2 mx-2 rounded-lg text-xs bg-[var(--accent)]/10 text-[var(--accent)] hover:bg-[var(--accent)]/20 transition-colors"
        >
          <Monitor className="w-3.5 h-3.5" />
          <span className="truncate">{activeSession.nodeName}</span>
          <span className="ml-auto h-2 w-2 rounded-full bg-[var(--ok)] animate-pulse" />
        </Link>
      )}

      {/* Footer */}
      <div className="flex-shrink-0 px-3 pb-3 pt-2.5 border-t border-[var(--line)] space-y-2">
        <div className="flex items-center gap-2 px-2">
          <span
            className="inline-block w-1.5 h-1.5 rounded-full bg-[var(--ok)]"
            style={STATUS_DOT_STYLE}
          />
          <span className="text-[10px] font-mono text-[var(--muted)] tabular-nums truncate">{serviceStatusLabel}</span>
        </div>
        <LanguagePicker />
      </div>
    </nav>
  );
});
