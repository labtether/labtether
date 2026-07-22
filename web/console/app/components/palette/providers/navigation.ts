import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import { navGroups } from "../../../lib/navigation";
import { meetsMinimumRole } from "../../../lib/roles";

export function createNavigationProvider(routerPush: (href: string) => void, role?: string): PaletteProvider {
  return {
    id: "navigation",
    group: "Pages",
    priority: 10,
    search(query: string): PaletteItem[] {
      const q = query.trim().toLowerCase();
      const items: PaletteItem[] = [];
      for (const group of navGroups) {
        for (const item of group.items) {
          if (!meetsMinimumRole(role, item.minimumRole)) continue;
          if (
            q === "" ||
            item.label.toLowerCase().includes(q) ||
            group.category.toLowerCase().includes(q)
          ) {
            items.push({
              id: `nav-${item.href}`,
              label: item.label,
              description: group.category,
              icon: item.icon,
              href: item.href,
              keywords: [group.category.toLowerCase()],
              action: () => routerPush(item.href),
            });
          }
        }
      }
      return items;
    },
  };
}
