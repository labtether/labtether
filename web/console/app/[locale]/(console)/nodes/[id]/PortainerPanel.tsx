"use client";

import { useMemo, useState } from "react";
import { Link } from "../../../../../i18n/navigation";
import { Badge } from "../../../../components/ui/Badge";
import { Card } from "../../../../components/ui/Card";
import { SubTabBar } from "../../../../components/ui/SubTabBar";
import { useFastStatus } from "../../../../contexts/StatusContext";
import { formatAge } from "../../../../console/formatters";
import type { Asset } from "../../../../console/models";
import { childParentKey, hostParentKey } from "../../../../console/taxonomy";
import { usePortainerCapabilities } from "./useDeviceCapabilities";
import { PortainerOverviewTab } from "./portainer/PortainerOverviewTab";
import { PortainerContainersTab } from "./portainer/PortainerContainersTab";
import { PortainerStacksTab } from "./portainer/PortainerStacksTab";
import { PortainerImagesTab } from "./portainer/PortainerImagesTab";
import { PortainerVolumesTab } from "./portainer/PortainerVolumesTab";
import { PortainerNetworksTab } from "./portainer/PortainerNetworksTab";

type PortainerPanelProps = {
  asset: Asset;
};

type LabelEntry = {
  key: string;
  value: string;
};

function panelBadgeStatus(value: string): "ok" | "pending" | "bad" {
  const normalized = value.trim().toLowerCase();
  if (normalized.includes("running") || normalized.includes("up") || normalized.includes("active")) {
    return "ok";
  }
  if (normalized.includes("paused") || normalized.includes("restarting")) {
    return "pending";
  }
  return "bad";
}

function formatTimestamp(value?: string): string {
  const raw = value?.trim() ?? "";
  if (!raw) {
    return "--";
  }
  const parsed = Date.parse(raw);
  if (!Number.isFinite(parsed)) {
    return raw;
  }
  return new Date(parsed).toLocaleString();
}

function parseLabels(raw?: string): LabelEntry[] {
  const value = raw?.trim() ?? "";
  if (!value) {
    return [];
  }
  try {
    const parsed = JSON.parse(value) as Record<string, unknown>;
    return Object.entries(parsed)
      .filter(([, entry]) => typeof entry === "string")
      .map(([key, entry]) => ({ key, value: entry as string }))
      .sort((left, right) => left.key.localeCompare(right.key));
  } catch {
    return [];
  }
}

function SummaryCard({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--panel-glass)] px-4 py-3">
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">{label}</p>
      <p className="mt-1 text-sm font-medium text-[var(--text)] break-all">{value || "--"}</p>
      {hint ? <p className="mt-1 text-xs text-[var(--muted)] break-all">{hint}</p> : null}
    </div>
  );
}

const ALL_HOST_TABS = [
  { id: "overview", label: "Overview" },
  { id: "containers", label: "Containers" },
  { id: "stacks", label: "Stacks" },
  { id: "images", label: "Images" },
  { id: "volumes", label: "Volumes" },
  { id: "networks", label: "Networks" },
] as const;

type HostTabId = (typeof ALL_HOST_TABS)[number]["id"];

function PortainerHostPanel({ asset }: { asset: Asset }) {
  const [activeTab, setActiveTab] = useState<HostTabId>("overview");
  const { data: caps } = usePortainerCapabilities(asset.id);
  const canExec = caps?.can_exec === true;

  // Filter tabs by capabilities response. Fall back to all host tabs while
  // the capabilities endpoint is loading so there is no layout shift.
  const visibleTabs = useMemo(() => {
    if (!caps) {
      return ALL_HOST_TABS;
    }
    const allowed = new Set(caps.tabs);
    return ALL_HOST_TABS.filter((t) => allowed.has(t.id));
  }, [caps]);

  const effectiveTab: HostTabId =
    visibleTabs.some((t) => t.id === activeTab) ? activeTab : "overview";

  return (
    <div className="space-y-0">
      <SubTabBar
        tabs={visibleTabs as unknown as Array<{ id: string; label: string }>}
        activeTab={effectiveTab}
        onTabChange={(id) => setActiveTab(id as HostTabId)}
      />
      {effectiveTab === "overview" && <PortainerOverviewTab assetId={asset.id} />}
      {effectiveTab === "containers" && <PortainerContainersTab assetId={asset.id} canExec={canExec} />}
      {effectiveTab === "stacks" && <PortainerStacksTab assetId={asset.id} />}
      {effectiveTab === "images" && <PortainerImagesTab assetId={asset.id} />}
      {effectiveTab === "volumes" && <PortainerVolumesTab assetId={asset.id} />}
      {effectiveTab === "networks" && <PortainerNetworksTab assetId={asset.id} />}
    </div>
  );
}

