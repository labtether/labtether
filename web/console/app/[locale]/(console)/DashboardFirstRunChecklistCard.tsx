"use client";

import { Link } from "../../../i18n/navigation";
import { useEffect, useMemo, useState } from "react";
import { ArrowRight, CheckCircle2, ChevronDown, ChevronRight, Circle } from "lucide-react";
import { Card } from "../../components/ui/Card";
import { Button } from "../../components/ui/Button";

const DISMISS_KEY = "labtether.dashboard.firstRun.dismissed";
const COLLAPSE_KEY = "labtether.dashboard.firstRun.collapsed";

type DashboardFirstRunChecklistCardProps = {
  loading: boolean;
  hasError: boolean;
  deviceCount: number;
  connectorCount: number;
  recentCommandCount: number;
  recentLogCount: number;
  onAddDevice: () => void;
};

type ChecklistItem = {
  id: string;
  label: string;
  description: string;
  done: boolean;
};

export function DashboardFirstRunChecklistCard({
  loading,
  hasError,
  deviceCount,
  connectorCount,
  recentCommandCount,
  recentLogCount,
  onAddDevice,
}: DashboardFirstRunChecklistCardProps) {
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
      {
        id: "hub",
        label: "Hub healthy",
        description: "Dashboard can load status without startup errors.",
        done: hubHealthy,
      },
      {
        id: "device",
        label: "First device connected",
        description: "Add one node with agent install flow.",
        done: deviceCount > 0,
      },
      {
        id: "connector",
        label: "First connector configured",
        description: "Save one connector and run first sync.",
        done: connectorCount > 0,
      },
      {
        id: "terminal",
        label: "Terminal workflow validated",
        description: "Run at least one command session.",
        done: recentCommandCount > 0,
      },
      {
        id: "logs",
        label: "Observability verified",
        description: "Confirm recent logs are visible.",
        done: recentLogCount > 0,
      },
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
          <h2 className="text-sm font-medium text-[var(--text)]">First-Run Checklist</h2>
          <span className="text-xs text-[var(--muted)] ml-1">
            {allComplete ? "complete" : `${completed}/${items.length}`}
          </span>
        </button>
        <button
          type="button"
          onClick={dismiss}
          className="text-xs text-[var(--muted)] transition-colors hover:text-[var(--text)]"
        >
          Dismiss
        </button>
      </div>

      {!collapsed && (
        <>
          <p className="mt-1 text-xs text-[var(--muted)]">
            {allComplete
              ? "Great start. Core workflows are validated."
              : "Finish this once to lock in a reliable setup."}
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
                  <p className="text-xs font-medium text-[var(--text)]">{item.label}</p>
                  <p className="text-[11px] text-[var(--muted)]">{item.description}</p>
                </div>
              </div>
            ))}
          </div>

          {!allComplete ? (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <Button size="sm" variant="primary" onClick={onAddDevice}>
                + Add Device
              </Button>
              <Link href="/settings" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                Open Connector Settings <ArrowRight size={12} />
              </Link>
              <Link href="/terminal" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                Open Terminal <ArrowRight size={12} />
              </Link>
              <Link href="/logs" className="text-xs text-[var(--accent)] hover:underline inline-flex items-center gap-1">
                Open Logs <ArrowRight size={12} />
              </Link>
            </div>
          ) : (
            <div className="mt-3 text-xs text-[var(--ok)]">Checklist complete. You can dismiss this card at any time.</div>
          )}
        </>
      )}
    </Card>
  );
}
