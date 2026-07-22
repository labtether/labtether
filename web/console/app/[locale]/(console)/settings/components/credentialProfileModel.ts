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

export function parseCredentialMetadata(raw: string): Record<string, string> {
  const trimmed = raw.trim();
  if (!trimmed) return {};
  const parsed: unknown = JSON.parse(trimmed);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Metadata must be a JSON object.");
  }
  const entries = Object.entries(parsed as Record<string, unknown>);
  if (entries.length > 32) throw new Error("Metadata can contain at most 32 entries.");
  const metadata: Record<string, string> = {};
  for (const [key, value] of entries) {
    if (typeof value !== "string") throw new Error(`Metadata value for “${key}” must be a string.`);
    metadata[key] = value;
  }
  return metadata;
}

export function buildCredentialCreatePayload(draft: CredentialCreateDraft): CredentialCreatePayload {
  const name = draft.name.trim();
  if (!name) throw new Error("Name is required.");
  if (draft.secret.length === 0) throw new Error("Secret is required.");
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

const referenceLabels: Record<string, string> = {
  asset_terminal_configs: "asset terminal configurations",
  asset_desktop_configs: "asset desktop configurations",
  asset_protocol_configs: "asset protocol configurations",
  terminal_session_bookmarks: "terminal bookmarks",
  group_jump_chains: "group jump chains",
  hub_collectors: "collectors",
  remote_bookmarks: "remote bookmarks",
  file_connections: "file connections",
};

export function credentialReferenceMessage(references: CredentialReference[]): string {
  if (references.length === 0) return "This credential profile is still in use.";
  const details = references.map((reference) => {
    const label = referenceLabels[reference.resource] ?? reference.resource;
    return `${reference.count} ${label}`;
  });
  return `Remove its live dependencies first: ${details.join(", ")}.`;
}
