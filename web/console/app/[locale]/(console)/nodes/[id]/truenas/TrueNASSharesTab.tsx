"use client";

import { useCallback, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { truenasAction, useTrueNASList } from "./useTrueNASData";

export type TrueNASSMBShare = {
  id: string | number;
  name: string;
  path: string;
  enabled: boolean;
  comment?: string;
};

export type TrueNASNFSShare = {
  id: string | number;
  path: string;
  enabled: boolean;
  networks?: string[];
  hosts?: string[];
};

type Props = {
  assetId: string;
};

function enabledBadge(enabled: boolean): "ok" | "bad" {
  return enabled ? "ok" : "bad";
}

export function TrueNASSharesTab({ assetId }: Props) {
  const {
    data: smbShares,
    loading: smbLoading,
    error: smbError,
    refresh: refreshSmb,
  } = useTrueNASList<TrueNASSMBShare>(assetId, "shares/smb");
  const {
    data: nfsShares,
    loading: nfsLoading,
    error: nfsError,
    refresh: refreshNfs,
  } = useTrueNASList<TrueNASNFSShare>(assetId, "shares/nfs");

  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);
  const actionSeq = useRef(0);

  const doToggle = useCallback(
    async (type: "smb" | "nfs", id: string | number, enabled: boolean) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`toggle-${type}-${id}`);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/shares/${type}/${encodeURIComponent(String(id))}`,
          "PUT",
          { enabled: !enabled },
        );
        if (actionSeq.current === seq) {
          type === "smb" ? refreshSmb() : refreshNfs();
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "toggle failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refreshSmb, refreshNfs],
  );

  const doDelete = useCallback(
    async (type: "smb" | "nfs", id: string | number) => {
      const seq = ++actionSeq.current;
      setActionError(null);
      setActionInFlight(`delete-${type}-${id}`);
      setConfirmDelete(null);
      try {
        await truenasAction(
          `/api/truenas/assets/${encodeURIComponent(assetId)}/shares/${type}/${encodeURIComponent(String(id))}`,
          "DELETE",
        );
        if (actionSeq.current === seq) {
          type === "smb" ? refreshSmb() : refreshNfs();
        }
      } catch (err) {
        if (actionSeq.current === seq) {
          setActionError(err instanceof Error ? err.message : "delete failed");
        }
      } finally {
        if (actionSeq.current === seq) setActionInFlight(null);
      }
    },
    [assetId, refreshSmb, refreshNfs],
  );

  const loading = smbLoading || nfsLoading;

  return (
    <div className="space-y-4">
      {actionError ? (
        <div className="rounded-md border border-[var(--bad)] px-3 py-2">
          <p className="text-xs text-[var(--bad)]">{actionError}</p>
        </div>
      ) : null}

      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">SMB Shares</h2>
          <Button size="sm" variant="ghost" onClick={refreshSmb} disabled={smbLoading}>
            {smbLoading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
        {smbError ? (
          <p className="text-xs text-[var(--bad)]">{smbError}</p>
        ) : smbShares.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No SMB shares found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Path</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Enabled</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Comment</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {smbShares.map((share) => (
                  <tr key={String(share.id)}>
                    <td className="py-2 font-medium text-[var(--text)]">{share.name}</td>
                    <td className="py-2 text-[var(--muted)] max-w-[180px] truncate">{share.path}</td>
                    <td className="py-2">
                      <div className="flex items-center gap-2">
                        <Badge status={enabledBadge(share.enabled)} size="sm" dot />
                        <span className="text-[var(--muted)]">{share.enabled ? "Yes" : "No"}</span>
                      </div>
                    </td>
                    <td className="py-2 text-[var(--muted)]">{share.comment || "--"}</td>
                    <td className="py-2 text-right">
                      {confirmDelete === `smb-${share.id}` ? (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={actionInFlight === `delete-smb-${share.id}`}
                            onClick={() => { void doDelete("smb", share.id); }}
                          >
                            Confirm
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setConfirmDelete(null)}
                          >
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight}
                            loading={actionInFlight === `toggle-smb-${share.id}`}
                            onClick={() => { void doToggle("smb", share.id, share.enabled); }}
                          >
                            {share.enabled ? "Disable" : "Enable"}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight}
                            onClick={() => setConfirmDelete(`smb-${share.id}`)}
                          >
                            Delete
                          </Button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">NFS Shares</h2>
          <Button size="sm" variant="ghost" onClick={refreshNfs} disabled={nfsLoading}>
            {nfsLoading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
        {nfsError ? (
          <p className="text-xs text-[var(--bad)]">{nfsError}</p>
        ) : nfsShares.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No NFS shares found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Path</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Enabled</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Networks</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Hosts</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {nfsShares.map((share) => (
                  <tr key={String(share.id)}>
                    <td className="py-2 font-medium text-[var(--text)] max-w-[180px] truncate">
                      {share.path}
                    </td>
                    <td className="py-2">
                      <div className="flex items-center gap-2">
                        <Badge status={enabledBadge(share.enabled)} size="sm" dot />
                        <span className="text-[var(--muted)]">{share.enabled ? "Yes" : "No"}</span>
                      </div>
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {share.networks?.join(", ") || "--"}
                    </td>
                    <td className="py-2 text-[var(--muted)]">
                      {share.hosts?.join(", ") || "--"}
                    </td>
                    <td className="py-2 text-right">
                      {confirmDelete === `nfs-${share.id}` ? (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            loading={actionInFlight === `delete-nfs-${share.id}`}
                            onClick={() => { void doDelete("nfs", share.id); }}
                          >
                            Confirm
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setConfirmDelete(null)}
                          >
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight}
                            loading={actionInFlight === `toggle-nfs-${share.id}`}
                            onClick={() => { void doToggle("nfs", share.id, share.enabled); }}
                          >
                            {share.enabled ? "Disable" : "Enable"}
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight}
                            onClick={() => setConfirmDelete(`nfs-${share.id}`)}
                          >
                            Delete
                          </Button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}
