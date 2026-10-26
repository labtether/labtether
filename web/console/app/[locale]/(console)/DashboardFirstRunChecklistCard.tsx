"use client";

import { Link } from "../../../i18n/navigation";
import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { ArrowRight, CheckCircle2, ChevronDown, ChevronRight, Circle } from "lucide-react";
import { Card } from "../../components/ui/Card";
import { Button } from "../../components/ui/Button";
import { COMMUNITY_URL, GITHUB_REPO_URL, MCP_GATEWAY_DOCS_URL } from "../../lib/externalLinks";

const DISMISS_KEY = "labtether.dashboard.firstRun.dismissed";
const COLLAPSE_KEY = "labtether.dashboard.firstRun.collapsed";

type DashboardFirstRunChecklistCardProps = {
  loading: boolean;
  hasError: boolean;
  deviceCount: number;
  connectorCount: number;
  recentCommandCount: number;
  recentLogCount: number;
  canWrite: boolean;
  canAdmin: boolean;
  onAddDevice: () => void;
};

type ChecklistItemId = "hub" | "device" | "connector" | "terminal" | "logs";

type ChecklistItem = {
  id: ChecklistItemId;
  done: boolean;
};

export function DashboardFirstRunChecklistCard({
  loading,
  hasError,
  deviceCount,
  connectorCount,
  recentCommandCount,
  recentLogCount,
  canWrite,
  canAdmin,
  onAddDevice,
}: DashboardFirstRunChecklistCardProps) {
  const t = useTranslations("dashboard.firstRun");
  const [dismissed, setDismissed] = useState(false);
  const [collapsed, setCollapsed] = useState(true);

  useEffect(() => {
    if (typeof window === "undefined") return;
    setDismissed(window.localStorage.getItem(DISMISS_KEY) === "true");
    const stored = window.localStorage.getItem(COLLAPSE_KEY);
    // Default to collapsed unless user explicitly expanded
    setCollapsed(stored !== "false");
  }, []);

  const items = useMemo<ChecklistItem[]>(() => {
    const hubHealthy = !loading && !hasError;
    return [
      { id: "hub", done: hubHealthy },
      { id: "device", done: deviceCount > 0 },
      { id: "connector", done: connectorCount > 0 },
      { id: "terminal", done: recentCommandCount > 0 },
      { id: "logs", done: recentLogCount > 0 },
    ];
  }, [loading, hasError, deviceCount, connectorCount, recentCommandCount, recentLogCount]);

  const completed = items.filter((item) => item.done).length;
  const allComplete = completed === items.length;

  if (dismissed) {
    return null;
  }

  const dismiss = () => {
    setDismissed(true);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(DISMISS_KEY, "true");
    }
  };

  const toggleCollapse = () => {
    const next = !collapsed;
    setCollapsed(next);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(COLLAPSE_KEY, String(next));
    }
  };

  return (
    <Card className="mb-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <button type="button" onClick={toggleCollapse} className="flex items-center gap-1.5 cursor-pointer group">
          {collapsed ? (
            <ChevronRight size={14} className="text-[var(--muted)] group-hover:text-[var(--text)] transition-colors" />
          ) : (
            <ChevronDown size={14} className="text-[var(--muted)] group-hover:text-[var(--text)] transition-colors" />
          )}
          <h2 className="text-sm font-medium text-[var(--text)]">{t("title")}</h2>
          <span className="text-xs text-[var(--muted)] ml-1">
            {allComplete ? t("complete") : t("progress", { completed, total: items.length })}
          </span>
        </button>
        <button
          type="button"
          onClick={dismiss}
          className="text-xs text-[var(--muted)] transition-colors hover:text-[var(--text)]"
        >
          {t("dismiss")}
        </button>
      </div>

      {!collapsed && (
        <>
          <p className="mt-1 text-xs text-[var(--muted)]">
            {allComplete ? t("summaryComplete") : t("summaryPending")}
          </p>

          <div className="mt-3 space-y-2">
            {items.map((item) => (
              <div key={item.id} className="flex items-start gap-2 rounded-md border border-[var(--panel-border)] p-2">
                <div className="pt-0.5">
                  {item.done ? (
                    <CheckCircle2 size={14} className="text-[var(--ok)]" />
                  ) : (
                    <Circle size={14} className="text-[var(--muted)]" />
                  )}
                </div>
                <div className="min-w-0">
                  <p className="text-xs font-medium text-[var(--text)]">{t(`items.${item.id}.label`)}</p>
                  <p className="text-[11px] text-[var(--muted)]">{t(`items.${item.id}.description`)}</p>
                </div>
              </div>
            ))}
          </div>

          {!allComplete ? (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {canWrite ? (
                <Button size="sm" variant="primary" onClick={onAddDevice}>
                  {t("addDevice")}
                </Button>
              ) : null}
              {canAdmin ? (
                <Link href="/settings" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                  {t("openConnectorSettings")} <ArrowRight size={12} />
                </Link>
              ) : null}
              {canWrite ? (
                <Link href="/terminal" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                  {t("openTerminal")} <ArrowRight size={12} />
                </Link>
              ) : null}
              <Link href="/logs" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                {t("openLogs")} <ArrowRight size={12} />
              </Link>
            </div>
          ) : (
            <div className="mt-4">
              <p className="text-xs font-medium text-[var(--text)]">
                {t("nextStepsTitle")}
              </p>
              <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2">
                {canAdmin ? (
                  <Link
                    href="/settings"
                    className="block rounded-lg px-3 py-2 bg-[var(--panel-glass)] border border-[var(--panel-border)] text-xs text-[var(--text-secondary)] hover:text-[var(--text)] transition-colors"
                  >
                    {t("setupNotifications")}
                  </Link>
                ) : null}
                <a
                  href={MCP_GATEWAY_DOCS_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="block rounded-lg px-3 py-2 bg-[var(--panel-glass)] border border-[var(--panel-border)] text-xs text-[var(--text-secondary)] hover:text-[var(--text)] transition-colors"
                >
                  {t("connectMcp")}
                </a>
                <a
                  href={COMMUNITY_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="block rounded-lg px-3 py-2 bg-[var(--panel-glass)] border border-[var(--panel-border)] text-xs text-[var(--text-secondary)] hover:text-[var(--text)] transition-colors"
                >
                  {t("community")}
                </a>
                <a
                  href={GITHUB_REPO_URL}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="block rounded-lg px-3 py-2 bg-[var(--panel-glass)] border border-[var(--panel-border)] text-xs text-[var(--text-secondary)] hover:text-[var(--text)] transition-colors"
                >
                  {t("starGithub")}
                </a>
              </div>
            </div>
          )}
        </>
      )}
    </Card>
  );
}
