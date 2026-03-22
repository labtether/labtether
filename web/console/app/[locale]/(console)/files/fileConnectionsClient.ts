"use client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface FileConnection {
  id: string;
  name: string;
  protocol: string; // "sftp" | "smb" | "ftp" | "webdav"
  host: string;
  port?: number;
  initial_path: string;
  credential_id?: string;
  extra_config?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CreateFileConnectionRequest {
  name: string;
  protocol: string;
  host: string;
  port?: number;
  initial_path?: string;
  username: string;
  secret: string;
  passphrase?: string; // for passphrase-protected SSH private keys
  auth_method?: string; // "password" or "private_key"
  extra_config?: Record<string, unknown>;
}

export interface UpdateFileConnectionRequest {
  name?: string;
  protocol?: string;
  host?: string;
  port?: number;
  initial_path?: string;
  username?: string;
  secret?: string;
  auth_method?: string;
  extra_config?: Record<string, unknown>;
}

export interface TestResult {
  success: boolean;
  latency_ms?: number;
  error?: string;
}

export interface ConnectionFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
  permissions?: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type ErrorPayload = { error?: string };

async function responseError(response: Response, fallbackMessage: string): Promise<Error> {
  const payload: ErrorPayload = await response.json().catch(() => ({}));
  return new Error(payload.error || fallbackMessage);
}

async function requireOK(response: Response, fallbackMessage: string): Promise<void> {
  if (response.ok) return;
  throw await responseError(response, fallbackMessage);
}

function connectionURL(subpath: string, params?: URLSearchParams): string {
  const qs = params ? `?${params.toString()}` : "";
  return `/api/file-connections/${subpath}${qs}`;
}

// ---------------------------------------------------------------------------
// Connection CRUD
// ---------------------------------------------------------------------------

export async function listFileConnections(): Promise<FileConnection[]> {
  const response = await fetch(connectionURL(""), { cache: "no-store" });
  await requireOK(response, `failed to list file connections (${response.status})`);
  const data = await response.json();
  return Array.isArray(data.connections) ? data.connections : [];
}

export async function createFileConnection(
  req: CreateFileConnectionRequest,
): Promise<FileConnection> {
  const response = await fetch(connectionURL(""), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  await requireOK(response, `failed to create file connection (${response.status})`);
  const data = await response.json();
  return data.connection;
}

export async function updateFileConnection(
  id: string,
  req: UpdateFileConnectionRequest,
): Promise<FileConnection> {
  const response = await fetch(connectionURL(encodeURIComponent(id)), {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  await requireOK(response, `failed to update file connection (${response.status})`);
  const data = await response.json();
  return data.connection;
}

export async function deleteFileConnection(id: string): Promise<void> {
  const response = await fetch(connectionURL(encodeURIComponent(id)), {
    method: "DELETE",
  });
  await requireOK(response, `failed to delete file connection (${response.status})`);
}

export async function testFileConnection(id: string): Promise<TestResult> {
  const response = await fetch(connectionURL(`${encodeURIComponent(id)}/test`), {
    method: "POST",
  });
  await requireOK(response, `failed to test file connection (${response.status})`);
  return response.json();
}

export async function testFileConnectionStateless(
  req: CreateFileConnectionRequest,
): Promise<TestResult> {
  const response = await fetch(connectionURL("test"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  await requireOK(response, `failed to test file connection (${response.status})`);
  return response.json();
}

// ---------------------------------------------------------------------------
// File operations on protocol connections
// ---------------------------------------------------------------------------

export async function listConnectionFiles(
  connId: string,
  path: string,
  showHidden: boolean,
): Promise<ConnectionFileEntry[]> {
  const params = new URLSearchParams({ path });
  if (showHidden) {
    params.set("show_hidden", "true");
  }

  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/list`, params),
    { cache: "no-store" },
  );
  await requireOK(response, `failed to list connection files (${response.status})`);
  const data = await response.json();
  if (!Array.isArray(data.entries)) return [];
  return data.entries
    .filter((e: unknown): e is Record<string, unknown> => !!e && typeof e === "object" && !Array.isArray(e))
    .map((e: Record<string, unknown>): ConnectionFileEntry => ({
      name: typeof e.name === "string" ? e.name : "",
      path: typeof e.path === "string" ? e.path : "",
      is_dir: typeof e.is_dir === "boolean" ? e.is_dir : false,
      size: typeof e.size === "number" && Number.isFinite(e.size) ? e.size : 0,
      mod_time: typeof e.mod_time === "string" ? e.mod_time : "",
      permissions: typeof e.permissions === "string" ? e.permissions : undefined,
    }))
    .filter((e: ConnectionFileEntry) => e.name !== "");
}

export async function downloadConnectionFile(
  connId: string,
  path: string,
): Promise<Blob> {
  const params = new URLSearchParams({ path });
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/download`, params),
  );
  await requireOK(response, `download failed (${response.status})`);
  return response.blob();
}

export async function uploadConnectionFile(
  connId: string,
  path: string,
  file: File,
): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/upload`, params),
    { method: "POST", body: file },
  );
  await requireOK(response, `upload failed (${response.status})`);
}

type UploadXhrHandlers = {
  onProgress?: (event: ProgressEvent<EventTarget>) => void;
  onLoad?: (xhr: XMLHttpRequest) => void;
  onError?: () => void;
  onAbort?: () => void;
};

export function startUploadConnectionFileXhr(
  connId: string,
  path: string,
  file: File,
  handlers: UploadXhrHandlers,
): XMLHttpRequest {
  const params = new URLSearchParams({ path });
  const xhr = new XMLHttpRequest();
  xhr.open("POST", connectionURL(`${encodeURIComponent(connId)}/upload`, params));
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

export async function mkdirConnection(connId: string, path: string): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/mkdir`, params),
    { method: "POST" },
  );
  await requireOK(response, `mkdir failed (${response.status})`);
}

export async function deleteConnectionFile(connId: string, path: string): Promise<void> {
  const params = new URLSearchParams({ path });
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/delete`, params),
    { method: "DELETE" },
  );
  await requireOK(response, `delete failed (${response.status})`);
}

export async function renameConnectionFile(
  connId: string,
  oldPath: string,
  newPath: string,
): Promise<void> {
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/rename`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ old_path: oldPath, new_path: newPath }),
    },
  );
  await requireOK(response, `rename failed (${response.status})`);
}

export async function copyConnectionFile(
  connId: string,
  srcPath: string,
  dstPath: string,
): Promise<void> {
  const response = await fetch(
    connectionURL(`${encodeURIComponent(connId)}/copy`),
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ src_path: srcPath, dst_path: dstPath }),
    },
  );
  await requireOK(response, `copy failed (${response.status})`);
}
