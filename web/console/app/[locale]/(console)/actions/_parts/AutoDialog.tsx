"use client";

import * as Dialog from "@radix-ui/react-dialog";
import type { ReactNode } from "react";
import { Button } from "../../../../components/ui/Button";

/**
 * A 2.0 confirm dialog: a title, a line or two, then Cancel and the confirm button on the right (red for deletes).
 * While pending it cannot be closed. error shows the hub's refusal under the body.
 */
export function AutoDialog({
  open,
  title,
  children,
  cancelLabel,
  confirmLabel,
  onConfirm,
  onClose,
  danger = false,
  pending = false,
  confirmDisabled = false,
  error,
}: {
  open: boolean;
  title: string;
  children?: ReactNode;
  cancelLabel: string;
  confirmLabel: string;
  onConfirm: () => void;
  onClose: () => void;
  danger?: boolean;
  pending?: boolean;
  confirmDisabled?: boolean;
  error?: string | null;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(next) => { if (!next && !pending) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="au-scrim" />
        <Dialog.Content className="au-dlg" aria-describedby={undefined}>
          <Dialog.Title className="au-dlg-t">{title}</Dialog.Title>
          {children ? <div className="au-dlg-b">{children}</div> : null}
          {error ? <p role="alert" className="au-err">{error}</p> : null}
          <div className="au-dlg-a">
            <Button type="button" variant="ghost" onClick={onClose} disabled={pending}>{cancelLabel}</Button>
            <Button
              type="button"
              variant={danger ? "danger" : "primary"}
              onClick={onConfirm}
              disabled={pending || confirmDisabled}
              aria-busy={pending || undefined}
            >
              {confirmLabel}
            </Button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
