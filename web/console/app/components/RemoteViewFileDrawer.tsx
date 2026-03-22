"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  X, Upload, RefreshCw, FolderOpen, ChevronRight, Eye, EyeOff,
  ArrowUp, Download, Loader2,
} from "lucide-react";
import { listFiles } from "../[locale]/(console)/files/fileOpsClient";
import type { FileEntry } from "../[locale]/(console)/files/fileOpsClient";
import { fileIconComponent, formatSize } from "../[locale]/(console)/files/filesPageUtils";
import { useFileDownload } from "../hooks/useFileDownload";

type Props = {
  nodeId: string;
  open: boolean;
  onClose: () => void;
};

export function RemoteViewFileDrawer({ nodeId, open, onClose }: Props) {
  const [currentPath, setCurrentPath] = useState("");
  const [homePath, setHomePath] = useState("");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showHidden, setShowHidden] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<{ name: string; percent: number } | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const dragCounterRef = useRef(0);
  const { downloading, progress, filename, downloadFile } = useFileDownload(nodeId);

  const fetchEntries = useCallback(async (path: string) => {
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
      setError(err instanceof Error ? err.message : "Failed to list files");
      setEntries([]);
    } finally {
      setLoading(false);
    }
  }, [nodeId, showHidden, homePath]);

  useEffect(() => {
    if (open) void fetchEntries(currentPath || "");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, showHidden]);

  const navigateTo = useCallback((path: string) => {
    setCurrentPath(path);
    void fetchEntries(path);
  }, [fetchEntries]);

  const handleFileClick = useCallback((entry: FileEntry) => {
    const childPath = currentPath.endsWith("/")
      ? `${currentPath}${entry.name}`
      : `${currentPath}/${entry.name}`;
    if (entry.is_dir) {
      navigateTo(childPath);
    } else {
      void downloadFile(childPath);
    }
  }, [currentPath, navigateTo, downloadFile]);

  // Build breadcrumb segments relative to home
  const relativePath = homePath && currentPath.startsWith(homePath)
    ? currentPath.slice(homePath.length)
    : currentPath;
  const breadcrumbSegments = relativePath.split("/").filter(Boolean);
  const isAtHome = !relativePath || relativePath === "/";

  const handleUpload = useCallback((files: FileList | File[]) => {
    const fileArray = Array.from(files);
    if (fileArray.length === 0) return;
    const file = fileArray[0];
    const uploadPath = currentPath.endsWith("/")
      ? `${currentPath}${file.name}`
      : `${currentPath}/${file.name}`;
    setUploadProgress({ name: file.name, percent: 0 });
    const xhr = new XMLHttpRequest();
    xhr.open("POST", `/api/files/${encodeURIComponent(nodeId)}/upload?path=${encodeURIComponent(uploadPath)}`);
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        setUploadProgress({ name: file.name, percent: Math.round((e.loaded / e.total) * 100) });
      }
    };
    xhr.onload = () => { setUploadProgress(null); void fetchEntries(currentPath); };
    xhr.onerror = () => { setUploadProgress(null); setError("Upload failed"); };
    xhr.send(file);
  }, [nodeId, currentPath, fetchEntries]);

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounterRef.current++;
    if (e.dataTransfer.types.includes("Files")) setDragOver(true);
  }, []);
  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounterRef.current--;
    if (dragCounterRef.current === 0) setDragOver(false);
  }, []);
  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCounterRef.current = 0;
    setDragOver(false);
    if (e.dataTransfer.files.length > 0) handleUpload(e.dataTransfer.files);
  }, [handleUpload]);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") { onClose(); e.stopPropagation(); }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  if (!open) return null;

  const sorted = [...entries].sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  const hasTransfer = downloading || !!uploadProgress;

  return (
    <div
      className="absolute top-0 right-0 bottom-0 z-30 flex flex-col"
      style={{
        width: 300,
        background: "rgba(12, 12, 16, 0.96)",
        backdropFilter: "blur(12px)",
        borderLeft: "1px solid rgba(255, 255, 255, 0.06)",
        animation: "slideInRight 0.15s ease-out",
      }}
      onDragEnter={handleDragEnter}
      onDragOver={(e) => e.preventDefault()}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <style>{`@keyframes slideInRight { from { transform: translateX(100%); opacity: 0; } to { transform: translateX(0); opacity: 1; } }`}</style>

      {/* ── Header ── */}
      <div className="flex items-center justify-between px-3 h-9 shrink-0" style={{ borderBottom: "1px solid rgba(255,255,255,0.06)" }}>
        <span className="text-xs font-semibold tracking-wide uppercase text-white/50">Files</span>
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="flex items-center justify-center w-6 h-6 rounded text-white/30 hover:text-white/70 hover:bg-white/5 transition-colors"
            title="Upload file"
          >
            <Upload size={12} />
          </button>
          <button
            type="button"
            onClick={() => void fetchEntries(currentPath)}
            className="flex items-center justify-center w-6 h-6 rounded text-white/30 hover:text-white/70 hover:bg-white/5 transition-colors"
            title="Refresh"
            disabled={loading}
          >
            <RefreshCw size={12} className={loading ? "animate-spin" : ""} />
          </button>
          <button
            type="button"
            onClick={() => setShowHidden((v) => !v)}
            className="flex items-center justify-center w-6 h-6 rounded text-white/30 hover:text-white/70 hover:bg-white/5 transition-colors"
            title={showHidden ? "Hide hidden files" : "Show hidden files"}
          >
            {showHidden ? <Eye size={12} /> : <EyeOff size={12} />}
          </button>
          <div className="w-px h-3.5 bg-white/8 mx-0.5" />
          <button
            type="button"
            onClick={onClose}
            className="flex items-center justify-center w-6 h-6 rounded text-white/30 hover:text-white/70 hover:bg-white/5 transition-colors"
            title="Close"
          >
            <X size={12} />
          </button>
        </div>
        <input ref={fileInputRef} type="file" className="hidden" onChange={(e) => { if (e.target.files) handleUpload(e.target.files); e.target.value = ""; }} />
      </div>

      {/* ── Breadcrumb ── */}
      <div
        className="flex items-center gap-0 px-3 h-7 shrink-0 overflow-x-auto scrollbar-none"
        style={{ borderBottom: "1px solid rgba(255,255,255,0.04)" }}
      >
        <button
          type="button"
          onClick={() => navigateTo("")}
          className={`text-xs shrink-0 transition-colors ${isAtHome ? "text-white/60 font-medium" : "text-blue-400/80 hover:text-blue-300"}`}
        >
          ~
        </button>
        {breadcrumbSegments.map((segment, idx) => {
          const isLast = idx === breadcrumbSegments.length - 1;
          const targetPath = homePath + "/" + breadcrumbSegments.slice(0, idx + 1).join("/");
          return (
            <span key={idx} className="flex items-center shrink-0">
              <ChevronRight size={9} className="text-white/15 mx-0.5" />
              <button
                type="button"
                onClick={() => navigateTo(targetPath)}
                className={`text-xs transition-colors ${isLast ? "text-white/60 font-medium" : "text-blue-400/80 hover:text-blue-300"}`}
              >
                {segment}
              </button>
            </span>
          );
        })}
      </div>

      {/* ── File list ── */}
      <div className="flex-1 overflow-y-auto relative">
        {/* Drag overlay */}
        {dragOver && (
          <div className="absolute inset-0 z-10 flex items-center justify-center m-2 rounded-lg" style={{ background: "rgba(59, 130, 246, 0.08)", border: "2px dashed rgba(59, 130, 246, 0.3)" }}>
            <div className="text-center">
              <Upload size={20} className="mx-auto mb-1" style={{ color: "rgba(59, 130, 246, 0.5)" }} />
              <p className="text-xs" style={{ color: "rgba(59, 130, 246, 0.7)" }}>Drop to upload here</p>
            </div>
          </div>
        )}

        {loading && entries.length === 0 ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 size={16} className="animate-spin text-white/20" />
          </div>
        ) : error ? (
          <div className="px-4 py-10 text-center">
            <p className="text-xs text-red-400/80 leading-relaxed">{error}</p>
            <button type="button" onClick={() => void fetchEntries(currentPath)} className="mt-3 text-[10px] text-blue-400/80 hover:text-blue-300 transition-colors">
              Try again
            </button>
          </div>
        ) : sorted.length === 0 ? (
          <div className="px-4 py-12 text-center">
            <FolderOpen size={20} className="mx-auto mb-2 text-white/10" />
            <p className="text-xs text-white/25">No files here</p>
          </div>
        ) : (
          <div className="py-0.5">
            {/* Parent directory */}
            {!isAtHome && (
              <button
                type="button"
                onClick={() => {
                  const segments = currentPath.split("/").filter(Boolean);
                  const parent = "/" + segments.slice(0, -1).join("/");
                  navigateTo(parent.length <= (homePath || "").length ? "" : parent);
                }}
                className="flex items-center gap-2.5 w-full px-3 h-7 text-left hover:bg-white/[0.03] transition-colors"
              >
                <ArrowUp size={12} className="text-white/20 shrink-0" />
                <span className="text-xs text-white/35">..</span>
              </button>
            )}

            {sorted.map((entry) => {
              const Icon = fileIconComponent(entry.name, entry.is_dir);
              return (
                <button
                  key={entry.name}
                  type="button"
                  onClick={() => handleFileClick(entry)}
                  className="flex items-center gap-2.5 w-full px-3 h-7 text-left hover:bg-white/[0.03] transition-colors group"
                >
                  <Icon
                    size={13}
                    className={`shrink-0 ${entry.is_dir ? "text-blue-400/50 group-hover:text-blue-400/70" : "text-white/20 group-hover:text-white/35"}`}
                  />
                  <span className="text-xs text-white/70 group-hover:text-white/90 truncate flex-1 min-w-0 transition-colors">
                    {entry.name}
                  </span>
                  {!entry.is_dir && (
                    <span className="text-[10px] text-white/15 shrink-0 tabular-nums">
                      {formatSize(entry.size)}
                    </span>
                  )}
                  {!entry.is_dir && (
                    <Download size={10} className="text-white/0 group-hover:text-white/30 shrink-0 transition-colors" />
                  )}
                  {entry.is_dir && (
                    <ChevronRight size={10} className="text-white/0 group-hover:text-white/20 shrink-0 transition-colors" />
                  )}
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Transfer progress ── */}
      {hasTransfer && (
        <div className="px-3 py-2 shrink-0" style={{ borderTop: "1px solid rgba(255,255,255,0.06)" }}>
          <div className="flex items-center gap-2">
            <div className="flex-1 min-w-0">
              <p className="text-[10px] text-white/40 truncate">
                {downloading ? `${filename}` : `${uploadProgress?.name}`}
              </p>
              <div className="mt-1 h-[3px] rounded-full overflow-hidden" style={{ background: "rgba(255,255,255,0.06)" }}>
                <div
                  className="h-full rounded-full transition-all duration-300 ease-out"
                  style={{
                    width: `${downloading ? (progress >= 0 ? progress : 30) : (uploadProgress?.percent ?? 0)}%`,
                    background: downloading ? "var(--accent)" : "var(--ok)",
                    animation: (downloading && progress < 0) ? "pulse 2s ease-in-out infinite" : undefined,
                  }}
                />
              </div>
            </div>
            <span className="text-[10px] text-white/25 shrink-0 tabular-nums w-8 text-right">
              {downloading ? (progress >= 0 ? `${progress}%` : "") : `${uploadProgress?.percent ?? 0}%`}
            </span>
          </div>
        </div>
      )}

      {/* ── Footer ── */}
      {!hasTransfer && sorted.length > 0 && (
        <div className="px-3 h-6 shrink-0 flex items-center" style={{ borderTop: "1px solid rgba(255,255,255,0.04)" }}>
          <span className="text-[10px] text-white/15 tabular-nums">
            {sorted.filter((e) => e.is_dir).length} folders, {sorted.filter((e) => !e.is_dir).length} files
          </span>
        </div>
      )}
    </div>
  );
}
