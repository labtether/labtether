"use client";

import { useCallback, useRef, useState } from "react";
import type { ReactNode } from "react";

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
      title={disabled ? "File transfer requires LabTether agent" : undefined}
      onDragEnter={onDragEnter}
      onDragLeave={onDragLeave}
      onDragOver={onDragOver}
      onDrop={onDrop}
    >
      {children}
      {dragging && (
        <div className="fileDropZone">
          <span>Drop files to upload to {targetDir}</span>
        </div>
      )}
      {upload && (
        <div className="fileProgressOverlay">
          {queueInfo && (
            <span style={{ opacity: 0.55, fontSize: "0.75rem" }}>
              File {queueInfo.index} of {queueInfo.total}
            </span>
          )}
          <span>{upload.name}</span>
          {upload.status === "uploading" && (
            <span style={{ opacity: 0.7, fontSize: "0.8rem" }}>
              {pct}% &nbsp;&middot;&nbsp; {formatBytes(upload.loaded)} /{" "}
              {formatBytes(upload.total)}
              {speedBytesPerSec > 0 && (
                <>
                  {" "}
                  &nbsp;&middot;&nbsp;{" "}
                  {formatBytes(Math.round(speedBytesPerSec))}/s
                </>
              )}
            </span>
          )}
          {upload.status === "done" && (
            <span style={{ opacity: 0.7, fontSize: "0.8rem" }}>
              {formatBytes(upload.total)} &nbsp;&middot;&nbsp; Done
            </span>
          )}
          {upload.status === "error" && (
            <span
              style={{ opacity: 0.7, fontSize: "0.8rem", color: "var(--bad)" }}
            >
              Upload failed
            </span>
          )}
          <div className="fileProgressBar">
            <div
              className="fileProgressFill"
              style={{
                width: upload.status === "uploading" ? `${pct}%` : "100%",
                backgroundColor:
                  upload.status === "error"
                    ? "var(--bad)"
                    : upload.status === "done"
                      ? "var(--ok)"
                      : "var(--accent)",
                opacity: upload.status === "uploading" ? 0.65 : 1,
              }}
            />
          </div>
        </div>
      )}
    </div>
  );
}
