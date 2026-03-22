import { Cable, Database, Home, Layers, Server } from "lucide-react";
import type { PanelContext, PanelDef } from "./devicePanelTypes";

export function buildConnectorPanels(ctx: PanelContext): PanelDef[] {
  const panels: PanelDef[] = [];

  panels.push({
    id: "connect",
    label: "Connect",
    icon: Cable,
    summary: (ctx) => {
      const total = ctx.protocolCount + ctx.webServiceCount;
      if (total === 0) return ["No connections"];
      const parts: string[] = [];
      if (ctx.protocolCount > 0) parts.push(`${ctx.protocolCount} protocol${ctx.protocolCount !== 1 ? "s" : ""}`);
      if (ctx.webServiceCount > 0) parts.push(`${ctx.webServiceCount} web service${ctx.webServiceCount !== 1 ? "s" : ""}`);
      return parts;
    },
  });

  if (ctx.isHomeAssistantAsset) {
    panels.push({
      id: "homeassistant",
      label: "Home Assistant",
      icon: Home,
      summary: (context) => {
        const lines: string[] = [];
        if (context.isHomeAssistantHub) {
          const discovered = context.metadata["discovered"];
          if (discovered !== undefined) {
            lines.push(`${discovered} entities synced`);
          }
          const baseURL = context.metadata["collector_base_url"] ?? context.metadata["base_url"];
          if (baseURL !== undefined) {
            lines.push(baseURL);
          }
        } else {
          const state = context.metadata["state"];
          const domain = context.metadata["domain"];
          if (state !== undefined) {
            lines.push(`State ${state}`);
          }
          if (domain !== undefined) {
            lines.push(domain);
          }
        }
        return lines.length > 0 ? lines : ["Home Assistant details"];
      },
    });
  }

  if (ctx.isProxmoxAsset) {
    panels.push({
      id: "proxmox",
      label: "Proxmox",
      icon: Server,
      summary: (context) => {
        const proxmoxType = context.metadata["proxmox_type"];
        return proxmoxType !== undefined ? [proxmoxType] : ["Proxmox management"];
      },
    });
  }

  if (ctx.isTrueNASAsset) {
    panels.push({
      id: "truenas",
      label: "TrueNAS",
      icon: Database,
      summary: () => ["TrueNAS management"],
    });
  }

  if (ctx.isPBSAsset) {
    panels.push({
      id: "pbs",
      label: "PBS",
      icon: Database,
      summary: () => ["Backup server"],
    });
  }

  if (ctx.asset.source === "portainer") {
    panels.push({
      id: "portainer",
      label: "Portainer",
      icon: Server,
      summary: (context) => {
        const lines: string[] = [];
        if (context.asset.type === "container-host") {
          const containerCount = context.metadata["portainer_container_count"];
          const stackCount = context.metadata["portainer_stack_count"];
          if (containerCount !== undefined) {
            lines.push(`${containerCount} containers`);
          }
          if (stackCount !== undefined) {
            lines.push(`${stackCount} stacks`);
          }
        } else if (context.asset.type === "container") {
          const state = context.metadata["state"];
          const image = context.metadata["image"];
          if (state !== undefined) {
            lines.push(`State ${state}`);
          }
          if (image !== undefined) {
            lines.push(image);
          }
        } else if (context.asset.type === "stack" || context.asset.type === "compose-stack") {
          const state = context.metadata["status"];
          const memberCount = context.metadata["portainer_stack_container_count"];
          if (state !== undefined) {
            lines.push(state);
          }
          if (memberCount !== undefined) {
            lines.push(`${memberCount} containers`);
          }
        }
        return lines.length > 0 ? lines : ["Portainer details"];
      },
    });
  }

  for (const [key, { count }] of ctx.infraCategories.entries()) {
    panels.push({
      id: key,
      label: key.charAt(0).toUpperCase() + key.slice(1),
      icon: Layers,
      summary: () => [`${count} items`],
    });
  }

  return panels;
}
