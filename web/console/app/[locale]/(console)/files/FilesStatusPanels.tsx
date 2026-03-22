"use client";

import { AlertTriangle } from "lucide-react";
import { Button } from "../../../components/ui/Button";
import { Card } from "../../../components/ui/Card";
import { Input } from "../../../components/ui/Input";

type TransferProgress = {
  fileName: string;
  loaded: number;
  total: number;
};

type FilesStatusPanelsProps = {
  showMkdir: boolean;
  mkdirName: string;
  onMkdirNameChange: (value: string) => void;
  onCreateDir: () => void;
  onCancelMkdir: () => void;
  uploadProgress: TransferProgress | null;
  onCancelUpload: () => void;
  downloadProgress: TransferProgress | null;
  onCancelDownload: () => void;
  error: string | null;
  showNoAgentWarning: boolean;
};

export function FilesStatusPanels({
  showMkdir,
  mkdirName,
  onMkdirNameChange,
  onCreateDir,
  onCancelMkdir,
  uploadProgress,
  onCancelUpload,
  downloadProgress,
  onCancelDownload,
  error,
  showNoAgentWarning,
}: FilesStatusPanelsProps) {
  return (
    <>
      {showMkdir && (
        <Card className="flex items-center gap-3">
          <Input
            type="text"
            placeholder="Folder name..."
            value={mkdirName}
            onChange={(event) => onMkdirNameChange(event.target.value)}
            onKeyDown={(event) => event.key === "Enter" && onCreateDir()}
            autoFocus
          />
          <Button size="sm" variant="primary" onClick={onCreateDir}>Create</Button>
          <Button size="sm" onClick={onCancelMkdir}>Cancel</Button>
        </Card>
      )}

      {uploadProgress && (
        <Card className="space-y-2">
          <div className="flex items-center gap-3">
            <span className="flex-1 text-sm truncate">Uploading: {uploadProgress.fileName}</span>
            <span className="text-sm font-medium tabular-nums font-mono">
              {uploadProgress.total > 0 ? Math.round((uploadProgress.loaded / uploadProgress.total) * 100) : 0}%
            </span>
            <Button size="sm" variant="danger" onClick={onCancelUpload}>Cancel</Button>
          </div>
          <div className="h-1 rounded-full bg-[var(--surface)] overflow-hidden">
            <div
              className="h-full rounded-full bg-[var(--accent)] transition-[width] duration-300"
              style={{
                width: `${uploadProgress.total > 0 ? (uploadProgress.loaded / uploadProgress.total) * 100 : 0}%`,
                boxShadow: "0 0 8px var(--accent-glow)",
              }}
            />
          </div>
        </Card>
      )}

      {downloadProgress && (
        <Card className="space-y-2">
          <div className="flex items-center gap-3">
            <span className="flex-1 text-sm truncate">Downloading: {downloadProgress.fileName}</span>
            <span className="text-sm font-medium tabular-nums font-mono">
              {downloadProgress.total > 0
                ? `${Math.round((downloadProgress.loaded / downloadProgress.total) * 100)}%`
                : downloadProgress.loaded > 0
                  ? `${(downloadProgress.loaded / 1024 / 1024).toFixed(1)} MB`
                  : "Starting..."}
            </span>
            <Button size="sm" variant="danger" onClick={onCancelDownload}>Cancel</Button>
          </div>
          <div className="h-1 rounded-full bg-[var(--surface)] overflow-hidden">
            <div
              className={`h-full rounded-full bg-[var(--accent)] transition-[width] duration-300${downloadProgress.total <= 0 ? " animate-pulse" : ""}`}
              style={{
                ...(downloadProgress.total > 0 ? { width: `${(downloadProgress.loaded / downloadProgress.total) * 100}%` } : {}),
                boxShadow: "0 0 8px var(--accent-glow)",
              }}
            />
          </div>
        </Card>
      )}

      {error && (
        <Card className="flex items-start gap-3 border-[var(--bad)]/30">
          <AlertTriangle className="w-5 h-5 text-[var(--bad)] flex-shrink-0 mt-0.5" strokeWidth={1.5} />
          <p className="text-sm text-[var(--bad)]">{error}</p>
        </Card>
      )}

      {showNoAgentWarning && (
        <Card className="flex items-start gap-3 border-[var(--bad)]/30">
          <AlertTriangle className="w-5 h-5 text-[var(--bad)] flex-shrink-0 mt-0.5" strokeWidth={1.5} />
          <p className="text-sm text-[var(--bad)]">
            File transfer requires a connected agent. This device does not have an agent connected.
          </p>
        </Card>
      )}
    </>
  );
}
