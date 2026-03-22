"use client";

import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { diskStatusBadge } from "./truenasTabModel";
import type { TrueNASDiskHealth, TrueNASSummary } from "./truenasTabModel";

type TrueNASSmartHealthCardProps = {
  loading: boolean;
  error: string | null;
  summary?: TrueNASSummary;
  warnings: string[];
  disks: TrueNASDiskHealth[];
  onRefresh: () => void;
};

export function TrueNASSmartHealthCard({ loading, error, summary, warnings, disks, onRefresh }: TrueNASSmartHealthCardProps) {
  return (
    <Card>
      <div className="flex items-center justify-between mb-3 gap-3 flex-wrap">
        <h2 className="text-sm font-medium text-[var(--text)]">SMART and Disk Health</h2>
        <Button size="sm" onClick={onRefresh} disabled={loading}>Refresh</Button>
      </div>
      {loading ? (
        <p className="text-sm text-[var(--muted)]">Loading disk health...</p>
      ) : error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : (
        <>
          {summary ? (
            <div className="grid grid-cols-2 md:grid-cols-5 gap-2 mb-3">
              <SummaryTile label="Total" value={summary.total} />
              <SummaryTile label="Healthy" value={summary.healthy} accent="ok" />
              <SummaryTile label="Warning" value={summary.warning} accent="pending" />
              <SummaryTile label="Critical" value={summary.critical} accent="bad" />
              <SummaryTile label="Unknown" value={summary.unknown} />
            </div>
          ) : null}

          {warnings.length > 0 ? (
            <ul className="mb-3 space-y-1">
              {warnings.map((warning) => (
                <li key={warning} className="text-xs text-[var(--warn)]">{warning}</li>
              ))}
            </ul>
          ) : null}

          {disks.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-[var(--line)]">
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Disk</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Status</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Temp</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">SMART</th>
                    <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Last Test</th>
                  </tr>
                </thead>
                <tbody>
                  {disks.map((disk) => (
                    <tr key={disk.name} className="border-b border-[var(--line)] border-opacity-30">
                      <td className="py-1 px-2 text-[var(--text)]">
                        <span className="font-medium">{disk.name}</span>
                        {disk.model ? <span className="text-[var(--muted)] ml-2">{disk.model}</span> : null}
                      </td>
                      <td className="py-1 px-2">
                        <Badge status={diskStatusBadge(disk.status)} size="sm" />
                      </td>
                      <td className="py-1 px-2 text-[var(--muted)]">
                        {typeof disk.temperature_celsius === "number" ? `${disk.temperature_celsius.toFixed(1)} C` : "n/a"}
                      </td>
                      <td className="py-1 px-2 text-[var(--muted)]">
                        {disk.last_test_status || disk.smart_health || "n/a"}
                      </td>
                      <td className="py-1 px-2 text-[var(--muted)]">
                        {disk.last_test_at ? new Date(disk.last_test_at).toLocaleString() : "n/a"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <p className="text-xs text-[var(--muted)]">No disks returned by the TrueNAS API.</p>
          )}
        </>
      )}
    </Card>
  );
}

export type SummaryTileProps = {
  label: string;
  value: number;
  accent?: "ok" | "pending" | "bad";
};

function SummaryTile({ label, value, accent }: SummaryTileProps) {
  const textClass = accent === "ok"
    ? "text-[var(--ok)]"
    : accent === "pending"
      ? "text-[var(--warn)]"
      : accent === "bad"
        ? "text-[var(--bad)]"
        : "text-[var(--text)]";

  return (
    <div className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
      <p className="text-[10px] uppercase tracking-wide text-[var(--muted)]">{label}</p>
      <p className={`text-sm font-medium ${textClass}`}>{value}</p>
    </div>
  );
}
