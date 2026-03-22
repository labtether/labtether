"use client";

import {
  listConnectionFiles,
  downloadConnectionFile,
  uploadConnectionFile,
  startUploadConnectionFileXhr,
  mkdirConnection,
  deleteConnectionFile,
  renameConnectionFile,
  copyConnectionFile,
  type ConnectionFileEntry,
} from "./fileConnectionsClient";
import type { FileSource } from "./useFileTabsState";

export type FileEntry = {
  name: string;
  size: number;
  mode: string;
  mod_time: string;
  is_dir: boolean;
};

// ---------------------------------------------------------------------------
// Unified file entry type that both sources produce
// ---------------------------------------------------------------------------

export interface UnifiedFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  mode?: string;
  permissions?: string;
}

// ---------------------------------------------------------------------------
// Mappers
// ---------------------------------------------------------------------------

function agentEntryToUnified(entry: FileEntry, parentPath: string): UnifiedFileEntry {
  const path = parentPath === "/" ? `/${entry.name}` : `${parentPath}/${entry.name}`;
  return {
    name: entry.name,
    path,
    is_dir: entry.is_dir,
    size: entry.size,
    mod_time: entry.mod_time,
    mode: entry.mode,
  };
}

function connectionEntryToUnified(entry: ConnectionFileEntry): UnifiedFileEntry {
  return {
    name: entry.name,
    path: entry.path,
    is_dir: entry.is_dir,
    size: entry.size,
    mod_time: entry.mod_time,
    permissions: entry.permissions,
  };
}

// ---------------------------------------------------------------------------
// Unified operations that delegate to agent or connection APIs
// ---------------------------------------------------------------------------

export type UnifiedListFilesOptions = {
  showHidden?: boolean;
  fallbackError?: ErrorFallback;
};

export async function unifiedListFiles(
  source: FileSource,
  path: string,
  options?: UnifiedListFilesOptions,
): Promise<{ path?: string; entries: UnifiedFileEntry[] }> {
  if (source.type === "agent") {
    const data = await listFiles(source.assetId, path, {
      showHidden: options?.showHidden,
      fallbackError: options?.fallbackError,
    });
    const resolvedPath = data.path ?? path;
    return {
      path: data.path,
      entries: data.entries.map((e) => agentEntryToUnified(e, resolvedPath)),
    };
  }
  const entries = await listConnectionFiles(source.connectionId, path, options?.showHidden ?? false);
  return {
    entries: entries.map(connectionEntryToUnified),
  };
}

export async function unifiedDownloadFileBlob(
  source: FileSource,
  path: string,
  options?: DownloadBlobOptions,
): Promise<Blob> {
  if (source.type === "agent") {
    return downloadFileBlob(source.assetId, path, options);
  }
  return downloadConnectionFile(source.connectionId, path);
}

export function unifiedDownloadFileResponse(
  source: FileSource,
  path: string,
  options?: DownloadResponseOptions,
): Promise<Response> {
  if (source.type === "agent") {
    return downloadFileResponse(source.assetId, path, options);
  }
  // Connection download doesn't support abort signals natively,
  // but we return a fetch response for consistency.
  const params = new URLSearchParams({ path });
  return fetch(`/api/file-connections/${encodeURIComponent(source.connectionId)}/download?${params.toString()}`, {
    signal: options?.signal,
  });
}

export async function unifiedCreateDirectory(
  source: FileSource,
  path: string,
  options?: OperationOptions,
): Promise<void> {
  if (source.type === "agent") {
    return createDirectory(source.assetId, path, options);
  }
  return mkdirConnection(source.connectionId, path);
}

export async function unifiedDeleteFilePath(
  source: FileSource,
  path: string,
  options?: OperationOptions,
): Promise<void> {
  if (source.type === "agent") {
    return deleteFilePath(source.assetId, path, options);
  }
  return deleteConnectionFile(source.connectionId, path);
}

export async function unifiedRenameFilePath(
  source: FileSource,
  oldPath: string,
  newPath: string,
  options?: OperationOptions,
): Promise<void> {
  if (source.type === "agent") {
    return renameFilePath(source.assetId, oldPath, newPath, options);
  }
  return renameConnectionFile(source.connectionId, oldPath, newPath);
}

export async function unifiedCopyFilePath(
  source: FileSource,
  srcPath: string,
  dstPath: string,
  options?: OperationOptions,
): Promise<void> {
  if (source.type === "agent") {
    return copyFilePath(source.assetId, srcPath, dstPath, options);
  }
  return copyConnectionFile(source.connectionId, srcPath, dstPath);
}

export async function unifiedUploadFileViaFetch(
  source: FileSource,
  path: string,
  file: File,
  options?: OperationOptions,
): Promise<void> {
  if (source.type === "agent") {
    return uploadFileViaFetch(source.assetId, path, file, options);
  }
  return uploadConnectionFile(source.connectionId, path, file);
}

export function unifiedStartUploadFileXhr(
  source: FileSource,
  path: string,
  file: File,
  handlers: UploadXhrHandlers,
): XMLHttpRequest {
  if (source.type === "agent") {
    return startUploadFileXhr(source.assetId, path, file, handlers);
  }
  return startUploadConnectionFileXhr(source.connectionId, path, file, handlers);
}

type ErrorPayload = {
  error?: string;
};

type ListFilesPayload = {
  path?: string;
  entries?: FileEntry[];
  error?: string;
};

type ErrorFallback = string | ((status: number) => string);

type ListFilesOptions = {
  showHidden?: boolean;
  fallbackError?: ErrorFallback;
};

type OperationOptions = {
  fallbackError?: ErrorFallback;
};

type DownloadResponseOptions = {
  signal?: AbortSignal;
};

type DownloadBlobOptions = DownloadResponseOptions & OperationOptions;

