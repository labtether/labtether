"use client";

import dynamic from "next/dynamic";
import { useCallback, useEffect, useRef, useState } from "react";
import { Badge } from "../../../../../components/ui/Badge";
import { Button } from "../../../../../components/ui/Button";
import { Card } from "../../../../../components/ui/Card";
import { portainerAction, portainerFetch } from "./usePortainerData";
import { buildBrowserWsUrl } from "../../../../../lib/ws";

const XTerminal = dynamic(() => import("../../../../../components/XTerminal"), {
  ssr: false,
  loading: () => (
    <div className="flex items-center justify-center h-full min-h-[200px]">
      <span className="text-sm text-[var(--muted)]">Loading terminal...</span>
    </div>
  ),
});

type DockerContainer = {
  Id: string;
  Names: string[];
  Image: string;
  State: string;
  Status: string;
  Ports?: Array<{ IP?: string; PrivatePort: number; PublicPort?: number; Type: string }>;
};

function containerBadgeStatus(state: string): "ok" | "pending" | "bad" {
  const s = state.trim().toLowerCase();
  if (s === "running") return "ok";
  if (s === "paused" || s === "restarting") return "pending";
  return "bad";
}

function formatPorts(ports?: DockerContainer["Ports"]): string {
  if (!ports || ports.length === 0) return "--";
  return ports
    .filter((p) => p.PublicPort)
    .map((p) => `${p.PublicPort}:${p.PrivatePort}/${p.Type}`)
    .join(", ") || "--";
}

function containerName(c: DockerContainer): string {
  const name = c.Names?.[0] ?? c.Id.slice(0, 12);
  return name.startsWith("/") ? name.slice(1) : name;
}

type ExecShellState = {
  containerId: string;
  containerName: string;
  wsUrl: string;
};

type Props = {
  assetId: string;
  canExec: boolean;
};

export function PortainerContainersTab({ assetId, canExec }: Props) {
  const [containers, setContainers] = useState<DockerContainer[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionInFlight, setActionInFlight] = useState<string | null>(null);
  const [execShell, setExecShell] = useState<ExecShellState | null>(null);
  const seqRef = useRef(0);
  const latestRef = useRef(0);

  const load = useCallback(async () => {
    const id = ++seqRef.current;
    latestRef.current = id;
    setLoading(true);
    setError(null);
    try {
      const data = await portainerFetch<DockerContainer[]>(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/containers`,
      );
      if (latestRef.current !== id) return;
      setContainers(data ?? []);
    } catch (err) {
      if (latestRef.current !== id) return;
      setError(err instanceof Error ? err.message : "failed to load containers");
    } finally {
      if (latestRef.current === id) setLoading(false);
    }
  }, [assetId]);

  useEffect(() => {
    void load();
  }, [load]);

  const doAction = useCallback(async (containerId: string, action: "start" | "stop" | "restart") => {
    setActionError(null);
    setActionInFlight(`${containerId}-${action}`);
    try {
      await portainerAction(
        `/api/portainer/assets/${encodeURIComponent(assetId)}/containers/${encodeURIComponent(containerId)}/${action}`,
        "POST",
      );
      await load();
    } catch (err) {
      setActionError(err instanceof Error ? err.message : `failed to ${action} container`);
    } finally {
      setActionInFlight(null);
    }
  }, [assetId, load]);

  const openShell = useCallback((c: DockerContainer) => {
    const streamPath = `/portainer/assets/${encodeURIComponent(assetId)}/containers/${encodeURIComponent(c.Id)}/exec`;
    const wsUrl = buildBrowserWsUrl(streamPath);
    setExecShell({ containerId: c.Id, containerName: containerName(c), wsUrl });
  }, [assetId]);

  const closeShell = useCallback(() => {
    setExecShell(null);
  }, []);

  if (loading && containers.length === 0) {
    return <Card><p className="text-sm text-[var(--muted)]">Loading containers…</p></Card>;
  }

  if (error && containers.length === 0) {
    return <Card><p className="text-sm text-[var(--bad)]">{error}</p></Card>;
  }

  return (
    <>
      <Card>
        <div className="flex items-center justify-between mb-3">
          <h2 className="text-sm font-medium text-[var(--text)]">Containers</h2>
          <Button size="sm" variant="ghost" onClick={() => { void load(); }} disabled={loading}>
            {loading ? "Refreshing…" : "Refresh"}
          </Button>
        </div>
        {actionError && (
          <p className="mb-3 text-xs text-[var(--bad)]">{actionError}</p>
        )}
        {containers.length === 0 ? (
          <p className="text-sm text-[var(--muted)]">No containers found.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-[var(--line)]">
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Name</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">State</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Image</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Status</th>
                  <th className="py-2 text-left font-medium text-[var(--muted)]">Ports</th>
                  <th className="py-2 text-right font-medium text-[var(--muted)]">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--line)]">
                {containers.map((c) => {
                  const name = containerName(c);
                  return (
                    <tr key={c.Id}>
                      <td className="py-2 font-medium text-[var(--text)]">{name}</td>
                      <td className="py-2">
                        <div className="flex items-center gap-2">
                          <Badge status={containerBadgeStatus(c.State)} size="sm" />
                          <span className="text-[var(--muted)]">{c.State || "--"}</span>
                        </div>
                      </td>
                      <td className="py-2 max-w-48 truncate text-[var(--muted)]">{c.Image || "--"}</td>
                      <td className="py-2 text-[var(--muted)]">{c.Status || "--"}</td>
                      <td className="py-2 text-[var(--muted)]">{formatPorts(c.Ports)}</td>
                      <td className="py-2">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight || c.State === "running"}
                            loading={actionInFlight === `${c.Id}-start`}
                            onClick={() => { void doAction(c.Id, "start"); }}
                          >
                            Start
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight || c.State !== "running"}
                            loading={actionInFlight === `${c.Id}-stop`}
                            onClick={() => { void doAction(c.Id, "stop"); }}
                          >
                            Stop
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            disabled={!!actionInFlight}
                            loading={actionInFlight === `${c.Id}-restart`}
                            onClick={() => { void doAction(c.Id, "restart"); }}
                          >
                            Restart
                          </Button>
                          {canExec ? (
                            <Button
                              size="sm"
                              variant="ghost"
                              disabled={c.State !== "running"}
                              onClick={() => { openShell(c); }}
                            >
                              Shell
                            </Button>
                          ) : null}
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {execShell && (
        <PortainerExecModal
          containerName={execShell.containerName}
          wsUrl={execShell.wsUrl}
          onClose={closeShell}
        />
      )}
    </>
  );
}

type ExecModalProps = {
  containerName: string;
  wsUrl: string;
  onClose: () => void;
};

function PortainerExecModal({ containerName, wsUrl, onClose }: ExecModalProps) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60"
      onClick={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div className="flex flex-col w-[90vw] max-w-4xl h-[70vh] bg-[var(--surface)] rounded-lg overflow-hidden shadow-xl">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--line)] shrink-0">
          <span className="text-sm font-medium text-[var(--text)]">
            Shell — {containerName}
          </span>
          <Button size="sm" variant="ghost" onClick={onClose}>
            Close
          </Button>
        </div>
        <div className="flex-1 overflow-hidden">
          <XTerminal
            wsUrl={wsUrl}
            onDisconnected={onClose}
          />
        </div>
      </div>
    </div>
  );
}
