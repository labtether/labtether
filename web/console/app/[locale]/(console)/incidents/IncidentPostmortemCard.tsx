import { X } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";
import type { Incident } from "../../../console/models";

function formatMTTR(createdAt: string, resolvedAt: string): string {
  const start = new Date(createdAt).getTime();
  const end = new Date(resolvedAt).getTime();
  const diffMs = end - start;
  if (diffMs < 0) return "--";
  const totalMinutes = Math.floor(diffMs / 60_000);
  const totalHours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (totalHours >= 24) {
    const days = Math.floor(totalHours / 24);
    const hours = totalHours % 24;
    return `${days}d ${hours}h`;
  }
  return `${totalHours}h ${minutes}m`;
}

type IncidentPostmortemCardProps = {
  incident: Incident;
  pmRootCause: string;
  pmActionItems: string[];
  pmLessonsLearned: string;
  pmSaving: boolean;
  pmDirty: boolean;
  onRootCauseChange: (value: string) => void;
  onAddActionItem: () => void;
  onRemoveActionItem: (index: number) => void;
  onUpdateActionItem: (index: number, value: string) => void;
  onLessonsLearnedChange: (value: string) => void;
  onSave: () => Promise<void> | void;
};

export function IncidentPostmortemCard({
  incident,
  pmRootCause,
  pmActionItems,
  pmLessonsLearned,
  pmSaving,
  pmDirty,
  onRootCauseChange,
  onAddActionItem,
  onRemoveActionItem,
  onUpdateActionItem,
  onLessonsLearnedChange,
  onSave,
}: IncidentPostmortemCardProps) {
  return (
    <Card className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <h2>Postmortem</h2>
        {incident.resolved_at ? (
          <span className="text-xs px-2 py-0.5 rounded-lg border border-[var(--line)] text-[var(--muted)]" title="Mean Time To Resolve">
            MTTR: {formatMTTR(incident.created_at, incident.resolved_at)}
          </span>
        ) : null}
      </div>

      <div className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-[var(--muted)]" htmlFor="pm-root-cause">Root Cause</label>
          <textarea
            id="pm-root-cause"
            className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--muted)] transition-colors duration-150 resize-y"
            placeholder="What was the underlying root cause?"
            rows={3}
            value={pmRootCause}
            onChange={(e) => onRootCauseChange(e.target.value)}
          />
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-[var(--muted)]">Action Items</label>
          <ul className="space-y-2">
            {pmActionItems.map((item, idx) => (
              <li key={idx} className="flex items-center gap-2">
                <Input
                  type="text"
                  placeholder={`Action item ${idx + 1}`}
                  value={item}
                  onChange={(e) => onUpdateActionItem(idx, e.target.value)}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  title="Remove item"
                  onClick={() => onRemoveActionItem(idx)}
                  disabled={pmActionItems.length <= 1 && item === ""}
                >
                  <X size={14} />
                </Button>
              </li>
            ))}
          </ul>
          <Button type="button" size="sm" onClick={onAddActionItem}>
            Add Item
          </Button>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-[var(--muted)]" htmlFor="pm-lessons">Lessons Learned</label>
          <textarea
            id="pm-lessons"
            className="w-full bg-transparent border border-[var(--line)] rounded-lg px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:border-[var(--muted)] transition-colors duration-150 resize-y"
            placeholder="What did we learn? How do we prevent this in the future?"
            rows={3}
            value={pmLessonsLearned}
            onChange={(e) => onLessonsLearnedChange(e.target.value)}
          />
        </div>

        <Button
          variant="primary"
          disabled={pmSaving || !pmDirty}
          onClick={() => void onSave()}
        >
          {pmSaving ? "Saving..." : "Save Postmortem"}
        </Button>
      </div>
    </Card>
  );
}
