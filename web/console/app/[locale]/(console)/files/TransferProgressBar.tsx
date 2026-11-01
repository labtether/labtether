"use client";

import { ChevronDown, ChevronRight, Copy, Download, Upload, X } from "lucide-react";
import { useTranslations } from "next-intl";
import { useState } from "react";
import type { FileTransfer } from "./fileTransferClient";
import { formatSize } from "./fileWorkspaceUtils";
import type { Progress } from "./useFilesSource";

// Transfers under the folder: uploads from this browser, downloads to it, and copies between devices and
// connections. Shows only while there is one; running rows can be cancelled, finished ones stay until cleared.

interface TransferProgressBarProps {
  transfers: FileTransfer[];
  onCancel: (transferId: string) => void;
  onClearCompleted: () => void;
  upload?: Progress | null;
  onCancelUpload?: () => void;
  download?: Progress | null;
  onCancelDownload?: () => void;
}

type Row = {
  key: string;
  kind: "upload" | "download" | "copy";
  name: string;
  where: string;
  loaded: number;
  total: number;
  state: "run" | "done" | "failed";
  error?: string;
  onCancel?: () => void;
};

const pct = (r: Row) => (r.total > 0 ? Math.min(100, Math.round((r.loaded / r.total) * 100)) : 0);
const active = (t: FileTransfer) => t.status === "pending" || t.status === "in_progress";
const ICON = { upload: Upload, download: Download, copy: Copy };

export function TransferProgressBar({ transfers, onCancel, onClearCompleted, upload, onCancelUpload, download, onCancelDownload }: TransferProgressBarProps) {
  const t = useTranslations("files.xfer");
  const [shut, setShut] = useState(false);
  const rows: Row[] = [];
  if (upload) rows.push({ key: "up", kind: "upload", name: upload.fileName, where: "", loaded: upload.loaded, total: upload.total, state: "run", onCancel: onCancelUpload });
  if (download) rows.push({ key: "down", kind: "download", name: download.fileName, where: t("toBrowser"), loaded: download.loaded, total: download.total, state: "run", onCancel: onCancelDownload });
  for (const x of [...transfers].reverse()) {
    rows.push({
      key: x.id,
      kind: "copy",
      name: x.file_name,
      where: x.dest_path,
      loaded: x.bytes_transferred,
      total: x.file_size ?? 0,
      state: active(x) ? "run" : x.status === "completed" ? "done" : "failed",
      error: x.error,
      onCancel: active(x) ? () => onCancel(x.id) : undefined,
    });
  }
  if (rows.length === 0) return null;

  const running = rows.filter((r) => r.state === "run").length;
  const finished = transfers.some((x) => !active(x));

  const verb = (r: Row) => {
    if (r.state === "failed") return t(`${r.kind}Failed`, { name: r.name });
    return t(r.state === "run" ? `${r.kind}Run` : `${r.kind}Done`, { name: r.name });
  };
  const detail = (r: Row) => {
    if (r.state === "failed") return r.error || t("failed");
    if (r.state === "done") return r.total > 0 ? formatSize(r.total) : "";
    if (r.total > 0) return t("progress", { done: formatSize(r.loaded), total: formatSize(r.total), pct: pct(r) });
    return r.loaded > 0 ? formatSize(r.loaded) : t("starting");
  };

  return (
    <section className="fx-xbar" aria-label={t("title")}>
      <div className="fx-xh">
        <button type="button" className="fx-xtog" aria-expanded={!shut} onClick={() => setShut((v) => !v)}>
          {shut ? <ChevronRight aria-hidden="true" width={12} height={12} strokeWidth={2.4} /> : <ChevronDown aria-hidden="true" width={12} height={12} strokeWidth={2.4} />}
          <b>{t("title")}</b>
          <span className="fx-c3 fx-num">{running ? t("running", { count: running }) : t("done")}</span>
        </button>
        <span className="fx-sp" />
        {finished ? <button type="button" className="fx-btn quiet sm" onClick={onClearCompleted}>{t("clear")}</button> : null}
      </div>
      {shut ? null : (
        <div className="fx-xl" role="list">
          {rows.map((r) => {
            const Icon = ICON[r.kind];
            return (
              <div key={r.key} role="listitem" className={`fx-x${r.state === "run" ? "" : " fin"}${r.state === "failed" ? " bad" : ""}`}>
                <Icon className="fx-xg" aria-hidden="true" width={16} height={16} strokeWidth={1.75} />
                <span className="fx-xn">
                  <span className="fx-xname">{verb(r)}</span>
                  {r.where ? <span className="fx-mono fx-c3">{r.where}</span> : null}
                </span>
                <span className="fx-meter" aria-hidden="true"><i style={{ width: `${r.state === "done" ? 100 : pct(r)}%` }} /></span>
                <span className="fx-num fx-xt">{detail(r)}</span>
                {r.onCancel ? (
                  <button type="button" className="fx-ib sm" onClick={r.onCancel} aria-label={t("cancelOne", { name: r.name })} title={t("cancel")}>
                    <X aria-hidden="true" width={14} height={14} strokeWidth={2} />
                  </button>
                ) : <span />}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
