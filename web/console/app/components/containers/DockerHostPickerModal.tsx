"use client";

import { useRouter } from "../../../i18n/navigation";
import { Modal } from "../ui/Modal";
import { Card } from "../ui/Card";
import { MiniBar } from "../ui/MiniBar";
import { EmptyState } from "../ui/EmptyState";
import { Button } from "../ui/Button";
import { Server } from "lucide-react";
import type { DockerHostSummary, DockerContainer } from "../../../lib/docker";

type HostData = {
  host: DockerHostSummary;
  containers: DockerContainer[];
};

type Props = {
  open: boolean;
  onClose: () => void;
  mode: "container" | "stack";
  hostData: HostData[];
};

function HostCard({
  host,
  containers,
  onSelect,
}: {
  host: DockerHostSummary;
  containers: DockerContainer[];
  onSelect: () => void;
}) {
  const running = containers.filter((c) => c.state.toLowerCase() === "running").length;
  const total = containers.length;

  const avgCpu =
    running > 0
      ? containers.reduce((sum, c) => sum + (c.cpu_percent ?? 0), 0) / running
      : 0;
  const avgMem =
    running > 0
      ? containers.reduce((sum, c) => sum + (c.memory_percent ?? 0), 0) / running
      : 0;

  const stacks = [...new Set(containers.map((c) => c.stack_name).filter(Boolean))];

  const lastSeen = host.last_seen ? new Date(host.last_seen) : null;
  const secsAgo = lastSeen ? (Date.now() - lastSeen.getTime()) / 1000 : Infinity;
  const isOffline = secsAgo > 60;

  const relativeTime = lastSeen
    ? secsAgo < 60
      ? "just now"
      : secsAgo < 3600
        ? `${Math.floor(secsAgo / 60)}m ago`
        : `${Math.floor(secsAgo / 3600)}h ago`
    : "unknown";

  return (
    <Card
      interactive={host.source !== "portainer"}
      className={`${host.source !== "portainer" ? "cursor-pointer" : "cursor-default opacity-60"} ${isOffline ? "opacity-50" : ""}`}
    >
      <button
        type="button"
        className="w-full text-left space-y-2"
        onClick={onSelect}
      >
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm font-semibold text-[var(--text)] truncate">
            {host.agent_id}
          </span>
          <span className="shrink-0 rounded-md border border-[var(--line)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]">
            {host.engine_os}/{host.engine_arch}
          </span>
        </div>

        <p className="text-[10px] text-[var(--muted)]">
          Docker {host.engine_version}
        </p>

        <div className="grid grid-cols-3 gap-3 text-[10px]">
          <div>
            <span className="text-[var(--muted)]">Containers</span>
            <p className="font-mono tabular-nums text-[var(--text)]">
              {running}/{total}
            </p>
          </div>
          <div>
            <span className="text-[var(--muted)]">CPU</span>
            <MiniBar value={avgCpu} />
            <p className="font-mono tabular-nums text-[var(--text)]">
              {avgCpu.toFixed(1)}%
            </p>
          </div>
          <div>
            <span className="text-[var(--muted)]">Memory</span>
            <MiniBar value={avgMem} />
            <p className="font-mono tabular-nums text-[var(--text)]">
              {avgMem.toFixed(1)}%
            </p>
          </div>
        </div>

        {stacks.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {stacks.map((s) => (
              <span
                key={s}
                className="rounded-md border border-[var(--line)] bg-[var(--surface)] px-1.5 py-0.5 text-[10px] text-[var(--muted)]"
              >
                {s}
              </span>
            ))}
          </div>
        )}

        <p className={`text-[10px] ${isOffline ? "text-[var(--warn)]" : "text-[var(--muted)]"}`}>
          {isOffline ? `Offline \u2014 last seen ${relativeTime}` : `Last seen ${relativeTime}`}
        </p>

        {host.source === "portainer" && (
          <p className="text-[10px] text-[var(--muted)] italic">
            Deploy via Portainer not yet supported
          </p>
        )}
      </button>
    </Card>
  );
}

export function DockerHostPickerModal({ open, onClose, mode, hostData }: Props) {
  const router = useRouter();

  const handleSelect = (host: DockerHostSummary) => {
    if (host.source === "portainer") return;
    const nodeId = `docker-host-${host.normalized_id}`;
    const path = mode === "container"
      ? `/nodes/${encodeURIComponent(nodeId)}/new-container`
      : `/nodes/${encodeURIComponent(nodeId)}/new-compose`;
    router.push(path);
    onClose();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Select Docker Host"
      className="md:!max-w-2xl"
    >
      <div className="p-4 overflow-y-auto max-h-[calc(100vh-10rem)]">
        {hostData.length === 0 ? (
          <EmptyState
            icon={Server}
            title="No Docker hosts connected"
            description="Add a device with Docker to get started."
            action={
              <Button variant="primary" onClick={() => { router.push("/nodes"); onClose(); }}>
                Go to Devices
              </Button>
            }
          />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {hostData.map(({ host, containers }) => (
              <HostCard
                key={host.agent_id}
                host={host}
                containers={containers}
                onSelect={() => handleSelect(host)}
              />
            ))}
          </div>
        )}
      </div>
    </Modal>
  );
}
