import {
  Activity,
  BarChart3,
  Box,
  Clock,
  Cpu,
  FolderOpen,
  HardDrive,
  Layers,
  Monitor,
  Network,
  Package,
  ScrollText,
  Settings,
  Terminal,
  Users,
  Zap,
} from "lucide-react";
import type { PanelContext, PanelDef } from "./devicePanelTypes";

function formatBytes(bytes: number): string {
  if (bytes >= 1073741824) {
    return `${Math.round(bytes / 1073741824)} GB`;
  }
  if (bytes >= 1048576) {
    return `${Math.round(bytes / 1048576)} MB`;
  }
  return `${bytes} B`;
}

export function buildCorePanels(ctx: PanelContext): PanelDef[] {
  const panels: PanelDef[] = [];

  if (ctx.isHomeAssistantAsset) {
    return panels;
  }

  const isDockerHostPanelSource = ctx.asset.source === "docker" && ctx.asset.type === "container-host";
  if (ctx.mergedDockerHost !== null || isDockerHostPanelSource) {
    panels.push({
      id: "docker",
      label: "Docker",
      icon: Box,
      summary: (context) => {
        const containerCount = context.metadata["docker_container_count"];
        const stackCount = context.metadata["docker_stack_count"];
        if (containerCount !== undefined || stackCount !== undefined) {
          const lines: string[] = [];
          if (containerCount !== undefined) {
            lines.push(`${containerCount} containers`);
          }
          if (stackCount !== undefined) {
            lines.push(`${stackCount} stacks`);
          }
          return lines;
        }
        return ["Docker host"];
      },
      subTabs: ["containers", "stacks", "images"],
      defaultSub: "containers",
    });
  }

  panels.push({
    id: "system",
    label: "System",
    icon: Cpu,
    summary: (context) => {
      const coresRaw = context.metadata["cpu_cores_physical"] ?? context.metadata["cpu_threads_logical"];
      const ramRaw = context.metadata["memory_total_bytes"];
      const cpuModel = context.metadata["cpu_model"];

      const parts: string[] = [];
      const corePart = coresRaw !== undefined ? `${coresRaw} cores` : null;
      const ramPart = ramRaw !== undefined ? `${formatBytes(Number(ramRaw))} RAM` : null;
      const line1Parts = [corePart, ramPart].filter((part): part is string => part !== null);

      if (line1Parts.length > 0) {
        parts.push(line1Parts.join(" · "));
      }
      if (cpuModel !== undefined) {
        parts.push(cpuModel);
      }

      // Storage controller / PBS fallback: show storage summary when no CPU/RAM info.
      if (parts.length === 0) {
        const totalRaw = context.metadata["total_bytes"];
        const datastoreCount = context.metadata["datastore_count"];
        const version = context.metadata["version"];
        if (totalRaw !== undefined) {
          const storageLine = datastoreCount !== undefined
            ? `${formatBytes(Number(totalRaw))} across ${datastoreCount} datastores`
            : formatBytes(Number(totalRaw));
          parts.push(storageLine);
        }
        if (version !== undefined) {
          parts.push(`v${version}`);
        }
      }

      return parts.length > 0 ? parts : ["System info"];
    },
  });

  if (ctx.nodeHasAgent) {
    panels.push({
      id: "terminal",
      label: "Terminal",
      icon: Terminal,
      summary: () => ["Agent connected"],
    });
  }

  if (ctx.desktopEligible) {
    panels.push({
      id: "desktop",
      label: "Remote View",
      icon: Monitor,
      summary: () => ["Remote view available"],
    });
  }

  panels.push({
    id: "monitoring",
    label: "Monitoring",
    icon: BarChart3,
    summary: (context) => {
      const cpu = context.metadata["telemetry_cpu_pct"];
      const mem = context.metadata["telemetry_mem_pct"];
      const disk = context.metadata["telemetry_disk_pct"];
      if (cpu !== undefined || mem !== undefined || disk !== undefined) {
        const parts: string[] = [];
        if (cpu !== undefined) {
          parts.push(`CPU ${cpu}%`);
        }
        if (mem !== undefined) {
          parts.push(`MEM ${mem}%`);
        }
        if (disk !== undefined) {
          parts.push(`DSK ${disk}%`);
        }
        return [parts.join(" · ")];
      }
      return ["View metrics"];
    },
  });

  if (ctx.nodeHasAgent) {
    panels.push({
      id: "processes",
      label: "Processes",
      icon: Activity,
      summary: (context) => {
        const count = context.metadata["process_count"];
        return count !== undefined ? [`${count} running`] : ["View processes"];
      },
    });
  }

  if (ctx.supportsNetworkListing) {
    panels.push({
      id: "network",
      label: "Network",
      icon: Network,
      summary: (context) => {
        const count = context.metadata["network_interface_count"];
        return count !== undefined ? [`${count} interfaces`] : ["View network"];
      },
    });
  }

  if (ctx.nodeHasAgent) {
    panels.push({
      id: "files",
      label: "Files",
      icon: FolderOpen,
      summary: () => ["Browse filesystem"],
    });
  }

  if (ctx.supportsServiceListing) {
    panels.push({
      id: "services",
      label: "Services",
      icon: Layers,
      summary: () => ["View services"],
    });
  }

  if (ctx.supportsPackageListing) {
    panels.push({
      id: "packages",
      label: "Packages",
      icon: Package,
      summary: () => ["Installed packages"],
    });
  }

  panels.push({
    id: "logs",
    label: "Logs",
    icon: ScrollText,
    summary: () => ["View logs"],
  });

  panels.push({
    id: "actions",
    label: "Actions",
    icon: Zap,
    summary: () => ["Action history"],
  });

  if (ctx.supportsScheduleListing) {
    panels.push({
      id: "cron",
      label: "Cron / Timers",
      icon: Clock,
      summary: () => ["Scheduled tasks"],
    });
  }

  if (ctx.isLinuxAgentNode) {
    panels.push({
      id: "disks",
      label: "Disks",
      icon: HardDrive,
      summary: () => ["Disk management"],
    });
    panels.push({
      id: "users",
      label: "Users",
      icon: Users,
      summary: () => ["User accounts"],
    });
  }

  if (ctx.nodeHasAgent) {
    panels.push({
      id: "agent-settings",
      label: "Agent Settings",
      icon: Settings,
      summary: () => ["Configure agent"],
    });
  }

  return panels;
}
