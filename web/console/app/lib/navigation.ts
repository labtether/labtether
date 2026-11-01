import {
  Bell,
  Boxes,
  FileText,
  FolderOpen,
  GitBranch,
  Globe,
  LayoutDashboard,
  Monitor,
  Server,
  Settings,
  TerminalSquare,
  Zap,
  Clock,
  Webhook,
  UserCog,
  MapPin,
  Lock,
  Shield,
  ScrollText,
  type LucideIcon,
} from "lucide-react";

import { meetsMinimumRole, type MinimumRole } from "./roles";

export type NavItem = {
  href: string;
  label: string;
  translationKey: string;
  icon: LucideIcon;
  minimumRole?: MinimumRole;
};

export type NavGroup = {
  category: string;
  categoryKey: string;
  items: NavItem[];
};

/** A sidebar entry that stands for several pages, shown as a tab row across them (Automations, Settings). */
export type NavSection = NavItem & { id: "automations" | "settings"; tabs: NavItem[] };

// The 2.0 sidebar: one flat list of pages, then the per-device tools a little apart, then Settings at the bottom.
export const navMain: NavItem[] = [
  { href: "/", label: "Dashboard", translationKey: "dashboard", icon: LayoutDashboard },
  { href: "/nodes", label: "Devices", translationKey: "devices", icon: Server },
  { href: "/topology", label: "Map", translationKey: "map", icon: GitBranch },
  { href: "/services", label: "Services", translationKey: "services", icon: Globe },
  { href: "/containers", label: "Containers", translationKey: "containers", icon: Boxes },
  { href: "/alerts", label: "Alerts", translationKey: "alerts", icon: Bell },
  { href: "/actions", label: "Automations", translationKey: "automations", icon: Zap },
  { href: "/logs", label: "Logs", translationKey: "logs", icon: FileText },
];

export const navTools: NavItem[] = [
  { href: "/terminal", label: "Terminal", translationKey: "terminal", icon: TerminalSquare, minimumRole: "write" },
  { href: "/files", label: "Files", translationKey: "files", icon: FolderOpen, minimumRole: "write" },
  { href: "/remote-view", label: "Remote View", translationKey: "remoteView", icon: Monitor, minimumRole: "write" },
];

export const navSections: NavSection[] = [
  {
    id: "automations",
    href: "/actions",
    label: "Automations",
    translationKey: "automations",
    icon: Zap,
    tabs: [
      { href: "/actions", label: "Actions", translationKey: "actions", icon: Zap },
      { href: "/schedules", label: "Schedules", translationKey: "schedules", icon: Clock },
      { href: "/webhooks", label: "Webhooks", translationKey: "webhooks", icon: Webhook },
    ],
  },
  {
    id: "settings",
    href: "/settings",
    label: "Settings",
    translationKey: "settings",
    icon: Settings,
    tabs: [
      { href: "/settings", label: "General", translationKey: "general", icon: Settings, minimumRole: "admin" },
      { href: "/users", label: "Users", translationKey: "users", icon: UserCog },
      { href: "/groups", label: "Groups", translationKey: "groups", icon: MapPin },
      { href: "/security", label: "Security", translationKey: "security", icon: Lock, minimumRole: "admin" },
      { href: "/reliability", label: "Health", translationKey: "health", icon: Shield },
      { href: "/audit-log", label: "Audit Log", translationKey: "auditLog", icon: ScrollText },
    ],
  },
];

export const settingsSection = navSections.find((s) => s.id === "settings") as NavSection;

/** Every page once, for the ⌘K palette and role checks. Section pages are listed under their section's name. */
export const navGroups: NavGroup[] = [
  {
    category: "Pages",
    categoryKey: "categories.pages",
    items: [...navMain.filter((i) => !navSections.some((s) => s.href === i.href)), ...navTools],
  },
  ...navSections.map((s) => ({ category: s.label, categoryKey: s.translationKey, items: s.tabs })),
];

export const isActiveHref = (pathname: string, href: string) =>
  href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(`${href}/`);

/** The section whose pages include this path, if any. */
export const sectionFor = (pathname: string) =>
  navSections.find((s) => s.tabs.some((t) => isActiveHref(pathname, t.href)));

/** The tabs this role may open; the first is where the sidebar entry leads. */
export function sectionTabs(section: NavSection, role?: string): NavItem[] {
  return section.tabs.filter((t) => meetsMinimumRole(role, t.minimumRole));
}
