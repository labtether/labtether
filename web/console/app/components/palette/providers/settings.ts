import { Settings, Globe, Palette, UserCog, Lock, BellRing, Cpu } from "lucide-react";
import type { PaletteItem, PaletteProvider } from "../../../contexts/PaletteContext";
import { meetsMinimumRole, type MinimumRole } from "../../../lib/roles";

interface SettingsEntry {
  id: string;
  label: string;
  description: string;
  href: string;
  keywords: string[];
  minimumRole?: MinimumRole;
}

const SETTINGS_ITEMS: SettingsEntry[] = [
  {
    id: "settings-main",
    label: "Settings",
    description: "General hub settings and configuration",
    href: "/settings",
    keywords: ["settings", "configuration", "general", "retention", "backup", "export", "database", "postgres", "reset"],
    minimumRole: "admin",
  },
  {
    id: "settings-users",
    label: "Users",
    description: "Account security and user administration",
    href: "/users",
    keywords: ["users", "accounts", "roles", "members"],
  },
  {
    id: "settings-notifications",
    label: "Notifications",
    description: "Channels that alerts are sent to",
    href: "/settings/notifications",
    keywords: ["notifications", "channels", "email", "slack", "discord", "ntfy", "webhook", "gotify"],
    minimumRole: "admin",
  },
  {
    id: "settings-security",
    label: "Security",
    description: "HTTPS, single sign-on, saved logins and API keys",
    href: "/security",
    keywords: ["security", "tls", "ssl", "certificate", "https", "tailscale", "oidc", "sso", "api", "keys", "tokens", "prometheus", "mcp"],
    minimumRole: "admin",
  },
  {
    id: "settings-agents",
    label: "Agents",
    description: "Connect agents, enrollment tokens and the hub SSH key",
    href: "/settings/agents",
    keywords: ["agents", "agent", "setup", "install", "enroll", "enrollment", "token", "ssh"],
    minimumRole: "admin",
  },
  {
    id: "settings-discovery",
    label: "Discovery",
    description: "Network and asset discovery configuration",
    href: "/settings?tab=advanced",
    keywords: ["discovery", "network", "scan", "detect"],
    minimumRole: "admin",
  },
  {
    id: "settings-appearance",
    label: "Appearance",
    description: "Theme, fonts, and visual preferences",
    href: "/settings?tab=general",
    keywords: ["appearance", "theme", "fonts", "colors", "dark", "light"],
    minimumRole: "admin",
  },
];

const ICONS = {
  "settings-main": Settings,
  "settings-users": UserCog,
  "settings-notifications": BellRing,
  "settings-security": Lock,
  "settings-agents": Cpu,
  "settings-discovery": Globe,
  "settings-appearance": Palette,
} as const;

export function createSettingsProvider(routerPush: (href: string) => void, role?: string): PaletteProvider {
  return {
    id: "settings",
    group: "Settings",
    priority: 60,
    search(query: string): PaletteItem[] {
      const q = query.trim().toLowerCase();
      return SETTINGS_ITEMS.filter((entry) => {
        if (!meetsMinimumRole(role, entry.minimumRole)) return false;
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
