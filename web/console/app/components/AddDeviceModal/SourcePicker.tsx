"use client";

import { Archive, Server, Container, Database, HardDrive, Ship, House, Pencil } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { AddDeviceCompatPrefill } from "./types";

export type SourceType = "agent" | "proxmox" | "pbs" | "docker" | "portainer" | "truenas" | "homeassistant" | "manual";

type SourceOption = {
  id: SourceType;
  label: string;
  description: string;
  icon: LucideIcon;
  available: boolean;
};

const sources: SourceOption[] = [
  { id: "agent", label: "Agent", description: "Install LabTether agent on any device", icon: Server, available: true },
  { id: "proxmox", label: "Proxmox VE", description: "Connect a Proxmox cluster via API", icon: Container, available: true },
  { id: "pbs", label: "Proxmox Backup", description: "Connect a PBS server via API", icon: Archive, available: true },
  { id: "docker", label: "Docker", description: "Connect Docker hosts and workloads", icon: Ship, available: true },
  { id: "portainer", label: "Portainer", description: "Connect via Portainer API", icon: HardDrive, available: true },
  { id: "truenas", label: "TrueNAS", description: "Connect a TrueNAS system via API", icon: Database, available: true },
  { id: "homeassistant", label: "Home Assistant", description: "Install integration or review add-on setup", icon: House, available: true },
  { id: "manual", label: "Manual Device", description: "SSH, Telnet, VNC, RDP, or ARD", icon: Pencil, available: true },
];

type SourcePickerProps = {
  onSelect: (source: SourceType) => void;
  prefillBySource?: Partial<Record<SourceType, AddDeviceCompatPrefill>>;
};

export function SourcePicker({ onSelect, prefillBySource = {} }: SourcePickerProps) {
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-[var(--line)] bg-[var(--surface)] px-3 py-2">
        <p className="text-xs font-medium text-[var(--text)]">Default Setup</p>
        <p className="text-[11px] text-[var(--muted)]">
          Common connection settings are shown inline. Use per-source advanced settings only for uncommon overrides.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        {sources.map((source) => {
          const Icon = source.icon;
          const suggestion = prefillBySource[source.id];
          return (
            <button
              key={source.id}
              disabled={!source.available}
              onClick={() => onSelect(source.id)}
              className={`relative flex flex-col items-start gap-2 p-4 rounded-lg border text-left transition-colors duration-150 ${
                source.available
                  ? "border-[var(--line)] hover:border-[var(--muted)] hover:bg-[var(--hover)] cursor-pointer"
                  : "border-[var(--line)] opacity-40 cursor-not-allowed"
              }`}
            >
              {!source.available && (
                <span className="absolute top-2 right-2 text-[10px] font-medium uppercase tracking-wider text-[var(--muted)]">
                  Coming Soon
                </span>
              )}
              {source.available && suggestion && (
                <span className="absolute top-2 right-2 text-[10px] font-medium uppercase tracking-wider text-[var(--ok)]">
                  Detected
                </span>
              )}
              <Icon size={20} strokeWidth={1.5} className="text-[var(--muted)]" />
              <div>
                <p className="text-sm font-medium text-[var(--text)]">{source.label}</p>
                <p className="text-xs text-[var(--muted)]">
                  {source.description}
                  {suggestion ? ` (prefill from ${suggestion.serviceName || suggestion.baseURL})` : ""}
                </p>
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
