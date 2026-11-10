"use client";

import { Download } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { Button } from "../../../components/ui/Button";
import { ConfirmDialog } from "../../../components/ui/ConfirmDialog";
import { Modal } from "../../../components/ui/Modal";
import { formatSize } from "./fileWorkspaceUtils";

// Dialogs of the file views: Delete… (one or several items), New folder, Upload of several dropped files, Replace
// before an upload over files of the same name, and the text viewer for small text files (numbered lines, Download).

const BAD_NAME = /[/\\\0]/;
const MAX_LINES = 2000;
const MAX_CLASH_NAMES = 3;

export function DeleteConfirmDialog({ names, where, onCancel, onConfirm, busy }: {
  names: string[] | null;
  where: string;
  onCancel: () => void;
  onConfirm: () => void;
  busy?: boolean;
}) {
  const t = useTranslations("files.dialog");
  const count = names?.length ?? 0;
  return (
    <ConfirmDialog
      open={count > 0}
      onClose={onCancel}
      onConfirm={onConfirm}
      title={count === 1 ? t("deleteOne", { name: names?.[0] ?? "" }) : t("deleteMany", { count })}
      message={count === 1 ? t("deleteOneBody", { where }) : t("deleteManyBody", { count, where, names: (names ?? []).slice(0, 8).join(", ") })}
      confirmLabel={t("delete")}
      danger
      loading={busy}
    />
  );
}

export function UploadConfirmDialog({ files, where, onCancel, onConfirm }: {
  files: File[] | null;
  where: string;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const t = useTranslations("files.dialog");
  const count = files?.length ?? 0;
  const bytes = (files ?? []).reduce((sum, f) => sum + f.size, 0);
  return (
    <ConfirmDialog
      open={count > 0}
      onClose={onCancel}
      onConfirm={onConfirm}
      title={t("uploadTitle", { count, size: formatSize(bytes) })}
      message={t("uploadBody", { where })}
      confirmLabel={t("upload")}
    />
  );
}

/**
 * Before an upload over files of the same name: Replace uploads every chosen file, Skip these (shown when some chosen
 * files do not clash) uploads only those, Cancel uploads nothing. container is the fullscreen element, if any.
 */
export function UploadReplaceDialog({ ask, where, onCancel, onSkip, onReplace, container }: {
  ask: { clashing: { name: string }[]; rest: unknown[] } | null;
  where: string;
  onCancel: () => void;
  onSkip: () => void;
  onReplace: () => void;
  container?: HTMLElement | null;
}) {
  const t = useTranslations("files.dialog");
  const names = (ask?.clashing ?? []).map((file) => file.name);
  const count = names.length;
  const shown = names.slice(0, MAX_CLASH_NAMES).join(", ");
  const list = count > MAX_CLASH_NAMES ? t("replaceMore", { names: shown, count: count - MAX_CLASH_NAMES }) : shown;
  let message = "";
  if (count === 1) message = t("replaceOneBody", { name: names[0], where });
  else if (count > 1) message = t("replaceManyBody", { names: list, where });
  return (
    <ConfirmDialog
      open={count > 0}
      onClose={onCancel}
      onConfirm={onReplace}
      title={count > 0 ? t("replaceTitle", { count }) : ""}
      message={message}
      confirmLabel={t("replace")}
      extra={ask && ask.rest.length > 0 ? { label: t("replaceSkip"), onClick: onSkip } : undefined}
      danger
      container={container}
    />
  );
}

/** Asks for a name; a name with / is refused. Used for New folder and for Rename on a device's Files tab. */
export function NameDialog({ open, title, initial = "", confirmLabel, busy, onCancel, onSubmit }: {
  open: boolean;
  title: string;
  initial?: string;
  confirmLabel: string;
  busy?: boolean;
  onCancel: () => void;
  onSubmit: (name: string) => void;
}) {
  return (
    <Modal open={open} onClose={onCancel} title={title} className="max-w-sm">
      {open ? <NameForm initial={initial} confirmLabel={confirmLabel} busy={busy} onCancel={onCancel} onSubmit={onSubmit} /> : null}
    </Modal>
  );
}

function NameForm({ initial, confirmLabel, busy, onCancel, onSubmit }: {
  initial: string;
  confirmLabel: string;
  busy?: boolean;
  onCancel: () => void;
  onSubmit: (name: string) => void;
}) {
  const t = useTranslations("files.dialog");
  const [value, setValue] = useState(initial);
  const name = value.trim();
  const bad = BAD_NAME.test(name);
  const ok = name !== "" && !bad && name !== initial;
  return (
    <form
      className="fx-dlg"
      onSubmit={(e) => {
        e.preventDefault();
        if (ok) onSubmit(name);
      }}
    >
      <label className="fx-field">
        <span>{t("name")}</span>
        <input
          className="fx-input"
          value={value}
          autoFocus
          spellCheck={false}
          autoComplete="off"
          aria-invalid={bad || undefined}
          onChange={(e) => setValue(e.target.value)}
        />
      </label>
      {bad ? <p className="fx-field-err" role="alert">{t("badName")}</p> : null}
      <div className="fx-dlg-acts">
        <Button type="button" variant="ghost" size="sm" onClick={onCancel} disabled={busy}>{t("cancel")}</Button>
        <Button type="submit" variant="primary" size="sm" disabled={!ok} loading={busy}>{confirmLabel}</Button>
      </div>
    </form>
  );
}

export function TextPreviewOverlay({ previewContent, onClose, onDownload }: {
  previewContent: { name: string; content: string } | null;
  onClose: () => void;
  onDownload: () => void;
}) {
  const t = useTranslations("files.viewer");
  const lines = previewContent ? previewContent.content.replace(/\n$/, "").split("\n") : [];
  const shown = lines.slice(0, MAX_LINES);
  return (
    <Modal open={previewContent !== null} onClose={onClose} title={previewContent?.name ?? ""} className="fx-viewer md:max-w-3xl">
      <div className="fx-view">
        <div className="fx-view-h">
          <span className="fx-c3 fx-num">{t("lines", { count: lines.length })}</span>
          <span className="fx-sp" />
          <Button size="sm" onClick={onDownload}>
            <Download aria-hidden="true" width={14} height={14} strokeWidth={2} />
            {t("download")}
          </Button>
          <Button size="sm" variant="ghost" onClick={onClose}>{t("close")}</Button>
        </div>
        <pre className="fx-pre" tabIndex={0} aria-label={t("contents", { name: previewContent?.name ?? "" })}>
          {shown.map((l, i) => (
            <span key={i} className="fx-pre-l"><span className="fx-ln" aria-hidden="true">{i + 1}</span>{l}{"\n"}</span>
          ))}
        </pre>
        {lines.length > MAX_LINES ? <p className="fx-c3 fx-view-more">{t("more", { count: lines.length - MAX_LINES })}</p> : null}
      </div>
    </Modal>
  );
}
