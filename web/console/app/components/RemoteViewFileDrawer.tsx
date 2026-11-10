"use client";

import { useCallback, useContext, useEffect, useRef, useState, type DragEvent } from "react";
import { useTranslations } from "next-intl";
import { ArrowUp, ChevronRight, Download, Eye, EyeOff, RefreshCw, Upload, X } from "lucide-react";
import { listFiles } from "../[locale]/(console)/files/fileOpsClient";
import type { FileEntry } from "../[locale]/(console)/files/fileOpsClient";
import { formatSize } from "../[locale]/(console)/files/filesPageUtils";
import { UploadReplaceDialog } from "../[locale]/(console)/files/FilesOverlays";
import { joinPath, namesBelow, parentPath, pathStyleOf } from "../[locale]/(console)/files/remotePaths";
import { useUploadReplace } from "../[locale]/(console)/files/useUploadReplace";
import { useFileDownload } from "../hooks/useFileDownload";
import { ToolbarPortalContext } from "./remote-toolbar/ToolbarButtons";

type Props = {
  nodeId: string;
  open: boolean;
  onClose: () => void;
};

/**
 * Files on the remote machine, over the right of the screen: one folder (home first), Upload from this computer (or
 * drop files on the drawer), Download by choosing a file, with the transfer's progress at the bottom. An upload over
 * a file of the same name asks first. Goes through the device's agent.
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
  // In fullscreen only the viewer is shown, so the Replace dialog is put inside it.
  const dialogContainer = useContext(ToolbarPortalContext);

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

  const startUpload = useCallback(
    (files: File[], dir: string) => {
      const file = files[0];
      if (!file) return;
      setUpload({ name: file.name, percent: 0 });
      const xhr = new XMLHttpRequest();
      xhr.open("POST", `/api/files/${encodeURIComponent(nodeId)}/upload?path=${encodeURIComponent(joinPath(dir, file.name))}`);
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) setUpload({ name: file.name, percent: Math.round((event.loaded / event.total) * 100) });
      };
      xhr.onload = () => {
        setUpload(null);
        void fetchEntries(dir);
      };
      xhr.onerror = () => {
        setUpload(null);
        setError(t("uploadFailed"));
      };
      xhr.send(file);
    },
    [nodeId, fetchEntries, t],
  );

  // The drawer uploads one file. When the folder has a file of that name, it waits for Replace or Cancel.
  const listFolder = useCallback(async (dir: string) => (await listFiles(nodeId, dir, { showHidden: true })).entries, [nodeId]);
  const uploadUnread = useCallback(() => setError(t("listFailed")), [t]);
  const replace = useUploadReplace({
    held: { path: currentPath, entries, showHidden, ready: !loading && !error },
    list: listFolder,
    upload: startUpload,
    onUnread: uploadUnread,
  });
  const { ask: replaceAsk, start: startReplaceCheck, cancel: cancelReplace } = replace;
  const handleUpload = (files: FileList | File[]) => startReplaceCheck(Array.from(files).slice(0, 1), currentPath);

  useEffect(() => {
    if (!open) cancelReplace();
  }, [open, cancelReplace]);

  useEffect(() => {
    if (!open) return undefined;
    const onKey = (event: KeyboardEvent) => {
      // Escape closes the Replace dialog first, then the drawer.
      if (event.key === "Escape" && !replaceAsk) {
        onClose();
        event.stopPropagation();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose, replaceAsk]);

  if (!open) return null;

  // The folders from the shared folder down to the open one; a Windows device answers with backslash paths.
  const segments = namesBelow(currentPath, homePath) ?? [];
  const atHome = segments.length === 0;
  const separator = pathStyleOf(currentPath) === "windows" ? "\\" : "/";
  const sorted = [...entries].sort((a, b) => (a.is_dir !== b.is_dir ? (a.is_dir ? -1 : 1) : a.name.localeCompare(b.name)));
  const goUp = () => {
    const parent = parentPath(currentPath);
    navigateTo(namesBelow(parent, homePath)?.length ? parent : "");
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
              <span aria-hidden="true">{separator}</span>
              <button
                type="button"
                onClick={() => navigateTo(segments.slice(0, i + 1).reduce(joinPath, homePath))}
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
                onClick={() => (entry.is_dir ? navigateTo(joinPath(currentPath, entry.name)) : void downloadFile(joinPath(currentPath, entry.name)))}
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
      <UploadReplaceDialog
        ask={replaceAsk}
        where={replaceAsk?.dir ?? ""}
        onCancel={cancelReplace}
        onSkip={replace.skip}
        onReplace={replace.replace}
        container={dialogContainer}
      />
    </aside>
  );
}
