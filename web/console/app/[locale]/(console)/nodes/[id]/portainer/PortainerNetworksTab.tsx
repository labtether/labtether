"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { Modal } from "../../../../../components/ui/Modal";
import { portainerAction, portainerFetch } from "./usePortainerData";

type DockerNetwork = {
  Id: string;
  Name: string;
  Driver: string;
  Scope: string;
  IPAM?: {
    Config?: Array<{ Subnet?: string; Gateway?: string }>;
  };
};

function networkSubnet(n: DockerNetwork): string {
  return n.IPAM?.Config?.[0]?.Subnet ?? "--";
}

function networkGateway(n: DockerNetwork): string {
  return n.IPAM?.Config?.[0]?.Gateway ?? "--";
}

type Props = {
  assetId: string;
};

export function PortainerNetworksTab({ assetId }: Props) {
  const [networks, setNetworks] = useState<DockerNetwork[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newNetworkName, setNewNetworkName] = useState("");
  const [newNetworkDriver, setNewNetworkDriver] = useState("bridge");
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
      const data = await portainerFetch<DockerNetwork[]>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/networks`,
      );
      if (latestRef.current !== id) return;
      setNetworks(data ?? []);
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load networks");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doRemove = useCallback(async (networkId: string) => {
    setActionError(null);
    setActionInFlight(networkId);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/networks/${encodeURIComponent(networkId)}`,
        "DELETE",
      );
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "failed to remove network");
    } finally {
      setActionInFlight(null);
    }
  }, [assetId, load]);

  const doCreate = useCallback(async () => {
    if (!newNetworkName.trim()) return;
    setCreateError(null);
    setCreating(true);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/networks`,
        "POST",
        { name: newNetworkName.trim(), driver: newNetworkDriver.trim() || "bridge" },
      );
      setCreateOpen(false);
      setNewNetworkName("");
      setNewNetworkDriver("bridge");
      await load();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "failed to create network");
    } finally {
      setCreating(false);
    }
  }, [assetId, load, newNetworkName, newNetworkDriver]);

  if (loading && networks.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading networks…</p></Card>;
  }

  if (error && networks.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <>
      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Networks</h2>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={() => { setCreateError(null); setNewNetworkName(""); setNewNetworkDriver("bridge"); setCreateOpen(true); }}>
              Create Network
            </Button>
            <Button size="sm" variant="ghost" onClick={() => { void load(); }} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </Button>
          </div>
        </div>
        {actionError && (
          <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p>
        )}
        {networks.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No networks found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Driver</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Subnet</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Gateway</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Scope</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {networks.map((n) => (
                  <tr key={n.Id}>
                    <td className="py-2 font-medium text-[var(--text)]">{n.Name}</td>
                    <td className="py-2 text-[var(--muted)]">{n.Driver || "--"}</td>
                    <td className="py-2 text-[var(--muted)]">{networkSubnet(n)}</td>
                    <td className="py-2 text-[var(--muted)]">{networkGateway(n)}</td>
                    <td className="py-2 text-[var(--muted)]">{n.Scope || "--"}</td>
                    <td className="py-2">
                      <div className="flex justify-end">
                        <Button
                          size="sm"
                          variant="danger"
                          disabled={!!actionInFlight}
                          loading={actionInFlight === n.Id}
                          onClick={() => { void doRemove(n.Id); }}
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

      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Create Network">
        <div className="p-5 space-y-4">
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1.5">Network Name</label>
            <input
              type="text"
              className="w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
              placeholder="my-network"
              value={newNetworkName}
              onChange={(e) => setNewNetworkName(e.target.value)}
              autoFocus
            />
          </div>
          <div>
            <label className="block text-xs font-medium text-[var(--muted)] mb-1.5">Driver</label>
            <input
              type="text"
              className="w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:outline-none"
              placeholder="bridge"
              value={newNetworkDriver}
              onChange={(e) => setNewNetworkDriver(e.target.value)}
            />
          </div>
          {createError && <p className="text-xs text-[var(--bad)]">{createError}</p>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setCreateOpen(false)} disabled={creating}>
              Cancel
            </Button>
            <Button variant="primary" size="sm" onClick={() => { void doCreate(); }} loading={creating} disabled={!newNetworkName.trim()}>
              Create
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
