// What a device's agent offers, for every place in the console that shows a tab, a button or a menu item for it.
//
// Most agents (every Go agent released so far) send no signal: they keep today's rules, where a connected agent
// offers everything and a missing or empty cap_* value counts as supported. An agent that says its advertisement is
// complete (agent_capability_mode=explicit) is offered only what the Hub lists in agentAdvertisements[id].offered;
// the Hub refuses the rest with 422 agent_unsupported. The Hub owns that rule (internal/agentcaps); the console
// reads its answer and never works it out again from metadata.

/** One device's entry in the status aggregate's agentAdvertisements map. */
export type AgentAdvertisement = {
  explicit: boolean;
  /** "monitoring" for the monitoring edition, else "". */
  edition: "monitoring" | "";
  /** Hub request types the Hub sends this agent ("disk.list", "update.request:self", …). */
  offered: readonly string[];
};

export type AgentAdvertisements = Readonly<Record<string, AgentAdvertisement>>;

/** A thing the console can show for a device's agent. */
export type AgentFeature =
  | "terminal" | "files" | "processes" | "disks" | "users" | "agentSettings" | "power"
  | "network" | "networkActions" | "services" | "serviceActions" | "packages" | "packageActions"
  | "schedules" | "logQuery" | "sshKeys" | "vnc" | "webrtc" | "remoteView";

/** The Hub request type each feature needs (protocol message names, as in internal/agentcaps messageNeeds). */
const FEATURE_REQUEST: Record<Exclude<AgentFeature, "remoteView">, string> = {
  terminal: "terminal.start",
  files: "file.list",
  processes: "process.list",
  disks: "disk.list",
  users: "users.list",
  agentSettings: "agent.settings.apply",
  power: "power.action",
  network: "network.list",
  networkActions: "network.action",
  services: "service.list",
  serviceActions: "service.action",
  packages: "package.list",
  packageActions: "package.action",
  schedules: "cron.list",
  logQuery: "journal.query",
  sshKeys: "ssh_key.install",
  vnc: "desktop.start",
  webrtc: "webrtc.start",
};

export type DeviceAgentOffers = Record<AgentFeature, boolean> & {
  /** The agent said its advertisement is complete: what is not listed is not offered. */
  explicit: boolean;
  /** The monitoring edition, shown as "Monitoring only". */
  monitoringOnly: boolean;
};

export type DeviceAgentInput = {
  metadata: Record<string, string> | undefined;
  /** The device's agent is connected now. */
  hasAgent: boolean;
  platform: string | undefined;
  advertisement?: AgentAdvertisement | null;
};

const EMPTY: AgentAdvertisements = Object.freeze({});

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * One map entry, or null when it is malformed: not an object, explicit not a boolean, or offered present and not a
 * list of strings. A null entry is read as no advertisement, so the device keeps today's rules. offered left out
 * means none: the Hub omits an empty list.
 */
export function parseAgentAdvertisement(value: unknown): AgentAdvertisement | null {
  if (!isObject(value) || typeof value.explicit !== "boolean") return null;
  const rawOffered = value.offered ?? [];
  if (!Array.isArray(rawOffered) || rawOffered.some((entry) => typeof entry !== "string")) return null;
  const edition = typeof value.edition === "string" && value.edition.trim().toLowerCase() === "monitoring" ? "monitoring" : "";
  const offered = (rawOffered as string[]).map((entry) => entry.trim().toLowerCase()).filter((entry) => entry !== "");
  return { explicit: value.explicit, edition, offered };
}

/** The status aggregate's agentAdvertisements map; malformed entries are left out. */
export function parseAgentAdvertisements(value: unknown): AgentAdvertisements {
  if (!isObject(value)) return EMPTY;
  const out: Record<string, AgentAdvertisement> = {};
  for (const [id, entry] of Object.entries(value)) {
    const parsed = parseAgentAdvertisement(entry);
    if (parsed) out[id] = parsed;
  }
  return Object.keys(out).length ? out : EMPTY;
}

