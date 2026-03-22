import { Settings, Shield, Key, Database, Globe, Palette, UserCog, Lock } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";

interface SettingsEntry {
  id: string;
  label: string;
  description: string;
  href: string;
  keywords: string[];
  adminOnly?: boolean;
}

const SETTINGS_ITEMS: SettingsEntry[] = [
  {
    id: "settings-main",
    label: "Settings",
    description: "General hub settings and configuration",
    href: "/settings",
    keywords: ["settings", "configuration", "general"],
  },
  {
    id: "settings-users",
    label: "Users",
    description: "Manage user accounts and roles",
    href: "/users",
    keywords: ["users", "accounts", "roles", "members"],
    adminOnly: true,
  },
  {
    id: "settings-security",
    label: "Security",
    description: "TLS, agent setup, database, and security settings",
    href: "/security",
    keywords: ["security", "tls", "ssl", "certificate", "https", "agent", "setup", "install", "endpoint", "database", "postgres", "storage", "db"],
    adminOnly: true,
  },
  {
    id: "settings-discovery",
    label: "Discovery",
    description: "Network and asset discovery configuration",
    href: "/settings/discovery",
    keywords: ["discovery", "network", "scan", "detect"],
  },
  {
    id: "settings-appearance",
    label: "Appearance",
    description: "Theme, fonts, and visual preferences",
    href: "/settings/appearance",
    keywords: ["appearance", "theme", "fonts", "colors", "dark", "light"],
  },
];

const ICONS = {
  "settings-main": Settings,
  "settings-users": UserCog,
  "settings-security": Lock,
  "settings-discovery": Globe,
  "settings-appearance": Palette,
} as const;

export function createSettingsProvider(routerPush: (href: string) => void, isAdmin: boolean): PaletteProvider {
  return {
    id: "settings",
    group: "Settings",
    priority: 60,
    search(query: string): PaletteItem[] {
      const q = query.trim().toLowerCase();
      return SETTINGS_ITEMS.filter((entry) => {
        if (entry.adminOnly && !isAdmin) return false;
        if (q === "") return true;
        return (
          entry.label.toLowerCase().includes(q) ||
          entry.description.toLowerCase().includes(q) ||
          entry.keywords.some((kw) => kw.includes(q))
        );
      }).map((entry) => ({
        id: entry.id,
        label: entry.label,
        description: entry.description,
        icon: ICONS[entry.id as keyof typeof ICONS],
        href: entry.href,
        keywords: entry.keywords,
        action: () => routerPush(entry.href),
      }));
    },
  };
}
