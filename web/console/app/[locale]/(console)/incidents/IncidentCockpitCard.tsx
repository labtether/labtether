import { Card } from "../../../components/ui/Card";
import type { Incident, IncidentEvent } from "../../../console/models";
import { formatMetadataLabel } from "../../../console/formatters";
import { IncidentTimeline } from "./IncidentTimeline";

type IncidentCockpitCardProps = {
  incident: Incident;
  events: IncidentEvent[];
};

export function IncidentCockpitCard({ incident, events }: IncidentCockpitCardProps) {
  return (
    <Card className="mb-4">
      <h2>{incident.title}</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mt-3">
        <div className="flex flex-wrap gap-4">
          <div className="text-center">
            <span className="text-xs text-[var(--muted)]">Source</span>
            <strong className="text-lg font-medium text-[var(--text)] block">{incident.source === "alert_auto" ? "Alert-driven" : "Manual"}</strong>
          </div>
          <div className="text-center">
            <span className="text-xs text-[var(--muted)]">Assignee</span>
            <strong className="text-lg font-medium text-[var(--text)] block">{incident.assignee || "Unassigned"}</strong>
          </div>
          <div className="text-center">
            <span className="text-xs text-[var(--muted)]">Status</span>
            <strong className="text-lg font-medium text-[var(--text)] block">{formatMetadataLabel(incident.status)}</strong>
          </div>
        </div>

        <IncidentTimeline events={events} />
      </div>
    </Card>
  );
}
