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
            description: [asset.platform, asset.type].filter(Boolean).join(" · "),
            icon: Server,
            href,
            keywords: [...(asset.tags ?? []), asset.platform ?? "", asset.type].filter(Boolean),
            action: () => onSelectDevice(href),
          };
        });
    },
  };
}
