import { Clock } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import { loadRecentTargets } from "../../terminal/TerminalPane";

function timeAgo(isoString: string): string {
  if (!isoString) return "";
  const diff = Date.now() - new Date(isoString).getTime();
  if (!Number.isFinite(diff) || diff < 0) return "";
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return `${seconds}s ago`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function createRecentProvider(routerPush: (href: string) => void): PaletteProvider {
  const recents = loadRecentTargets();
  return {
    id: "recent",
    group: "Recent",
    priority: 15,
    search(query: string): PaletteItem[] {
      const q = query.trim().toLowerCase();
      return recents
        .filter((rt) => {
          if (q === "") return true;
          return (
            rt.name.toLowerCase().includes(q) ||
            rt.id.toLowerCase().includes(q) ||
            rt.type.toLowerCase().includes(q)
          );
        })
        .map((rt) => {
          const href = `/terminal?target=${rt.id}`;
          const ago = timeAgo(rt.lastConnected);
          return {
            id: `recent-${rt.id}`,
            label: rt.name || rt.id,
            description: [rt.type, ago].filter(Boolean).join(" · "),
            icon: Clock,
            href,
            keywords: [rt.type],
            action: () => routerPush(href),
          };
        });
    },
  };
}
