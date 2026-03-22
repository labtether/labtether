import { Zap } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";

const QUICK_CONNECT_REGEX = /^([\w.-]+@)?[\w.-]+(:\d+)?$/;

export function createQuickConnectProvider(routerPush: (href: string) => void): PaletteProvider {
  return {
    id: "quick-connect",
    group: "Quick Connect",
    priority: 5,
    search(query: string): PaletteItem[] {
      const q = query.trim();
      if (!QUICK_CONNECT_REGEX.test(q)) return [];

      const href = `/terminal?quickConnect=${encodeURIComponent(q)}`;
      return [
        {
          id: `quick-connect-${q}`,
          label: `Connect to ${q}`,
          description: "Open terminal with quick connect",
          icon: Zap,
          href,
          keywords: ["ssh", "connect", "quick"],
          action: () => routerPush(href),
        },
      ];
    },
  };
}
