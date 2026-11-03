"use client";

import { useState } from "react";

import { credentialKindLabels } from "../../../../components/credentials/CredentialProfileSelect";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { Modal } from "../../../../components/ui/Modal";
import {
  credentialKinds,
  type CredentialKind,
  type CredentialProfile,
} from "../../../../hooks/useCredentialProfiles";
import {
  buildCredentialCreatePayload,
  credentialReferenceMessage,
  type CredentialCreateDraft,
  type CredentialReference,
} from "./credentialProfileModel";

/* Create, rotate and delete dialogs for saved logins (the hub's credential profiles). Secrets go to the hub once and never come back. */

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

export async function credentialRequest(path: string, init: RequestInit): Promise<unknown> {
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
      : "The hub could not change this login.";
    throw new CredentialRequestError(message, parseReferences(payload?.references));
  }
  return payload;
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

export function CreateCredentialDialog({ open, onClose, onCreated }: CreateDialogProps) {
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
      setError(requestError instanceof Error ? requestError.message : "The hub could not add this login.");
    } finally {
      setSaving(false);
    }
  };

  const privateKey = draft.kind === "ssh_private_key";
  return (
    <Modal open={open} onClose={onClose} title="Add login" className="max-w-xl">
      <form onSubmit={(event) => void submit(event)} className="st-modal-form">
        <p className="st-msg">The hub encrypts the secret and never shows it again.</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <label className="st-field">
          <span className="st-lbl">Name</span>
          <Input value={draft.name} onChange={(event) => update("name", event.target.value)} maxLength={120} required />
        </label>
        <label className="st-field">
          <span className="st-lbl">Type</span>
          <Select value={draft.kind} onChange={(event) => update("kind", event.target.value as CredentialKind)} className="w-full">
            {credentialKinds.map((kind) => <option key={kind} value={kind}>{credentialKindLabels[kind] ?? kind}</option>)}
          </Select>
        </label>
        <label className="st-field">
          <span className="st-lbl">Username (optional)</span>
          <Input value={draft.username} onChange={(event) => update("username", event.target.value)} maxLength={64} autoComplete="off" />
        </label>
        <label className="st-field">
          <span className="st-lbl">Description (optional)</span>
          <Input value={draft.description} onChange={(event) => update("description", event.target.value)} maxLength={4096} />
        </label>
        <label className="st-field">
          <span className="st-lbl">{privateKey ? "Private key" : "Secret"}</span>
          {privateKey ? (
            <textarea
              value={draft.secret}
              onChange={(event) => update("secret", event.target.value)}
              maxLength={16384}
              required
              spellCheck={false}
              autoComplete="new-password"
              className="st-ta"
            />
          ) : (
            <Input type="password" value={draft.secret} onChange={(event) => update("secret", event.target.value)} maxLength={16384} required autoComplete="new-password" />
          )}
          <span className="st-hint">Spaces at the start and end are kept.</span>
        </label>
        <label className="st-field">
          <span className="st-lbl">Passphrase (optional)</span>
          <Input type="password" value={draft.passphrase} onChange={(event) => update("passphrase", event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        {/* Metadata is for scripts and the API; it waits behind Advanced so the form asks only what a login needs. */}
        <details className="st-details" open={draft.metadataText.trim() !== "" && draft.metadataText.trim() !== "{}"}>
          <summary className="st-link">Advanced</summary>
          <label className="st-field st-fold">
            <span className="st-lbl">Metadata (JSON, text values)</span>
            <textarea
              value={draft.metadataText}
              onChange={(event) => update("metadataText", event.target.value)}
              spellCheck={false}
              className="st-ta"
            />
          </label>
        </details>
        <div className="st-dlg-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button type="submit" variant="primary" loading={saving}>Add login</Button>
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

export function RotateCredentialDialog({ profile, onClose, onRotated }: RotateDialogProps) {
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
      <form onSubmit={(event) => void submit(event)} className="st-modal-form">
        <p className="st-msg warn">Everything that uses this login switches to the new secret at once. The old secret cannot be recovered.</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <label className="st-field">
          <span className="st-lbl">{privateKey ? "Replacement private key" : "Replacement secret"}</span>
          {privateKey ? (
            <textarea value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required spellCheck={false} autoComplete="new-password" className="st-ta" />
          ) : (
            <Input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required autoComplete="new-password" />
          )}
          <span className="st-hint">Spaces at the start and end are kept.</span>
        </label>
        <label className="st-field">
          <span className="st-lbl">Replacement passphrase (optional)</span>
          <Input type="password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        <label className="st-field">
          <span className="st-lbl">Rotation reason</span>
          <Input value={reason} onChange={(event) => setReason(event.target.value)} maxLength={512} required />
        </label>
        <label className="st-field">
          <span className="st-lbl">Type ROTATE to confirm</span>
          <Input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
        </label>
        <div className="st-dlg-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>Cancel</Button>
          <Button type="submit" variant="danger" className="st-danger-fill" loading={saving} disabled={!canRotate}>Rotate</Button>
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

export function DeleteCredentialDialog({ profile, onClose, onDeleted }: DeleteDialogProps) {
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
      <div className="st-modal-form">
        <p className="st-msg">The hub deletes it only when no device, bookmark, jump chain, collector or file connection still uses it.</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <div className="st-dlg-acts">
          <Button variant="ghost" onClick={onClose} disabled={deleting}>Cancel</Button>
          <Button variant="danger" className="st-danger-fill" onClick={() => void remove()} loading={deleting}>Delete</Button>
        </div>
      </div>
    </Modal>
  );
}
