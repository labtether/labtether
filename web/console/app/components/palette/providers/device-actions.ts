import { TerminalSquare, Monitor, Info, FolderOpen, FileText } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import type { FastStatusSlice } from "../../../contexts/StatusContext";

interface ActionDef {
  key: string;
  label: string;
  icon: LucideIcon;
  hrefSuffix: string;
  description: string;
}

const DEVICE_ACTIONS: ActionDef[] = [
  {
    key: "terminal",
    label: "Terminal",
    icon: TerminalSquare,
    hrefSuffix: "",
    description: "Open terminal session",
  },
  {
    key: "desktop",
    label: "Desktop",
    icon: Monitor,
    hrefSuffix: "/desktop",
    description: "Open remote desktop",
  },
  {
    key: "details",
    label: "Details",
    icon: Info,
    hrefSuffix: "",
    description: "View device details",
  },
  {
    key: "files",
    label: "Files",
    icon: FolderOpen,
    hrefSuffix: "/files",
    description: "Browse device files",
  },
  {
    key: "logs",
    label: "Logs",
    icon: FileText,
    hrefSuffix: "/logs",
    description: "View device logs",
  },
];

export function createDeviceActionsProvider(
  getStatus: () => FastStatusSlice | null,
  routerPush: (href: string) => void,
): PaletteProvider {
  return {
    id: "device-actions",
    group: "Actions",
    priority: 30,
    shortcut: ">",
    search(query: string): PaletteItem[] {
      const status = getStatus();
      if (!status) return [];

      // Strip leading ">" trigger character
      const stripped = query.startsWith(">") ? query.slice(1) : query;
      const q = stripped.trim().toLowerCase();

      const items: PaletteItem[] = [];

      for (const asset of status.assets) {
        const deviceName = asset.name || asset.id;
        const deviceMatches =
          q === "" ||
          deviceName.toLowerCase().includes(q) ||
          asset.id.toLowerCase().includes(q);

        if (!deviceMatches) continue;

        for (const actionDef of DEVICE_ACTIONS) {
          const actionMatches =
            q === "" ||
            deviceName.toLowerCase().includes(q) ||
            actionDef.label.toLowerCase().includes(q) ||
            actionDef.description.toLowerCase().includes(q);

          if (!actionMatches) continue;

          const baseHref = `/nodes/${asset.id}`;
          const href =
            actionDef.key === "terminal"
              ? `/terminal?target=${asset.id}`
              : actionDef.key === "details"
              ? baseHref
              : `${baseHref}${actionDef.hrefSuffix}`;

          items.push({
            id: `device-action-${asset.id}-${actionDef.key}`,
            label: `${deviceName}: ${actionDef.label}`,
            description: actionDef.description,
            icon: actionDef.icon,
            href,
            action: () => routerPush(href),
          });
        }
      }

      return items;
    },
  };
}
