"use client";

import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from "react";

import { Badge } from "../../../../components/ui/Badge";
import {
  backupStaleness,
  formatBytes,
  formatRelativeEpoch,
  normalizePBSGroupsResponse,
  normalizePBSSnapshotsResponse,
  usageThreshold,
  type PBSBackupGroupEntry,
  type PBSDatastoreGroups,
  type PBSDatastoreSummary,
  type PBSSnapshotEntry,
} from "./pbsTabModel";

type Props = {
  store: PBSDatastoreSummary;
  assetId: string;
};

export function PBSDatastoreDrilldown({ store, assetId }: Props) {
  const [groups, setGroups] = useState<PBSDatastoreGroups | null>(null);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [groupsError, setGroupsError] = useState<string | null>(null);

  const [expandedGroup, setExpandedGroup] = useState<string | null>(null);
  const [snapshots, setSnapshots] = useState<PBSSnapshotEntry[]>([]);
  const [snapshotsLoading, setSnapshotsLoading] = useState(false);
  const [snapshotsError, setSnapshotsError] = useState<string | null>(null);

  const groupsSeqRef = useRef(0);
  const latestGroupsRef = useRef(0);
  const snapshotsSeqRef = useRef(0);
  const latestSnapshotsRef = useRef(0);

  const threshold = usageThreshold(store.usage_percent);
  const usedPct = Math.min(100, store.usage_percent ?? 0);

  // Fetch groups on mount
  useEffect(() => {
    const requestID = ++groupsSeqRef.current;
    latestGroupsRef.current = requestID;
    setGroupsLoading(true);
    setGroupsError(null);

    fetch(`/api/pbs/assets/${encodeURIComponent(assetId)}/groups`, { cache: "no-store" })
      .then(async (response) => {
        const payload = normalizePBSGroupsResponse(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(payload.error || `failed to load groups (${response.status})`);
        }
        if (latestGroupsRef.current !== requestID) return;
        const ds = payload.datastores.find((d) => d.store === store.store) ?? null;
        setGroups(ds);
      })
      .catch((err) => {
        if (latestGroupsRef.current !== requestID) return;
        setGroupsError(err instanceof Error ? err.message : "failed to load groups");
      })
      .finally(() => {
        if (latestGroupsRef.current === requestID) {
          setGroupsLoading(false);
        }
      });
  }, [assetId, store.store]);

  const fetchSnapshots = useCallback(
    async (groupKey: string, backupType: string, backupId: string) => {
      const requestID = ++snapshotsSeqRef.current;
      latestSnapshotsRef.current = requestID;
      setSnapshotsLoading(true);
      setSnapshotsError(null);
      setSnapshots([]);

      const params = new URLSearchParams({
        store: store.store,
        type: backupType,
        id: backupId,
      });

      try {
        const response = await fetch(
          `/api/pbs/assets/${encodeURIComponent(assetId)}/snapshots?${params.toString()}`,
          { cache: "no-store" },
        );
        const payload = normalizePBSSnapshotsResponse(await response.json().catch(() => null));
        if (!response.ok) {
          throw new Error(payload.error || `failed to load snapshots (${response.status})`);
        }
        if (latestSnapshotsRef.current !== requestID) return;
        setSnapshots(payload.snapshots);
      } catch (err) {
        if (latestSnapshotsRef.current !== requestID) return;
        setSnapshotsError(err instanceof Error ? err.message : "failed to load snapshots");
      } finally {
        if (latestSnapshotsRef.current === requestID) {
          setSnapshotsLoading(false);
        }
      }
    },
    [assetId, store.store],
  );

  function handleGroupClick(group: PBSBackupGroupEntry) {
    const key = `${group.backup_type}/${group.backup_id}`;
    if (expandedGroup === key) {
      setExpandedGroup(null);
      setSnapshots([]);
      setSnapshotsError(null);
    } else {
      setExpandedGroup(key);
      void fetchSnapshots(key, group.backup_type, group.backup_id);
    }
  }

  return (
    <div className="bg-[var(--surface)] rounded-lg m-1 p-4 space-y-5">
      {/* Section 1: Storage Detail */}
      <section>
        <h3 className="text-xs font-medium text-[var(--text)] mb-2">Storage</h3>
        <div className="space-y-1.5">
          <div className="h-3 w-full rounded-full bg-[var(--panel-glass)] border border-[var(--panel-border)] overflow-hidden">
            <div
              className="h-full rounded-full transition-[width,background-color] duration-[var(--dur-fast)]"
              style={{
                width: `${usedPct}%`,
                backgroundColor:
                  threshold === "bad"
                    ? "var(--bad)"
                    : threshold === "warn"
                    ? "var(--warn)"
                    : "var(--ok)",
              }}
            />
          </div>
          <div className="flex items-center gap-4 text-xs text-[var(--muted)]">
            <span>
              <span style={{ color: threshold === "bad" ? "var(--bad)" : threshold === "warn" ? "var(--warn)" : "var(--ok)" }}>
                {typeof store.usage_percent === "number"
                  ? `${store.usage_percent.toFixed(1)}%`
                  : "n/a"}
              </span>{" "}
              used
            </span>
            <span>Used: {formatBytes(store.used_bytes)}</span>
            <span>Avail: {formatBytes(store.avail_bytes)}</span>
            <span>Total: {formatBytes(store.total_bytes)}</span>
          </div>
        </div>
      </section>

      {/* Section 2: GC Status — not included in current PBSDatastoreSummary type */}
      {/* The frontend model does not yet expose gc_status; skip rendering until the field is added. */}

      {/* Section 3: Backup Groups */}
      <section>
        <h3 className="text-xs font-medium text-[var(--text)] mb-2">Backup Groups</h3>

        {groupsLoading && !groups ? (
          <p className="text-xs text-[var(--muted)]">Loading groups...</p>
        ) : groupsError ? (
          <p className="text-xs text-[var(--bad)]">{groupsError}</p>
        ) : !groups || groups.groups.length === 0 ? (
          <p className="text-xs text-[var(--muted)]">No groups in this datastore.</p>
        ) : (
          <GroupsTable groups={groups} expandedGroup={expandedGroup} onGroupClick={handleGroupClick} snapshotsLoading={snapshotsLoading} snapshotsError={snapshotsError} snapshots={snapshots} />
        )}
      </section>
    </div>
  );
}

