"use client";

import { useState } from "react";

import { SubTabBar } from "../../../../components/ui/SubTabBar";
import { PBSBackupGroupsTab } from "./pbs/PBSBackupGroupsTab";
import { PBSCertificatesTab } from "./pbs/PBSCertificatesTab";
import { PBSDatastoresTab } from "./pbs/PBSDatastoresTab";
import { PBSGCTab } from "./pbs/PBSGCTab";
import { PBSOverviewTab } from "./pbs/PBSOverviewTab";
import { PBSPruneJobsTab } from "./pbs/PBSPruneJobsTab";
import { PBSSnapshotsTab } from "./pbs/PBSSnapshotsTab";
import { PBSSyncJobsTab } from "./pbs/PBSSyncJobsTab";
import { PBSTasksTab } from "./pbs/PBSTasksTab";
import { PBSTrafficControlTab } from "./pbs/PBSTrafficControlTab";
import { PBSVerificationTab } from "./pbs/PBSVerificationTab";

type Props = {
  assetId: string;
};

const ALL_PBS_TABS = [
  { id: "overview", label: "Overview" },
  { id: "datastores", label: "Datastores" },
  { id: "groups", label: "Backup Groups" },
  { id: "snapshots", label: "Snapshots" },
  { id: "verification", label: "Verification" },
  { id: "gc", label: "Garbage Collection" },
  { id: "prune-jobs", label: "Prune Jobs" },
  { id: "sync-jobs", label: "Sync Jobs" },
  { id: "tasks", label: "Tasks" },
  { id: "traffic", label: "Traffic Control" },
  { id: "certificates", label: "Certificates" },
] as const;

type PBSTabId = (typeof ALL_PBS_TABS)[number]["id"];

export function PBSTab({ assetId }: Props) {
  const [activeTab, setActiveTab] = useState<PBSTabId>("overview");

  return (
    <div className="space-y-0">
      <SubTabBar
        tabs={ALL_PBS_TABS as unknown as Array<{ id: string; label: string }>}
        activeTab={activeTab}
        onTabChange={(id) => setActiveTab(id as PBSTabId)}
      />
      {activeTab === "overview" && <PBSOverviewTab assetId={assetId} />}
      {activeTab === "datastores" && <PBSDatastoresTab assetId={assetId} />}
      {activeTab === "groups" && <PBSBackupGroupsTab assetId={assetId} />}
      {activeTab === "snapshots" && <PBSSnapshotsTab assetId={assetId} />}
      {activeTab === "verification" && <PBSVerificationTab assetId={assetId} />}
      {activeTab === "gc" && <PBSGCTab assetId={assetId} />}
      {activeTab === "prune-jobs" && <PBSPruneJobsTab assetId={assetId} />}
      {activeTab === "sync-jobs" && <PBSSyncJobsTab assetId={assetId} />}
      {activeTab === "tasks" && <PBSTasksTab assetId={assetId} />}
      {activeTab === "traffic" && <PBSTrafficControlTab assetId={assetId} />}
      {activeTab === "certificates" && <PBSCertificatesTab assetId={assetId} />}
    </div>
  );
}
