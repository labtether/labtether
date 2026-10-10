"use client";

import { ArrowDown,ArrowUp } from "lucide-react";
import { type ReactNode } from "react";
import type { Asset } from "../console/models";
import { MetricGauge } from "./MetricGauge";
import { Card } from "./ui/Card";

export function meta(asset: Asset, key: string): string {
  return asset.metadata?.[key]?.trim() ?? "";
}

export function parseBackupAgeDays(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^(?:0|[1-9]\d*)(?:\.\d+)?$/.test(trimmed)) {
    return null;
  }
  const days = Number(trimmed);
  return Number.isFinite(days) ? days : null;
}

export function wrapDrilldownCard(
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

export function SystemCard({ asset }: { asset: Asset }) {
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

export function HardwareCard({ asset }: { asset: Asset }) {
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

export function CPUCard({ asset, cpuPercent, onOpenDetails }: { asset: Asset; cpuPercent?: number; onOpenDetails?: () => void }) {
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

export function MemoryCard({ asset, memPercent, onOpenDetails }: { asset: Asset; memPercent?: number; onOpenDetails?: () => void }) {
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

export function StorageCard({ asset, diskPercent, onOpenDetails }: { asset: Asset; diskPercent?: number; onOpenDetails?: () => void }) {
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
      capacityLabel = /^(?:0|[1-9]\d*)$/.test(availRaw) && Number.isFinite(avail)
        ? `${fmt(avail)} free of ${fmt(total)}`
        : fmt(total);
    }
  }

  const days = parseBackupAgeDays(daysSinceBackup);
  const backupColor = backupState === "none" || days == null ? "bg-red-500" : days <= 1 ? "bg-emerald-500" : days <= 7 ? "bg-amber-500" : "bg-red-500";
  const backupText = backupState === "none" || days == null
    ? "No backups"
    : days < 1 ? "Today" : `${Math.floor(days)}d ago`;

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

export function NetworkCard({ asset, onOpenDetails }: { asset: Asset; onOpenDetails?: () => void }) {
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
