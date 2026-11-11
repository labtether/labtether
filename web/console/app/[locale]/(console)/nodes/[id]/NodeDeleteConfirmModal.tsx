"use client";

import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { Modal } from "../../../../components/ui/Modal";
import { useDeletePreview } from "./useDeletePreview";

type NodeDeleteConfirmModalProps = {
  open: boolean;
  /** The device being removed: the dialog asks the Hub what goes with it. */
  assetId?: string;
  assetName: string;
  confirmInput: string;
  deleting: boolean;
  error?: string | null;
  /** An agent device: it can come back with a new single-use token from Add device (Hub #298). */
  agent?: boolean;
  onConfirmInputChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
};

/** Remove a device: say what goes, and ask for its name before the red button works. */
export function NodeDeleteConfirmModal({
  open,
  assetId,
  assetName,
  confirmInput,
  deleting,
  error,
  agent = false,
  onConfirmInputChange,
  onCancel,
  onConfirm,
}: NodeDeleteConfirmModalProps) {
  const t = useTranslations("devices.page.remove");
  const ready = confirmInput === assetName && !deleting;
  const preview = useDeletePreview(open, assetId);
  return (
    <Modal open={open} onClose={onCancel} title={t("title", { name: assetName })}>
      <form
        className="kt-dlg-body dv-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready) onConfirm();
        }}
      >
        <p>{t("body", { name: assetName })}</p>
        {preview && preview.devices > 0 ? (
          <p>{t(preview.connection ? "alsoFound" : "alsoAttached", { count: preview.devices })}</p>
        ) : null}
        {preview?.saved_login ? <p>{t("alsoLogin")}</p> : null}
        {agent ? <p>{t("addBack")}</p> : null}
        <div className="dv-field">
          <label htmlFor="dv-remove-name">{t("typeName", { name: assetName })}</label>
          <input
            id="dv-remove-name"
            className="dv-input"
            value={confirmInput}
            onChange={(event) => onConfirmInputChange(event.target.value)}
            autoComplete="off"
            spellCheck={false}
            autoFocus
          />
        </div>
        {error ? <p className="dv-err" role="alert">{t("failed", { name: assetName })}</p> : null}
        <div className="kt-dlg-acts">
          <Button type="button" onClick={onCancel} disabled={deleting}>{t("cancel")}</Button>
          <Button type="submit" variant="danger" className="kt-danger-fill" disabled={!ready} loading={deleting}>
            {t("confirm")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
