"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Card } from "./ui/Card";
import { ArrowDown, ArrowUp } from "lucide-react";
import { Button } from "./ui/Button";
import { Input, Select } from "./ui/Input";
import { MetricGauge } from "./MetricGauge";
import { buildNodeMetadataSections } from "../console/models";
import type { Asset, TelemetryOverviewAsset } from "../console/models";

const COVERED_SECTIONS = new Set(["System", "Hardware", "CPU", "Memory", "Storage", "Network", "Live telemetry"]);

type DeviceOverviewGridProps = {
  asset: Asset;
  telemetry: TelemetryOverviewAsset | null;
  isProxmoxAsset: boolean;
  proxmoxKind: string;
  proxmoxNode: string;
  proxmoxVMID: string;
  canRunProxmoxQuickActions: boolean;
  proxmoxActionRunning: boolean;
  proxmoxActionMessage: string | null;
  proxmoxActionError: string | null;
  onProxmoxAction: (actionID: string, params?: Record<string, string>) => void;
  nodeActionRunning: boolean;
  nodeActionMessage: string | null;
  nodeActionError: string | null;
  onNodeQuickAction: (command: string, actionLabel?: string) => void;
  nodeSupportsNetworkActions: boolean;
  nodeNetworkMethodOptions: Array<{ value: string; label: string }>;
  nodeNetworkControlsLabel: string;
  nodeNetworkControlsHint: string;
  nodeNetworkActionRunning: boolean;
  nodeNetworkActionMessage: string | null;
  nodeNetworkActionError: string | null;
  onNodeNetworkAction: (
    action: "apply" | "rollback",
    options?: { method?: string; verifyTarget?: string; connection?: string },
  ) => void;
  nodeHasAgent: boolean;
  onDeleteClick: () => void;
  deleting: boolean;
  deleteError: string | null;
  onOpenMetricDetails?: (metric: string) => void;
};

function meta(asset: Asset, key: string): string {
  return asset.metadata?.[key]?.trim() ?? "";
}

function wrapDrilldownCard(
  card: ReactNode,
  onOpenDetails: (() => void) | undefined,
  ariaLabel: string,
) {
  if (!onOpenDetails) {
    return card;
  }
  return (
    <button
      type="button"
      onClick={onOpenDetails}
      aria-label={ariaLabel}
      className="block w-full rounded-lg text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]/60"
    >
      {card}
    </button>
  );
}

