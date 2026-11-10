import type { CredentialKind } from "../../../../hooks/useCredentialProfiles";

export type CredentialCreateDraft = {
  name: string;
  kind: CredentialKind;
  username: string;
  description: string;
  secret: string;
  passphrase: string;
  metadataText: string;
};

export type CredentialCreatePayload = {
  name: string;
  kind: CredentialKind;
  username?: string;
  description?: string;
  secret: string;
  passphrase?: string;
  metadata?: Record<string, string>;
};

/** Message keys under security.logins.errors for what the Add login form rejects before it reaches the hub. */
export type CredentialProblem =
  | "nameRequired"
  | "secretRequired"
  | "metadataInvalid"
  | "metadataObject"
  | "metadataTooMany"
  | "metadataText";

export class CredentialValidationError extends Error {
  readonly problem: CredentialProblem;
  readonly values: Record<string, string | number>;

  constructor(problem: CredentialProblem, values: Record<string, string | number> = {}) {
    super(problem);
    this.name = "CredentialValidationError";
    this.problem = problem;
    this.values = values;
  }
}

const maxMetadataEntries = 32;

export function parseCredentialMetadata(raw: string): Record<string, string> {
  const trimmed = raw.trim();
  if (!trimmed) return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    throw new CredentialValidationError("metadataInvalid");
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new CredentialValidationError("metadataObject");
  }
  const entries = Object.entries(parsed as Record<string, unknown>);
  if (entries.length > maxMetadataEntries) throw new CredentialValidationError("metadataTooMany", { max: maxMetadataEntries });
  const metadata: Record<string, string> = {};
  for (const [key, value] of entries) {
    if (typeof value !== "string") throw new CredentialValidationError("metadataText", { key });
    metadata[key] = value;
  }
  return metadata;
}

export function buildCredentialCreatePayload(draft: CredentialCreateDraft): CredentialCreatePayload {
  const name = draft.name.trim();
  if (!name) throw new CredentialValidationError("nameRequired");
  if (draft.secret.length === 0) throw new CredentialValidationError("secretRequired");
  const metadata = parseCredentialMetadata(draft.metadataText);
  return {
    name,
    kind: draft.kind,
    ...(draft.username.trim() ? { username: draft.username.trim() } : {}),
    ...(draft.description.trim() ? { description: draft.description.trim() } : {}),
    secret: draft.secret,
    ...(draft.passphrase.length > 0 ? { passphrase: draft.passphrase } : {}),
    ...(Object.keys(metadata).length > 0 ? { metadata } : {}),
  };
}

export type CredentialReference = { resource: string; count: number };

/** Message keys under security.logins.uses, one per kind of thing that can still hold a login. */
export type CredentialUse =
  | "terminalConfigs"
  | "desktopConfigs"
  | "protocolConfigs"
  | "terminalBookmarks"
  | "jumpChains"
  | "collectors"
  | "remoteBookmarks"
  | "fileConnections"
  | "other";

const referenceUses: Record<string, CredentialUse> = {
  asset_terminal_configs: "terminalConfigs",
  asset_desktop_configs: "desktopConfigs",
  asset_protocol_configs: "protocolConfigs",
  terminal_session_bookmarks: "terminalBookmarks",
  group_jump_chains: "jumpChains",
  hub_collectors: "collectors",
  remote_bookmarks: "remoteBookmarks",
  file_connections: "fileConnections",
};

/** The hub's reference counts as message keys; resources this console does not know are counted together as "other". */
export function credentialReferenceUses(references: CredentialReference[]): { use: CredentialUse; count: number }[] {
  const uses: { use: CredentialUse; count: number }[] = [];
  let other = 0;
  for (const reference of references) {
    const use = referenceUses[reference.resource];
    if (use) uses.push({ use, count: reference.count });
    else other += reference.count;
  }
  if (other > 0) uses.push({ use: "other", count: other });
  return uses;
}

export type CredentialAction = "add" | "rotate" | "delete";

/** Message keys under security.logins.errors for hub refusals the console can name itself. */
export type CredentialFailure = "forbidden" | "notFound" | "limit" | "protected" | "addFailed" | "rotateFailed" | "deleteFailed";

/**
 * Picks the words for a failed hub request. A 400 keeps the hub's own reason (null here), so the person sees what to
 * fix; other refusals get the console's words, and anything else the action's "could not" line.
 */
export function credentialFailure(action: CredentialAction, status: number, hubMessage: string): CredentialFailure | null {
  if (status === 400 && hubMessage) return null;
  if (status === 403) return "forbidden";
  if (status === 404) return "notFound";
  if (status === 409 && action === "add") return "limit";
  if (status === 409 && action === "delete") return "protected";
  return `${action}Failed`;
}
