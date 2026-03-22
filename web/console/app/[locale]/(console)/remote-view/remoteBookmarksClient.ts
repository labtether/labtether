import type { RemoteViewProtocol } from "./types";

export interface RemoteBookmark {
  id: string;
  label: string;
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
  has_credentials: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateBookmarkRequest {
  label: string;
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
  username?: string;
  password?: string;
}

export interface UpdateBookmarkRequest {
  label?: string;
  protocol?: RemoteViewProtocol;
  host?: string;
  port?: number;
  username?: string;
  password?: string;
}

const BASE = "/api/remote-bookmarks";

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status}: ${text}`);
  }
  return res.json();
}

export async function listBookmarks(): Promise<RemoteBookmark[]> {
  return fetchJSON<RemoteBookmark[]>(BASE);
}

export async function createBookmark(req: CreateBookmarkRequest): Promise<RemoteBookmark> {
  return fetchJSON<RemoteBookmark>(BASE, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function updateBookmark(id: string, req: UpdateBookmarkRequest): Promise<RemoteBookmark> {
  return fetchJSON<RemoteBookmark>(`${BASE}/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  });
}

export async function deleteBookmark(id: string): Promise<void> {
  const res = await fetch(`${BASE}/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status}: ${text}`);
  }
}

export async function getBookmarkCredentials(id: string): Promise<{ username?: string; password?: string }> {
  return fetchJSON(`${BASE}/${encodeURIComponent(id)}/credentials`);
}