function GroupsTable({ groups, expandedGroup, onGroupClick, snapshotsLoading, snapshotsError, snapshots }: { groups: PBSDatastoreGroups; expandedGroup: string | null; onGroupClick: (group: PBSBackupGroupEntry) => void; snapshotsLoading: boolean; snapshotsError: string | null; snapshots: PBSSnapshotEntry[] }) {
  const sortedGroups = useMemo(
    () => [...groups.groups].sort((a, b) => (b.last_backup ?? 0) - (a.last_backup ?? 0)),
    [groups],
  );

  return (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Type</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">ID</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Owner</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Snapshots</th>
                  <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Last Backup</th>
                </tr>
              </thead>
              <tbody>
                {sortedGroups.map((group) => {
                    const key = `${group.backup_type}/${group.backup_id}`;
                    const isGroupExpanded = expandedGroup === key;
                    const staleness = backupStaleness(group.last_backup);
                    return (
                      <Fragment key={key}>
                        <tr
                          className={`border-b border-[var(--line)] border-opacity-30 cursor-pointer transition-colors duration-[var(--dur-fast)] hover:bg-[var(--hover)] ${
                            isGroupExpanded ? "bg-[var(--hover)]" : ""
                          }`}
                          onClick={() => onGroupClick(group)}
                        >
                          <td className="py-1.5 px-2">
                            <span className="inline-flex items-center gap-1">
                              <span
                                className="text-[var(--muted)] text-[10px] select-none"
                                aria-hidden="true"
                              >
                                {isGroupExpanded ? "\u25BE" : "\u25B8"}
                              </span>
                              <GroupTypeBadge type={group.backup_type} />
                            </span>
                          </td>
                          <td className="py-1.5 px-2 text-[var(--text)] font-medium">
                            {group.backup_id}
                          </td>
                          <td className="py-1.5 px-2 text-[var(--muted)]">
                            {group.owner || "\u2014"}
                          </td>
                          <td className="py-1.5 px-2 text-[var(--muted)]">
                            {group.backup_count}
                          </td>
                          <td
                            className="py-1.5 px-2"
                            style={{ color: `var(--${staleness})` }}
                          >
                            {group.last_backup
                              ? formatRelativeEpoch(group.last_backup)
                              : "never"}
                          </td>
                        </tr>

                        {/* Section 4: Snapshots for expanded group */}
                        {isGroupExpanded && (
                          <tr>
                            <td colSpan={5} className="p-0 border-b border-[var(--line)] border-opacity-30">
                              <SnapshotList
                                loading={snapshotsLoading}
                                error={snapshotsError}
                                snapshots={snapshots}
                              />
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    );
                  })}
              </tbody>
            </table>
          </div>
  );
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function GroupTypeBadge({ type }: { type: string }) {
  const label = type === "vm" ? "VM" : type === "ct" ? "CT" : type === "host" ? "Host" : type;
  const colorClass =
    type === "vm"
      ? "bg-blue-500/10 text-blue-400"
      : type === "ct"
      ? "bg-purple-500/10 text-purple-400"
      : "bg-[var(--surface)] text-[var(--muted)]";
  return (
    <span
      className={`inline-block rounded px-1.5 py-0.5 text-xs font-medium ${colorClass}`}
    >
      {label}
    </span>
  );
}

function VerificationBadge({ state }: { state?: string }) {
  if (!state || state === "none" || state === "") {
    return <span className="text-[var(--muted)]">none</span>;
  }
  const status =
    state === "ok"
      ? "ok"
      : state === "failed"
      ? "bad"
      : "pending";
  return <Badge status={status} size="sm" />;
}

function SnapshotList({
  loading,
  error,
  snapshots,
}: {
  loading: boolean;
  error: string | null;
  snapshots: PBSSnapshotEntry[];
}) {
  // useMemo must be called unconditionally (Rules of Hooks) before any
  // early returns. All hook calls are grouped here at the top.
  const sorted = useMemo(
    () => [...snapshots].sort((a, b) => b.backup_time - a.backup_time),
    [snapshots],
  );

  // Non-data states: render after all hooks have been called.
  if (loading) {
    return (
      <div className="px-3 py-2 text-xs text-[var(--muted)]">Loading snapshots...</div>
    );
  }
  if (error) {
    return <div className="px-3 py-2 text-xs text-[var(--bad)]">{error}</div>;
  }
  if (snapshots.length === 0) {
    return (
      <div className="px-3 py-2 text-xs text-[var(--muted)]">No snapshots found.</div>
    );
  }

  return (
    <div className="bg-[var(--surface)]/60 px-3 py-2">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-[var(--line)] border-opacity-50">
            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Time</th>
            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Size</th>
            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Protected</th>
            <th className="py-1 px-2 text-left text-[var(--muted)] font-medium">Verification</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((snap) => (
              <tr
                key={`${snap.backup_type}-${snap.backup_id}-${snap.backup_time}`}
                className="border-b border-[var(--line)] border-opacity-20"
              >
                <td className="py-1 px-2 text-[var(--text)]">
                  {formatRelativeEpoch(snap.backup_time)}
                </td>
                <td className="py-1 px-2 text-[var(--muted)]">
                  {snap.size !== undefined ? formatBytes(snap.size) : "n/a"}
                </td>
                <td className="py-1 px-2">
                  {snap.protected ? (
                    <span className="text-[var(--ok)] text-xs font-medium">protected</span>
                  ) : (
                    <span className="text-[var(--muted)]">&mdash;</span>
                  )}
                </td>
                <td className="py-1 px-2">
                  <VerificationBadge state={snap.verification?.state} />
                </td>
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  );
}
