"use client";

import { useCallback, useEffect, useRef, useState, type DragEvent } from "react";
import { useTranslations } from "next-intl";
import { ArrowUp, ChevronRight, Download, Eye, EyeOff, RefreshCw, Upload, X } from "lucide-react";
import { listFiles } from "../[locale]/(console)/files/fileOpsClient";
import type { FileEntry } from "../[locale]/(console)/files/fileOpsClient";
import { formatSize } from "../[locale]/(console)/files/filesPageUtils";
import { useFileDownload } from "../hooks/useFileDownload";

type Props = {
  nodeId: string;
  open: boolean;
  onClose: () => void;
};

const join = (dir: string, name: string) => (dir.endsWith("/") ? `${dir}${name}` : `${dir}/${name}`);

/**
 * Files on the remote machine, over the right of the screen: one folder (home first), Upload from this computer (or
 * drop files on the drawer), Download by choosing a file, with the transfer's progress at the bottom. Goes through the
 * device's agent.
 */
export function RemoteViewFileDrawer({ nodeId, open, onClose }: Props) {
  const t = useTranslations("desktop.files");
  const [currentPath, setCurrentPath] = useState("");
  const [homePath, setHomePath] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [upload, setUpload] = useState<{ name: string; percent: number } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragCounterRef = useRef(0);
  const { downloading, progress, filename, downloadFile } = useFileDownload(nodeId);

  const fetchEntries = useCallback(
    async (path: string) => {
      setLoading(true);
      setError(null);
      try {
        const result = await listFiles(nodeId, path, { showHidden });
        setEntries(result.entries ?? []);
        if (result.path) {
          setCurrentPath(result.path);
          if (!homePath) setHomePath(result.path);
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : t("listFailed"));
        setEntries([]);
      } finally {
        setLoading(false);
      }
    },
    [nodeId, showHidden, homePath, t],
  );

  useEffect(() => {
    if (open) void fetchEntries(currentPath || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, showHidden]);

  const navigateTo = useCallback(
    (path: string) => {
      setCurrentPath(path);
      void fetchEntries(path);
    },
    [fetchEntries],
  );

  const handleUpload = useCallback(
    (files: FileList | File[]) => {
      const file = Array.from(files)[0];
      if (!file) return;
      setUpload({ name: file.name, percent: 0 });
      const xhr = new XMLHttpRequest();
      xhr.open("POST", `/api/files/${encodeURIComponent(nodeId)}/upload?path=${encodeURIComponent(join(currentPath, file.name))}`);
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) setUpload({ name: file.name, percent: Math.round((event.loaded / event.total) * 100) });
      };
      xhr.onload = () => {
        setUpload(null);
        void fetchEntries(currentPath);
      };
      xhr.onerror = () => {
        setUpload(null);
        setError(t("uploadFailed"));
      };
      xhr.send(file);
    },
    [nodeId, currentPath, fetchEntries, t],
  );

  useEffect(() => {
    if (!open) return undefined;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onClose();
        event.stopPropagation();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  const relative = homePath && currentPath.startsWith(homePath) ? currentPath.slice(homePath.length) : currentPath;
  const segments = relative.split("/").filter(Boolean);
  const atHome = !relative || relative === "/";
  const sorted = [...entries].sort((a, b) => (a.is_dir !== b.is_dir ? (a.is_dir ? -1 : 1) : a.name.localeCompare(b.name)));
  const goUp = () => {
    const parent = `/${currentPath.split("/").filter(Boolean).slice(0, -1).join("/")}`;
    navigateTo(parent.length <= homePath.length ? "" : parent);
  };
  const onDrag = (event: DragEvent, step: 1 | -1) => {
    event.preventDefault();
    dragCounterRef.current += step;
    if (step > 0 && event.dataTransfer.types.includes("Files")) setDragOver(true);
    if (dragCounterRef.current === 0) setDragOver(false);
  };
  const transfer = downloading
    ? { name: filename, percent: progress >= 0 ? progress : null, label: t("downloading") }
    : upload
      ? { name: upload.name, percent: upload.percent, label: t("uploading") }
      : null;

  return (
    <aside
      className="rv-drawer"
      aria-label={t("label")}
      onDragEnter={(event) => onDrag(event, 1)}
      onDragOver={(event) => event.preventDefault()}
      onDragLeave={(event) => onDrag(event, -1)}
      onDrop={(event) => {
        event.preventDefault();
        dragCounterRef.current = 0;
        setDragOver(false);
        if (event.dataTransfer.files.length > 0) handleUpload(event.dataTransfer.files);
      }}
    >
      <div className="rv-drawer-h">
        <nav className="rv-path mono" aria-label={t("path")}>
          <button type="button" onClick={() => navigateTo("")} aria-current={atHome ? "location" : undefined}>
            ~
          </button>
          {segments.map((segment, i) => (
            <span key={`${i}-${segment}`}>
              <span aria-hidden="true">/</span>
              <button
                type="button"
                onClick={() => navigateTo(`${homePath}/${segments.slice(0, i + 1).join("/")}`)}
                aria-current={i === segments.length - 1 ? "location" : undefined}
              >
                {segment}
              </button>
            </span>
          ))}
        </nav>
        <button type="button" className="rv-ib sm" onClick={() => void fetchEntries(currentPath)} disabled={loading} aria-label={t("refresh")} title={t("refresh")}>
          <RefreshCw aria-hidden="true" className={loading ? "rv-spin" : undefined} />
        </button>
        <button
          type="button"
          className="rv-ib sm"
          onClick={() => setShowHidden((v) => !v)}
          aria-pressed={showHidden}
          aria-label={t("hidden")}
          title={t("hidden")}
        >
          {showHidden ? <Eye aria-hidden="true" /> : <EyeOff aria-hidden="true" />}
        </button>
        <button type="button" className="rv-ib sm" onClick={onClose} aria-label={t("close")} title={t("close")}>
          <X aria-hidden="true" />
        </button>
      </div>
      <div className="rv-drawer-up">
        <button type="button" className="rv-btn sm" onClick={() => fileInputRef.current?.click()}>
          <Upload aria-hidden="true" />
          {t("upload")}
        </button>
        <input
          ref={fileInputRef}
          type="file"
          hidden
          onChange={(event) => {
            if (event.target.files) handleUpload(event.target.files);
            event.target.value = "";
          }}
        />
      </div>
      <div className="rv-drawer-list">
        {dragOver ? <div className="rv-drop">{t("dropHere")}</div> : null}
        {loading && entries.length === 0 ? (
          <p className="rv-drawer-note">{t("loading")}</p>
        ) : error ? (
          <div className="rv-drawer-note">
            <p className="rv-err">{error}</p>
            <button type="button" className="rv-btn sm" onClick={() => void fetchEntries(currentPath)}>
              {t("tryAgain")}
            </button>
          </div>
        ) : sorted.length === 0 ? (
          <p className="rv-drawer-note">{t("empty")}</p>
        ) : (
          <>
            {atHome ? null : (
              <button type="button" className="rv-file" onClick={goUp}>
                <span className="rv-file-n">..</span>
                <ArrowUp aria-hidden="true" />
              </button>
            )}
            {sorted.map((entry) => (
              <button
                key={entry.name}
                type="button"
                className="rv-file"
                onClick={() => (entry.is_dir ? navigateTo(join(currentPath, entry.name)) : void downloadFile(join(currentPath, entry.name)))}
                title={entry.is_dir ? undefined : t("download", { name: entry.name })}
              >
                <span className="rv-file-n">{entry.name}</span>
                {entry.is_dir ? null : <span className="rv-file-s">{formatSize(entry.size)}</span>}
                {entry.is_dir ? <ChevronRight aria-hidden="true" /> : <Download aria-hidden="true" />}
              </button>
            ))}
          </>
        )}
      </div>
      {transfer ? (
        <div className="rv-xfer" role="status">
          <span className="rv-file-n">{transfer.name}</span>
          <span className="rv-file-s">{transfer.percent === null ? transfer.label : `${transfer.label} ${transfer.percent}%`}</span>
          <i className="rv-prog" aria-hidden="true">
            <b style={{ width: `${transfer.percent ?? 30}%` }} />
          </i>
        </div>
      ) : null}
    </aside>
  );
}
