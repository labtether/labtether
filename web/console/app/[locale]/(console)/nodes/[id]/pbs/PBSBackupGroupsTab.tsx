"use client";

import { useCallback, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { PBSBackupCoverageCard } from "../PBSBackupCoverageCard";
import { pbsAction } from "./usePBSData";

type Props = {
  assetId: string;
};

export function PBSBackupGroupsTab({ assetId }: Props) {
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doForgetGroup = useCallback(
    async (store: string, backupType: string, backupId: string) => {
      const key = `${store}/${backupType}/${backupId}`;
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setActionInFlight(`forget-${key}`);
      try {
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/groups/forget`,
          "POST",
          { store, backup_type: backupType, backup_id: backupId },
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Forgot group ${backupType}/${backupId} in ${store}`);
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "forget failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId],
  );

  // Expose forget action via a context or just render the card plus a note.
  // The core group table is rendered by PBSBackupCoverageCard which handles
  // its own data fetching. The forget action is surfaced here as a secondary
  // action panel to keep the existing card working without modification.
  return (
    <div className="space-y-4">
      {actionError ? <p className="text-xs text-[var(--bad)]">{actionError}</p> : null}
      {actionSuccess ? <p className="text-xs text-[var(--ok)]">{actionSuccess}</p> : null}

      <PBSBackupCoverageCard assetId={assetId} />

      <ForgetGroupForm
        assetId={assetId}
        actionInFlight={actionInFlight}
        onForget={doForgetGroup}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Forget group form
// ---------------------------------------------------------------------------

type ForgetFormProps = {
  assetId: string;
  actionInFlight: string | null;
  onForget: (store: string, backupType: string, backupId: string) => void;
};

function ForgetGroupForm({ actionInFlight, onForget }: ForgetFormProps) {
  const [store, setStore] = useState("");
  const [backupType, setBackupType] = useState("vm");
  const [backupId, setBackupId] = useState("");
  const [showForm, setShowForm] = useState(false);

  if (!showForm) {
    return (
      <div className="flex justify-end">
        <Button size="sm" variant="ghost" onClick={() => setShowForm(true)}>
          Forget Group
        </Button>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] p-4 space-y-3">
      <h3 className="text-sm font-medium text-[var(--text)]">Forget Backup Group</h3>
      <p className="text-xs text-[var(--warn)]">
        This permanently removes all snapshots in the group. This cannot be undone.
      </p>
      <div className="flex flex-wrap gap-2 items-end">
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">Datastore</span>
          <input
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={store}
            onChange={(e) => setStore(e.target.value)}
            placeholder="store-name"
          />
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">Type</span>
          <select
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={backupType}
            onChange={(e) => setBackupType(e.target.value)}
          >
            <option value="vm">vm</option>
            <option value="ct">ct</option>
            <option value="host">host</option>
          </select>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs text-[var(--muted)]">ID</span>
          <input
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={backupId}
            onChange={(e) => setBackupId(e.target.value)}
            placeholder="100"
          />
        </label>
        <Button
          size="sm"
          variant="danger"
          disabled={!store || !backupId || !!actionInFlight}
          loading={actionInFlight?.startsWith("forget-") ?? false}
          onClick={() => {
            if (store && backupId) onForget(store, backupType, backupId);
          }}
        >
          Forget
        </Button>
        <Button size="sm" variant="ghost" onClick={() => setShowForm(false)}>
          Cancel
        </Button>
      </div>
    </div>
  );
}
