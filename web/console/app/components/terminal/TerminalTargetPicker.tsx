"use client";

import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box, Check, ChevronDown, History, Search, Server } from "lucide-react";
import type { Asset } from "../../console/models";
import { assetFreshnessLabel } from "../../console/formatters";
import { assetTypeIcon, childParentKey, friendlyTypeLabel, hostParentKey, isDeviceTier, isInfraHost } from "../../console/taxonomy";
import { SegmentedTabs } from "../ui/SegmentedTabs";
import { loadRecentTargets, saveRecentTarget, type RecentTarget } from "./terminalRecentTargets";

type TargetKind = "devices" | "containers";

const containerLikeTypes = new Set([
  "container",
  "docker-container",
  "pod",
  "deployment",
  "app",
  "stack",
  "compose-stack",
]);


function isContainerTarget(asset: Asset): boolean {
  return containerLikeTypes.has(asset.type);
}

function freshnessBadgeClass(lastSeenAt: string): string {
  const freshness = assetFreshnessLabel(lastSeenAt);
  if (freshness === "online") return "border-[var(--ok)]/35 bg-[var(--ok-glow)] text-[var(--ok)]";
  if (freshness === "unresponsive") return "border-[var(--warn)]/35 bg-[var(--warn-glow)] text-[var(--warn)]";
  if (freshness === "offline") return "border-[var(--bad)]/35 bg-[var(--bad-glow)] text-[var(--bad)]";
  return "border-[var(--line)] bg-[var(--surface)] text-[var(--muted)]";
}

