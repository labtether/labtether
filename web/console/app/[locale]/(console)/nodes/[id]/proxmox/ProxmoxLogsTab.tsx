"use client";

import { useState } from "react";
import { Card } from "../../../../../components/ui/Card";
import { useProxmoxFetch } from "./useProxmoxData";

type SyslogResponse = {
  lines?: string[];
  total?: number;
};

const LIMIT_OPTIONS = [100, 250, 500, 1000] as const;
type Limit = (typeof LIMIT_OPTIONS)[number];

type Props = {
  proxmoxNode: string;
  proxmoxCollectorID: string;
};

export function ProxmoxLogsTab({ proxmoxNode, proxmoxCollectorID }: Props) {
  const [limit, setLimit] = useState<Limit>(250);

  const collectorSuffix = proxmoxCollectorID
    ? `&collector_id=${encodeURIComponent(proxmoxCollectorID)}`
    : "";

  const path = proxmoxNode
    ? `/api/proxmox/nodes/${encodeURIComponent(proxmoxNode)}/syslog?limit=${limit}${collectorSuffix}`
    : null;

  const { data, loading, error, refresh } = useProxmoxFetch<SyslogResponse>(path);

  const lines: string[] = data?.lines ?? (Array.isArray(data) ? (data as string[]) : []);

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-medium text-[var(--text)]">
          Syslog{lines.length > 0 ? ` (${lines.length} lines)` : ""}
        </h2>
        <div className="ml-auto flex items-center gap-2">
          <select
            className="rounded border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--text)]"
            value={limit}
            onChange={(e) => { setLimit(Number(e.target.value) as Limit); }}
          >
            {LIMIT_OPTIONS.map((opt) => (
              <option key={opt} value={opt}>Last {opt} lines</option>
            ))}
          </select>
          <button
            className="text-xs text-[var(--accent)] hover:underline"
            onClick={refresh}
            disabled={loading}
          >
            {loading ? "Loading..." : "Refresh"}
          </button>
        </div>
      </div>
      {error ? (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      ) : loading && lines.length === 0 ? (
        <p className="text-xs text-[var(--muted)]">Loading syslog...</p>
      ) : lines.length > 0 ? (
        <pre className="max-h-[600px] overflow-auto rounded bg-[var(--surface)] p-3 text-[10px] leading-relaxed text-[var(--muted)] whitespace-pre-wrap">
          {lines.join("\n")}
        </pre>
      ) : (
        <p className="text-xs text-[var(--muted)]">No syslog entries returned.</p>
      )}
    </Card>
  );
}
