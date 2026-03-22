"use client";

import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";
import { Modal } from "../../../../components/ui/Modal";

type NodeDeleteConfirmModalProps = {
  open: boolean;
  assetName: string;
  confirmInput: string;
  deleting: boolean;
  error?: string | null;
  onConfirmInputChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
};

export function NodeDeleteConfirmModal({
  open,
  assetName,
  confirmInput,
  deleting,
  error,
  onConfirmInputChange,
  onCancel,
  onConfirm,
}: NodeDeleteConfirmModalProps) {
  return (
    <Modal open={open} onClose={onCancel} title="Confirm Node Deletion">
      <div className="px-5 py-4 space-y-4">
        <p className="text-xs text-[var(--muted)]">
          This will permanently delete <strong>{assetName}</strong> and all associated data.
          Type <strong>{assetName}</strong> below to confirm.
        </p>
        <Input
          placeholder={`Type "${assetName}" to confirm`}
          value={confirmInput}
          onChange={(event) => onConfirmInputChange(event.target.value)}
          autoFocus
          onKeyDown={(event) => {
            if (event.key === "Enter" && confirmInput === assetName) {
              onConfirm();
            }
          }}
        />
        {error && (
          <p className="text-xs text-[var(--bad)]">{error}</p>
        )}
        <div className="flex items-center gap-3">
          <Button onClick={onCancel}>
            Cancel
          </Button>
          <Button
            variant="danger"
            className="bg-[var(--bad-glow)]"
            disabled={confirmInput !== assetName || deleting}
            onClick={onConfirm}
          >
            {deleting ? "Deleting..." : "Confirm Delete"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
