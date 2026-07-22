"use client";

import { useState } from "react";
import { KeyRound, Plus, RotateCcw, ShieldCheck, Trash2 } from "lucide-react";

import { credentialKindLabels } from "../../../../components/credentials/CredentialProfileSelect";
import { Badge } from "../../../../components/ui/Badge";
import { Button } from "../../../../components/ui/Button";
import { Card } from "../../../../components/ui/Card";
import { Input, Select } from "../../../../components/ui/Input";
import { Modal } from "../../../../components/ui/Modal";
import {
  credentialKinds,
  type CredentialKind,
  type CredentialProfile,
  useCredentialProfiles,
} from "../../../../hooks/useCredentialProfiles";
import {
  buildCredentialCreatePayload,
  credentialReferenceMessage,
  type CredentialCreateDraft,
  type CredentialReference,
} from "./credentialProfileModel";

type ErrorPayload = {
  error?: unknown;
  references?: unknown;
};

class CredentialRequestError extends Error {
  references: CredentialReference[];

  constructor(message: string, references: CredentialReference[] = []) {
    super(message);
    this.name = "CredentialRequestError";
    this.references = references;
  }
}

function parseReferences(value: unknown): CredentialReference[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((entry) => {
    if (!entry || typeof entry !== "object") return [];
    const record = entry as Record<string, unknown>;
    if (typeof record.resource !== "string" || typeof record.count !== "number") return [];
    return [{ resource: record.resource, count: record.count }];
  });
}

async function credentialRequest(path: string, init: RequestInit): Promise<unknown> {
  const response = await fetch(`/api/settings/credentials${path}`, init);
  let payload: ErrorPayload | null = null;
  try {
    payload = await response.json() as ErrorPayload;
  } catch {
    payload = null;
  }
  if (!response.ok) {
    const message = typeof payload?.error === "string" && payload.error.trim()
      ? payload.error.trim()
      : `Credential request failed (${response.status}).`;
    throw new CredentialRequestError(message, parseReferences(payload?.references));
  }
  return payload;
}

function profileKind(profile: CredentialProfile): string {
  return credentialKindLabels[profile.kind] ?? profile.kind;
}

function profileTimestamp(profile: CredentialProfile): string {
  const value = profile.rotated_at ?? profile.updated_at;
  if (!value) return "Unknown";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "Unknown" : date.toLocaleString();
}

const initialCreateDraft: CredentialCreateDraft = {
  name: "",
  kind: "ssh_password",
  username: "",
  description: "",
  secret: "",
  passphrase: "",
  metadataText: "{}",
};

type CreateDialogProps = {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
};