/** Today's cap_* rule: a missing or empty value counts as supported (legacyDefault), else the value must list it. */
export function supportsCapability(
  metadata: Record<string, string> | undefined,
  capabilityKey: string,
  capability: string,
  legacyDefault: boolean,
): boolean {
  const raw = metadata?.[capabilityKey];
  if (typeof raw !== "string" || raw.trim() === "") return legacyDefault;
  return raw.split(",").some((value) => value.trim().toLowerCase() === capability.toLowerCase());
}

/** Today's rules, for an agent that sent no signal. */
function todaysOffers({ metadata, hasAgent, platform }: DeviceAgentInput): Record<AgentFeature, boolean> {
  const isLinux = platform === "linux";
  const cap = (key: string, token: string, legacyDefault = true) => hasAgent && supportsCapability(metadata, key, token, legacyDefault);
  const services = cap("cap_services", "list");
  const packages = cap("cap_packages", "list");
  return {
    terminal: hasAgent,
    files: hasAgent,
    processes: hasAgent,
    disks: hasAgent,
    users: hasAgent,
    agentSettings: hasAgent,
    power: hasAgent,
    network: cap("cap_network", "list"),
    networkActions: cap("cap_network", "action", isLinux),
    services,
    serviceActions: services,
    packages,
    packageActions: packages,
    schedules: cap("cap_schedules", "list"),
    logQuery: cap("cap_logs", "query"),
    // These never depended on the agent: the device page's own rules (asset type, source) decide them.
    sshKeys: true,
    vnc: true,
    webrtc: true,
    remoteView: true,
  };
}

/**
 * What the console may show for a device's agent. For an explicit agent a feature needs today's rule and the Hub's
 * offer; Remote View also stays for Windows, whose RDP goes through the Hub's gateway and not the agent.
 */
export function deviceAgentOffers(input: DeviceAgentInput): DeviceAgentOffers {
  const today = todaysOffers(input);
  const ad = input.advertisement ?? null;
  const monitoringOnly = ad?.edition === "monitoring";
  if (!ad?.explicit) return { ...today, explicit: false, monitoringOnly };

  const offered = new Set(ad.offered);
  const result = { ...today };
  for (const feature of Object.keys(FEATURE_REQUEST) as Array<keyof typeof FEATURE_REQUEST>) {
    result[feature] = today[feature] && offered.has(FEATURE_REQUEST[feature]);
  }
  const isWindows = (input.platform ?? "").trim().toLowerCase() === "windows";
  result.remoteView = result.vnc || result.webrtc || isWindows;
  return { ...result, explicit: true, monitoringOnly };
}

/**
 * Whether an explicit agent leaves a feature out, from its advertisement alone. For pickers outside the device page
 * (Terminal, Files, Remote View), which list devices and have no device metadata to hand. False for every other agent.
 */
export function agentWithholds(advertisement: AgentAdvertisement | null | undefined, feature: AgentFeature, platform?: string): boolean {
  if (!advertisement?.explicit) return false;
  return !deviceAgentOffers({ metadata: undefined, hasAgent: true, platform, advertisement })[feature];
}

/** The Hub's fixed answer for a request the agent did not advertise (v1 error text; v2 uses the code). */
export const AGENT_UNSUPPORTED_MESSAGE = "this device's agent does not support this";
export const AGENT_UNSUPPORTED_CODE = "agent_unsupported";

/**
 * Whether an error is the Hub's "this device's agent does not support this": the v1 text, the v2 code, an Error
 * carrying either, or a parsed body ({ error } or { error, message }).
 */
export function isAgentUnsupported(value: unknown): boolean {
  if (value instanceof Error) return isAgentUnsupported(value.message);
  if (isObject(value)) return isAgentUnsupported(value.error) || isAgentUnsupported(value.message);
  if (typeof value !== "string") return false;
  const text = value.trim().toLowerCase();
  return text === AGENT_UNSUPPORTED_MESSAGE || text === AGENT_UNSUPPORTED_CODE;
}