type UploadXhrHandlers = {
  onProgress?: (event: ProgressEvent<EventTarget>) => void;
  onLoad?: (xhr: XMLHttpRequest) => void;
  onError?: () => void;
  onAbort?: () => void;
};

function filesOperationURL(assetID: string, operation: string, params: URLSearchParams): string {
  return `/api/files/${encodeURIComponent(assetID)}/${operation}?${params.toString()}`;
}

function normalizeFileEntryList(value: unknown): FileEntry[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((entry) => {
      if (!entry || typeof entry !== "object" || Array.isArray(entry)) {
        return null;
      }
      const raw = entry as Record<string, unknown>;
      return {
        name: typeof raw.name === "string" ? raw.name : "",
        size: typeof raw.size === "number" && Number.isFinite(raw.size) ? raw.size : 0,
        mode: typeof raw.mode === "string" ? raw.mode : "",
        mod_time: typeof raw.mod_time === "string" ? raw.mod_time : "",
        is_dir: typeof raw.is_dir === "boolean" ? raw.is_dir : false,
      };
    })
    .filter((entry): entry is FileEntry => entry !== null);
}

function resolveFallbackMessage(
  status: number,
  fallback: ErrorFallback | undefined,
  defaultMessage: string,
): string {
  if (typeof fallback === "function") {
    return fallback(status);
  }
  if (typeof fallback === "string" && fallback.trim() !== "") {
    return fallback;
  }
  return defaultMessage;
}

async function responseError(response: Response, fallbackMessage: string): Promise<Error> {
  const payload = await response.json().catch(() => ({} as ErrorPayload));
  return new Error(payload.error || fallbackMessage);
}

async function requireOK(
  response: Response,
  fallback: ErrorFallback | undefined,
  defaultMessage: string,
): Promise<void> {
  if (response.ok) {
    return;
  }
  const message = resolveFallbackMessage(response.status, fallback, defaultMessage);
  throw await responseError(response, message);
}

export async function listFiles(assetID: string, path: string, options?: ListFilesOptions): Promise<{
  path?: string;
  entries: FileEntry[];
}> {
  const params = new URLSearchParams({ path });
  if (options?.showHidden) {
    params.set("show_hidden", "true");
  }

  const response = await fetch(filesOperationURL(assetID, "list", params), {
    cache: "no-store",
  });
  const payload = await response.json().catch(() => ({} as ListFilesPayload));
  if (!response.ok) {
    const message = resolveFallbackMessage(response.status, options?.fallbackError, `Failed (${response.status})`);
    throw new Error(payload.error || message);
  }

  return {
    path: typeof payload.path === "string" ? payload.path : undefined,
    entries: normalizeFileEntryList(payload.entries),
  };
}

export function downloadFileResponse(
  assetID: string,
  path: string,
  options?: DownloadResponseOptions,
): Promise<Response> {
  const params = new URLSearchParams({ path });
  return fetch(filesOperationURL(assetID, "download", params), {
    signal: options?.signal,
  });
}

export async function downloadFileBlob(
  assetID: string,
  path: string,
  options?: DownloadBlobOptions,
): Promise<Blob> {
  const response = await downloadFileResponse(assetID, path, { signal: options?.signal });
  await requireOK(response, options?.fallbackError, `download failed (${response.status})`);
  return response.blob();
}

export async function createDirectory(
  assetID: string,
  path: string,
  options?: OperationOptions,
): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(filesOperationURL(assetID, "mkdir", params), {
    method: "POST",
  });
  await requireOK(response, options?.fallbackError, `mkdir failed (${response.status})`);
}

export async function deleteFilePath(
  assetID: string,
  path: string,
  options?: OperationOptions,
): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(filesOperationURL(assetID, "delete", params), {
    method: "DELETE",
  });
  await requireOK(response, options?.fallbackError, `delete failed (${response.status})`);
}

export async function renameFilePath(
  assetID: string,
  oldPath: string,
  newPath: string,
  options?: OperationOptions,
): Promise<void> {
  const params = new URLSearchParams({ old_path: oldPath, new_path: newPath });
  const response = await fetch(filesOperationURL(assetID, "rename", params), {
    method: "POST",
  });
  await requireOK(response, options?.fallbackError, `rename failed (${response.status})`);
}

export async function copyFilePath(
  assetID: string,
  srcPath: string,
  dstPath: string,
  options?: OperationOptions,
): Promise<void> {
  const params = new URLSearchParams({ src_path: srcPath, dst_path: dstPath });
  const response = await fetch(filesOperationURL(assetID, "copy", params), {
    method: "POST",
  });
  await requireOK(response, options?.fallbackError, `copy failed (${response.status})`);
}

export async function uploadFileViaFetch(
  assetID: string,
  path: string,
  file: File,
  options?: OperationOptions,
): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(filesOperationURL(assetID, "upload", params), {
    method: "POST",
    body: file,
  });
  await requireOK(response, options?.fallbackError, `upload failed (${response.status})`);
}

export function startUploadFileXhr(
  assetID: string,
  path: string,
  file: File,
  handlers: UploadXhrHandlers,
): XMLHttpRequest {
  const params = new URLSearchParams({ path });
  const xhr = new XMLHttpRequest();

  xhr.open("POST", filesOperationURL(assetID, "upload", params));
  if (handlers.onProgress) {
    xhr.upload.onprogress = handlers.onProgress;
  }
  if (handlers.onLoad) {
    xhr.onload = () => handlers.onLoad?.(xhr);
  }
  if (handlers.onError) {
    xhr.onerror = () => handlers.onError?.();
  }
  if (handlers.onAbort) {
    xhr.onabort = () => handlers.onAbort?.();
  }

  xhr.send(file);
  return xhr;
}
