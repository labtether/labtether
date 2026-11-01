"use client";

// Backups: the datastores with their use, backup coverage per group, and the server's details.

import { Button } from "../../../../../components/ui/Button";
import { DiKV, DiNote, DiSection, DiSheet } from "../proxmox/DiList";
import type { PBSAssetDetailsResponse } from "../pbsTabModel";
import { PBSCoverageList } from "./PBSCoverageList";
import { PBSDatastoreList } from "./PBSDatastoreList";
import { newestBackup, pbsStores, tasksLastDay } from "./pbsViewModel";
import { usePBSWords } from "./usePBSWords";

type Props = {
  assetId: string;
  details: PBSAssetDetailsResponse;
  loading: boolean;
  onRefresh: () => void;
};

export function PBSBackupsView({ assetId, details, loading, onRefresh }: Props) {
  const { t, tk, when } = usePBSWords();
  const stores = pbsStores(details);
  const day = tasksLastDay(details.tasks ?? []);
  const last = newestBackup(stores);

  return (
    <>
      <DiSection
        id="pbs-datastores"
        title={t("datastores")}
        right={(
          <>
            {details.version ? <span>{t("version", { version: details.version })}</span> : null}
            <Button size="sm" variant="ghost" onClick={onRefresh} disabled={loading}>{tk("refresh")}</Button>
          </>
        )}
      >
        {(details.warnings ?? []).map((warning) => (
          <DiNote key={warning} tone="warn">{warning}</DiNote>
        ))}
        <PBSDatastoreList assetId={assetId} datastores={stores} onChanged={onRefresh} />
      </DiSection>

      <PBSCoverageList assetId={assetId} />

      <DiSection id="pbs-details" title={t("details")}>
        <DiSheet kind="tight">
          <DiKV
            columns={2}
            items={[
              [t("kv.version"), details.version || ""],
              [t("kv.node"), details.node || "localhost"],
              [t("kv.lastBackup"), last ? when(last) : tk("never")],
              [t("kv.tasksDay"), t("tasksDay", { ok: day.ok, failed: day.failed })],
            ]}
          />
        </DiSheet>
      </DiSection>
    </>
  );
}
