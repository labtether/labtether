// Quick connect's form state and the rules that turn it into a connection: protocol defaults, pasted addresses,
// which security choices apply to which protocol, and which choices may be saved in a bookmark.
import { parsePortInput } from "../../../lib/portParsing";
import { defaultPort, type RemoteViewProtocol } from "./types";

export interface AdhocConnectParams {
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
  username?: string;
  password?: string;
  allowInsecureVNC?: boolean;
  ignoreCertificate?: boolean;
  allowLegacySecurity?: boolean;
  certificateFingerprints?: string;
  spiceSecurityMode?: "tls" | "cleartext";
  spiceCAPEM?: string;
  saveBookmark?: { label: string };
}

export interface QuickConnectDraft {
  protocol: RemoteViewProtocol;
  host: string;
  port: string;
  saveBookmark: boolean;
  bookmarkName: string;
  username: string;
  password: string;
  allowInsecureVNC: boolean;
  ignoreCertificate: boolean;
  allowLegacySecurity: boolean;
  certificateFingerprints: string;
  spiceSecurityMode: "tls" | "cleartext";
  spiceCAPEM: string;
}

export function emptyDraft(host = ""): QuickConnectDraft {
  return {
    protocol: "vnc",
    host,
    port: String(defaultPort("vnc")),
    saveBookmark: false,
    bookmarkName: "",
    username: "",
    password: "",
    allowInsecureVNC: false,
    ignoreCertificate: false,
    allowLegacySecurity: false,
    certificateFingerprints: "",
    spiceSecurityMode: "tls",
    spiceCAPEM: "",
  };
}

const URI = /^(vnc|rdp|spice|ard):\/\/([^:/]+)(?::(\d+))?$/i;

/** "rdp://desk.example.com:3390" → its protocol, host and port (the protocol's port when none is given). */
export function parseURI(input: string): { protocol: RemoteViewProtocol; host: string; port: number } | null {
  const match = input.trim().match(URI);
  if (!match) return null;
  const protocol = match[1].toLowerCase() as RemoteViewProtocol;
  return { protocol, host: match[2], port: match[3] ? parsePortInput(match[3], defaultPort(protocol)) : defaultPort(protocol) };
}

/** RDP choices that weaken security (an untrusted or pinned certificate, old RDP security) are for one session only. */
export function canSaveQuickConnectBookmark(
  protocol: RemoteViewProtocol,
  ignoreCertificate: boolean,
  allowLegacySecurity: boolean,
  certificateFingerprints: string,
): boolean {
  return protocol !== "rdp" || (!ignoreCertificate && !allowLegacySecurity && certificateFingerprints.trim() === "");
}

export const canSaveDraft = (d: QuickConnectDraft) =>
  canSaveQuickConnectBookmark(d.protocol, d.ignoreCertificate, d.allowLegacySecurity, d.certificateFingerprints);

/** A plain VNC or ARD connection cannot start until the person allows plain VNC. */
export const connectBlocked = (d: QuickConnectDraft) => (d.protocol === "vnc" || d.protocol === "ard") && !d.allowInsecureVNC;

/** Switching protocol drops the other protocols' security choices, and moves the port along when it was the default. */
export function withProtocol(d: QuickConnectDraft, protocol: RemoteViewProtocol, port?: number): QuickConnectDraft {
  const portWasDefault = d.port === "" || d.port === String(defaultPort(d.protocol));
  return {
    ...d,
    protocol,
    port: port != null ? String(port) : portWasDefault ? String(defaultPort(protocol)) : d.port,
    ...(protocol === "rdp" ? {} : { ignoreCertificate: false, allowLegacySecurity: false, certificateFingerprints: "" }),
    ...(protocol === "vnc" || protocol === "ard" ? {} : { allowInsecureVNC: false }),
    ...(protocol === "spice" ? {} : { spiceSecurityMode: "tls" as const, spiceCAPEM: "" }),
  };
}

/** Typing or pasting into Host: an address like vnc://host:port fills protocol, host and port at once. */
export function withHostInput(d: QuickConnectDraft, value: string): QuickConnectDraft {
  const parsed = parseURI(value);
  if (!parsed) return { ...d, host: value };
  return { ...withProtocol(d, parsed.protocol, parsed.port), host: parsed.host };
}

/** The form opened from a search: an address, host:port or a plain host fills the first step. */
export function draftFor(input: string): QuickConnectDraft {
  const value = input.trim();
  if (parseURI(value)) return withHostInput(emptyDraft(), value);
  const hostPort = /^([^:[\]\s]+):(\d{1,5})$/.exec(value);
  return hostPort ? { ...emptyDraft(hostPort[1]), port: hostPort[2] } : emptyDraft(value);
}

/** The connection to open. withLogin false connects without the typed username and password. */
export function toConnectParams(d: QuickConnectDraft, withLogin: boolean): AdhocConnectParams {
  const host = d.host.trim();
  const plainVnc = d.protocol === "vnc" || d.protocol === "ard";
  return {
    protocol: d.protocol,
    host,
    port: parsePortInput(d.port, defaultPort(d.protocol)),
    ...(withLogin && d.username.trim() ? { username: d.username.trim() } : {}),
    ...(withLogin && d.password ? { password: d.password } : {}),
    ...(plainVnc && d.allowInsecureVNC ? { allowInsecureVNC: true } : {}),
    ...(d.protocol === "rdp" && d.ignoreCertificate ? { ignoreCertificate: true } : {}),
    ...(d.protocol === "rdp" && d.allowLegacySecurity ? { allowLegacySecurity: true } : {}),
    ...(d.protocol === "rdp" && d.certificateFingerprints.trim() ? { certificateFingerprints: d.certificateFingerprints.trim() } : {}),
    ...(d.protocol === "spice"
      ? { spiceSecurityMode: d.spiceSecurityMode, ...(d.spiceSecurityMode === "tls" && d.spiceCAPEM.trim() ? { spiceCAPEM: d.spiceCAPEM.trim() } : {}) }
      : {}),
    ...(d.saveBookmark && canSaveDraft(d) ? { saveBookmark: { label: d.bookmarkName.trim() || host } } : {}),
  };
}
