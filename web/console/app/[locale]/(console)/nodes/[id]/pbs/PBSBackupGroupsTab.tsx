"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Button } from "../../../../../components/ui/Button";
import { PBSBackupCoverageCard } from "../PBSBackupCoverageCard";
import { normalizePBSGroupsResponse } from "../pbsTabModel";
import { pbsAction } from "./usePBSData";

type Props = {
  assetId: string;
};

export function PBSBackupGroupsTab({ assetId }: Props) {
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [groupsRefreshKey, setGroupsRefreshKey] = useState(0);
  const actionSeq = useRef(0);

  const doForgetGroup = useCallback(
    async (store: string, backupType: string, backupId: string): Promise<boolean> => {
      const key = `${store}/${backupType}/${backupId}`;
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionSuccess(null);
      setActionInFlight(`forget-${key}`);
      try {
        const params = new URLSearchParams({
          store,
          "backup-type": backupType,
          "backup-id": backupId,
        });
        await pbsAction(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/groups/forget?${params.toString()}`,
          "DELETE",
        );
        if (actionSeq.current === seq) {
          setActionSuccess(`Forgot group ${backupType}/${backupId} in ${store}`);
          setGroupsRefreshKey((current) => current + 1);
          return true;
        }
        return false;
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "forget failed");
        }
        return false;
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId],
  );

  return (
    <div className="space-y-4">
      {actionError ? <p role="alert" className="text-xs text-[var(--bad)]">{actionError}</p> : null}
      {actionSuccess ? <p role="status" className="text-xs text-[var(--ok)]">{actionSuccess}</p> : null}

      <PBSBackupCoverageCard assetId={assetId} refreshKey={groupsRefreshKey} />

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
  onForget: (store: string, backupType: string, backupId: string) => Promise<boolean>;
};

function ForgetGroupForm({ assetId, actionInFlight, onForget }: ForgetFormProps) {
  const [showForm, setShowForm] = useState(false);
  const [groupOptions, setGroupOptions] = useState<
    Array<{ key: string; store: string; backupType: string; backupId: string }>
  >([]);
  const [selectedGroupKey, setSelectedGroupKey] = useState("");
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  useEffect(() => {
    if (!showForm) return;
    let cancelled = false;
    setGroupsLoading(true);
    setGroupsError(null);
    fetch(`/api/pbs/assets/${encodeURIComponent(assetId)}/groups`, { cache: "no-store" })
      .then(async (response) => {
        const payload = normalizePBSGroupsResponse(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(payload.error || `failed to load backup groups (${response.status})`);
        }
        if (cancelled) return;
        const options = payload.datastores.flatMap((datastore) =>
          datastore.groups.map((group) => ({
            key: `${datastore.store}/${group.backup_type}/${group.backup_id}`,
            store: datastore.store,
            backupType: group.backup_type,
            backupId: group.backup_id,
          })),
        );
        setGroupOptions(options);
        setSelectedGroupKey((current) => current || options[0]?.key || "");
      })
      .catch((err) => {
        if (!cancelled) {
          setGroupsError(err instanceof Error ? err.message : "failed to load backup groups");
        }
      })
      .finally(() => {
        if (!cancelled) setGroupsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [assetId, showForm]);

  const selectedGroup = useMemo(
    () => groupOptions.find((group) => group.key === selectedGroupKey),
    [groupOptions, selectedGroupKey],
  );

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
          <span className="text-xs text-[var(--muted)]">Backup group</span>
          <select
            aria-label="Backup group"
            className="rounded border border-[var(--line)] bg-[var(--panel-glass)] text-xs text-[var(--text)] px-2 py-1"
            value={selectedGroupKey}
            onChange={(e) => {
              setSelectedGroupKey(e.target.value);
              setConfirming(false);
            }}
            disabled={groupsLoading || groupOptions.length === 0}
          >
            {groupOptions.length === 0 ? (
              <option value="">{groupsLoading ? "Loading groups..." : "No groups available"}</option>
            ) : (
              groupOptions.map((group) => (
                <option key={group.key} value={group.key}>
                  {group.store} — {group.backupType}/{group.backupId}
                </option>
              ))
            )}
          </select>
        </label>
        {confirming && selectedGroup ? (
          <>
            <span role="alert" className="text-xs text-[var(--warn)]">
              Confirm permanent removal of {selectedGroup.backupType}/{selectedGroup.backupId} from {selectedGroup.store}.
            </span>
            <Button
              size="sm"
              variant="danger"
              disabled={!!actionInFlight}
              loading={actionInFlight?.startsWith("forget-") ?? false}
              onClick={async () => {
                const forgotten = await onForget(
                  selectedGroup.store,
                  selectedGroup.backupType,
                  selectedGroup.backupId,
                );
                if (!forgotten) return;
                setConfirming(false);
                setShowForm(false);
                setSelectedGroupKey("");
                setGroupOptions([]);
              }}
            >
              Confirm Forget
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setConfirming(false)}>
              Back
            </Button>
          </>
        ) : (
          <Button
            size="sm"
            variant="danger"
            disabled={!selectedGroup || !!actionInFlight}
            onClick={() => setConfirming(true)}
          >
            Review Forget
          </Button>
        )}
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            setConfirming(false);
            setShowForm(false);
          }}
        >
          Cancel
        </Button>
      </div>
      {groupsError ? <p role="alert" className="text-xs text-[var(--bad)]">{groupsError}</p> : null}
    </div>
  );
}
