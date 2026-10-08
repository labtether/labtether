"use client";
import { useEffect, useRef, useState } from "react";
import {
  ClipboardCopy,
  ClipboardPaste,
  Download,
  FolderOpen,
  Loader2,
} from "lucide-react";
import type { RemoteViewToolbarProps } from "./types";
import { IconButton } from "./ToolbarButtons";

export function useToolbarTransferState(
  clipboardLastSync: RemoteViewToolbarProps["clipboardLastSync"] = "idle",
) {
  const [clipboardFlash, setClipboardFlash] = useState<
    "success" | "error" | null
  >(null);
  const [showDownloadInput, setShowDownloadInput] = useState(false);
  const [downloadPath, setDownloadPath] = useState("");
  const downloadInputRef = useRef<HTMLInputElement>(null);
  const clipboardFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const prevLastSync = useRef(clipboardLastSync);
  useEffect(() => {
    if (
      clipboardLastSync !== prevLastSync.current &&
      clipboardLastSync !== "idle"
    ) {
      setClipboardFlash(clipboardLastSync);
      if (clipboardFlashTimerRef.current)
        clearTimeout(clipboardFlashTimerRef.current);
      clipboardFlashTimerRef.current = setTimeout(
        () => setClipboardFlash(null),
        1500,
      );
    }
    prevLastSync.current = clipboardLastSync;
  }, [clipboardLastSync]);
  useEffect(
    () => () => {
      if (clipboardFlashTimerRef.current)
        clearTimeout(clipboardFlashTimerRef.current);
    },
    [],
  );
  return {
    clipboardFlash,
    showDownloadInput,
    setShowDownloadInput,
    downloadPath,
    setDownloadPath,
    downloadInputRef,
  };
}

type TransferProps = Pick<
  RemoteViewToolbarProps,
  | "onClipboardPull"
  | "onClipboardPush"
  | "clipboardSyncing"
  | "onFileDrawerToggle"
  | "fileDrawerOpen"
  | "fileDownloading"
  | "onDownloadFile"
> & {
  transfer: ReturnType<typeof useToolbarTransferState>;
};
export function ToolbarTransferControls({
  onClipboardPull,
  onClipboardPush,
  clipboardSyncing = false,
  onFileDrawerToggle,
  fileDrawerOpen = false,
  fileDownloading = false,
  onDownloadFile,
  transfer,
}: TransferProps) {
  const {
    clipboardFlash,
    showDownloadInput,
    setShowDownloadInput,
    downloadPath,
    setDownloadPath,
    downloadInputRef,
  } = transfer;
  const clipboardControls =
    onClipboardPull || onClipboardPush ? (
      <div
        className="flex items-center gap-0.5 rounded-md transition-[box-shadow,border-color,opacity] duration-300"
        style={{
          boxShadow:
            clipboardFlash === "success"
              ? "0 0 6px var(--ok)"
              : clipboardFlash === "error"
                ? "0 0 6px var(--bad)"
                : "none",
          border:
            clipboardFlash === "success"
              ? "1px solid var(--ok)"
              : clipboardFlash === "error"
                ? "1px solid var(--bad)"
                : "1px solid transparent",
          opacity: clipboardSyncing ? 0.6 : 1,
          animation: clipboardSyncing
            ? "pulse 1.5s ease-in-out infinite"
            : "none",
        }}
      >
        {onClipboardPull && (
          <IconButton
            onClick={onClipboardPull}
            title="Pull clipboard from remote"
          >
            <ClipboardPaste className="w-3.5 h-3.5" />
          </IconButton>
        )}
        {onClipboardPush && (
          <IconButton
            onClick={onClipboardPush}
            title="Push clipboard to remote"
          >
            <ClipboardCopy className="w-3.5 h-3.5" />
          </IconButton>
        )}
      </div>
    ) : null;
  const downloadControls = onFileDrawerToggle ? (
    <IconButton
      onClick={onFileDrawerToggle}
      active={fileDrawerOpen}
      title={fileDrawerOpen ? "Close file browser" : "Browse & transfer files"}
    >
      {fileDownloading ? (
        <Loader2 className="w-3.5 h-3.5 animate-spin" />
      ) : (
        <FolderOpen className="w-3.5 h-3.5" />
      )}
    </IconButton>
  ) : onDownloadFile ? (
    <>
      <IconButton
        onClick={() => {
          setShowDownloadInput((prev) => !prev);
          if (!showDownloadInput) {
            setTimeout(() => downloadInputRef.current?.focus(), 0);
          }
        }}
        active={showDownloadInput}
        title="Download file from remote"
      >
        {fileDownloading ? (
          <Loader2 className="w-3.5 h-3.5 animate-spin" />
        ) : (
          <Download className="w-3.5 h-3.5" />
        )}
      </IconButton>
      {showDownloadInput && (
        <input
          ref={downloadInputRef}
          type="text"
          placeholder="/path/to/file"
          value={downloadPath}
          onChange={(e) => setDownloadPath(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && downloadPath.trim()) {
              onDownloadFile(downloadPath.trim());
              setDownloadPath("");
              setShowDownloadInput(false);
            } else if (e.key === "Escape") {
              setShowDownloadInput(false);
              setDownloadPath("");
            }
            e.stopPropagation();
          }}
          onBlur={() => {
            setShowDownloadInput(false);
            setDownloadPath("");
          }}
          className="rounded-md border border-white/20 bg-white/5 text-white/90 placeholder:text-white/30 px-2 py-0.5 outline-none focus:border-[var(--accent)] focus:ring-1 focus:ring-[var(--accent)]"
          style={{ fontSize: 11, width: 160 }}
        />
      )}
    </>
  ) : null;
  return (
    <>
      {clipboardControls}
      {downloadControls}
    </>
  );
}
