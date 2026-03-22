import { Link } from "../../../../i18n/navigation";
import { ShieldCheck } from "lucide-react";
import { Badge } from "../../../components/ui/Badge";
import { Card } from "../../../components/ui/Card";
import { EmptyState } from "../../../components/ui/EmptyState";
import { Select } from "../../../components/ui/Input";
import { SkeletonRow } from "../../../components/ui/Skeleton";
import type { Incident } from "../../../console/models";

export type IncidentStatusFilter = Incident["status"] | "all";

export function filterIncidentsByStatus(incidents: Incident[], statusFilter: IncidentStatusFilter): Incident[] {
  return incidents.filter((inc) => {
    if (statusFilter !== "all" && inc.status !== statusFilter) return false;
    return true;
  });
}

type IncidentsListViewProps = {
  incidents: Incident[];
  loading: boolean;
  statusFilter: IncidentStatusFilter;
  onStatusFilterChange: (status: IncidentStatusFilter) => void;
  onSelectIncident: (incident: Incident) => void;
};

export function IncidentsListView({
  incidents,
  loading,
  statusFilter,
  onStatusFilterChange,
  onSelectIncident,
}: IncidentsListViewProps) {
  const filteredIncidents = filterIncidentsByStatus(incidents, statusFilter);

  return (
    <>
      <Card className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <span className="text-xs text-[var(--muted)]">Status</span>
          <Select
            value={statusFilter}
            onChange={(e) => onStatusFilterChange(e.target.value as IncidentStatusFilter)}
          >
            <option value="all">All statuses</option>
            <option value="open">Open</option>
            <option value="investigating">Investigating</option>
            <option value="mitigated">Mitigated</option>
            <option value="resolved">Resolved</option>
            <option value="closed">Closed</option>
          </Select>
        </div>
      </Card>

      <Card className="mb-4">
        <h2>Issues</h2>
        {loading ? (
          <div className="space-y-1">
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
          </div>
        ) : filteredIncidents.length === 0 ? (
          <EmptyState
            icon={ShieldCheck}
            title="All clear"
            description="No open issues right now. Issues are created automatically from critical alerts, or you can open them manually."
            action={<Link href="/alerts" className="text-xs text-[var(--accent)] hover:underline">View Alerts</Link>}
          />
        ) : (
          <ul className="divide-y divide-[var(--line)]">
            {filteredIncidents.map((incident) => (
              <li
                key={incident.id}
                className="flex items-center gap-3 py-2.5 cursor-pointer hover:bg-[var(--hover)] transition-colors duration-150"
                onClick={() => onSelectIncident(incident)}
              >
                <Badge status={incident.severity} />
                <div className="flex-1 min-w-0">
                  <span className="text-sm font-medium text-[var(--text)] block truncate">{incident.title}</span>
                  <span className="text-xs text-[var(--muted)] block">
                    {incident.source === "alert_auto" ? "Alert-driven" : "Manual"}
                    {incident.assignee ? ` \u00b7 ${incident.assignee}` : ""}
                    {` \u00b7 ${new Date(incident.created_at).toLocaleString()}`}
                  </span>
                </div>
                <Badge status={incident.status} />
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
