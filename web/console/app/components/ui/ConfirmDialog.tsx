"use client";

import { useTranslations } from "next-intl";
import { Modal } from "./Modal";
import { Button } from "./Button";

type ConfirmDialogProps = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  loading?: boolean;
  /** A second choice, shown between Cancel and the confirm button. */
  extra?: { label: string; onClick: () => void };
  /** Passed to Modal: the fullscreen element when the opener is inside one. */
  container?: HTMLElement | null;
};

/** 2.0 confirm: title, one plain paragraph, Cancel and the confirm button bottom right (status red when destructive). */
export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel,
  danger = false,
  loading = false,
  extra,
  container,
}: ConfirmDialogProps) {
  const t = useTranslations("common");
  return (
    <Modal open={open} onClose={onClose} title={title} className="confirm" container={container}>
      <div className="kt-dlg-body">
        <p>{message}</p>
        <div className="kt-dlg-acts">
          <Button onClick={onClose} disabled={loading}>
            {t("cancel")}
          </Button>
          {extra ? (
            <Button onClick={extra.onClick} disabled={loading}>
              {extra.label}
            </Button>
          ) : null}
          <Button
            variant={danger ? "danger" : "primary"}
            className={danger ? "kt-danger-fill" : ""}
            onClick={onConfirm}
            loading={loading}
          >
            {confirmLabel ?? t("confirm")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
