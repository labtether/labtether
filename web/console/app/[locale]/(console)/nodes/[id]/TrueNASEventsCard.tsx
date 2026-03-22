"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { eventLevelBadge, formatRelativeTime } from "./truenasTabModel";
import type { TrueNASEvent } from "./truenasTabModel";

type TrueNASEventsCardProps = {
  events: TrueNASEvent[];
  loading: boolean;
  onRefresh: () => void;
};

export function TrueNASEventsCard({ events, loading, onRefresh }: TrueNASEventsCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">Live TrueNAS Events</h2>
        <Button size="sm" onClick={onRefresh} disabled={loading}>Refresh</Button>
      </div>
      {events.length > 0 ? (
        <ul className="divide-y divide-[var(--line)]">
          {events.map((event) => (
            <li key={event.id} className="py-2.5 flex items-center gap-3">
              <Badge status={eventLevelBadge(event.level)} size="sm" />
              <span className="text-xs text-[var(--text)] flex-1 truncate">{event.message}</span>
              <span className="text-xs text-[var(--muted)] shrink-0" title={event.timestamp ? new Date(event.timestamp).toLocaleString() : ""}>
                {formatRelativeTime(event.timestamp)}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-xs text-[var(--muted)]">No recent TrueNAS events for this asset.</p>
      )}
    </Card>
  );
}
