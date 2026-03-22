"use client";

import { useMemo, useState } from "react";
import { SubTabBar } from "../../../../components/ui/SubTabBar";
import { useTrueNASCapabilities } from "./useDeviceCapabilities";
import { TrueNASDatasetsTab } from "./truenas/TrueNASDatasetsTab";
import { TrueNASDisksTab } from "./truenas/TrueNASDisksTab";
import { TrueNASEventsTab } from "./truenas/TrueNASEventsTab";
import { TrueNASOverviewTab } from "./truenas/TrueNASOverviewTab";
import { TrueNASPoolsTab } from "./truenas/TrueNASPoolsTab";
import { TrueNASReplicationTab } from "./truenas/TrueNASReplicationTab";
import { TrueNASServicesTab } from "./truenas/TrueNASServicesTab";
import { TrueNASSharesTab } from "./truenas/TrueNASSharesTab";
import { TrueNASSnapshotsTab } from "./truenas/TrueNASSnapshotsTab";
import { TrueNASVMsTab } from "./truenas/TrueNASVMsTab";

type Props = {
  assetId: string;
};

const ALL_TRUENAS_TABS = [
  { id: "overview", label: "Overview" },
  { id: "pools", label: "Pools" },
  { id: "datasets", label: "Datasets" },
  { id: "shares", label: "Shares" },
  { id: "disks", label: "Disks" },
  { id: "services", label: "Services" },
  { id: "snapshots", label: "Snapshots" },
  { id: "replication", label: "Replication" },
  { id: "vms", label: "VMs" },
  { id: "events", label: "Events" },
] as const;

type TrueNASTabId = (typeof ALL_TRUENAS_TABS)[number]["id"];

export function TrueNASTab({ assetId }: Props) {
  const [activeTab, setActiveTab] = useState<TrueNASTabId>("overview");
  const { data: caps } = useTrueNASCapabilities(assetId);

  // When capability detection is unavailable, fall back to the historical full
  // tab set instead of hiding SCALE-only tabs on transient failures.
  const visibleTabs = useMemo(() => {
    if (!caps) {
      return ALL_TRUENAS_TABS;
    }
    const allowed = new Set(caps.tabs);
    return ALL_TRUENAS_TABS.filter((t) => allowed.has(t.id));
  }, [caps]);

  const effectiveTab: TrueNASTabId =
    visibleTabs.some((t) => t.id === activeTab) ? activeTab : "overview";

  return (
    <div className="space-y-0">
      <SubTabBar
        tabs={visibleTabs as unknown as Array<{ id: string; label: string }>}
        activeTab={effectiveTab}
        onTabChange={(id) => setActiveTab(id as TrueNASTabId)}
      />
      {effectiveTab === "overview" && <TrueNASOverviewTab assetId={assetId} />}
      {effectiveTab === "pools" && <TrueNASPoolsTab assetId={assetId} />}
      {effectiveTab === "datasets" && <TrueNASDatasetsTab assetId={assetId} />}
      {effectiveTab === "shares" && <TrueNASSharesTab assetId={assetId} />}
      {effectiveTab === "disks" && <TrueNASDisksTab assetId={assetId} />}
      {effectiveTab === "services" && <TrueNASServicesTab assetId={assetId} />}
      {effectiveTab === "snapshots" && <TrueNASSnapshotsTab assetId={assetId} />}
      {effectiveTab === "replication" && <TrueNASReplicationTab assetId={assetId} />}
      {effectiveTab === "vms" && <TrueNASVMsTab assetId={assetId} />}
      {effectiveTab === "events" && <TrueNASEventsTab assetId={assetId} />}
    </div>
  );
}
