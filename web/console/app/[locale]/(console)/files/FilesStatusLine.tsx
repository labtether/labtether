"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { formatSize } from "./fileWorkspaceUtils";

// The one line under a folder: the count when it tells something (while searching or picking), the picked items'
// size, and the actions on the picked items. Empty, it is not shown.

export function FilesStatusLine({ shown, total, searching, picked, pickedBytes, onDownload, onDelete, onClear, extra }: {
  shown: number;
  total: number;
  searching: boolean;
  picked: number;
  pickedBytes: number;
  onDownload?: () => void;
  onDelete?: () => void;
  onClear: () => void;
  extra?: ReactNode;
}) {
  const t = useTranslations("files.status");
  const bits: string[] = [];
  if (searching) bits.push(t("filtered", { shown, count: total }));
  else if (picked) bits.push(t("count", { count: total }));
  if (picked) bits.push(pickedBytes > 0 ? t("pickedSize", { count: picked, size: formatSize(pickedBytes) }) : t("picked", { count: picked }));
  if (!bits.length && !extra) return null;
  return (
    <div className="fx-status" aria-live="polite">
      <span className="fx-num">{bits.join(" · ")}</span>
      {extra}
      <span className="fx-sp" />
      {picked ? (
        <span className="fx-status-acts">
          {onDownload ? <button type="button" className="fx-btn sm" onClick={onDownload}>{t("download")}</button> : null}
          {onDelete ? <button type="button" className="fx-btn sm danger" onClick={onDelete}>{t("delete")}</button> : null}
          <button type="button" className="fx-btn sm quiet" onClick={onClear}>{t("clear")}</button>
        </span>
      ) : null}
    </div>
  );
}
