"use client";

import { useState, type FormEvent } from "react";
import { Button } from "../../../../components/ui/Button";
import { Input, Select } from "../../../../components/ui/Input";
import { Modal } from "../../../../components/ui/Modal";
import {
  SSH_HUB_KEY_ROTATION_CONFIRMATION,
  SSH_HUB_KEY_ROTATION_REASON_MAX_LENGTH,
  type SSHHubKeyRotationPayload,
  type SSHHubKeyType,
} from "./sshHubKeyModel";

type SSHHubKeyRotateDialogProps = {
  currentKeyType: SSHHubKeyType;
  loading: boolean;
  error: string;
  onClose: () => void;
  onConfirm: (payload: SSHHubKeyRotationPayload) => Promise<boolean>;
};

export function SSHHubKeyRotateDialog({
  currentKeyType,
  loading,
  error,
  onClose,
  onConfirm,
}: SSHHubKeyRotateDialogProps) {
  const [keyType, setKeyType] = useState<SSHHubKeyType>(currentKeyType);
  const [reason, setReason] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const confirmationMatches = confirmation === SSH_HUB_KEY_ROTATION_CONFIRMATION;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!confirmationMatches || loading) return;
    const succeeded = await onConfirm({
      key_type: keyType,
      reason: reason.trim(),
      confirm: SSH_HUB_KEY_ROTATION_CONFIRMATION,
    });
    if (succeeded) onClose();
  }

  return (
    <Modal open onClose={onClose} title="Rotate SSH hub key" className="max-w-lg">
      <form className="space-y-4 px-5 py-4" onSubmit={(event) => { void handleSubmit(event); }}>
        <p className="text-sm leading-relaxed text-[var(--text-secondary)]">
          LabTether will stage the replacement public key on connected agents before activating it.
          A failed stage or save leaves the current key active.
        </p>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-[var(--text)]" htmlFor="ssh-hub-key-type">
            Key type
          </label>
          <Select
            id="ssh-hub-key-type"
            value={keyType}
            onChange={(event) => setKeyType(event.target.value as SSHHubKeyType)}
            disabled={loading}
          >
            <option value="ed25519">Ed25519 (recommended)</option>
            <option value="rsa">RSA 4096</option>
          </Select>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-[var(--text)]" htmlFor="ssh-hub-key-reason">
            Maintenance note (optional)
          </label>
          <Input
            id="ssh-hub-key-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            maxLength={SSH_HUB_KEY_ROTATION_REASON_MAX_LENGTH}
            placeholder="Change ticket or reason; do not include secrets"
            disabled={loading}
          />
        </div>

        <div className="rounded-lg border border-[var(--bad)]/30 bg-[var(--bad-glow)] p-3">
          <label className="text-xs font-medium text-[var(--bad)]" htmlFor="ssh-hub-key-confirmation">
            Type {SSH_HUB_KEY_ROTATION_CONFIRMATION} to confirm
          </label>
          <Input
            id="ssh-hub-key-confirmation"
            className="mt-2 font-mono"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            error={confirmation.length > 0 && !confirmationMatches}
            disabled={loading}
          />
          <p className="mt-2 text-xs text-[var(--muted)]">
            Devices that are offline will receive the active key when they reconnect.
          </p>
        </div>

        {error ? <p role="alert" className="text-sm text-[var(--bad)]">{error}</p> : null}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button type="submit" variant="danger" loading={loading} disabled={!confirmationMatches}>
            Rotate key
          </Button>
        </div>
      </form>
    </Modal>
  );
}
