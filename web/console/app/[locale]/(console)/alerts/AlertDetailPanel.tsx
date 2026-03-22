"use client";

import { Link } from "../../../../i18n/navigation";
import { Button } from "../../../components/ui/Button";
import type { AlertInstance } from "../../../console/models";

type AlertDetailPanelProps = {
  alert: AlertInstance;
  rules: Array<{ id: string; name: string }>;
  onAck: (id: string) => void;
  onResolve: (id: string) => void;
  onGoToRule: (ruleId: string) => void;
};

function formatTimestamp(ts: string): string {
  return new Date(ts).toLocaleString();
}

export function AlertDetailPanel({
  alert,
  rules,
  onAck,
  onResolve,
  onGoToRule,
}: AlertDetailPanelProps) {
  const matchedRule = rules.find((rule) => rule.id === alert.rule_id);
  const labels = alert.labels ? Object.entries(alert.labels) : [];
  const annotations = alert.annotations ? Object.entries(alert.annotations) : [];

  return (
    <div className="px-4 py-3 space-y-4 border-t border-[var(--line)] bg-[var(--surface)]">
      <div className="space-y-2">
        <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">State Timeline</p>
        <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
          <div>
            <dt className="text-xs text-[var(--muted)]">Firing since</dt>
            <dd className="text-xs text-[var(--text)]">{formatTimestamp(alert.started_at)}</dd>
          </div>
          <div>
            <dt className="text-xs text-[var(--muted)]">Last fired</dt>
            <dd className="text-xs text-[var(--text)]">{formatTimestamp(alert.last_fired_at)}</dd>
          </div>
          {alert.status === "acknowledged" ? (
            <div>
              <dt className="text-xs text-[var(--muted)]">Acknowledged at</dt>
              <dd className="text-xs text-[var(--text)]">{formatTimestamp(alert.updated_at)}</dd>
            </div>
          ) : null}
          {alert.resolved_at ? (
            <div>
              <dt className="text-xs text-[var(--muted)]">Resolved at</dt>
              <dd className="text-xs text-[var(--text)]">{formatTimestamp(alert.resolved_at)}</dd>
            </div>
          ) : null}
        </dl>
      </div>

      {labels.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Labels</p>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
            {labels.map(([key, value]) => (
              <div key={key}>
                <dt className="text-xs text-[var(--muted)]">{key}</dt>
                <dd className="text-xs text-[var(--text)]">{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}

      {annotations.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-medium uppercase tracking-wider text-[var(--muted)]">Annotations</p>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5">
            {annotations.map(([key, value]) => (
              <div key={key}>
                <dt className="text-xs text-[var(--muted)]">{key}</dt>
                <dd className="text-xs text-[var(--text)]">{value}</dd>
              </div>
            ))}
          </dl>
        </div>
      ) : null}

      <div className="flex items-center gap-2 pt-2">
        {matchedRule ? (
          <Button size="sm" onClick={() => onGoToRule(alert.rule_id)}>
            Open Rule
          </Button>
        ) : null}
        <Link href="/incidents">
          <Button size="sm">Open Issues</Button>
        </Link>
        {alert.status === "firing" ? (
          <Button variant="primary" size="sm" onClick={() => onAck(alert.id)}>
            Acknowledge
          </Button>
        ) : null}
        {alert.status === "firing" || alert.status === "acknowledged" ? (
          <Button size="sm" onClick={() => onResolve(alert.id)}>
            Resolve
          </Button>
        ) : null}
      </div>
    </div>
  );
}
