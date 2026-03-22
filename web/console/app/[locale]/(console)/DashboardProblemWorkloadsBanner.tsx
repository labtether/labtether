import { Link } from "../../../i18n/navigation";
import { memo, useMemo } from "react";
import { Badge } from "../../components/ui/Badge";
import { Card } from "../../components/ui/Card";
import type { Asset } from "../../console/models";
import { childParentKey, hostParentKey, isInfraHost } from "../../console/taxonomy";

interface DashboardProblemWorkloadsBannerProps {
  problemWorkloads: Asset[];
  allAssets: Asset[];
}

export const DashboardProblemWorkloadsBanner = memo(function DashboardProblemWorkloadsBanner({
  problemWorkloads,
  allAssets,
}: DashboardProblemWorkloadsBannerProps) {
  // Build a host lookup map once instead of O(n) find per workload
  const hostByParentKey = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const asset of allAssets) {
      if (isInfraHost(asset)) {
        map.set(hostParentKey(asset), asset);
      }
    }
    return map;
  }, [allAssets]);

  if (problemWorkloads.length === 0) {
    return null;
  }

  return (
    <Card className="mb-4 border-[var(--warn)]/20">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-[var(--warn)] text-sm font-medium">!</span>
        <h2 className="text-sm font-medium text-[var(--text)]">
          {problemWorkloads.length} {problemWorkloads.length === 1 ? "issue" : "issues"} across your fleet
        </h2>
      </div>
      <div className="divide-y divide-[var(--panel-border)]">
        {problemWorkloads.slice(0, 5).map((workload) => {
          const parentKey = childParentKey(workload);
          const host = parentKey ? hostByParentKey.get(parentKey) : undefined;
          return (
            <div key={workload.id} className="flex items-center gap-3 py-1.5 text-sm">
              <Badge status={workload.status === "offline" || workload.status === "down" ? "bad" : "pending"} dot size="sm" />
              <Link href={`/nodes/${workload.id}`} className="text-[var(--text)] hover:underline truncate flex-1">
                {workload.name}
              </Link>
              <span className="text-xs text-[var(--muted)]">{workload.status}</span>
              {host ? <span className="text-xs text-[var(--muted)]">on {host.name}</span> : null}
            </div>
          );
        })}
      </div>
    </Card>
  );
});
