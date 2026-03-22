"use client";

import React from "react";
import { ConfirmDialog } from "../../../../components/ui/ConfirmDialog";
import { Modal } from "../../../../components/ui/Modal";
import { Button } from "../../../../components/ui/Button";
import { Input } from "../../../../components/ui/Input";

type EntryTarget = { name: string; path: string };

type FilesTabStatusProps = {
  error: string | null;
  actionMessage: string | null;
};

type FilesTabOverlaysProps = {
  enableWriteOpen: boolean;
  onCloseEnableWrite: () => void;
  onConfirmEnableWrite: () => void;
  deleteTarget: EntryTarget | null;
  onCloseDelete: () => void;
  onConfirmDelete: () => void;
  actionBusy: boolean;
  renameTarget: EntryTarget | null;
  renameName: string;
  onRenameNameChange: (next: string) => void;
  onCommitRename: () => void;
  onCancelRename: () => void;
};

export function FilesTabStatus({ error, actionMessage }: FilesTabStatusProps) {
  return (
    <>
      {error ? <p style={{ marginTop: 0, marginBottom: "8px", fontSize: "12px", color: "var(--bad)" }}>{error}</p> : null}
      {actionMessage ? <p style={{ marginTop: 0, marginBottom: "8px", fontSize: "12px", color: "var(--ok)" }}>{actionMessage}</p> : null}
    </>
  );
}

export function FilesTabOverlays({
  enableWriteOpen,
  onCloseEnableWrite,
  onConfirmEnableWrite,
  deleteTarget,
  onCloseDelete,
  onConfirmDelete,
  actionBusy,
  renameTarget,
  renameName,
  onRenameNameChange,
  onCommitRename,
  onCancelRename,
}: FilesTabOverlaysProps) {
  return (
    <>
      <ConfirmDialog
        open={enableWriteOpen}
        onClose={onCloseEnableWrite}
        onConfirm={onConfirmEnableWrite}
        title="Enable Write Actions"
        message="Enabling write actions allows you to upload, create, rename, move, and delete files on this node. Proceed with caution — deletions cannot be undone."
        confirmLabel="Enable"
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={onCloseDelete}
        onConfirm={onConfirmDelete}
        title="Delete Entry"
        message={`Delete "${deleteTarget?.name ?? ""}"? This cannot be undone.`}
        confirmLabel="Delete"
        danger
        loading={actionBusy}
      />

      <Modal
        open={renameTarget !== null}
        onClose={onCancelRename}
        title="Rename Entry"
        className="max-w-sm"
      >
        <div className="px-5 py-4 space-y-4">
          <Input
            value={renameName}
            onChange={(event: React.ChangeEvent<HTMLInputElement>) => onRenameNameChange(event.target.value)}
            onKeyDown={(event: React.KeyboardEvent<HTMLInputElement>) => {
              if (event.key === "Enter" && renameName.trim() && renameName.trim() !== renameTarget?.name && !/[/\0]/.test(renameName)) onCommitRename();
              if (event.key === "Escape") onCancelRename();
            }}
            autoFocus
            placeholder="New name"
          />
          {/[/\0]/.test(renameName) && (
            <p className="text-[10px] text-[var(--bad)]">Name cannot contain / or null characters</p>
          )}
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={onCancelRename}
              disabled={actionBusy}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={onCommitRename}
              loading={actionBusy}
              disabled={renameName.trim() === "" || renameName.trim() === renameTarget?.name || /[/\0]/.test(renameName)}
            >
              Rename
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
