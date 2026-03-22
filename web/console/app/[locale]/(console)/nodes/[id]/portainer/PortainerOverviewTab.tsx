"use client";

import { Card } from "../../../../../components/ui/Card";
import { usePortainerOverview } from "./usePortainerData";

type Props = {
  assetId: string;
};

function StatItem({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{label}</p>
      <p className="mt-1 text-sm font-medium text-[var(--text)]">{String(value)}</p>
    </div>
  );
}

export function PortainerOverviewTab({ assetId }: Props) {
  const { data, loading, error } = usePortainerOverview(assetId);

  if (loading && !data) {
    return (
      <Card>
        <p className="text-sm text-[var(--muted)]">Loading overview…</p>
      </Card>
    );
  }

  if (error && !data) {
    return (
      <Card>
        <p className="text-sm text-[var(--bad)]">{error}</p>
      </Card>
    );
  }

  if (!data) {
    return (
      <Card>
        <p className="text-sm text-[var(--muted)]">No overview data available.</p>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Server Info</h2>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <StatItem label="Version" value={data.version || "--"} />
          <StatItem label="Endpoint" value={data.endpoint || "--"} />
          <StatItem label="URL" value={data.url || "--"} />
        </div>
      </Card>

      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Resources</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-5">
          <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Containers</p>
            <p className="mt-1 text-sm font-medium text-[var(--text)]">{data.containers.running} running</p>
            <p className="mt-0.5 text-xs text-[var(--muted)]">{data.containers.stopped} stopped · {data.containers.total} total</p>
          </div>
          <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Stacks</p>
            <p className="mt-1 text-sm font-medium text-[var(--text)]">{data.stacks.running} running</p>
            <p className="mt-0.5 text-xs text-[var(--muted)]">{data.stacks.stopped} stopped · {data.stacks.total} total</p>
          </div>
          <StatItem label="Images" value={data.images.count} />
          <StatItem label="Volumes" value={data.volumes.count} />
          <StatItem label="Networks" value={data.networks.count} />
        </div>
      </Card>
    </div>
  );
}
