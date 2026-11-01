"use client";

// The Proxmox Backup Server tab of a device page: Backups (datastores, coverage, server details), Snapshots, Jobs
// (verification, prune, garbage collection, sync) and Tasks, with traffic control and certificates under More.

import { useState } from "react";
import { useTranslations } from "next-intl";

import { Button } from "../../../../components/ui/Button";
import { useStatusControls } from "../../../../contexts/StatusContext";
import { DiSeg, type DiOption } from "./proxmox/DiControls";
import { DiEmpty, DiNote, DiPage, DiSheet } from "./proxmox/DiList";
import { PBSBackupsView } from "./pbs/PBSBackupsView";
import { PBSCertificatesView } from "./pbs/PBSCertificatesView";
import { PBSJobsView } from "./pbs/PBSJobsView";
import { PBSSnapshotsView } from "./pbs/PBSSnapshotsView";
import { PBSTasksView } from "./pbs/PBSTasksView";
import { PBSTrafficView } from "./pbs/PBSTrafficView";
import { usePBSDetails } from "./pbs/usePBSData";

type Props = {
  assetId: string;
};

type PBSView = "backups" | "snapshots" | "jobs" | "tasks" | "traffic" | "certificates";
const VIEWS: PBSView[] = ["backups", "snapshots", "jobs", "tasks", "traffic", "certificates"];

export function PBSTab({ assetId }: Props) {
  const t = useTranslations("devices-proxmox.pbs");
  const tk = useTranslations("devices-proxmox.kit");
  const { fetchStatus } = useStatusControls();
  const { details, loading, error, refresh } = usePBSDetails(assetId, fetchStatus);
  const [view, setView] = useState<PBSView>("backups");
  const options: DiOption<PBSView>[] = VIEWS.map((id) => ({ id, label: t(`view.${id}`) }));
  const needsDetails = view === "backups" || view === "snapshots" || view === "tasks";

  return (
    <DiPage>
      <div className="di-seg-row">
        <DiSeg options={options} value={view} onChange={setView} label={tk("views")} inline={4} />
      </div>

      {error ? (
        <DiNote tone="bad" action={<Button size="sm" variant="ghost" onClick={refresh} disabled={loading}>{tk("retry")}</Button>}>
          {error}
        </DiNote>
      ) : null}

      {needsDetails && !details ? (
        <DiSheet>
          <DiEmpty title={loading || !error ? t("loading") : t("unreachable")} />
        </DiSheet>
      ) : null}

      {view === "backups" && details ? (
        <PBSBackupsView assetId={assetId} details={details} loading={loading} onRefresh={refresh} />
      ) : null}
      {view === "snapshots" && details ? <PBSSnapshotsView assetId={assetId} details={details} /> : null}
      {view === "jobs" ? <PBSJobsView assetId={assetId} details={details} onRefreshDetails={refresh} /> : null}
      {view === "tasks" && details ? <PBSTasksView details={details} onRefreshDetails={refresh} /> : null}
      {view === "traffic" ? <PBSTrafficView assetId={assetId} /> : null}
      {view === "certificates" ? <PBSCertificatesView assetId={assetId} /> : null}
    </DiPage>
  );
}
