"use client";

import { Button } from "../../../../../components/ui/Button";

type Props = {
  message: string;
  confirmLabel?: string;
  busy?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function PBSActionConfirmation({
  message,
  confirmLabel = "Confirm",
  busy = false,
  onConfirm,
  onCancel,
}: Props) {
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--warn)] px-3 py-2 text-xs text-[var(--text)]"
    >
      <span>{message}</span>
      <Button size="sm" variant="danger" loading={busy} disabled={busy} onClick={onConfirm}>
        {confirmLabel}
      </Button>
      <Button size="sm" variant="ghost" disabled={busy} onClick={onCancel}>
        Cancel
      </Button>
    </div>
  );
}
