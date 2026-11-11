"use client";

// The snapshots of one backup group, opened under its Coverage row: when, size, verified, protected.

import { useMemo } from "react";

import { DiDots, DiNote } from "../proxmox/DiList";
import { sizeText } from "../proxmox/diFormat";
import { normalizePBSSnapshotsResponse, type PBSSnapshotsResponse } from "../pbsTabModel";
import { pbsErrorDetail, usePBSResource } from "./usePBSData";
import { usePBSWords } from "./usePBSWords";

const EMPTY: PBSSnapshotsResponse = { store: "", snapshots: [] };

type Props = {
  assetId: string;
  store: string;
  /** The group's namespace; empty for the root. */
  ns?: string;
  type: string;
  id: string;
};

/** "Verified", "Verify failed" or "Not verified", from a snapshot's verification state. */
export function verificationWord(state: string | undefined, t: (key: string) => string): { text: string; bad: boolean } {
  if (state === "ok") return { text: t("snap.verified"), bad: false };
  if (state === "failed") return { text: t("snap.verifyFailed"), bad: true };
  if (state && state !== "none") return { text: state, bad: false };
  return { text: t("snap.notVerified"), bad: false };
}

export function PBSGroupSnapshots({ assetId, store, ns = "", type, id }: Props) {
  const { t, tk, when } = usePBSWords();
  const params = new URLSearchParams({ store, type, id });
  const snaps = usePBSResource(
    `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots?${params.toString()}`,
    normalizePBSSnapshotsResponse,
    EMPTY,
  );
  // The list holds this type/id from every namespace; keep the group's own.
  const sorted = useMemo(
    () => snaps.data.snapshots.filter((snap) => (snap.ns ?? "") === ns).sort((a, b) => b.backup_time - a.backup_time),
    [snaps.data, ns],
  );

  return (
    <div className="di-pbs-panel" role="region" aria-label={t("snap.of", { name: `${type}/${id}` })}>
      {snaps.error ? (
        <DiNote tone="bad">{[t("snap.loadFailed"), pbsErrorDetail(snaps.error)].filter(Boolean).join(" ")}</DiNote>
      ) : null}
      {!snaps.loaded ? <p className="di-pbs-line quiet">{tk("loading")}</p> : null}
      {snaps.loaded && !snaps.error && sorted.length === 0 ? <p className="di-pbs-line quiet">{t("snap.none")}</p> : null}
      {sorted.map((snap) => {
        const verify = verificationWord(snap.verification?.state, (key) => t(key));
        return (
          <p key={snap.backup_time} className="di-pbs-line">
            <span className="di-pbs-when">{when(snap.backup_time)}</span>
            <span className="di-grey">
              <DiDots parts={[
                sizeText(snap.size),
                <span key="v" className={verify.bad ? "di-bad" : undefined}>{verify.text}</span>,
                snap.protected ? t("snap.protected") : null,
                snap.comment,
              ]} />
            </span>
          </p>
        );
      })}
    </div>
  );
}