interface TargetPickerProps {
  assets: Asset[];
  selectedTargetID: string;
  connectedAgentIDs: Set<string>;
  onSelect: (id: string) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface TargetRowProps {
  asset: Asset;
  showHost?: boolean;
  selectedTargetID: string;
  connectedAgentIDs: Set<string>;
  kind: TargetKind;
  containerHostByID: Map<string, string>;
  onSelect: (id: string) => void;
}

const TargetRow = memo(function TargetRow({
  asset,
  showHost = false,
  selectedTargetID,
  connectedAgentIDs,
  kind,
  containerHostByID,
  onSelect,
}: TargetRowProps) {
  const Icon = assetTypeIcon(asset.type);
  const isSelected = asset.id === selectedTargetID;
  const freshness = assetFreshnessLabel(asset.last_seen_at);
  const hasAgent = connectedAgentIDs.has(asset.id);
  const hostLabel = showHost ? containerHostByID.get(asset.id) ?? "Unassigned host" : "";
  const meta =
    kind === "containers"
      ? `${friendlyTypeLabel(asset.type)} on ${hostLabel} • ${asset.source}`
      : [friendlyTypeLabel(asset.type), asset.platform, asset.source]
          .filter(Boolean)
          .join(" • ");

  return (
    <button
      type="button"
      onClick={() => onSelect(asset.id)}
      className={`group flex w-full items-start gap-3 rounded-lg border px-3 py-2 text-left transition-colors ${
        isSelected
          ? "border-[var(--accent)]/40 bg-[var(--accent-subtle)]"
          : "border-[var(--line)] bg-[var(--surface)] hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
      }`}
    >
      <span className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-[var(--line)] bg-[var(--panel)] text-[var(--muted)]">
        <Icon size={15} />
      </span>
      <span className="min-w-0 flex-1 pt-0.5">
        <span className="block truncate text-sm text-[var(--text)]">{asset.name || asset.id}</span>
        <span className="block truncate text-xs text-[var(--muted)]">{meta}</span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1">
        <span
          className={`rounded-full border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] ${freshnessBadgeClass(asset.last_seen_at)}`}
        >
          {freshness}
        </span>
        {hasAgent ? (
          <span className="rounded-full border border-[var(--accent)]/35 bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--accent-text)]">
            agent
          </span>
        ) : null}
        {isSelected ? (
          <span className="inline-flex items-center gap-1 rounded-full border border-[var(--accent)]/35 bg-[var(--accent-subtle)] px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--accent-text)]">
            <Check size={9} />
            selected
          </span>
        ) : null}
      </span>
    </button>
  );
});

export function TerminalTargetPicker({
  assets,
  selectedTargetID,
  connectedAgentIDs,
  onSelect,
  open,
  onOpenChange,
}: TargetPickerProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [query, setQuery] = useState("");
  const [kind, setKind] = useState<TargetKind>("devices");
  const [recentTargetData, setRecentTargetData] = useState<RecentTarget[]>([]);

  const byID = useMemo(() => {
    const map = new Map<string, Asset>();
    for (const asset of assets) {
      map.set(asset.id, asset);
    }
    return map;
  }, [assets]);

  const selectedTarget = selectedTargetID ? byID.get(selectedTargetID) ?? null : null;
  const selectedKind: TargetKind =
    selectedTarget && isContainerTarget(selectedTarget) ? "containers" : "devices";

  const devices = useMemo(
    () => assets.filter((asset) => isDeviceTier(asset)).sort((a, b) => a.name.localeCompare(b.name)),
    [assets],
  );
  const containers = useMemo(
    () => assets.filter((asset) => isContainerTarget(asset)).sort((a, b) => a.name.localeCompare(b.name)),
    [assets],
  );

  useEffect(() => {
    setRecentTargetData(loadRecentTargets());
  }, []);

  useEffect(() => {
    if (!open) return undefined;

    const onEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onOpenChange(false);
      }
    };

    window.addEventListener("keydown", onEscape);
    return () => {
      window.removeEventListener("keydown", onEscape);
    };
  }, [open, onOpenChange]);

  useEffect(() => {
    if (open) {
      setKind(selectedKind);
      const focusTimer = window.setTimeout(() => searchInputRef.current?.focus(), 0);
      return () => window.clearTimeout(focusTimer);
    }
    setQuery("");
  }, [open, selectedKind]);

  const rememberRecentTarget = useCallback((asset: Asset) => {
    const target: RecentTarget = {
      id: asset.id,
      name: asset.name || asset.id,
      type: isContainerTarget(asset) ? "container" : "device",
      lastConnected: new Date().toISOString(),
    };
    setRecentTargetData(saveRecentTarget(target));
  }, []);

  const queryValue = query.trim().toLowerCase();
  const matchesQuery = useCallback(
    (asset: Asset) => {
      if (!queryValue) return true;
      const fields = [
        asset.name,
        asset.id,
        friendlyTypeLabel(asset.type),
        asset.type,
        asset.source,
        asset.platform ?? "",
        ...(asset.tags ?? []),
        ...(asset.metadata ? Object.values(asset.metadata) : []),
      ];
      return fields.join(" ").toLowerCase().includes(queryValue);
    },
    [queryValue],
  );

  const visibleTargets = useMemo(() => {
    const pool = kind === "devices" ? devices : containers;
    return pool.filter(matchesQuery);
  }, [containers, devices, kind, matchesQuery]);

  const resolvedRecentTargets = useMemo(
    () =>
      recentTargetData
        .map((rt) => byID.get(rt.id))
        .filter((asset): asset is Asset => Boolean(asset))
        .filter((asset) => {
          const targetKind: TargetKind = isContainerTarget(asset) ? "containers" : "devices";
          return targetKind === kind && matchesQuery(asset);
        }),
    [recentTargetData, byID, kind, matchesQuery],
  );

  const recentTargetSet = useMemo(
    () => new Set(resolvedRecentTargets.map((asset) => asset.id)),
    [resolvedRecentTargets],
  );
  const sectionTargets = useMemo(
    () => visibleTargets.filter((asset) => !recentTargetSet.has(asset.id)),
    [recentTargetSet, visibleTargets],
  );
  const totalVisible = visibleTargets.length;

  const containerHostByID = useMemo(() => {
    const infraHostByParentKey = new Map<string, Asset>();
    for (const asset of devices) {
      if (!isInfraHost(asset)) continue;
      infraHostByParentKey.set(hostParentKey(asset), asset);
    }

    const hostByContainer = new Map<string, string>();
    for (const container of containers) {
      const parentKey = childParentKey(container);
      const host = parentKey ? infraHostByParentKey.get(parentKey) : undefined;
      const fallbackHost =
        container.metadata?.node ||
        container.metadata?.host ||
        container.metadata?.endpoint_name ||
        container.metadata?.endpoint_id ||
        "Unassigned host";
      hostByContainer.set(container.id, host?.name || fallbackHost);
    }
    return hostByContainer;
  }, [containers, devices]);

  const groupedContainers = useMemo(() => {
    if (kind !== "containers") return [];
    const grouped = new Map<string, Asset[]>();
    for (const asset of sectionTargets) {
      const hostLabel = containerHostByID.get(asset.id) ?? "Unassigned host";
      const list = grouped.get(hostLabel) ?? [];
      list.push(asset);
      grouped.set(hostLabel, list);
    }
    return Array.from(grouped.entries())
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([hostLabel, hostTargets]) => ({
        hostLabel,
        targets: hostTargets.sort((a, b) => a.name.localeCompare(b.name)),
      }));
  }, [containerHostByID, kind, sectionTargets]);

  const handleSelectTarget = useCallback(
    (id: string) => {
      if (id === selectedTargetID) {
        onOpenChange(false);
        return;
      }
      onSelect(id);
      onOpenChange(false);
      const asset = byID.get(id);
      if (asset) rememberRecentTarget(asset);
    },
    [byID, onOpenChange, onSelect, rememberRecentTarget, selectedTargetID],
  );

  const kindOptions = useMemo(
    () => [
      {
        id: "devices" as const,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <Server size={12} />
            <span>Devices</span>
            <span className="text-[10px] text-[var(--muted)]">{devices.length}</span>
          </span>
        ),
        ariaLabel: "Show device targets",
      },
      {
        id: "containers" as const,
        label: (
          <span className="inline-flex items-center gap-1.5">
            <Box size={12} />
            <span>Containers</span>
            <span className="text-[10px] text-[var(--muted)]">{containers.length}</span>
          </span>
        ),
        ariaLabel: "Show container targets",
      },
    ],
    [containers.length, devices.length],
  );

  return (
    <div className="min-w-0 flex-1">
      <button
        type="button"
        onClick={() => onOpenChange(true)}
        className="flex h-9 w-full min-w-0 items-center gap-2 rounded-lg border border-[var(--line)] bg-[var(--panel)] px-2 transition-colors hover:border-[var(--accent)]/35 hover:bg-[var(--hover)]"
        title="Choose terminal target"
      >
        <span className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded border border-[var(--line)] bg-[var(--surface)] text-[var(--muted)]">
          {selectedTarget ? (
            (() => {
              const Icon = assetTypeIcon(selectedTarget.type);
              return <Icon size={13} />;
            })()
          ) : (
            <Search size={13} />
          )}
        </span>
        <span className="min-w-0 flex-1 text-left">
          <span className="block truncate text-xs text-[var(--text)]">
            {selectedTarget ? selectedTarget.name || selectedTarget.id : "Select target"}
          </span>
          <span className="block truncate text-[10px] uppercase tracking-[0.06em] text-[var(--muted)]">
            {selectedTarget
              ? `${selectedKind === "devices" ? "Device" : "Container"} target`
              : "Choose Device or Container"}
          </span>
        </span>
        <ChevronDown
          size={14}
          className={`shrink-0 text-[var(--muted)] transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>

      {open ? (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Choose terminal target"
          className="fixed inset-0 z-[70] flex items-start justify-center p-4 pt-[8vh]"
        >
          <button
            type="button"
            aria-label="Close target picker"
            onClick={() => onOpenChange(false)}
            className="absolute inset-0 bg-black/76"
          />

          <div className="relative z-10 w-full max-w-[56rem] overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--panel)] shadow-[var(--shadow-lg)]">
            <div className="border-b border-[var(--line)] p-4">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    Connect Terminal
                  </p>
                  <p className="mt-1 text-sm font-semibold text-[var(--text)]">
                    Pick a target type first, then choose the endpoint
                  </p>
                  <p className="mt-0.5 text-xs text-[var(--muted)]">
                    Devices open host shells. Containers open workload shells.
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => onOpenChange(false)}
                  className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-2 py-1 text-xs text-[var(--muted)] transition-colors hover:bg-[var(--hover)] hover:text-[var(--text)]"
                >
                  Done
                </button>
              </div>

              <div className="mt-3 grid gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)]">
                <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-1">
                  <SegmentedTabs
                    value={kind}
                    options={kindOptions}
                    onChange={setKind}
                    size="sm"
                  />
                </div>
                <div className="relative">
                  <Search
                    size={14}
                    className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--muted)]"
                  />
                  <input
                    ref={searchInputRef}
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={
                      kind === "devices"
                        ? "Search devices by name, type, platform, or tag..."
                        : "Search containers by name, host, source, or tag..."
                    }
                    className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] py-2 pl-8 pr-3 text-sm text-[var(--text)] outline-none transition-colors focus:border-[var(--accent)]"
                  />
                </div>
              </div>

              <div className="mt-2 flex items-center justify-between text-xs text-[var(--muted)]">
                <span>{kind === "devices" ? "Devices" : "Containers"} visible</span>
                <span>{totalVisible} result{totalVisible === 1 ? "" : "s"}</span>
              </div>
            </div>

            <div className="max-h-[min(64vh,38rem)] overflow-y-auto p-4">
              {resolvedRecentTargets.length > 0 ? (
                <div className="mb-4">
                  <div className="mb-2 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    <History size={11} />
                    Recent {kind === "devices" ? "Devices" : "Containers"}
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {resolvedRecentTargets.map((asset) => (
                      <TargetRow
                        key={`recent-${asset.id}`}
                        asset={asset}
                        showHost={kind === "containers"}
                        selectedTargetID={selectedTargetID}
                        connectedAgentIDs={connectedAgentIDs}
                        kind={kind}
                        containerHostByID={containerHostByID}
                        onSelect={handleSelectTarget}
                      />
                    ))}
                  </div>
                </div>
              ) : null}

              {kind === "devices" && sectionTargets.length > 0 ? (
                <div>
                  <div className="mb-2 flex items-center gap-1 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                    <Server size={11} />
                    Devices
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    {sectionTargets.map((asset) => (
                      <TargetRow
                        key={asset.id}
                        asset={asset}
                        selectedTargetID={selectedTargetID}
                        connectedAgentIDs={connectedAgentIDs}
                        kind={kind}
                        containerHostByID={containerHostByID}
                        onSelect={handleSelectTarget}
                      />
                    ))}
                  </div>
                </div>
              ) : null}

              {kind === "containers" && groupedContainers.length > 0 ? (
                <div className="space-y-3">
                  {groupedContainers.map((group) => (
                    <div key={group.hostLabel}>
                      <div className="mb-2 flex items-center justify-between text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--muted)]">
                        <span className="truncate">{group.hostLabel}</span>
                        <span>{group.targets.length}</span>
                      </div>
                      <div className="grid grid-cols-1 gap-2">
                        {group.targets.map((asset) => (
                          <TargetRow
                            key={asset.id}
                            asset={asset}
                            showHost
                            selectedTargetID={selectedTargetID}
                            connectedAgentIDs={connectedAgentIDs}
                            kind={kind}
                            containerHostByID={containerHostByID}
                            onSelect={handleSelectTarget}
                          />
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}

              {totalVisible === 0 ? (
                <div className="rounded-lg border border-dashed border-[var(--line)] bg-[var(--surface)] px-4 py-8 text-center text-sm text-[var(--muted)]">
                  {kind === "devices"
                    ? "No devices match your search."
                    : "No containers match your search."}
                </div>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
