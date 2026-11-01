import { Zap } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";

const QUICK_CONNECT_REGEX = /^([\w.-]+@)?[\w.-]+(:\d+)?$/;

export function createQuickConnectProvider(routerPush: (href: string) => void, enabled = true): PaletteProvider {
  return {
    id: "quick-connect",
    group: "Quick connect",
    // After pages, devices and actions: any plain word looks like a host name, so this group must not take Enter
    // from a page or device the query names.
    priority: 50,
    search(query: string): PaletteItem[] {
      if (!enabled) return [];
      const q = query.trim();
      if (!QUICK_CONNECT_REGEX.test(q)) return [];

      const href = `/terminal?quickConnect=${encodeURIComponent(q)}`;
      return [
        {
          id: `quick-connect-${q}`,
          label: `Connect to ${q}`,
          description: "Terminal",
          icon: Zap,
          href,
          keywords: ["ssh", "connect", "quick"],
          action: () => routerPush(href),
        },
      ];
    },
  };
}
