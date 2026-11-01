"use client";

// The TrueNAS tab of a device: its pools one line each, then a switch between datasets, snapshots, shares,
// replication and disks, with services, VMs, events and the system details under More. The views TrueNAS offers
// come from its capabilities; when those are not known yet, every view shows.

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { RefreshCw } from "lucide-react";
import { Button } from "../../../../components/ui/Button";
import { DiPage } from "./proxmox/DiList";
import { DiSeg } from "./proxmox/DiControls";
import { useTrueNASCapabilities } from "./useDeviceCapabilities";
import { TrueNASDatasetsTab } from "./truenas/TrueNASDatasetsTab";
import { TrueNASDisksTab } from "./truenas/TrueNASDisksTab";
import { TrueNASEventsTab } from "./truenas/TrueNASEventsTab";
import { TrueNASOverviewTab } from "./truenas/TrueNASOverviewTab";
import { TrueNASPoolLines } from "./truenas/TrueNASPoolsTab";
import { TrueNASReplicationTab } from "./truenas/TrueNASReplicationTab";
import { TrueNASServicesTab } from "./truenas/TrueNASServicesTab";
import { TrueNASSharesTab } from "./truenas/TrueNASSharesTab";
import { TrueNASSnapshotsTab } from "./truenas/TrueNASSnapshotsTab";
import { TrueNASVMsTab } from "./truenas/TrueNASVMsTab";
import type { TrueNASPool } from "./truenas/truenasTypes";
import { useTrueNASList } from "./truenas/useTrueNASData";

type Props = {
  assetId: string;
};

const VIEWS = ["datasets", "snapshots", "shares", "replication", "disks", "services", "vms", "events", "overview"] as const;
type View = (typeof VIEWS)[number];

export function TrueNASTab({ assetId }: Props) {
  const t = useTranslations("devices-truenas.views");
  const [view, setView] = useState<View>("datasets");
  const [round, setRound] = useState(0);
  const { data: caps } = useTrueNASCapabilities(assetId);
  const pools = useTrueNASList<TrueNASPool>(assetId, "pools");

  const allowed = useMemo(() => (caps ? new Set(caps.tabs) : null), [caps]);
  const views = VIEWS.filter((id) => !allowed || allowed.has(id));
  const current: View | undefined = views.includes(view) ? view : views[0];
  const showPools = !allowed || allowed.has("pools");
  const poolSizes = useMemo(
    () => new Map(pools.data.filter((pool) => typeof pool.size === "number").map((pool) => [pool.name, pool.size as number])),
    [pools.data],
  );

  const refresh = () => {
    pools.refresh();
    setRound((value) => value + 1);
  };

  const body = (() => {
    switch (current) {
      case "datasets": return <TrueNASDatasetsTab assetId={assetId} poolSizes={poolSizes} />;
      case "snapshots": return <TrueNASSnapshotsTab assetId={assetId} />;
      case "shares": return <TrueNASSharesTab assetId={assetId} />;
      case "replication": return <TrueNASReplicationTab assetId={assetId} />;
      case "disks": return <TrueNASDisksTab assetId={assetId} />;
      case "services": return <TrueNASServicesTab assetId={assetId} />;
      case "vms": return <TrueNASVMsTab assetId={assetId} />;
      case "events": return <TrueNASEventsTab assetId={assetId} />;
      case "overview": return <TrueNASOverviewTab assetId={assetId} />;
      default: return null;
    }
  })();

  return (
    <DiPage className="di-tn">
      {showPools ? (
        <TrueNASPoolLines assetId={assetId} pools={pools.data} loading={pools.loading} error={pools.error} onRefresh={pools.refresh} />
      ) : null}
      {current ? (
        <div className="di-seg-row">
          <DiSeg
            label={t("label")}
            inline={5}
            value={current}
            onChange={setView}
            options={views.map((id) => ({ id, label: t(id) }))}
          />
          <Button size="sm" variant="ghost" className="di-tn-refresh" onClick={refresh} aria-label={t("refresh")}>
            <RefreshCw size={14} aria-hidden="true" />
          </Button>
        </div>
      ) : null}
      <div className="di-tn-view" key={`${current}-${round}`}>{body}</div>
    </DiPage>
  );
}
