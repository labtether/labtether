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
    allowInsecureVNC?: boolean;
    ignoreCertificate?: boolean;
    allowLegacySecurity?: boolean;
    certificateFingerprints?: string;
    spiceSecurityMode?: "tls" | "cleartext";
    spiceCAPEM?: string;
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

/** All supported protocols with display metadata. */
export const PROTOCOLS: { value: RemoteViewProtocol; label: string }[] = [
  { value: "vnc", label: "VNC" },
  { value: "rdp", label: "RDP" },
  { value: "spice", label: "SPICE" },
  { value: "ard", label: "ARD" },
];
