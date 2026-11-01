import { Server } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import type { FastStatusSlice } from "../../../contexts/StatusContext";

export function createDevicesProvider(
  getStatus: () => FastStatusSlice | null,
  onSelectDevice: (href: string) => void,
): PaletteProvider {
  return {
    id: "devices",
    group: "Devices",
    priority: 20,
    search(query: string): PaletteItem[] {
      const status = getStatus();
      if (!status) return [];
      const q = query.trim().toLowerCase();
      return status.assets
        .filter((asset) => {
          if (q === "") return true;
          const searchable = [
            asset.name,
            asset.id,
            asset.platform ?? "",
            asset.type,
            deviceAddress(asset.metadata) ?? "",
            ...(asset.tags ?? []),
          ]
            .join(" ")
            .toLowerCase();
          return searchable.includes(q);
        })
        .map((asset) => {
          const href = `/nodes/${asset.id}`;
          return {
            id: `device-${asset.id}`,
            label: asset.name || asset.id,
            description: deviceAddress(asset.metadata),
            icon: Server,
            href,
            keywords: [...(asset.tags ?? []), asset.platform ?? "", asset.type].filter(Boolean),
            action: () => onSelectDevice(href),
          };
        });
    },
  };
}

/** The device's address, the hint the 2.0 palette shows beside its name. */
function deviceAddress(meta: Record<string, string> | undefined): string | undefined {
  if (!meta) return undefined;
  return meta.ip_address || meta.ip || meta.primary_ip || undefined;
}
