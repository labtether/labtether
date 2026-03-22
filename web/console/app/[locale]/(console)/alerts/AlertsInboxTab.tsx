"use client";

import { Link } from "../../../../i18n/navigation";
import { useMemo, useState } from "react";
import { ArrowDown, ArrowUp, Bell } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Select } from "../../../components/ui/Input";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import type { AlertInstance } from "../../../console/models";
import type {
  AlertSeverityFilter,
  AlertStateFilter,
} from "./alertsPageTypes";
import { AlertDetailPanel } from "./AlertDetailPanel";

type AlertsInboxTabProps = {
  loading: boolean;
  instances: AlertInstance[];
  rules: Array<{ id: string; name: string }>;
  severityFilter: AlertSeverityFilter;
  stateFilter: AlertStateFilter;
  onSeverityFilterChange: (value: AlertSeverityFilter) => void;
  onStateFilterChange: (value: AlertStateFilter) => void;
  onAck: (id: string) => void;
  onResolve: (id: string) => void;
  onGoToRule: (ruleId: string) => void;
};

export function AlertsInboxTab({
  loading,
  instances,
  rules,
  severityFilter,
  stateFilter,
  onSeverityFilterChange,
  onStateFilterChange,
  onAck,
  onResolve,
  onGoToRule,
}: AlertsInboxTabProps) {
  const [selectedAlertId, setSelectedAlertId] = useState<string | null>(null);
  const [sortField, setSortField] = useState<"started_at" | "severity">("started_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");

  const filteredAlerts = useMemo(() => {
    return instances.filter((alert) => {
      if (severityFilter !== "all" && alert.severity !== severityFilter) return false;
      if (stateFilter !== "all" && alert.status !== stateFilter) return false;
      return true;
    });
  }, [instances, severityFilter, stateFilter]);

  const sortedAlerts = useMemo(() => {
    const severityOrder: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 };
    return [...filteredAlerts].sort((a, b) => {
      let cmp: number;
      if (sortField === "severity") {
        cmp = (severityOrder[a.severity] ?? 4) - (severityOrder[b.severity] ?? 4);
      } else {
        cmp = new Date(a.started_at).getTime() - new Date(b.started_at).getTime();
      }
      return sortDir === "desc" ? -cmp : cmp;
    });
  }, [filteredAlerts, sortField, sortDir]);

  return (
    <Card className="mb-4">
      <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)]">// Alert Inbox</p>
        <div className="flex w-full flex-col gap-2 sm:ml-auto sm:w-auto sm:flex-row sm:items-center">
          <Select
            className="w-full sm:w-auto"
            value={severityFilter}
            onChange={(event) => onSeverityFilterChange(event.target.value as AlertSeverityFilter)}
          >
            <option value="all">All Severities</option>
            <option value="critical">Critical</option>
            <option value="high">High</option>
            <option value="medium">Medium</option>
            <option value="low">Low</option>
          </Select>
          <Select
            className="w-full sm:w-auto"
            value={stateFilter}
            onChange={(event) => onStateFilterChange(event.target.value as AlertStateFilter)}
          >
            <option value="all">All States</option>
            <option value="firing">Active</option>
            <option value="acknowledged">Acknowledged</option>
            <option value="resolved">Resolved</option>
          </Select>
          <Select
            className="w-full sm:w-auto"
            value={sortField}
            onChange={(event) => setSortField(event.target.value as "started_at" | "severity")}
          >
            <option value="started_at">Sort by Time</option>
            <option value="severity">Sort by Severity</option>
          </Select>
          <button
            type="button"
            className="p-1.5 rounded hover:bg-[var(--hover)] text-[var(--muted)]"
            onClick={() => setSortDir((d) => (d === "asc" ? "desc" : "asc"))}
            title={sortDir === "desc" ? "Newest / highest first" : "Oldest / lowest first"}
          >
            {sortDir === "desc" ? <ArrowDown size={14} /> : <ArrowUp size={14} />}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="space-y-1">
          <SkeletonRow />
          <SkeletonRow />
          <SkeletonRow />
        </div>
      ) : sortedAlerts.length === 0 ? (
        <EmptyState
          icon={Bell}
          title="No active alerts"
          description="Nothing is firing right now. Your lab looks stable."
          action={<Link href="/incidents" className="text-xs text-[var(--accent)] hover:underline">View Issues</Link>}
        />
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {sortedAlerts.map((alert) => {
            const isSelected = selectedAlertId === alert.id;
            return (
              <li key={alert.id} className={`py-0${isSelected ? " bg-[var(--hover)]" : ""}`}>
                <div
                  className="flex items-center gap-3 py-2.5 px-1 cursor-pointer hover:bg-[var(--hover)] transition-colors duration-150"
                  onClick={() => setSelectedAlertId(isSelected ? null : alert.id)}
                >
                  <Badge status={alert.severity} />
                  <div className="flex-1 min-w-0">
                    <span className="text-sm font-medium text-[var(--text)] block truncate">{alert.labels?.rule_name ?? alert.rule_id}</span>
                    <span className="text-xs text-[var(--muted)] block truncate">
                      {alert.annotations?.summary ?? `Started ${new Date(alert.started_at).toLocaleString()}`}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge status={alert.status} />
                    {alert.status === "firing" ? (
                      <Button size="sm" onClick={(event) => { event.stopPropagation(); onAck(alert.id); }}>Acknowledge</Button>
                    ) : null}
                    {alert.status === "acknowledged" ? (
                      <Button size="sm" onClick={(event) => { event.stopPropagation(); onResolve(alert.id); }}>Resolve</Button>
                    ) : null}
                  </div>
                </div>
                {isSelected ? (
                  <AlertDetailPanel
                    alert={alert}
                    rules={rules}
                    onAck={onAck}
                    onResolve={onResolve}
                    onGoToRule={onGoToRule}
                  />
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </Card>
  );
}
