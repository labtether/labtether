import type { ReactNode } from "react";

/** The words the cluster graph draws on its tiles and edges, already in the page's language. */
export type ClusterFlowWords = {
  local: string;
  guests: (count: number) => string;
  linkedTo: (name: string) => string;
  links: (count: number) => string;
  auto: string;
  manual: string;
  missing: string;
  missingTarget: string;
  autoLink: (guest: string, host: string) => string;
  manualLink: (guest: string, host: string) => string;
  missingLink: (target: string) => string;
};

/** A small neutral label on a graph edge; the tooltip says what the link is. */
export function flowEdgeBadge(text: string, tooltip: string): ReactNode {
  return (
    <span title={tooltip} className="di-ct-edge">
      {text}
    </span>
  );
}

const SOURCE_NAMES: Record<string, string> = {
  truenas: "TrueNAS", docker: "Docker", portainer: "Portainer", pbs: "PBS", proxmox: "Proxmox",
  homeassistant: "Home Assistant", agent: "Agent",
};

/** The integration a host comes from, as its product name ("TrueNAS", "Agent"). */
export function sourceBadgeLabel(source?: string): string {
  const normalized = (source ?? "").trim().toLowerCase();
  if (!normalized) return "External";
  return SOURCE_NAMES[normalized] ?? normalized.charAt(0).toUpperCase() + normalized.slice(1);
}

/** A HA state as a status tone: errors are problems, moves are warnings, stopped is off, started shows nothing. */
export function haStateTone(state?: string): "bad" | "warn" | "off" | "" {
  const s = (state ?? "").toLowerCase();
  if (s === "error" || s === "fence" || s === "recovery") return "bad";
  if (s === "migrate" || s === "relocate" || s === "freeze") return "warn";
  if (s === "stopped" || s === "disabled") return "off";
  return "";
}

/** A guest status as a status tone: failed is a problem, stopped is off, running shows nothing. */
export function guestStatusTone(status?: string): "bad" | "off" | "" {
  const s = (status ?? "").toLowerCase();
  if (s === "error" || s === "failed") return "bad";
  if (s === "stopped" || s === "paused" || s === "offline") return "off";
  return "";
}
