"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Boxes } from "lucide-react";
import { PageHeader } from "../../../components/PageHeader";
import { Card } from "../../../components/ui/Card";
import { Select } from "../../../components/ui/Input";
import { EmptyState } from "../../../components/ui/EmptyState";
import { ContainerFleetOverview } from "../../../components/containers/ContainerFleetOverview";
import { ContainerHostCard } from "../../../components/containers/ContainerHostCard";
import type { HostWithContainers } from "../../../components/containers/ContainerFleetOverview";
import {
  fetchDockerHosts,
  fetchDockerContainers,
  fetchPortainerEndpoints,
  fetchPortainerContainers,
  type DockerHostSummary,
  type DockerContainer,
} from "../../../../lib/docker";
import { Plus } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Tip } from "../../../components/ui/Tip";
import { DockerHostPickerModal } from "../../../components/containers/DockerHostPickerModal";
import { useToast } from "../../../contexts/ToastContext";
import { useRouter } from "../../../../i18n/navigation";

type StateFilter = "all" | "running" | "stopped";

type HostData = {
  host: DockerHostSummary;
  containers: DockerContainer[];
};

export default function ContainersPage() {
  const [hostData, setHostData] = useState<HostData[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Filters
  const [hostFilter, setHostFilter] = useState<string>("all");
  const [stateFilter, setStateFilter] = useState<StateFilter>("all");
  const [stackFilter, setStackFilter] = useState<string>("all");
  const [pickerMode, setPickerMode] = useState<"container" | "stack" | null>(null);
  const { addToast } = useToast();
  const router = useRouter();

  const handleAddAction = useCallback(
    (mode: "container" | "stack") => {
      if (loading || hostData.length === 0) return;
      if (hostData.length === 1) {
        const nodeId = `docker-host-${hostData[0].host.normalized_id}`;
        const path = mode === "container"
          ? `/nodes/${encodeURIComponent(nodeId)}/new-container`
          : `/nodes/${encodeURIComponent(nodeId)}/new-compose`;
        addToast("info", `Deploying to ${hostData[0].host.agent_id}`);
        router.push(path);
        return;
      }
      setPickerMode(mode);
    },
    [loading, hostData, addToast, router],
  );

  const load = useCallback(async () => {
    try {
      const [dockerHosts, portainerEndpoints] = await Promise.all([
        fetchDockerHosts(),
        fetchPortainerEndpoints(),
      ]);

      const taggedDockerHosts = dockerHosts.map((h) => ({ ...h, source: "docker" as const }));
      const allHosts = [...taggedDockerHosts, ...portainerEndpoints];

      const results = await Promise.allSettled(
        allHosts.map(async (host) => {
          const containers = host.source === "portainer"
            ? await fetchPortainerContainers(host.agent_id)
            : await fetchDockerContainers(host.agent_id);
          return { host, containers };
        })
      );

      const loaded: HostData[] = [];
      for (const result of results) {
        if (result.status === "fulfilled") {
          loaded.push(result.value);
        }
      }
      setHostData(loaded);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load container fleet data");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const interval = setInterval(() => void load(), 15_000);
    return () => clearInterval(interval);
  }, [load]);

  // Derived stacks for filter dropdown
  const allStacks = useMemo(() => {
    const names = new Set<string>();
    for (const { containers } of hostData) {
      for (const c of containers) {
        if (c.stack_name) names.add(c.stack_name);
      }
    }
    return Array.from(names).sort();
  }, [hostData]);

  // Apply filters
  const filteredHosts = useMemo((): HostWithContainers[] => {
    return hostData
      .filter(({ host }) => hostFilter === "all" || host.agent_id === hostFilter)
      .map(({ host, containers }) => {
        const filtered = containers.filter((c) => {
          if (stateFilter === "running" && c.state.toLowerCase() !== "running") return false;
          if (stateFilter === "stopped" && c.state.toLowerCase() === "running") return false;
          if (stackFilter !== "all" && c.stack_name !== stackFilter) return false;
          return true;
        });
        return { host, containers: filtered };
      })
      .filter(({ containers }) => containers.length > 0 || hostFilter !== "all");
  }, [hostData, hostFilter, stateFilter, stackFilter]);

  const hasAnyContainers = hostData.some(({ containers }) => containers.length > 0);

  return (
    <>
      <PageHeader
        title="Containers"
        subtitle="Fleet-level container observability across all Docker hosts"
        action={
          <div className="flex gap-2">
            {!loading && hostData.length === 0 ? (
              <Tip content="No Docker hosts available">
                <Button variant="ghost" size="sm" disabled>
                  <Plus size={14} />
                  Add Stack
                </Button>
              </Tip>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                loading={loading && hostData.length === 0}
                disabled={loading && hostData.length === 0}
                onClick={() => handleAddAction("stack")}
              >
                <Plus size={14} />
                Add Stack
              </Button>
            )}
            {!loading && hostData.length === 0 ? (
              <Tip content="No Docker hosts available">
                <Button variant="primary" size="sm" disabled>
                  <Plus size={14} />
                  Add Container
                </Button>
              </Tip>
            ) : (
              <Button
                variant="primary"
                size="sm"
                loading={loading && hostData.length === 0}
                disabled={loading && hostData.length === 0}
                onClick={() => handleAddAction("container")}
              >
                <Plus size={14} />
                Add Container
              </Button>
            )}
          </div>
        }
      />

      {/* Filters */}
      <Card className="mb-6 flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2">
          <label className="text-xs text-[var(--muted)] whitespace-nowrap">Host</label>
          <Select
            value={hostFilter}
            onChange={(e) => setHostFilter(e.target.value)}
            className="min-w-[140px]"
          >
            <option value="all">All hosts</option>
            {hostData.map(({ host }) => (
              <option key={host.agent_id} value={host.agent_id}>
                {host.agent_id}
              </option>
            ))}
          </Select>
        </div>

        <div className="flex items-center gap-2">
          <label className="text-xs text-[var(--muted)] whitespace-nowrap">State</label>
          <Select
            value={stateFilter}
            onChange={(e) => setStateFilter(e.target.value as StateFilter)}
            className="min-w-[120px]"
          >
            <option value="all">All states</option>
            <option value="running">Running</option>
            <option value="stopped">Stopped</option>
          </Select>
        </div>

        {allStacks.length > 0 ? (
          <div className="flex items-center gap-2">
            <label className="text-xs text-[var(--muted)] whitespace-nowrap">Stack</label>
            <Select
              value={stackFilter}
              onChange={(e) => setStackFilter(e.target.value)}
              className="min-w-[140px]"
            >
              <option value="all">All stacks</option>
              {allStacks.map((stack) => (
                <option key={stack} value={stack}>
                  {stack}
                </option>
              ))}
            </Select>
          </div>
        ) : null}

        {loading ? (
          <span className="ml-auto text-[10px] font-mono text-[var(--muted)]">Refreshing...</span>
        ) : null}
      </Card>

      {loading && hostData.length === 0 ? (
        <div className="space-y-3">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-14 rounded-lg bg-[var(--panel-glass)] border border-[var(--panel-border)] animate-pulse"
            />
          ))}
        </div>
      ) : error != null ? (
        <Card>
          <p className="text-sm text-[var(--bad)]">{error}</p>
        </Card>
      ) : !hasAnyContainers ? (
        <EmptyState
          icon={Boxes}
          title="No containers found"
          description="No Docker hosts with containers were discovered. Make sure the LabTether agent is running on your Docker hosts."
        />
      ) : filteredHosts.length === 0 ? (
        <EmptyState
          title="No containers match your filters"
          description="Try adjusting the filters above."
        />
      ) : (
        <div className="space-y-8">
          {/* Fleet overview (summary + top-N) */}
          <section>
            <p className="text-[10px] font-mono uppercase tracking-[0.06em] text-[var(--muted)] mb-3">
              // fleet overview
            </p>
            <ContainerFleetOverview hosts={filteredHosts} />
          </section>

          {/* Per-host cards */}
          <section>
            <p className="text-[10px] font-mono uppercase tracking-[0.06em] text-[var(--muted)] mb-3">
              // per-host
            </p>
            <div className="space-y-3">
              {filteredHosts.map(({ host, containers }) => (
                <ContainerHostCard
                  key={host.agent_id}
                  host={host}
                  containers={containers}
                  defaultOpen={filteredHosts.length === 1}
                />
              ))}
            </div>
          </section>
        </div>
      )}
      <DockerHostPickerModal
        open={pickerMode !== null}
        onClose={() => setPickerMode(null)}
        mode={pickerMode ?? "container"}
        hostData={hostData}
      />
    </>
  );
}
