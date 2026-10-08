export type SessionType = "terminal" | "desktop";
export type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

export interface SpiceTicket {
  wsUrl: string;
  password: string;
  type?: string;
  ca?: string;
  proxy?: string;
}

export type SessionConnectionState =
  | "idle"
  | "connecting"
  | "authenticating" // desktop only
  | "connected"
  | "error";

export type SessionConnectionPhase =
  | "idle"
  | "creating-session"
  | "requesting-ticket"
  | "opening-stream"
  | "starting-shell"
  | "reconnecting"
  | "connected"
  | "error";

export interface SessionConnectionProgress {
  phase: SessionConnectionPhase;
  message: string;
  phaseElapsedMs: number;
  totalElapsedMs: number;
}

export interface SessionStreamStatus {
  type?: string;
  stage?: string;
  message?: string;
  attempt?: number;
  attempts?: number;
  elapsed_ms?: number;
  hop_index?: number;
  hop_count?: number;
  hop_host?: string;
}

export interface QuickConnectParams {
  host: string;
  port?: number;
  username: string;
  auth_method: "password" | "private_key";
  password?: string;
  private_key?: string;
  passphrase?: string;
  strict_host_key?: boolean;
}

export interface UseSessionOptions {
  type: SessionType;
  /** When set, skips the device picker and always targets this asset. */
  fixedTarget?: string;
  /** Enables auto-reconnect for terminal sessions on abnormal disconnects. */
  autoReconnect?: boolean;
  /** When set, creates an ephemeral quick-connect session instead of an asset-based one. */
  quickConnectParams?: QuickConnectParams;
}

export interface TerminalConnectOptions {
  terminalShell?: string;
  protocol?: DesktopProtocol;
  display?: string;
  record?: boolean;
  directTarget?: {
    host: string;
    port: number;
    username?: string;
    password?: string;
    allow_insecure_vnc?: boolean;
    ignore_certificate?: boolean;
    allow_legacy_security?: boolean;
    certificate_fingerprints?: string;
    spice_security_mode?: "tls" | "cleartext";
    spice_ca_pem?: string;
  };
}
