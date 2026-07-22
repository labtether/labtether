import {
  Bell,
  Boxes,
  Clock,
  FileText,
  FolderOpen,
  GitBranch,
  Globe,
  LayoutDashboard,
  Lock,
  MapPin,
  Monitor,
  ScrollText,
  Server,
  Settings,
  Shield,
  TerminalSquare,
  UserCog,
  Webhook,
  Zap,
  type LucideIcon,
} from "lucide-react";

import type { MinimumRole } from "./roles";

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

export const navGroups: NavGroup[] = [
  {
    category: "Overview",
    categoryKey: "categories.overview",
    items: [
      { href: "/", label: "Dashboard", translationKey: "dashboard", icon: LayoutDashboard },
      { href: "/nodes", label: "Devices", translationKey: "devices", icon: Server },
      { href: "/topology", label: "Topology", translationKey: "topology", icon: GitBranch },
      { href: "/services", label: "Services", translationKey: "services", icon: Globe },
      { href: "/containers", label: "Containers", translationKey: "containers", icon: Boxes },
      { href: "/terminal", label: "Terminal", translationKey: "terminal", icon: TerminalSquare, minimumRole: "write" },
    ],
  },
  {
    category: "Operations",
    categoryKey: "categories.operations",
    items: [
      { href: "/files", label: "Files", translationKey: "files", icon: FolderOpen },
      { href: "/remote-view", label: "Remote View", translationKey: "remoteView", icon: Monitor, minimumRole: "write" },
      { href: "/logs", label: "Logs", translationKey: "logs", icon: FileText },
      { href: "/alerts", label: "Alerts", translationKey: "alerts", icon: Bell },
      { href: "/actions", label: "Actions", translationKey: "actions", icon: Zap },
      { href: "/webhooks", label: "Webhooks", translationKey: "webhooks", icon: Webhook },
      { href: "/schedules", label: "Schedules", translationKey: "schedules", icon: Clock },
    ],
  },
  {
    category: "System",
    categoryKey: "categories.system",
    items: [
      { href: "/groups", label: "Groups", translationKey: "groups", icon: MapPin },
      { href: "/reliability", label: "Health", translationKey: "health", icon: Shield },
      { href: "/users", label: "Users", translationKey: "users", icon: UserCog },
      { href: "/security", label: "Security", translationKey: "security", icon: Lock, minimumRole: "admin" },
      { href: "/audit-log", label: "Audit Log", translationKey: "auditLog", icon: ScrollText },
      { href: "/settings", label: "Settings", translationKey: "settings", icon: Settings, minimumRole: "admin" },
    ],
  },
];
