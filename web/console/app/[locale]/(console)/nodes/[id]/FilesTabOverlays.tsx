"use client";

import { useTranslations } from "next-intl";
import { ConfirmDialog } from "../../../../components/ui/ConfirmDialog";
import { DeleteConfirmDialog, NameDialog } from "../../files/FilesOverlays";

type EntryTarget = { name: string; path: string };

type FilesTabStatusProps = {
  error: string | null;
  actionMessage: string | null;
};

type FilesTabOverlaysProps = {
  where: string;
  enableWriteOpen: boolean;
  onCloseEnableWrite: () => void;
  onConfirmEnableWrite: () => void;
  deleteTargets: EntryTarget[] | null;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
  actionBusy: boolean;
  newFolderOpen: boolean;
  onCloseNewFolder: () => void;
  onCreateFolder: (name: string) => void;
};

/** The last result of a change on this device: an error in red, or what was done. */
export function FilesTabStatus({ error, actionMessage }: FilesTabStatusProps) {
  if (error) return <p className="fx-err" role="alert">{error}</p>;
  if (actionMessage) return <p className="fx-note" role="status">{actionMessage}</p>;
  return null;
}

export function FilesTabOverlays({
  where,
  enableWriteOpen,
  onCloseEnableWrite,
  onConfirmEnableWrite,
  deleteTargets,
  onCloseDelete,
  onConfirmDelete,
  actionBusy,
  newFolderOpen,
  onCloseNewFolder,
  onCreateFolder,
}: FilesTabOverlaysProps) {
  const t = useTranslations("files");
  return (
    <>
      <ConfirmDialog
        open={enableWriteOpen}
        onClose={onCloseEnableWrite}
        onConfirm={onConfirmEnableWrite}
        title={t("device.allowTitle")}
        message={t("device.allowBody")}
        confirmLabel={t("device.allow")}
      />
      <DeleteConfirmDialog
        names={deleteTargets ? deleteTargets.map((d) => d.name) : null}
        where={where}
        onCancel={onCloseDelete}
        onConfirm={onConfirmDelete}
        busy={actionBusy}
      />
      <NameDialog
        open={newFolderOpen}
        title={t("dialog.newFolder")}
        confirmLabel={t("dialog.create")}
        busy={actionBusy}
        onCancel={onCloseNewFolder}
        onSubmit={onCreateFolder}
      />
    </>
  );
}
