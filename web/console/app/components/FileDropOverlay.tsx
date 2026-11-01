"use client";

import { useCallback, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useTranslations } from "next-intl";

interface FileDropOverlayProps {
  assetId: string;
  children: ReactNode;
  disabled?: boolean;
  targetDir?: string;
  uploadFile?: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;
}

type UploadState = {
  name: string;
  status: "uploading" | "done" | "error";
  loaded: number;
  total: number;
  startedAt: number;
};

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024)
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

export default function FileDropOverlay({
  assetId,
  children,
  disabled = false,
  targetDir = "~",
  uploadFile: customUploadFile,
}: FileDropOverlayProps) {
  const t = useTranslations("files.drop");
  const [dragging, setDragging] = useState(false);
  const [upload, setUpload] = useState<UploadState | null>(null);
  const [queueInfo, setQueueInfo] = useState<{
    index: number;
    total: number;
  } | null>(null);
  const dragCounterRef = useRef(0);

  const onDragEnter = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      if (disabled) return;
      dragCounterRef.current += 1;
      if (event.dataTransfer.types.includes("Files")) {
        setDragging(true);
      }
    },
    [disabled],
  );

  const onDragLeave = useCallback((event: React.DragEvent) => {
    event.preventDefault();
    dragCounterRef.current -= 1;
    if (dragCounterRef.current <= 0) {
      dragCounterRef.current = 0;
      setDragging(false);
    }
  }, []);

  const onDragOver = useCallback(
    (event: React.DragEvent) => {
      event.preventDefault();
      if (disabled) return;
      event.dataTransfer.dropEffect = "copy";
    },
    [disabled],
  );

  const uploadFileViaHTTP = useCallback(
    (file: File, url: string): Promise<void> => {
      return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        const startedAt = Date.now();

        xhr.upload.addEventListener("progress", (event) => {
          if (event.lengthComputable) {
            setUpload((prev) =>
              prev
                ? { ...prev, loaded: event.loaded, total: event.total }
                : null,
            );
          }
        });

        xhr.addEventListener("load", () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            setUpload((prev) =>
              prev
                ? {
                    ...prev,
                    status: "done",
                    loaded: file.size,
                    total: file.size,
                  }
                : null,
            );
            resolve();
          } else {
            reject(new Error(`upload failed (${xhr.status})`));
          }
        });

        xhr.addEventListener("error", () => {
          reject(new Error("upload failed (network error)"));
        });

        xhr.addEventListener("abort", () => {
          reject(new Error("upload aborted"));
        });

        xhr.open("POST", url);
        xhr.setRequestHeader(
          "Content-Type",
          file.type || "application/octet-stream",
        );

        setUpload({
          name: file.name,
          status: "uploading",
          loaded: 0,
          total: file.size,
          startedAt,
        });

        xhr.send(file);
      });
    },
    [],
  );

  const onDrop = useCallback(
    async (event: React.DragEvent) => {
      event.preventDefault();
      dragCounterRef.current = 0;
      setDragging(false);
      if (disabled) return;

      const files = Array.from(event.dataTransfer.files);
      if (files.length === 0) return;

      const total = files.length;

      for (let i = 0; i < files.length; i++) {
        const file = files[i];
        setQueueInfo(total > 1 ? { index: i + 1, total } : null);

        const normalizedDir =
          targetDir.endsWith("\\") || targetDir.endsWith("/")
            ? targetDir.slice(0, -1)
            : targetDir;
        const separator = normalizedDir.includes("\\") ? "\\" : "/";
        const targetPath = `${normalizedDir}${separator}${file.name}`;

        try {
          if (customUploadFile) {
            setUpload({
              name: file.name,
              status: "uploading",
              loaded: 0,
              total: file.size,
              startedAt: Date.now(),
            });
            await customUploadFile(file, targetPath, (loaded, totalBytes) => {
              setUpload((prev) =>
                prev ? { ...prev, loaded, total: totalBytes } : null,
              );
            });
            setUpload((prev) =>
              prev
                ? {
                    ...prev,
                    status: "done",
                    loaded: file.size,
                    total: file.size,
                  }
                : null,
            );
          } else {
            const url = `/api/files/${encodeURIComponent(assetId)}/upload?path=${encodeURIComponent(targetPath)}`;
            await uploadFileViaHTTP(file, url);
          }
        } catch {
          setUpload((prev) => (prev ? { ...prev, status: "error" } : null));
        }

        await new Promise((resolve) => setTimeout(resolve, 1200));
      }

      setUpload(null);
      setQueueInfo(null);
    },
    [assetId, customUploadFile, disabled, targetDir, uploadFileViaHTTP],
  );

  const pct =
    upload && upload.total > 0
      ? Math.round((upload.loaded / upload.total) * 100)
      : 0;

  const elapsedSec =
    upload && upload.startedAt > 0 ? (Date.now() - upload.startedAt) / 1000 : 0;

  const speedBytesPerSec =
    upload && elapsedSec > 0 ? upload.loaded / elapsedSec : 0;

  return (
    <div
      className="relative h-full w-full"
      onDragEnter={onDragEnter}
      onDragLeave={onDragLeave}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      {children}
      {dragging && (
        <div className="fx-rdrop">
          <span>{t("dropTo", { path: targetDir })}</span>
        </div>
      )}
      {upload && (
        <div className="fx-rprog" role="status">
          {queueInfo && (
            <span className="fx-rprog-q">{t("queue", { index: queueInfo.index, total: queueInfo.total })}</span>
          )}
          <span className="fx-rprog-n">{upload.name}</span>
          {upload.status === "uploading" && (
            <span className="fx-rprog-d">
              {speedBytesPerSec > 0
                ? t("progressSpeed", { pct, done: formatBytes(upload.loaded), total: formatBytes(upload.total), speed: formatBytes(Math.round(speedBytesPerSec)) })
                : t("progress", { pct, done: formatBytes(upload.loaded), total: formatBytes(upload.total) })}
            </span>
          )}
          {upload.status === "done" && (
            <span className="fx-rprog-d">{t("done", { total: formatBytes(upload.total) })}</span>
          )}
          {upload.status === "error" && (
            <span className="fx-rprog-d bad">{t("failed")}</span>
          )}
          <div className="fx-meter">
            <i className={upload.status === "error" ? "bad" : upload.status === "done" ? "ok" : undefined} style={{ width: upload.status === "uploading" ? `${pct}%` : "100%" }} />
          </div>
        </div>
      )}
    </div>
  );
}
