import type { ReactNode } from "react";
import { Bell, Activity, FileText, AlignLeft, RefreshCw, Zap } from "lucide-react";
import type { IncidentEvent } from "../../../console/models";

function timelineKindIcon(kind: string): ReactNode {
  switch (kind) {
    case "alert":
    case "alert_linked":
      return <Bell size={14} />;
    case "metric":
      return <Activity size={14} />;
    case "log":
      return <FileText size={14} />;
    case "note":
      return <AlignLeft size={14} />;
    case "status_change":
      return <RefreshCw size={14} />;
    default:
      return <Zap size={14} />;
  }
}

type IncidentTimelineProps = {
  events: IncidentEvent[];
};

export function IncidentTimeline({ events }: IncidentTimelineProps) {
  return (
    <div>
      <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)] mb-2">What Happened</p>
      {events.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Nothing here yet. Related events, logs, and notes will appear here as they come in.</p>
      ) : (
        <ul className="divide-y divide-[var(--line)]">
          {events.map((entry) => (
            <li key={entry.id} className="flex items-start gap-3 py-2.5">
              <span className="text-[var(--muted)] mt-0.5 shrink-0">{timelineKindIcon(entry.kind)}</span>
              <div className="flex-1 min-w-0">
                <span className="text-sm text-[var(--text)] block">{entry.title}</span>
                {entry.detail ? <span className="text-xs text-[var(--muted)] block">{entry.detail}</span> : null}
              </div>
              <span className="text-xs text-[var(--muted)]">{new Date(entry.created_at).toLocaleString()}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
