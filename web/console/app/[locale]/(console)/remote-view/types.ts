// web/console/app/[locale]/(console)/remote-view/types.ts

export type RemoteViewTabType = "new" | "device" | "bookmark" | "adhoc";
export type RemoteViewProtocol = "vnc" | "rdp" | "spice" | "ard";
export type RemoteViewConnectionState =
  | "idle"
  | "connecting"
  | "authenticating"
  | "connected"
  | "disconnected"
  | "error";

export interface RemoteViewTab {
  id: string;
  type: RemoteViewTabType;
  label: string;
  protocol?: RemoteViewProtocol;
  target?: {
    host: string;
    port: number;
    assetId?: string;
    bookmarkId?: string;
  };
  connectionState: RemoteViewConnectionState;
  lastConnectedAt?: number;
}

/** Maps RemoteViewProtocol to the DesktopProtocol used by viewer components. ARD uses VNC transport. */
export function toDesktopProtocol(protocol: RemoteViewProtocol): "vnc" | "rdp" | "spice" {
  switch (protocol) {
    case "ard":
      return "vnc";
    case "vnc":
      return "vnc";
    case "rdp":
      return "rdp";
    case "spice":
      return "spice";
  }
}

/** Default port for each protocol. */
export function defaultPort(protocol: RemoteViewProtocol): number {
  switch (protocol) {
    case "vnc":
    case "ard":
      return 5900;
    case "rdp":
      return 3389;
    case "spice":
      return 5930;
  }
}

/** Protocol dot colors for the tab bar (Tailwind classes). */
export const PROTOCOL_DOT_COLOR: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500",
  rdp: "bg-blue-500",
  spice: "bg-amber-500",
  ard: "bg-purple-500",
};

/** Protocol badge styles for device/bookmark rows. */
export const PROTOCOL_BADGE_STYLE: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500/8 text-green-500/70 border-green-500/15",
  rdp: "bg-blue-500/8 text-blue-500/70 border-blue-500/15",
  spice: "bg-amber-500/8 text-amber-500/70 border-amber-500/15",
  ard: "bg-purple-500/8 text-purple-500/70 border-purple-500/15",
};

/** Protocol icon tint backgrounds. */
export const PROTOCOL_ICON_BG: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500/10",
  rdp: "bg-blue-500/10",
  spice: "bg-amber-500/10",
  ard: "bg-purple-500/10",
};

/** Protocol icon stroke colors. */
export const PROTOCOL_ICON_COLOR: Record<RemoteViewProtocol, string> = {
  vnc: "text-green-500/60",
  rdp: "text-blue-500/60",
  spice: "text-amber-500/60",
  ard: "text-purple-500/60",
};

/** Protocol selector button styles (selected vs unselected). */
export const PROTOCOL_SELECTOR_STYLE: Record<RemoteViewProtocol, { selected: string; unselected: string }> = {
  vnc: {
    selected: "bg-green-500/10 border-green-500/35 shadow-[0_0_16px_rgba(34,197,94,0.06)]",
    unselected: "bg-green-500/4 border-green-500/10",
  },
  rdp: {
    selected: "bg-blue-500/10 border-blue-500/35 shadow-[0_0_16px_rgba(59,130,246,0.06)]",
    unselected: "bg-blue-500/4 border-blue-500/10",
  },
  spice: {
    selected: "bg-amber-500/10 border-amber-500/35 shadow-[0_0_16px_rgba(245,158,11,0.06)]",
    unselected: "bg-amber-500/4 border-amber-500/10",
  },
  ard: {
    selected: "bg-purple-500/10 border-purple-500/35 shadow-[0_0_16px_rgba(168,85,247,0.06)]",
    unselected: "bg-purple-500/4 border-purple-500/10",
  },
};

/** Protocol name text colors for selector buttons. */
export const PROTOCOL_NAME_COLOR: Record<RemoteViewProtocol, { selected: string; unselected: string }> = {
  vnc: { selected: "text-green-500/95", unselected: "text-green-500/45" },
  rdp: { selected: "text-blue-500/95", unselected: "text-blue-500/45" },
  spice: { selected: "text-amber-500/95", unselected: "text-amber-500/45" },
  ard: { selected: "text-purple-500/95", unselected: "text-purple-500/45" },
};

/** All supported protocols with display metadata. */
export const PROTOCOLS: { value: RemoteViewProtocol; label: string }[] = [
  { value: "vnc", label: "VNC" },
  { value: "rdp", label: "RDP" },
  { value: "spice", label: "SPICE" },
  { value: "ard", label: "ARD" },
];
