"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { Modal } from "../../../../../components/ui/Modal";
import { portainerAction, portainerFetch } from "./usePortainerData";

type DockerVolume = {
  Name: string;
  Driver: string;
  Mountpoint: string;
};

type VolumesResponse = {
  Volumes: DockerVolume[];
};

type Props = {
  assetId: string;
};

export function PortainerVolumesTab({ assetId }: Props) {
  const [volumes, setVolumes] = useState<DockerVolume[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newVolumeName, setNewVolumeName] = useState("");
  const [newVolumeDriver, setNewVolumeDriver] = useState("local");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await portainerFetch<VolumesResponse>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/volumes`,
      );
      if (latestRef.current !== id) return;
      setVolumes(data?.Volumes ?? []);
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load volumes");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doRemove = useCallback(async (name: string) => {
    setActionError(null);
    setActionInFlight(name);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/volumes/${encodeURIComponent(name)}`,
        "DELETE",
      );
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "failed to remove volume");
    } finally {
      setActionInFlight(null);
    }
  }, [assetId, load]);

  const doCreate = useCallback(async () => {
    if (!newVolumeName.trim()) return;
    setCreateError(null);
    setCreating(true);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/volumes`,
        "POST",
        { name: newVolumeName.trim(), driver: newVolumeDriver.trim() || "local" },
      );
      setCreateOpen(false);
      setNewVolumeName("");
      setNewVolumeDriver("local");
      await load();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "failed to create volume");
    } finally {
      setCreating(false);
    }
  }, [assetId, load, newVolumeName, newVolumeDriver]);

  if (loading && volumes.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading volumes…</p></Card>;
  }

  if (error && volumes.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <>
      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Volumes</h2>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={() => { setCreateError(null); setNewVolumeName(""); setNewVolumeDriver("local"); setCreateOpen(true); }}>
              Create Volume
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { void load(); }} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </div>
        </div>
        {actionError && (
          <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p>
        )}
        {volumes.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No volumes found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Driver</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Mountpoint</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {volumes.map((v) => (
                  <tr key={v.Name}>
                    <td className="py-2 font-medium text-[var(--text)]">{v.Name}</td>
                    <td className="py-2 text-[var(--muted)]">{v.Driver || "--"}</td>
                    <td className="py-2 max-w-64 truncate text-[var(--muted)]">{v.Mountpoint || "--"}</td>
                    <td className="py-2">
                      <div className="flex justify-end">
                        <Button
                          size="sm"
                          variant="danger"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === v.Name}
                          onClick={() => { void doRemove(v.Name); }}
                        >
                          Remove
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Create Volume">
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1.5">Volume Name</label>
            <input
              type="text"
              className="w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
              placeholder="my-volume"
              value={newVolumeName}
              onChange={(e) => setNewVolumeName(e.target.value)}
              autoFocus
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1.5">Driver</label>
            <input
              type="text"
              className="w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
              placeholder="local"
              value={newVolumeDriver}
              onChange={(e) => setNewVolumeDriver(e.target.value)}
            />
          </div>
          {createError && <p className="text-xs text-[var(--bad)]">{createError}</p>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setCreateOpen(false)} disabled={creating}>
              Cancel
            </Button>
            <Button variant="primary" size="sm" onClick={() => { void doCreate(); }} loading={creating} disabled={!newVolumeName.trim()}>
              Create
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