function PortainerContainerPanel({ asset }: { asset: Asset }) {
  const status = useFastStatus();
  const assets = status?.assets;
  const endpointID = asset.metadata?.endpoint_id ?? "";
  const parentHost = useMemo(
    () => (assets ?? []).find((candidate) =>
      candidate.source === "portainer"
      && candidate.type === "container-host"
      && candidate.metadata?.endpoint_id === endpointID
    ) ?? null,
    [assets, endpointID],
  );
  const stackAsset = useMemo(
    () => (assets ?? []).find((candidate) =>
      candidate.source === "portainer"
      && (candidate.type === "stack" || candidate.type === "compose-stack")
      && candidate.metadata?.endpoint_id === endpointID
      && candidate.name.toLowerCase() === (asset.metadata?.stack ?? "").trim().toLowerCase()
    ) ?? null,
    [assets, asset.metadata?.stack, endpointID],
  );
  const labels = useMemo(() => parseLabels(asset.metadata?.labels_json), [asset.metadata?.labels_json]);
  const state = asset.metadata?.state || asset.metadata?.status || asset.status;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="Image" value={asset.metadata?.image || "--"} hint={asset.metadata?.container_id} />
        <SummaryCard label="State" value={state || "--"} />
        <SummaryCard label="Ports" value={asset.metadata?.ports || "--"} />
        <SummaryCard
          label="Endpoint"
          value={parentHost?.name || asset.metadata?.endpoint_id || "--"}
          hint={parentHost?.metadata?.url || parentHost?.metadata?.collector_base_url}
        />
        <SummaryCard
          label="Stack"
          value={asset.metadata?.stack || "--"}
          hint={asset.metadata?.created_at ? `Created ${formatTimestamp(asset.metadata.created_at)}` : undefined}
        />
      </div>

      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Links</h2>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div className="rounded-lg border border-[var(--line)] p-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Endpoint Host</p>
            {parentHost ? (
              <Link href={`/nodes/${encodeURIComponent(parentHost.id)}`} className="mt-1 inline-block text-sm text-[var(--accent)] hover:underline">
                {parentHost.name}
              </Link>
            ) : (
              <p className="mt-1 text-sm text-[var(--muted)]">Not available</p>
            )}
          </div>
          <div className="rounded-lg border border-[var(--line)] p-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Compose Stack</p>
            {stackAsset ? (
              <Link href={`/nodes/${encodeURIComponent(stackAsset.id)}`} className="mt-1 inline-block text-sm text-[var(--accent)] hover:underline">
                {stackAsset.name}
              </Link>
            ) : (
              <p className="mt-1 text-sm text-[var(--muted)]">{asset.metadata?.stack || "Standalone container"}</p>
            )}
          </div>
        </div>
      </Card>

      {labels.length > 0 ? (
        <Card>
          <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Labels</h2>
          <div className="grid grid-cols-1 gap-x-6 gap-y-1.5 sm:grid-cols-2">
            {labels.map((label) => (
              <div key={label.key} className="flex min-w-0 items-baseline gap-2">
                <span className="w-[180px] shrink-0 text-xs text-[var(--muted)]">{label.key}</span>
                <span className="truncate text-xs text-[var(--text)]">{label.value}</span>
              </div>
            ))}
          </div>
        </Card>
      ) : null}
    </div>
  );
}

function PortainerStackPanel({ asset }: { asset: Asset }) {
  const status = useFastStatus();
  const assets = status?.assets;
  const endpointID = asset.metadata?.endpoint_id ?? "";
  const parentHost = useMemo(
    () => (assets ?? []).find((candidate) =>
      candidate.source === "portainer"
      && candidate.type === "container-host"
      && candidate.metadata?.endpoint_id === endpointID
    ) ?? null,
    [assets, endpointID],
  );
  const memberContainers = useMemo(
    () => (assets ?? [])
      .filter((candidate) =>
        candidate.source === "portainer"
        && candidate.type === "container"
        && candidate.metadata?.endpoint_id === endpointID
        && (candidate.metadata?.stack ?? "").trim().toLowerCase() === asset.name.trim().toLowerCase()
      )
      .sort((left, right) => left.name.localeCompare(right.name)),
    [assets, asset.name, endpointID],
  );
  const statusLabel = asset.metadata?.status || asset.status;

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="Status" value={statusLabel || "--"} hint={asset.metadata?.type ? `Type: ${asset.metadata.type}` : undefined} />
        <SummaryCard label="Containers" value={asset.metadata?.portainer_stack_container_count || String(memberContainers.length)} hint="Matched to this stack" />
        <SummaryCard label="Endpoint" value={parentHost?.name || endpointID || "--"} hint={parentHost?.metadata?.url || parentHost?.metadata?.collector_base_url} />
        <SummaryCard label="Git / Entry" value={asset.metadata?.git_url || asset.metadata?.entry_point || "--"} hint={asset.metadata?.created_by ? `Created by ${asset.metadata.created_by}` : undefined} />
      </div>

      <Card>
        <h2 className="mb-3 text-sm font-medium text-[var(--text)]">Member Containers</h2>
        {memberContainers.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No containers are currently mapped to this stack.</p>
        ) : (
          <div className="space-y-3">
            {memberContainers.map((container) => (
              <div key={container.id} className="rounded-lg border border-[var(--line)] p-3">
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <Link href={`/nodes/${encodeURIComponent(container.id)}`} className="text-sm font-medium text-[var(--accent)] hover:underline">
                      {container.name}
                    </Link>
                    <p className="mt-1 truncate text-xs text-[var(--muted)]">{container.metadata?.image || "--"}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge status={panelBadgeStatus(container.metadata?.state || container.status)} size="sm" />
                    <span className="text-xs text-[var(--muted)]">{container.metadata?.state || container.metadata?.status || container.status || "--"}</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}

export function PortainerPanel({ asset }: PortainerPanelProps) {
  if (asset.type === "container-host") {
    return <PortainerHostPanel asset={asset} />;
  }
  if (asset.type === "container") {
    return <PortainerContainerPanel asset={asset} />;
  }
  if (asset.type === "stack" || asset.type === "compose-stack") {
    return <PortainerStackPanel asset={asset} />;
  }
  return (
    <Card>
      <p className="text-sm text-[var(--muted)]">No Portainer details are available for this asset type yet.</p>
    </Card>
  );
}