function CreateCredentialDialog({ open, onClose, onCreated }: CreateDialogProps) {
  const [draft, setDraft] = useState<CredentialCreateDraft>(initialCreateDraft);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const update = <K extends keyof CredentialCreateDraft>(key: K, value: CredentialCreateDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError(null);
    let payload;
    try {
      payload = buildCredentialCreatePayload(draft);
    } catch (validationError) {
      setError(validationError instanceof Error ? validationError.message : "Credential profile is invalid.");
      return;
    }
    setSaving(true);
    try {
      await credentialRequest("", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      setDraft(initialCreateDraft);
      onCreated();
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to create credential profile.");
    } finally {
      setSaving(false);
    }
  };

  const privateKey = draft.kind === "ssh_private_key";
  return (
    <Modal open={open} onClose={onClose} title="Create credential profile" className="max-w-xl">
      <form onSubmit={(event) => void submit(event)} className="max-h-[calc(100vh-7rem)] space-y-4 overflow-y-auto px-5 py-4">
        <p className="text-xs text-[var(--muted)]">Secrets are encrypted by the hub and never returned by the inventory API.</p>
        {error ? <p role="alert" className="text-xs text-[var(--bad)]">{error}</p> : null}
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Name</span>
          <Input value={draft.name} onChange={(event) => update("name", event.target.value)} maxLength={120} required />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Credential type</span>
          <Select value={draft.kind} onChange={(event) => update("kind", event.target.value as CredentialKind)} className="w-full">
            {credentialKinds.map((kind) => <option key={kind} value={kind}>{credentialKindLabels[kind] ?? kind}</option>)}
          </Select>
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Username (optional)</span>
          <Input value={draft.username} onChange={(event) => update("username", event.target.value)} maxLength={64} autoComplete="off" />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Description (optional)</span>
          <Input value={draft.description} onChange={(event) => update("description", event.target.value)} maxLength={4096} />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">{privateKey ? "Private key" : "Secret"}</span>
          {privateKey ? (
            <textarea
              value={draft.secret}
              onChange={(event) => update("secret", event.target.value)}
              maxLength={16384}
              required
              spellCheck={false}
              autoComplete="new-password"
              className="min-h-32 w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 font-mono text-xs text-[var(--text)] outline-none focus:border-[var(--accent)]"
            />
          ) : (
            <Input type="password" value={draft.secret} onChange={(event) => update("secret", event.target.value)} maxLength={16384} required autoComplete="new-password" />
          )}
          <span className="mt-1 block text-[10px]">Leading and trailing bytes are preserved exactly.</span>
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Passphrase (optional)</span>
          <Input type="password" value={draft.passphrase} onChange={(event) => update("passphrase", event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Metadata JSON (optional string values only)</span>
          <textarea
            value={draft.metadataText}
            onChange={(event) => update("metadataText", event.target.value)}
            spellCheck={false}
            className="min-h-20 w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 font-mono text-xs text-[var(--text)] outline-none focus:border-[var(--accent)]"
          />
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button type="submit" variant="primary" loading={saving}>Create profile</Button>
        </div>
      </form>
    </Modal>
  );
}

type RotateDialogProps = {
  profile: CredentialProfile | null;
  onClose: () => void;
  onRotated: () => void;
};

function RotateCredentialDialog({ profile, onClose, onRotated }: RotateDialogProps) {
  const [secret, setSecret] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [reason, setReason] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!profile) return null;
  const privateKey = profile.kind === "ssh_private_key";
  const canRotate = secret.length > 0 && reason.trim().length > 0 && confirmation === "ROTATE";

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!canRotate) return;
    setError(null);
    setSaving(true);
    try {
      await credentialRequest(`/${encodeURIComponent(profile.id)}/rotate`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          secret,
          ...(passphrase.length > 0 ? { passphrase } : {}),
          reason: reason.trim(),
        }),
      });
      setSecret("");
      setPassphrase("");
      setReason("");
      setConfirmation("");
      onRotated();
      onClose();
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : "Failed to rotate credential profile.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={`Rotate ${profile.name}`} className="max-w-lg">
      <form onSubmit={(event) => void submit(event)} className="max-h-[calc(100vh-7rem)] space-y-4 overflow-y-auto px-5 py-4">
        <p className="text-xs text-[var(--warn)]">Every live dependency will use the replacement immediately. The current secret cannot be recovered here.</p>
        {error ? <p role="alert" className="text-xs text-[var(--bad)]">{error}</p> : null}
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">{privateKey ? "Replacement private key" : "Replacement secret"}</span>
          {privateKey ? (
            <textarea value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required spellCheck={false} autoComplete="new-password" className="min-h-32 w-full rounded-lg border border-[var(--line)] bg-transparent px-3 py-2 font-mono text-xs text-[var(--text)] outline-none focus:border-[var(--accent)]" />
          ) : (
            <Input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required autoComplete="new-password" />
          )}
          <span className="mt-1 block text-[10px]">Leading and trailing bytes are preserved exactly.</span>
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Replacement passphrase (optional)</span>
          <Input type="password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Rotation reason</span>
          <Input value={reason} onChange={(event) => setReason(event.target.value)} maxLength={512} required />
        </label>
        <label className="block text-xs text-[var(--muted)]">
          <span className="mb-1 block font-medium">Type ROTATE to confirm</span>
          <Input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
        </label>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button type="submit" variant="danger" loading={saving} disabled={!canRotate}>Rotate profile</Button>
        </div>
      </form>
    </Modal>
  );
}

type DeleteDialogProps = {
  profile: CredentialProfile | null;
  onClose: () => void;
  onDeleted: () => void;
};

function DeleteCredentialDialog({ profile, onClose, onDeleted }: DeleteDialogProps) {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!profile) return null;

  const remove = async () => {
    setDeleting(true);
    setError(null);
    try {
      await credentialRequest(`/${encodeURIComponent(profile.id)}`, { method: "DELETE" });
      onDeleted();
      onClose();
    } catch (requestError) {
      if (requestError instanceof CredentialRequestError && requestError.references.length > 0) {
        setError(credentialReferenceMessage(requestError.references));
      } else {
        setError(requestError instanceof Error ? requestError.message : "Failed to delete credential profile.");
      }
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={`Delete ${profile.name}`} className="max-w-md">
      <div className="space-y-4 px-5 py-4">
        <p className="text-sm text-[var(--text-secondary)]">Deletion succeeds only when no asset, bookmark, jump chain, collector, or file connection still references this profile.</p>
        {error ? <p role="alert" className="text-xs text-[var(--bad)]">{error}</p> : null}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={deleting}>Cancel</Button>
          <Button variant="danger" onClick={() => void remove()} loading={deleting}>Delete profile</Button>
        </div>
      </div>
    </Modal>
  );
}