function SystemCard({ asset }: { asset: Asset }) {
  const rows: [string, string][] = [];
  const hostname = meta(asset, "hostname");
  const os = meta(asset, "os_pretty_name") || meta(asset, "os_name");
  const kernel = meta(asset, "kernel_release");
  const arch = meta(asset, "cpu_architecture");
  const agent = meta(asset, "agent");
  const proxType = meta(asset, "proxmox_type");
  const node = meta(asset, "node");
  const vmid = meta(asset, "vmid");

  if (hostname) rows.push(["Hostname", hostname]);
  if (os) rows.push(["OS", os]);
  if (kernel) rows.push(["Kernel", kernel]);
  if (arch) rows.push(["Architecture", arch]);
  if (agent) rows.push(["Agent", agent]);
  if (proxType) rows.push(["Proxmox Type", proxType]);
  if (node) rows.push(["Proxmox Node", node]);
  if (vmid) rows.push(["VMID", vmid]);

  if (rows.length === 0) return null;

  return (
    <Card>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">System</p>
      <dl className="space-y-1">
        {rows.map(([label, value]) => (
          <div key={label} className="flex items-baseline justify-between gap-2">
            <dt className="text-xs text-[var(--muted)] shrink-0">{label}</dt>
            <dd className="text-xs text-[var(--text)] text-right truncate">{value}</dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}

function HardwareCard({ asset }: { asset: Asset }) {
  const rows: [string, string][] = [];
  const vendor = meta(asset, "computer_vendor");
  const model = meta(asset, "computer_model");
  const chassis = meta(asset, "chassis_type");
  const mbVendor = meta(asset, "motherboard_vendor");
  const mbModel = meta(asset, "motherboard_model");

  if (vendor) rows.push(["Vendor", vendor]);
  if (model) rows.push(["Model", model]);
  if (chassis) rows.push(["Chassis", chassis]);
  if (mbVendor) rows.push(["Motherboard", `${mbVendor}${mbModel ? ` ${mbModel}` : ""}`]);

  if (rows.length === 0) return null;

  return (
    <Card>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">Hardware</p>
      <dl className="space-y-1">
        {rows.map(([label, value]) => (
          <div key={label} className="flex items-baseline justify-between gap-2">
            <dt className="text-xs text-[var(--muted)] shrink-0">{label}</dt>
            <dd className="text-xs text-[var(--text)] text-right truncate">{value}</dd>
          </div>
        ))}
      </dl>
    </Card>
  );
}

function CPUCard({ asset, cpuPercent, onOpenDetails }: { asset: Asset; cpuPercent?: number; onOpenDetails?: () => void }) {
  const cpuModel = meta(asset, "cpu_model");
  const cores = meta(asset, "cpu_cores_physical");
  const threads = meta(asset, "cpu_threads_logical");
  const maxMhz = meta(asset, "cpu_max_mhz");

  if (!cpuModel && !cores) return null;

  const stats: string[] = [];
  if (cores) stats.push(`${cores} cores`);
  if (threads) stats.push(`${threads} threads`);
  if (maxMhz) stats.push(maxMhz);

  const card = (
    <Card className={onOpenDetails ? "h-full transition-colors hover:border-[var(--accent)] hover:bg-[var(--surface)]" : ""}>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">CPU</p>
      {cpuModel && <p className="text-xs font-medium text-[var(--text)] mb-1 truncate">{cpuModel}</p>}
      {stats.length > 0 && (
        <p className="text-[10px] tabular-nums text-[var(--muted)] mb-2">{stats.join(" \u00b7 ")}</p>
      )}
      <MetricGauge label="Utilization" value={cpuPercent} />
      {onOpenDetails ? <p className="mt-2 text-[10px] text-[var(--accent)]">View full details</p> : null}
    </Card>
  );

  return wrapDrilldownCard(card, onOpenDetails, "Open CPU details");
}

function MemoryCard({ asset, memPercent, onOpenDetails }: { asset: Asset; memPercent?: number; onOpenDetails?: () => void }) {
  const totalRaw = meta(asset, "memory_total_bytes");

  if (!totalRaw && memPercent == null) return null;

  let totalFormatted = "";
  if (totalRaw) {
    const bytes = Number(totalRaw);
    if (Number.isFinite(bytes) && bytes > 0) {
      const gb = bytes / (1024 * 1024 * 1024);
      totalFormatted = gb >= 1 ? `${gb.toFixed(gb >= 10 ? 0 : 1)} GB` : `${(bytes / (1024 * 1024)).toFixed(0)} MB`;
    }
  }

  const card = (
    <Card className={onOpenDetails ? "h-full transition-colors hover:border-[var(--accent)] hover:bg-[var(--surface)]" : ""}>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">Memory</p>
      {totalFormatted && <p className="text-xs font-medium tabular-nums text-[var(--text)] mb-2">{totalFormatted}</p>}
      <MetricGauge label="Utilization" value={memPercent} />
      {onOpenDetails ? <p className="mt-2 text-[10px] text-[var(--accent)]">View full details</p> : null}
    </Card>
  );

  return wrapDrilldownCard(card, onOpenDetails, "Open memory details");
}

function StorageCard({ asset, diskPercent, onOpenDetails }: { asset: Asset; diskPercent?: number; onOpenDetails?: () => void }) {
  const totalRaw = meta(asset, "disk_root_total_bytes");
  const availRaw = meta(asset, "disk_root_available_bytes");
  const backupState = meta(asset, "backup_state");
  const daysSinceBackup = meta(asset, "days_since_backup");

  if (!totalRaw && diskPercent == null && !backupState) return null;

  let capacityLabel = "";
  if (totalRaw) {
    const total = Number(totalRaw);
    const avail = Number(availRaw);
    if (Number.isFinite(total) && total > 0) {
      const fmt = (b: number) => {
        const gb = b / (1024 * 1024 * 1024);
        return gb >= 1 ? `${gb.toFixed(gb >= 100 ? 0 : 1)} GB` : `${(b / (1024 * 1024)).toFixed(0)} MB`;
      };
      capacityLabel = Number.isFinite(avail) && avail > 0
        ? `${fmt(avail)} free of ${fmt(total)}`
        : fmt(total);
    }
  }

  const days = parseInt(daysSinceBackup, 10);
  const backupColor = backupState === "none" || isNaN(days) ? "bg-red-500" : days <= 1 ? "bg-emerald-500" : days <= 7 ? "bg-amber-500" : "bg-red-500";
  const backupText = backupState === "none" || isNaN(days)
    ? "No backups"
    : days === 0 ? "Today" : `${days}d ago`;

  const card = (
    <Card className={onOpenDetails ? "h-full transition-colors hover:border-[var(--accent)] hover:bg-[var(--surface)]" : ""}>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">Storage</p>
      <MetricGauge label="Root Disk" value={diskPercent} />
      {capacityLabel && <p className="text-[10px] tabular-nums text-[var(--muted)] mt-1">{capacityLabel}</p>}
      {backupState && (
        <div className="flex items-center gap-1.5 mt-2 pt-2 border-t border-[var(--line)]">
          <span className={`inline-block h-1.5 w-1.5 rounded-full ${backupColor}`} />
          <span className="text-[10px] tabular-nums text-[var(--muted)]">Backup: {backupText}</span>
        </div>
      )}
      {onOpenDetails ? <p className="mt-2 text-[10px] text-[var(--accent)]">View full details</p> : null}
    </Card>
  );

  return wrapDrilldownCard(card, onOpenDetails, "Open storage details");
}

function NetworkCard({ asset, onOpenDetails }: { asset: Asset; onOpenDetails?: () => void }) {
  const ifaceCount = meta(asset, "network_interface_count");
  const rxRaw = meta(asset, "network_rx_bytes_per_sec");
  const txRaw = meta(asset, "network_tx_bytes_per_sec");

  if (!ifaceCount && !rxRaw && !txRaw) return null;

  const fmtRate = (raw: string) => {
    const n = Number(raw);
    if (!Number.isFinite(n)) return "";
    if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB/s`;
    if (n >= 1024) return `${(n / 1024).toFixed(1)} KB/s`;
    return `${Math.round(n)} B/s`;
  };

  const card = (
    <Card className={onOpenDetails ? "h-full transition-colors hover:border-[var(--accent)] hover:bg-[var(--surface)]" : ""}>
      <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">Network</p>
      {ifaceCount && <p className="text-xs tabular-nums text-[var(--text)] mb-1">{ifaceCount} interface{ifaceCount !== "1" ? "s" : ""}</p>}
      <div className="space-y-0.5">
        {rxRaw && <p className="text-[10px] tabular-nums text-[var(--muted)] flex items-center gap-1"><ArrowDown size={10} className="text-emerald-500" /> {fmtRate(rxRaw)}</p>}
        {txRaw && <p className="text-[10px] tabular-nums text-[var(--muted)] flex items-center gap-1"><ArrowUp size={10} className="text-blue-400" /> {fmtRate(txRaw)}</p>}
      </div>
      {onOpenDetails ? <p className="mt-2 text-[10px] text-[var(--accent)]">View full details</p> : null}
    </Card>
  );

  return wrapDrilldownCard(card, onOpenDetails, "Open network details");
}

export function DeviceOverviewGrid(props: DeviceOverviewGridProps) {
  const {
    asset, telemetry, isProxmoxAsset, proxmoxKind, proxmoxNode, proxmoxVMID,
    canRunProxmoxQuickActions, proxmoxActionRunning, proxmoxActionMessage,
    proxmoxActionError, onProxmoxAction, nodeActionRunning, nodeActionMessage,
    nodeActionError, onNodeQuickAction, nodeSupportsNetworkActions,
    nodeNetworkMethodOptions, nodeNetworkControlsLabel, nodeNetworkControlsHint,
    nodeNetworkActionRunning, nodeNetworkActionMessage, nodeNetworkActionError,
    onNodeNetworkAction, nodeHasAgent, onDeleteClick, deleting, deleteError,
    onOpenMetricDetails,
  } = props;
  const [customNodeCommand, setCustomNodeCommand] = useState("");
  const [networkMethod, setNetworkMethod] = useState("auto");
  const [networkConnection, setNetworkConnection] = useState("");
  const [networkVerifyTarget, setNetworkVerifyTarget] = useState("");

  useEffect(() => {
    if (nodeNetworkMethodOptions.length === 0) {
      return;
    }
    if (!nodeNetworkMethodOptions.some((option) => option.value === networkMethod)) {
      setNetworkMethod(nodeNetworkMethodOptions[0]?.value ?? "auto");
    }
  }, [networkMethod, nodeNetworkMethodOptions]);

  const metrics = telemetry?.metrics;
  const haState = meta(asset, "hastate");
  const backupState = meta(asset, "backup_state");
  const daysSinceBackup = meta(asset, "days_since_backup");

  const allSections = useMemo(() => buildNodeMetadataSections(asset.metadata), [asset.metadata]);
  const extraSections = useMemo(() => allSections.filter(s => !COVERED_SECTIONS.has(s.title)), [allSections]);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
        <SystemCard asset={asset} />
        <HardwareCard asset={asset} />
        <CPUCard
          asset={asset}
          cpuPercent={metrics?.cpu_used_percent}
          onOpenDetails={onOpenMetricDetails ? () => onOpenMetricDetails("cpu_used_percent") : undefined}
        />
        <MemoryCard
          asset={asset}
          memPercent={metrics?.memory_used_percent}
          onOpenDetails={onOpenMetricDetails ? () => onOpenMetricDetails("memory_used_percent") : undefined}
        />
        <StorageCard
          asset={asset}
          diskPercent={metrics?.disk_used_percent}
          onOpenDetails={onOpenMetricDetails ? () => onOpenMetricDetails("disk_used_percent") : undefined}
        />
        <NetworkCard
          asset={asset}
          onOpenDetails={onOpenMetricDetails ? () => onOpenMetricDetails("network_rx_bytes_per_sec") : undefined}
        />
      </div>

      {extraSections.length > 0 && (
        <Card>
          <div className="space-y-4">
            {extraSections.map((section) => (
              <div key={section.title}>
                <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)] mb-2">{section.title}</p>
                <dl className="grid grid-cols-2 gap-x-6 gap-y-1">
                  {section.rows.map((row) => (
                    <div key={row.key} className="flex items-baseline justify-between gap-2">
                      <dt className="text-xs text-[var(--muted)]">{row.label}</dt>
                      <dd className="text-xs text-[var(--text)] text-right truncate">{row.value}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            ))}
          </div>
        </Card>
      )}

      {isProxmoxAsset && canRunProxmoxQuickActions && (
        <Card>
          <div className="flex items-center justify-between mb-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Proxmox</p>
            <p className="text-xs tabular-nums text-[var(--muted)]">
              {proxmoxKind.toUpperCase()}{proxmoxVMID ? ` ${proxmoxVMID}` : ""} on {proxmoxNode || "unknown"}
              {haState ? ` \u00b7 HA: ${haState}` : ""}
              {backupState && backupState !== "none" ? ` \u00b7 Backup: ${daysSinceBackup || "?"}d ago` : ""}
            </p>
          </div>
          {canRunProxmoxQuickActions && (
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-[10px] text-[var(--muted)] w-16 shrink-0">Lifecycle</span>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => onProxmoxAction(proxmoxKind === "lxc" ? "ct.start" : "vm.start")}>
                  Start
                </Button>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => onProxmoxAction(proxmoxKind === "lxc" ? "ct.shutdown" : "vm.shutdown")}>
                  Shutdown
                </Button>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => onProxmoxAction(proxmoxKind === "lxc" ? "ct.reboot" : "vm.reboot")}>
                  Reboot
                </Button>
                {proxmoxKind === "qemu" && (
                  <>
                    <Button size="sm" disabled={proxmoxActionRunning}
                      onClick={() => onProxmoxAction("vm.suspend")}>
                      Suspend
                    </Button>
                    <Button size="sm" disabled={proxmoxActionRunning}
                      onClick={() => onProxmoxAction("vm.resume")}>
                      Resume
                    </Button>
                  </>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-[10px] text-[var(--muted)] w-16 shrink-0">Data</span>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => {
                    const snapshot = `labtether-${new Date().toISOString().replace(/[:.]/g, "-")}`;
                    onProxmoxAction(proxmoxKind === "lxc" ? "ct.snapshot" : "vm.snapshot", { snapshot_name: snapshot });
                  }}>
                  Snapshot
                </Button>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => {
                    const storage = prompt("Backup storage (e.g. local):", "local");
                    if (storage) onProxmoxAction(proxmoxKind === "lxc" ? "ct.backup" : "vm.backup", { storage, mode: "snapshot" });
                  }}>
                  Backup
                </Button>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => {
                    const newId = prompt("New VMID (numeric):");
                    if (!newId || !/^\d+$/.test(newId.trim())) return;
                    const newName = prompt("New name (optional):", "");
                    onProxmoxAction(proxmoxKind === "lxc" ? "ct.clone" : "vm.clone", { new_id: newId.trim(), new_name: newName ?? "" });
                  }}>
                  Clone
                </Button>
              </div>
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-[10px] text-[var(--muted)] w-16 shrink-0">Advanced</span>
                <Button size="sm" disabled={proxmoxActionRunning}
                  onClick={() => {
                    const target = prompt("Target node name:");
                    if (target) onProxmoxAction(proxmoxKind === "lxc" ? "ct.migrate" : "vm.migrate", { target_node: target });
                  }}>
                  Migrate
                </Button>
                {proxmoxKind === "qemu" && (
                  <Button size="sm" disabled={proxmoxActionRunning}
                    onClick={() => {
                      const disk = prompt("Disk name (e.g. scsi0, virtio0):", "scsi0");
                      if (!disk) return;
                      const size = prompt("New size (e.g. +10G, 50G):");
                      if (!size) return;
                      onProxmoxAction("vm.disk_resize", { disk: disk.trim(), size: size.trim() });
                    }}>
                    Resize Disk
                  </Button>
                )}
                <Button size="sm" variant="danger" disabled={proxmoxActionRunning}
                  onClick={() => {
                    if (confirm("Force stop this VM/CT? This is equivalent to pulling the power cord.")) {
                      onProxmoxAction(proxmoxKind === "lxc" ? "ct.force_stop" : "vm.force_stop");
                    }
                  }}>
                  Force Stop
                </Button>
              </div>
              {proxmoxActionMessage && <p className="text-xs text-[var(--muted)]">{proxmoxActionMessage}</p>}
              {proxmoxActionError && <p className="text-xs text-red-500">{proxmoxActionError}</p>}
            </div>
          )}
        </Card>
      )}

      {nodeHasAgent && (
        <Card>
          <div className="flex items-center justify-between mb-3">
            <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">Quick Actions</p>
            <p className="text-xs text-[var(--muted)]">Runs directly on this node</p>
          </div>
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-1.5">
              <Button
                size="sm"
                disabled={nodeActionRunning}
                onClick={() => {
                  if (confirm("Restart this node now?")) {
                    onNodeQuickAction("systemctl reboot", "restart");
                  }
                }}
              >
                Restart
              </Button>
              <Button
                size="sm"
                variant="danger"
                disabled={nodeActionRunning}
                onClick={() => {
                  if (confirm("Shut down this node now?")) {
                    onNodeQuickAction("systemctl poweroff", "shutdown");
                  }
                }}
              >
                Shutdown
              </Button>
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-[10px] text-[var(--muted)]">Custom Command</span>
              <div className="flex items-center gap-2">
                <Input
                  value={customNodeCommand}
                  onChange={(event) => setCustomNodeCommand(event.target.value)}
                  placeholder="e.g. systemctl restart docker"
                />
                <Button
                  size="sm"
                  disabled={nodeActionRunning || customNodeCommand.trim() === ""}
                  onClick={() => {
                    const command = customNodeCommand.trim();
                    if (command === "") return;
                    onNodeQuickAction(command, "custom command");
                    setCustomNodeCommand("");
                  }}
                >
                  Run
                </Button>
              </div>
            </div>
            {nodeSupportsNetworkActions ? (
              <div className="flex flex-col gap-1.5 pt-2 border-t border-[var(--line)]">
                <span className="text-[10px] text-[var(--muted)]">{nodeNetworkControlsLabel}</span>
                <div className="grid grid-cols-1 md:grid-cols-[10rem_1fr_1fr_auto_auto] gap-2">
                  <Select
                    value={networkMethod}
                    onChange={(event) => setNetworkMethod(event.target.value)}
                    className="w-full"
                  >
                    {nodeNetworkMethodOptions.map((option) => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </Select>
                  <Input
                    value={networkConnection}
                    onChange={(event) => setNetworkConnection(event.target.value)}
                    placeholder="Connection/service (optional)"
                  />
                  <Input
                    value={networkVerifyTarget}
                    onChange={(event) => setNetworkVerifyTarget(event.target.value)}
                    placeholder="Verify target (optional, e.g. 1.1.1.1)"
                  />
                  <Button
                    size="sm"
                    disabled={nodeNetworkActionRunning}
                    onClick={() => {
                      if (confirm("Apply network configuration now?")) {
                        onNodeNetworkAction("apply", {
                          method: networkMethod,
                          connection: networkConnection,
                          verifyTarget: networkVerifyTarget,
                        });
                      }
                    }}
                  >
                    Apply Network
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={nodeNetworkActionRunning}
                    onClick={() => {
                      if (confirm("Rollback to the previous network snapshot?")) {
                        onNodeNetworkAction("rollback", {
                          method: networkMethod,
                          connection: networkConnection,
                          verifyTarget: networkVerifyTarget,
                        });
                      }
                    }}
                  >
                    Rollback
                  </Button>
                </div>
                {nodeNetworkControlsHint ? (
                  <p className="text-[10px] text-[var(--muted)]">{nodeNetworkControlsHint}</p>
                ) : null}
              </div>
            ) : null}
            {nodeActionMessage && <p className="text-xs text-[var(--muted)]">{nodeActionMessage}</p>}
            {nodeActionError && <p className="text-xs text-red-500">{nodeActionError}</p>}
            {nodeNetworkActionMessage && (
              <pre className="text-xs text-[var(--muted)] bg-[var(--surface)] rounded p-2 whitespace-pre-wrap max-h-40 overflow-auto">
                {nodeNetworkActionMessage}
              </pre>
            )}
            {nodeNetworkActionError && <p className="text-xs text-red-500">{nodeNetworkActionError}</p>}
          </div>
        </Card>
      )}

      <Card className="border-red-500/20">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-xs font-medium text-[var(--text)]">Danger Zone</p>
            <p className="text-[10px] text-[var(--muted)]">
              Permanently remove this device and all data.
              {nodeHasAgent ? " Agent will be disconnected." : ""}
            </p>
          </div>
          <Button variant="danger" size="sm" className="bg-red-500/10 shrink-0" onClick={onDeleteClick} disabled={deleting}>
            {deleting ? "Deleting..." : "Delete"}
          </Button>
        </div>
        {deleteError && <p className="text-xs text-red-500 mt-2">{deleteError}</p>}
      </Card>
    </div>
  );
}
