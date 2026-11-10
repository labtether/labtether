"use client";

import { useFormatter, useTranslations } from "next-intl";
import { useState } from "react";

import { useCredentialKindLabel } from "../../../../components/credentials/CredentialProfileSelect";
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
  credentialFailure,
  credentialReferenceUses,
  CredentialValidationError,
  type CredentialAction,
  type CredentialCreateDraft,
  type CredentialReference,
} from "./credentialProfileModel";

/* Create, rotate and delete dialogs for saved logins (the hub's credential profiles). Secrets go to the hub once and never come back. */

type ErrorPayload = {
  error?: unknown;
  references?: unknown;
};

class CredentialRequestError extends Error {
  status: number;
  /** The hub's own reason, or "" when it sent none. */
  hubMessage: string;
  /** What still uses the login, when the hub refused a delete for that reason; null otherwise. */
  references: CredentialReference[] | null;

  constructor(status: number, hubMessage: string, references: CredentialReference[] | null) {
    super(hubMessage || `credential request failed with ${status}`);
    this.name = "CredentialRequestError";
    this.status = status;
    this.hubMessage = hubMessage;
    this.references = references;
  }
}

function parseReferences(value: unknown): CredentialReference[] | null {
  if (!Array.isArray(value)) return null;
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
    const hubMessage = typeof payload?.error === "string" ? payload.error.trim() : "";
    throw new CredentialRequestError(response.status, hubMessage, parseReferences(payload?.references));
  }
  return payload;
}

/** Words for a failed request: what still uses the login, the console's own refusal line, or the hub's 400 reason. */
function useFailureText(): (action: CredentialAction, error: unknown) => string {
  const t = useTranslations("security.logins");
  const format = useFormatter();
  return (action, error) => {
    if (!(error instanceof CredentialRequestError)) return t(`errors.${action}Failed`);
    if (error.references) {
      const uses = credentialReferenceUses(error.references);
      if (uses.length === 0) return t("errors.inUse");
      const items = format.list(uses.map(({ use, count }) => t(`uses.${use}`, { count })), { type: "conjunction" });
      return t("errors.usedBy", { items });
    }
    const failure = credentialFailure(action, error.status, error.hubMessage);
    return failure ? t(`errors.${failure}`) : error.hubMessage;
  };
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
  const t = useTranslations("security");
  const kindLabel = useCredentialKindLabel();
  const failureText = useFailureText();
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
      setError(validationError instanceof CredentialValidationError
        ? t(`logins.errors.${validationError.problem}`, validationError.values)
        : failureText("add", validationError));
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
      setError(failureText("add", requestError));
    } finally {
      setSaving(false);
    }
  };

  const privateKey = draft.kind === "ssh_private_key";
  return (
    <Modal open={open} onClose={onClose} title={t("logins.add")} className="max-w-xl">
      <form onSubmit={(event) => void submit(event)} className="st-modal-form">
        <p className="st-msg">{t("logins.form.secretNote")}</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <label className="st-field">
          <span className="st-lbl">{t("logins.form.name")}</span>
          <Input value={draft.name} onChange={(event) => update("name", event.target.value)} maxLength={120} required />
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.form.type")}</span>
          <Select value={draft.kind} onChange={(event) => update("kind", event.target.value as CredentialKind)} className="w-full">
            {credentialKinds.map((kind) => <option key={kind} value={kind}>{kindLabel(kind)}</option>)}
          </Select>
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.form.username")}</span>
          <Input value={draft.username} onChange={(event) => update("username", event.target.value)} maxLength={64} autoComplete="off" />
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.form.description")}</span>
          <Input value={draft.description} onChange={(event) => update("description", event.target.value)} maxLength={4096} />
        </label>
        <label className="st-field">
          <span className="st-lbl">{privateKey ? t("logins.form.privateKey") : t("logins.form.secret")}</span>
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
          <span className="st-hint">{t("logins.form.edgesKept")}</span>
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.form.passphrase")}</span>
          <Input type="password" value={draft.passphrase} onChange={(event) => update("passphrase", event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        {/* Metadata is for scripts and the API; it waits behind Advanced so the form asks only what a login needs. */}
        <details className="st-details" open={draft.metadataText.trim() !== "" && draft.metadataText.trim() !== "{}"}>
          <summary className="st-link">{t("logins.form.advanced")}</summary>
          <label className="st-field st-fold">
            <span className="st-lbl">{t("logins.form.metadata")}</span>
            <textarea
              value={draft.metadataText}
              onChange={(event) => update("metadataText", event.target.value)}
              spellCheck={false}
              className="st-ta"
            />
          </label>
        </details>
        <div className="st-dlg-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>{t("cancel")}</Button>
          <Button type="submit" variant="primary" loading={saving}>{t("logins.add")}</Button>
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

/** The word the person types to confirm a rotation. It stays ROTATE in every language so the check never changes. */
const rotateWord = "ROTATE";

export function RotateCredentialDialog({ profile, onClose, onRotated }: RotateDialogProps) {
  const t = useTranslations("security");
  const failureText = useFailureText();
  const [secret, setSecret] = useState("");
  const [passphrase, setPassphrase] = useState("");
  const [reason, setReason] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!profile) return null;
  const privateKey = profile.kind === "ssh_private_key";
  const canRotate = secret.length > 0 && reason.trim().length > 0 && confirmation === rotateWord;

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
      setError(failureText("rotate", requestError));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={t("logins.rotateTitle", { name: profile.name })} className="max-w-lg">
      <form onSubmit={(event) => void submit(event)} className="st-modal-form">
        <p className="st-msg warn">{t("logins.rotateWarning")}</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <label className="st-field">
          <span className="st-lbl">{privateKey ? t("logins.newPrivateKey") : t("logins.newSecret")}</span>
          {privateKey ? (
            <textarea value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required spellCheck={false} autoComplete="new-password" className="st-ta" />
          ) : (
            <Input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} maxLength={16384} required autoComplete="new-password" />
          )}
          <span className="st-hint">{t("logins.form.edgesKept")}</span>
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.newPassphrase")}</span>
          <Input type="password" value={passphrase} onChange={(event) => setPassphrase(event.target.value)} maxLength={16384} autoComplete="new-password" />
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.reason")}</span>
          <Input value={reason} onChange={(event) => setReason(event.target.value)} maxLength={512} required />
        </label>
        <label className="st-field">
          <span className="st-lbl">{t("logins.confirmWord", { word: rotateWord })}</span>
          <Input value={confirmation} onChange={(event) => setConfirmation(event.target.value)} autoComplete="off" />
        </label>
        <div className="st-dlg-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={saving}>{t("cancel")}</Button>
          <Button type="submit" variant="danger" className="st-danger-fill" loading={saving} disabled={!canRotate}>{t("logins.rotateConfirm")}</Button>
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
  const t = useTranslations("security");
  const failureText = useFailureText();
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
      setError(failureText("delete", requestError));
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Modal open onClose={onClose} title={t("logins.deleteTitle", { name: profile.name })} className="max-w-md">
      <div className="st-modal-form">
        <p className="st-msg">{t("logins.deleteBody")}</p>
        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}
        <div className="st-dlg-acts">
          <Button variant="ghost" onClick={onClose} disabled={deleting}>{t("cancel")}</Button>
          <Button variant="danger" className="st-danger-fill" onClick={() => void remove()} loading={deleting}>{t("logins.deleteConfirm")}</Button>
        </div>
      </div>
    </Modal>
  );
}