export function CredentialProfilesCard() {
  const inventory = useCredentialProfiles();
  const [createOpen, setCreateOpen] = useState(false);
  const [rotateProfile, setRotateProfile] = useState<CredentialProfile | null>(null);
  const [deleteProfile, setDeleteProfile] = useState<CredentialProfile | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  const mutationComplete = (nextMessage: string) => {
    setMessage(nextMessage);
    inventory.refresh();
  };

  return (
    <Card className="mb-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <KeyRound size={16} aria-hidden="true" />
            <h2 className="text-sm font-semibold">Credential profiles</h2>
          </div>
          <p className="mt-1 text-xs text-[var(--muted)]">Encrypted, reusable credentials for protocols and infrastructure providers. Inventory responses contain metadata only.</p>
        </div>
        <Button variant="primary" size="sm" onClick={() => { setMessage(null); setCreateOpen(true); }}>
          <Plus size={13} aria-hidden="true" /> Create profile
        </Button>
      </div>

      <div aria-live="polite" className="mt-3">
        {message ? <p className="text-xs text-[var(--ok)]">{message}</p> : null}
        {inventory.error ? (
          <div role="alert" className="flex flex-wrap items-center gap-2 text-xs text-[var(--bad)]">
            <span>{inventory.error}</span>
            <Button size="sm" variant="ghost" onClick={inventory.refresh}>Retry</Button>
          </div>
        ) : null}
      </div>

      {inventory.loading ? <p className="mt-4 text-xs text-[var(--muted)]">Loading credential profiles…</p> : null}
      {!inventory.loading && !inventory.error && inventory.profiles.length === 0 ? (
        <p className="mt-4 rounded-lg border border-dashed border-[var(--line)] p-4 text-center text-xs text-[var(--muted)]">No reusable credential profiles have been created.</p>
      ) : null}
      {inventory.profiles.length > 0 ? (
        <ul className="mt-4 divide-y divide-[var(--line)]" aria-label="Credential profiles">
          {inventory.profiles.map((profile) => {
            const protectedIdentity = profile.kind === "hub_ssh_identity";
            return (
              <li key={profile.id} className="flex flex-wrap items-center justify-between gap-3 py-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-[var(--text)]">{profile.name}</span>
                    <Badge status={profile.status === "disabled" ? "disabled" : "online"} size="sm" />
                    {protectedIdentity ? <span className="inline-flex items-center gap-1 text-[10px] text-[var(--muted)]"><ShieldCheck size={11} /> Protected</span> : null}
                  </div>
                  <p className="mt-1 text-[11px] text-[var(--muted)]">{profileKind(profile)}{profile.username ? ` · ${profile.username}` : ""} · Last rotated {profileTimestamp(profile)}</p>
                  {profile.description ? <p className="mt-1 text-[11px] text-[var(--text-secondary)]">{profile.description}</p> : null}
                </div>
                <div className="flex items-center gap-1">
                  {!protectedIdentity ? (
                    <Button size="sm" variant="ghost" onClick={() => { setMessage(null); setRotateProfile(profile); }}>
                      <RotateCcw size={12} aria-hidden="true" /> Rotate
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="danger"
                    disabled={protectedIdentity}
                    title={protectedIdentity ? "The hub SSH identity cannot be deleted here" : `Delete ${profile.name}`}
                    onClick={() => { setMessage(null); setDeleteProfile(profile); }}
                  >
                    <Trash2 size={12} aria-hidden="true" /> Delete
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}

      <CreateCredentialDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={() => mutationComplete("Credential profile created.")}
      />
      <RotateCredentialDialog
        key={`rotate-${rotateProfile?.id ?? "none"}`}
        profile={rotateProfile}
        onClose={() => setRotateProfile(null)}
        onRotated={() => mutationComplete("Credential profile rotated.")}
      />
      <DeleteCredentialDialog
        key={`delete-${deleteProfile?.id ?? "none"}`}
        profile={deleteProfile}
        onClose={() => setDeleteProfile(null)}
        onDeleted={() => mutationComplete("Credential profile deleted.")}
      />
    </Card>
  );
}
