"use client";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface FileTransfer {
  id: string;
  source_type: string;
  source_id: string;
  source_path: string;
  dest_type: string;
  dest_id: string;
  dest_path: string;
  file_name: string;
  file_size?: number;
  bytes_transferred: number;
  status: string; // "pending" | "in_progress" | "completed" | "failed"
  error?: string;
  started_at?: string;
  completed_at?: string;
}

export interface StartTransferRequest {
  source_type: string;
  source_id: string;
  source_path: string;
  dest_type: string;
  dest_id: string;
  dest_path: string;
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

function transferURL(subpath: string): string {
  return `/api/file-transfers/${subpath}`;
}

// ---------------------------------------------------------------------------
// Transfer API
// ---------------------------------------------------------------------------

export async function startTransfer(req: StartTransferRequest): Promise<FileTransfer> {
  const response = await fetch(transferURL(""), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
  await requireOK(response, `failed to start transfer (${response.status})`);
  const data = await response.json();
  return data.transfer;
}

export async function getTransfer(id: string): Promise<FileTransfer> {
  const response = await fetch(transferURL(encodeURIComponent(id)), {
    cache: "no-store",
  });
  await requireOK(response, `failed to get transfer (${response.status})`);
  const data = await response.json();
  return data.transfer;
}

export async function cancelTransfer(id: string): Promise<void> {
  const response = await fetch(transferURL(encodeURIComponent(id)), {
    method: "DELETE",
  });
  await requireOK(response, `failed to cancel transfer (${response.status})`);
}
