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
      <form className="st-modal-form" onSubmit={(event) => { void handleSubmit(event); }}>
        <p className="st-msg">
          Connected agents get the new public key before it becomes active. If that fails, the current key stays active.
          Offline devices get the active key when they reconnect.
        </p>

        <div className="st-field">
          <label htmlFor="ssh-hub-key-type">
            Key type
          </label>
          <Select
            id="ssh-hub-key-type"
            value={keyType}
            onChange={(event) => setKeyType(event.target.value as SSHHubKeyType)}
            disabled={loading}
          >
            <option value="ed25519">Ed25519</option>
            <option value="rsa">RSA 4096</option>
          </Select>
        </div>

        <div className="st-field">
          <label htmlFor="ssh-hub-key-reason">
            Note (optional)
          </label>
          <Input
            id="ssh-hub-key-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            maxLength={SSH_HUB_KEY_ROTATION_REASON_MAX_LENGTH}
            placeholder="Change ticket or reason"
            disabled={loading}
          />
        </div>

        <div className="st-field">
          <label htmlFor="ssh-hub-key-confirmation">
            Type {SSH_HUB_KEY_ROTATION_CONFIRMATION} to confirm
          </label>
          <Input
            id="ssh-hub-key-confirmation"
            className="font-mono"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            error={confirmation.length > 0 && !confirmationMatches}
            disabled={loading}
          />
        </div>

        {error ? <p role="alert" className="st-msg bad">{error}</p> : null}

        <div className="st-dlg-acts">
          <Button type="button" variant="ghost" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button type="submit" variant="danger" className="st-danger-fill" loading={loading} disabled={!confirmationMatches}>
            Rotate key
          </Button>
        </div>
      </form>
    </Modal>
  );
}
