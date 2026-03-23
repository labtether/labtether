# Remote View Tab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a top-level Remote View tab to the LabTether console with multi-device tabbed remote desktop sessions (VNC, RDP, SPICE, ARD), a device picker landing page, bookmarks for external hosts, and an expanded toolbar.

**Architecture:** Mirrors the Files tab pattern — new route at `(console)/remote-view/` with its own tab state hook, tab bar, landing page, and session wrapper. Reuses existing viewer components (VNCViewer, GuacamoleViewer, SPICEViewer, WebRTCViewer) and adapts RemoteViewerShell's orchestration pattern. Backend adds a `remote_bookmarks` table and CRUD API mirroring the file-connections pattern.

**Tech Stack:** Next.js (App Router), TypeScript, React, Tailwind CSS, Go (hub API), Postgres

**Spec:** `notes/specs/2026-03-21-remote-view-tab-design.md`

---

## File Map

### New frontend files
| File | Responsibility |
|------|---------------|
| `web/console/app/[locale]/(console)/remote-view/page.tsx` | Page orchestrator — tab bar + active viewer or landing |
| `web/console/app/[locale]/(console)/remote-view/RemoteViewTabBar.tsx` | Tab bar with protocol dots, close, +, connection status |
| `web/console/app/[locale]/(console)/remote-view/useRemoteViewTabsState.ts` | Tab CRUD hook (add/remove/switch/update) |
| `web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx` | Device picker grid + bookmarks sidebar + quick connect |
| `web/console/app/[locale]/(console)/remote-view/RemoteViewSession.tsx` | Active viewer + toolbar + file drawer wrapper |
| `web/console/app/[locale]/(console)/remote-view/remoteBookmarksClient.ts` | API client for bookmark CRUD |
| `web/console/app/[locale]/(console)/remote-view/types.ts` | Shared types (RemoteViewTab, RemoteViewProtocol, etc.) |
| `web/console/app/api/remote-bookmarks/[[...path]]/route.ts` | Next.js API route — proxies to hub |

### New backend files
| File | Responsibility |
|------|---------------|
| `internal/persistence/postgres_remote_bookmarks.go` | Postgres store methods for remote bookmarks CRUD |
| `internal/hubapi/resources/remote_bookmark_handlers.go` | HTTP handlers for `/api/v1/remote-bookmarks` |

### Modified files
| File | Change |
|------|--------|
| `web/console/app/components/Sidebar.tsx:67` | Add Remote View nav item after Files |
| `web/console/app/components/RemoteViewToolbar.tsx` | Add `onScreenshot` prop and Screenshot button |
| `internal/persistence/types.go` | Add `RemoteBookmark` struct + `RemoteBookmarkStore` interface |
| `internal/persistence/postgres_schema_migrations.go` | Add migration 72: `remote_bookmarks` table |
| `internal/hubapi/resources/deps.go` | Add `RemoteBookmarkStore` field to `Deps` struct |
| `cmd/labtether/resources_bridge.go` | Add `handleRemoteBookmarks` bridge method + wire store |
| `cmd/labtether/http_handlers.go` | Register `/api/v1/remote-bookmarks` route with auth |

---

### Task 1: Shared Types

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/types.ts`

- [ ] **Step 1: Create the types file**

```typescript
// web/console/app/[locale]/(console)/remote-view/types.ts

export type RemoteViewTabType = "new" | "device" | "bookmark" | "adhoc";
export type RemoteViewProtocol = "vnc" | "rdp" | "spice" | "ard";
export type RemoteViewConnectionState =
  | "idle"
  | "connecting"
  | "authenticating"
  | "connected"
  | "disconnected"
  | "error";

export interface RemoteViewTab {
  id: string;
  type: RemoteViewTabType;
  label: string;
  protocol?: RemoteViewProtocol;
  target?: {
    host: string;
    port: number;
    assetId?: string;
    bookmarkId?: string;
  };
  connectionState: RemoteViewConnectionState;
  lastConnectedAt?: number;
}

/** Maps RemoteViewProtocol to the DesktopProtocol used by viewer components. ARD uses VNC transport. */
export function toDesktopProtocol(protocol: RemoteViewProtocol): "vnc" | "rdp" | "spice" {
  switch (protocol) {
    case "ard":
      return "vnc";
    case "vnc":
      return "vnc";
    case "rdp":
      return "rdp";
    case "spice":
      return "spice";
  }
}

/** Default port for each protocol. */
export function defaultPort(protocol: RemoteViewProtocol): number {
  switch (protocol) {
    case "vnc":
    case "ard":
      return 5900;
    case "rdp":
      return 3389;
    case "spice":
      return 5930;
  }
}

/** Protocol dot colors for the tab bar (Tailwind classes). */
export const PROTOCOL_DOT_COLOR: Record<RemoteViewProtocol, string> = {
  vnc: "bg-green-500",
  rdp: "bg-blue-500",
  spice: "bg-amber-500",
  ard: "bg-purple-500",
};
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors from `remote-view/types.ts`

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/types.ts
git commit -m "feat(remote-view): add shared types for remote view tab"
```

---

### Task 2: Tab State Hook

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/useRemoteViewTabsState.ts`
- Reference: `web/console/app/[locale]/(console)/files/useFileTabsState.ts` (mirror this pattern)

- [ ] **Step 1: Create the tab state hook**

```typescript
// web/console/app/[locale]/(console)/remote-view/useRemoteViewTabsState.ts
"use client";

import { useState, useCallback, useMemo } from "react";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";

function newTab(): RemoteViewTab {
  return {
    id: crypto.randomUUID(),
    type: "new",
    label: "New Tab",
    connectionState: "idle",
  };
}

export function useRemoteViewTabsState() {
  const [tabs, setTabs] = useState<RemoteViewTab[]>(() => [newTab()]);
  const [activeTabId, setActiveTabId] = useState<string>(() => tabs[0].id);

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId) ?? tabs[0],
    [tabs, activeTabId],
  );

  const addTab = useCallback((partial?: Partial<RemoteViewTab>) => {
    const tab: RemoteViewTab = { ...newTab(), ...partial };
    setTabs((prev) => [...prev, tab]);
    setActiveTabId(tab.id);
  }, []);

  const removeTab = useCallback(
    (tabId: string) => {
      setTabs((prev) => {
        const next = prev.filter((t) => t.id !== tabId);
        if (next.length === 0) {
          const fallback = newTab();
          setActiveTabId(fallback.id);
          return [fallback];
        }
        if (activeTabId === tabId) {
          const idx = prev.findIndex((t) => t.id === tabId);
          const target = next[Math.min(idx, next.length - 1)];
          setActiveTabId(target.id);
        }
        return next;
      });
    },
    [activeTabId],
  );

  const updateTab = useCallback(
    (tabId: string, updates: Partial<RemoteViewTab>) => {
      setTabs((prev) =>
        prev.map((t) => (t.id === tabId ? { ...t, ...updates } : t)),
      );
    },
    [],
  );

  const setConnectionState = useCallback(
    (tabId: string, state: RemoteViewConnectionState) => {
      updateTab(tabId, {
        connectionState: state,
        ...(state === "connected" ? { lastConnectedAt: Date.now() } : {}),
      });
    },
    [updateTab],
  );

  return {
    tabs,
    activeTabId,
    activeTab,
    addTab,
    removeTab,
    setActiveTab: setActiveTabId,
    updateTab,
    setConnectionState,
  };
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/useRemoteViewTabsState.ts
git commit -m "feat(remote-view): add tab state management hook"
```

---

### Task 3: Tab Bar Component

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/RemoteViewTabBar.tsx`
- Reference: `web/console/app/[locale]/(console)/files/FileTabBar.tsx` (mirror visual pattern)

- [ ] **Step 1: Create the tab bar component**

```typescript
// web/console/app/[locale]/(console)/remote-view/RemoteViewTabBar.tsx
"use client";

import { X, Plus } from "lucide-react";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";
import { PROTOCOL_DOT_COLOR } from "./types";

interface RemoteViewTabBarProps {
  tabs: RemoteViewTab[];
  activeTabId: string;
  onAddTab: () => void;
  onRemoveTab: (tabId: string) => void;
  onSetActiveTab: (tabId: string) => void;
  /** Connection state + latency for the active tab (shown in status pill) */
  connectionState?: RemoteViewConnectionState;
  latencyMs?: number | null;
}

function protocolDotClass(protocol: string | undefined): string {
  if (!protocol) return "bg-[var(--muted)]";
  return PROTOCOL_DOT_COLOR[protocol as keyof typeof PROTOCOL_DOT_COLOR] ?? "bg-[var(--muted)]";
}

function statusColor(state: RemoteViewConnectionState): string {
  switch (state) {
    case "connected":
      return "bg-green-500";
    case "connecting":
    case "authenticating":
      return "bg-amber-500 animate-pulse";
    case "error":
      return "bg-red-500";
    default:
      return "bg-zinc-500";
  }
}

function statusLabel(state: RemoteViewConnectionState): string {
  switch (state) {
    case "connected":
      return "Connected";
    case "connecting":
      return "Connecting";
    case "authenticating":
      return "Authenticating";
    case "disconnected":
      return "Disconnected";
    case "error":
      return "Error";
    default:
      return "Idle";
  }
}

export default function RemoteViewTabBar({
  tabs,
  activeTabId,
  onAddTab,
  onRemoveTab,
  onSetActiveTab,
  connectionState,
  latencyMs,
}: RemoteViewTabBarProps) {
  return (
    <div className="flex items-center gap-0.5 px-2 py-1 border-b border-[var(--border-primary)] overflow-x-auto bg-[var(--bg-secondary)]">
      {tabs.map((tab) => {
        const isActive = tab.id === activeTabId;
        return (
          <button
            key={tab.id}
            onClick={() => onSetActiveTab(tab.id)}
            className={`group flex items-center gap-1.5 px-2.5 py-1.5 rounded-md text-xs whitespace-nowrap transition-colors ${
              isActive
                ? "bg-[var(--bg-primary)] text-[var(--text-primary)] shadow-sm"
                : "text-[var(--text-secondary)] hover:bg-[var(--bg-primary)]/50"
            }`}
          >
            <span className={`w-2 h-2 rounded-full flex-shrink-0 ${protocolDotClass(tab.protocol)}`} />
            <span className="max-w-[140px] truncate">{tab.label}</span>
            <span
              role="button"
              tabIndex={-1}
              className={`ml-0.5 p-0.5 rounded hover:bg-[var(--bg-tertiary)] ${
                isActive ? "opacity-60 hover:opacity-100" : "opacity-0 group-hover:opacity-60 hover:!opacity-100"
              }`}
              onClick={(e) => {
                e.stopPropagation();
                onRemoveTab(tab.id);
              }}
            >
              <X className="w-3 h-3" />
            </span>
          </button>
        );
      })}

      <button
        onClick={onAddTab}
        className="flex-shrink-0 p-1.5 rounded-md text-[var(--text-secondary)] hover:bg-[var(--bg-primary)]/50 transition-colors"
        title="New tab"
      >
        <Plus className="w-3.5 h-3.5" />
      </button>

      <div className="flex-1" />

      {/* Connection status pill */}
      {connectionState && connectionState !== "idle" && (
        <div className="flex items-center gap-1.5 px-2 py-1 rounded-md bg-[var(--bg-primary)] text-xs flex-shrink-0">
          <span className={`w-1.5 h-1.5 rounded-full ${statusColor(connectionState)}`} />
          <span className="text-[var(--text-secondary)]">{statusLabel(connectionState)}</span>
          {connectionState === "connected" && latencyMs != null && (
            <span className="text-[var(--text-tertiary)] ml-1">{latencyMs}ms</span>
          )}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/RemoteViewTabBar.tsx
git commit -m "feat(remote-view): add tab bar component with protocol dots and status pill"
```

---

### Task 4: Bookmarks API Client (Frontend)

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/remoteBookmarksClient.ts`
- Reference: `web/console/app/[locale]/(console)/files/fileConnectionsClient.ts` (mirror fetch pattern)

- [ ] **Step 1: Create the bookmarks client**

```typescript
// web/console/app/[locale]/(console)/remote-view/remoteBookmarksClient.ts
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

/** Get stored credentials for a bookmark (returned only on connect, not on list). */
export async function getBookmarkCredentials(id: string): Promise<{ username?: string; password?: string }> {
  return fetchJSON(`${BASE}/${encodeURIComponent(id)}/credentials`);
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/remoteBookmarksClient.ts
git commit -m "feat(remote-view): add bookmarks API client"
```

---

### Task 5: New Tab Landing Page

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx`
- Reference: `web/console/app/[locale]/(console)/files/NewTabPage.tsx` (layout pattern)

- [ ] **Step 1: Create the NewTabPage component**

This component has three sections: Quick Connect bar at top, Available Devices grid on left, Bookmarks list on right.

```typescript
// web/console/app/[locale]/(console)/remote-view/NewTabPage.tsx
"use client";

import { useState, useEffect, useMemo, useCallback, type FormEvent } from "react";
import { Search, Plus, Trash2, Monitor as MonitorIcon } from "lucide-react";
import { useFastStatus } from "@/app/contexts/StatusContext";
import { PROTOCOL_DOT_COLOR, defaultPort, type RemoteViewProtocol } from "./types";
import {
  listBookmarks,
  createBookmark,
  deleteBookmark,
  type RemoteBookmark,
  type CreateBookmarkRequest,
} from "./remoteBookmarksClient";

interface NewTabPageProps {
  onConnectDevice: (assetId: string, name: string, protocol: RemoteViewProtocol) => void;
  onConnectBookmark: (bookmark: RemoteBookmark) => void;
  onConnectAdhoc: (host: string, port: number, protocol: RemoteViewProtocol) => void;
}

const PROTOCOLS: { value: RemoteViewProtocol; label: string }[] = [
  { value: "vnc", label: "VNC" },
  { value: "rdp", label: "RDP" },
  { value: "spice", label: "SPICE" },
  { value: "ard", label: "ARD" },
];

/** Parse a URL like vnc://host:port into parts. Falls back to `fallbackProtocol` for bare hostnames. */
function parseQuickConnect(
  input: string,
  fallbackProtocol: RemoteViewProtocol,
): {
  protocol: RemoteViewProtocol;
  host: string;
  port: number;
} | null {
  const trimmed = input.trim();
  if (!trimmed) return null;

  const schemeMatch = trimmed.match(/^(vnc|rdp|spice|ard):\/\/(.+)/i);
  if (schemeMatch) {
    const proto = schemeMatch[1].toLowerCase() as RemoteViewProtocol;
    const rest = schemeMatch[2];
    const [host, portStr] = rest.split(":");
    return {
      protocol: proto,
      host: host || "",
      port: portStr ? parseInt(portStr, 10) : defaultPort(proto),
    };
  }

  // Bare hostname — use the protocol dropdown selection as fallback
  const [host, portStr] = trimmed.split(":");
  return {
    protocol: fallbackProtocol,
    host,
    port: portStr ? parseInt(portStr, 10) : defaultPort(fallbackProtocol),
  };
}

export default function NewTabPage({
  onConnectDevice,
  onConnectBookmark,
  onConnectAdhoc,
}: NewTabPageProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [bookmarks, setBookmarks] = useState<RemoteBookmark[]>([]);
  const [bookmarksLoading, setBookmarksLoading] = useState(true);
  const [quickProtocol, setQuickProtocol] = useState<RemoteViewProtocol>("vnc");

  // Load bookmarks
  useEffect(() => {
    listBookmarks()
      .then(setBookmarks)
      .catch(() => setBookmarks([]))
      .finally(() => setBookmarksLoading(false));
  }, []);

  // Get devices with remote view capabilities from fast status
  const status = useFastStatus();
  const assets = status?.assets;
  const remoteViewDevices = useMemo(() => {
    if (!assets) return [];
    return assets
      .filter((a) => {
        const caps = a.metadata?.remote_view_capabilities;
        return caps && Array.isArray(caps) && caps.length > 0;
      })
      .map((a) => ({
        id: a.id,
        name: a.name,
        ip: a.metadata?.primary_ip as string | undefined,
        protocols: (a.metadata?.remote_view_capabilities ?? []) as RemoteViewProtocol[],
        online: a.status === "online",
      }));
  }, [assets]);

  // Filter devices by search
  const filteredDevices = useMemo(() => {
    if (!searchQuery) return remoteViewDevices;
    const q = searchQuery.toLowerCase();
    return remoteViewDevices.filter(
      (d) => d.name.toLowerCase().includes(q) || (d.ip && d.ip.includes(q)),
    );
  }, [remoteViewDevices, searchQuery]);

  const handleQuickConnect = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      const parsed = parseQuickConnect(searchQuery, quickProtocol);
      if (parsed) {
        onConnectAdhoc(parsed.host, parsed.port, parsed.protocol);
      }
    },
    [searchQuery, quickProtocol, onConnectAdhoc],
  );

  const handleDeviceClick = useCallback(
    (device: (typeof remoteViewDevices)[0]) => {
      if (!device.online) return;
      if (device.protocols.length === 1) {
        onConnectDevice(device.id, device.name, device.protocols[0]);
      }
      // Multi-protocol: for now connect with first protocol.
      // TODO: Show protocol picker popover for multi-protocol devices.
      else if (device.protocols.length > 1) {
        onConnectDevice(device.id, device.name, device.protocols[0]);
      }
    },
    [onConnectDevice],
  );

  const handleDeleteBookmark = useCallback(async (id: string) => {
    await deleteBookmark(id);
    setBookmarks((prev) => prev.filter((b) => b.id !== id));
  }, []);

  return (
    <div className="flex-1 flex flex-col gap-6 p-4 md:p-6 overflow-y-auto">
      {/* Quick Connect */}
      <form onSubmit={handleQuickConnect} className="max-w-2xl mx-auto w-full">
        <div className="relative flex">
          <div className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-tertiary)]">
            <Search className="w-4 h-4" />
          </div>
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="vnc://host:port, rdp://host, or search devices..."
            className="flex-1 pl-9 pr-3 py-2.5 bg-[var(--bg-secondary)] border border-[var(--border-primary)] rounded-l-lg text-sm text-[var(--text-primary)] placeholder:text-[var(--text-tertiary)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
          />
          <select
            value={quickProtocol}
            onChange={(e) => setQuickProtocol(e.target.value as RemoteViewProtocol)}
            className="px-2 py-2.5 bg-[var(--bg-secondary)] border-y border-[var(--border-primary)] text-xs text-[var(--text-secondary)]"
          >
            {PROTOCOLS.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
          <button
            type="submit"
            className="px-4 py-2.5 bg-[var(--accent)] text-white text-sm font-medium rounded-r-lg hover:opacity-90 transition-opacity"
          >
            Connect
          </button>
        </div>
        <p className="text-xs text-[var(--text-tertiary)] mt-1.5">
          Supports vnc://, rdp://, spice://, ard:// — or just enter a hostname
        </p>
      </form>

      {/* Two-column: Devices + Bookmarks */}
      <div className="max-w-4xl mx-auto w-full grid grid-cols-1 md:grid-cols-[1fr_280px] gap-6">
        {/* Left: Available Devices */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-3">
            Available Devices
          </h3>
          {filteredDevices.length === 0 ? (
            <p className="text-sm text-[var(--text-tertiary)] py-8 text-center">
              {searchQuery
                ? "No devices match your search"
                : "No devices with remote view capabilities"}
            </p>
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              {filteredDevices.map((device) => (
                <button
                  key={device.id}
                  onClick={() => handleDeviceClick(device)}
                  disabled={!device.online}
                  className={`text-left p-3 rounded-lg border transition-colors ${
                    device.online
                      ? "bg-[var(--bg-secondary)] border-[var(--border-primary)] hover:border-[var(--accent)]/50 cursor-pointer"
                      : "bg-[var(--bg-secondary)]/50 border-[var(--border-primary)]/50 opacity-50 cursor-not-allowed"
                  }`}
                >
                  <div className="flex items-center gap-2 mb-2">
                    <MonitorIcon className="w-4 h-4 text-[var(--text-tertiary)]" />
                    <span className="text-sm font-medium text-[var(--text-primary)] truncate">
                      {device.name}
                    </span>
                  </div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-2">
                    {device.ip ?? "No IP"} {!device.online && "· offline"}
                  </div>
                  <div className="flex gap-1 flex-wrap">
                    {device.protocols.map((proto) => (
                      <span
                        key={proto}
                        className={`px-1.5 py-0.5 rounded text-[10px] font-medium uppercase ${PROTOCOL_DOT_COLOR[proto].replace("bg-", "bg-")}/20 text-${PROTOCOL_DOT_COLOR[proto].replace("bg-", "")}`}
                      >
                        {proto}
                      </span>
                    ))}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Right: Bookmarks */}
        <div>
          <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-3">
            Bookmarks
          </h3>
          <div className="flex flex-col gap-1.5">
            {bookmarksLoading ? (
              <p className="text-xs text-[var(--text-tertiary)] py-4 text-center">Loading...</p>
            ) : (
              <>
                {bookmarks.map((bm) => (
                  <div
                    key={bm.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => onConnectBookmark(bm)}
                    onKeyDown={(e) => e.key === "Enter" && onConnectBookmark(bm)}
                    className="flex items-center gap-2 p-2.5 rounded-lg bg-[var(--bg-secondary)] border border-[var(--border-primary)] hover:border-[var(--accent)]/50 cursor-pointer transition-colors group"
                  >
                    <span
                      className={`w-2 h-2 rounded-full flex-shrink-0 ${PROTOCOL_DOT_COLOR[bm.protocol]}`}
                    />
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-[var(--text-primary)] truncate">{bm.label}</div>
                      <div className="text-xs text-[var(--text-tertiary)] font-mono truncate">
                        {bm.protocol}://{bm.host}:{bm.port}
                      </div>
                    </div>
                    <button
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteBookmark(bm.id);
                      }}
                      className="p-1 rounded opacity-0 group-hover:opacity-60 hover:!opacity-100 hover:bg-[var(--bg-tertiary)] transition-opacity"
                    >
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => {
                    /* TODO: open add bookmark form */
                  }}
                  className="flex items-center justify-center gap-1.5 p-2.5 rounded-lg border border-dashed border-[var(--border-primary)] text-[var(--text-tertiary)] hover:border-[var(--accent)]/50 hover:text-[var(--text-secondary)] transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span className="text-xs">Add bookmark</span>
                </button>
              </>
            )}
          </div>

          {/* Protocol legend */}
          <div className="mt-6">
            <h4 className="text-[10px] font-semibold uppercase tracking-wide text-[var(--text-tertiary)] mb-2">
              Protocols
            </h4>
            <div className="flex flex-col gap-1">
              {PROTOCOLS.map((p) => (
                <div key={p.value} className="flex items-center gap-1.5">
                  <span className={`w-2 h-2 rounded-full ${PROTOCOL_DOT_COLOR[p.value]}`} />
                  <span className="text-xs text-[var(--text-tertiary)]">
                    {p.label} ({defaultPort(p.value)})
                  </span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
```

Note: The protocol badge styling in device cards uses a simplified approach. During implementation, adapt to the project's existing badge/pill patterns if different. The multi-protocol device popover is left as a TODO — connect with first protocol for now and iterate.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors (some warnings about unused imports are OK at this stage)

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/NewTabPage.tsx
git commit -m "feat(remote-view): add new tab landing page with device picker and bookmarks"
```

---

### Task 6: Remote View Session Wrapper

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/RemoteViewSession.tsx`
- Reference: `web/console/app/components/RemoteViewerShell.tsx` (adapt this pattern)

- [ ] **Step 1: Create the session wrapper**

This component adapts the `RemoteViewerShell` pattern for the tabbed context. It selects the correct viewer by protocol, wires up the toolbar, file drawer, and handles connection lifecycle. Read `RemoteViewerShell.tsx` before implementing — it has the exact viewer prop signatures and orchestration logic to follow.

The component should:
1. Accept the active tab's target info (protocol, host, port, assetId)
2. Use `useDesktopTabState` from `nodes/[id]/useDesktopTabState.ts` for viewer state (quality, scaling, recording, etc.)
3. Use `useDesktopSession` or equivalent hook for WebSocket connection lifecycle
4. Mount the correct viewer component based on `toDesktopProtocol(protocol)`
5. Mount `RemoteViewToolbar` and `RemoteViewFileDrawer`
6. Report connection state changes back to the parent via `onConnectionStateChange` callback

Implementation note: The exact wiring depends on how the existing desktop session hooks work with arbitrary host/port targets vs. asset-backed sessions. During implementation, read the following files to understand the connection flow:
- `web/console/app/[locale]/(console)/nodes/[id]/useDesktopTabState.ts`
- `web/console/app/components/RemoteViewerShell.tsx`
- `web/console/app/contexts/DesktopSessionContext.tsx`

```typescript
// web/console/app/[locale]/(console)/remote-view/RemoteViewSession.tsx
"use client";

import { useRef, useCallback, useState } from "react";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";
import { toDesktopProtocol } from "./types";
// Import viewer components — exact imports depend on their export patterns:
// import VNCViewer from "@/app/components/VNCViewer";
// import GuacamoleViewer from "@/app/components/GuacamoleViewer";
// import SPICEViewer from "@/app/components/SPICEViewer";
// import RemoteViewToolbar from "@/app/components/RemoteViewToolbar";
// import RemoteViewFileDrawer from "@/app/components/RemoteViewFileDrawer";

interface RemoteViewSessionProps {
  tab: RemoteViewTab;
  onConnectionStateChange: (state: RemoteViewConnectionState) => void;
}

export default function RemoteViewSession({ tab, onConnectionStateChange }: RemoteViewSessionProps) {
  const viewerWrapperRef = useRef<HTMLDivElement>(null);
  const [fileDrawerOpen, setFileDrawerOpen] = useState(false);

  // Viewer state — adapt from useDesktopTabState pattern
  const [quality, setQuality] = useState("auto");
  const [scalingMode, setScalingMode] = useState<"fit" | "native" | "fill">("fit");
  const [isFullscreen, setIsFullscreen] = useState(false);

  const desktopProtocol = tab.protocol ? toDesktopProtocol(tab.protocol) : "vnc";

  const handleDesktopConnect = useCallback(() => {
    onConnectionStateChange("connected");
  }, [onConnectionStateChange]);

  const handleDesktopDisconnect = useCallback(() => {
    onConnectionStateChange("disconnected");
  }, [onConnectionStateChange]);

  const handleDesktopError = useCallback(() => {
    onConnectionStateChange("error");
  }, [onConnectionStateChange]);

  const handleScreenshot = useCallback(() => {
    // Capture canvas from viewer wrapper and download
    const canvas = viewerWrapperRef.current?.querySelector("canvas");
    if (!canvas) return;
    const link = document.createElement("a");
    link.download = `${tab.label}-${new Date().toISOString().slice(0, 19)}.png`;
    link.href = (canvas as HTMLCanvasElement).toDataURL("image/png");
    link.click();
  }, [tab.label]);

  // Determine if file drawer is available (only for managed devices with agents)
  const canShowFileDrawer = tab.type === "device" && !!tab.target?.assetId;

  return (
    <div ref={viewerWrapperRef} className="flex-1 flex flex-col min-h-0 bg-black relative">
      {/* Toolbar — docked below tab bar */}
      {/* <RemoteViewToolbar
        layout="dock"
        connectionState={tab.connectionState === "disconnected" ? "idle" : tab.connectionState}
        protocol={desktopProtocol}
        quality={quality}
        onQualityChange={setQuality}
        scalingMode={scalingMode}
        onScalingModeChange={setScalingMode}
        isFullscreen={isFullscreen}
        onFullscreenToggle={() => setIsFullscreen(!isFullscreen)}
        onScreenshot={handleScreenshot}
        fileDrawerOpen={fileDrawerOpen}
        onFileDrawerToggle={() => setFileDrawerOpen(!fileDrawerOpen)}
        // ... remaining props wired from viewer state
      /> */}

      {/* Viewer area */}
      <div className="flex-1 relative min-h-0">
        {/* Viewer component selected by protocol — uncomment during implementation:
        {desktopProtocol === "vnc" && <VNCViewer ... />}
        {desktopProtocol === "rdp" && <GuacamoleViewer ... />}
        {desktopProtocol === "spice" && <SPICEViewer ... />}
        */}
        <div className="flex items-center justify-center h-full text-[var(--text-tertiary)] text-sm">
          Connecting to {tab.target?.host}:{tab.target?.port} via {tab.protocol}...
        </div>
      </div>

      {/* File drawer */}
      {/* {canShowFileDrawer && tab.target?.assetId && (
        <RemoteViewFileDrawer
          nodeId={tab.target.assetId}
          open={fileDrawerOpen}
          onClose={() => setFileDrawerOpen(false)}
        />
      )} */}
    </div>
  );
}
```

Note: The viewer component imports and toolbar/drawer wiring are commented out as scaffolding. During implementation, follow `RemoteViewerShell.tsx` exactly — it has the complete prop wiring for each viewer and the toolbar. The key adaptation is that connection target comes from `tab.target` instead of from the node detail context.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/RemoteViewSession.tsx
git commit -m "feat(remote-view): add session wrapper with viewer scaffolding"
```

---

### Task 7: Page Orchestrator

**Files:**
- Create: `web/console/app/[locale]/(console)/remote-view/page.tsx`
- Reference: `web/console/app/[locale]/(console)/files/page.tsx` (orchestrator pattern)

- [ ] **Step 1: Create the page component**

```typescript
// web/console/app/[locale]/(console)/remote-view/page.tsx
"use client";

import { useCallback } from "react";
import { useRemoteViewTabsState } from "./useRemoteViewTabsState";
import RemoteViewTabBar from "./RemoteViewTabBar";
import NewTabPage from "./NewTabPage";
import RemoteViewSession from "./RemoteViewSession";
import { defaultPort } from "./types";
import type { RemoteViewProtocol, RemoteViewConnectionState } from "./types";
import type { RemoteBookmark } from "./remoteBookmarksClient";

export default function RemoteViewPage() {
  const tabs = useRemoteViewTabsState();
  const activeTab = tabs.activeTab;

  const isNewTab = activeTab?.type === "new";
  const isSessionTab =
    activeTab?.type === "device" ||
    activeTab?.type === "bookmark" ||
    activeTab?.type === "adhoc";

  // --- Tab actions ---

  const handleConnectDevice = useCallback(
    (assetId: string, name: string, protocol: RemoteViewProtocol) => {
      tabs.updateTab(activeTab.id, {
        type: "device",
        label: name,
        protocol,
        target: { host: "", port: defaultPort(protocol), assetId },
        connectionState: "connecting",
      });
    },
    [tabs, activeTab],
  );

  const handleConnectBookmark = useCallback(
    (bookmark: RemoteBookmark) => {
      tabs.updateTab(activeTab.id, {
        type: "bookmark",
        label: bookmark.label,
        protocol: bookmark.protocol,
        target: {
          host: bookmark.host,
          port: bookmark.port,
          bookmarkId: bookmark.id,
        },
        connectionState: "connecting",
      });
    },
    [tabs, activeTab],
  );

  const handleConnectAdhoc = useCallback(
    (host: string, port: number, protocol: RemoteViewProtocol) => {
      tabs.updateTab(activeTab.id, {
        type: "adhoc",
        label: host,
        protocol,
        target: { host, port },
        connectionState: "connecting",
      });
    },
    [tabs, activeTab],
  );

  const handleConnectionStateChange = useCallback(
    (state: RemoteViewConnectionState) => {
      tabs.setConnectionState(activeTab.id, state);
    },
    [tabs, activeTab],
  );

  return (
    <div className="flex flex-col h-full">
      <RemoteViewTabBar
        tabs={tabs.tabs}
        activeTabId={tabs.activeTabId}
        onAddTab={() => tabs.addTab()}
        onRemoveTab={tabs.removeTab}
        onSetActiveTab={tabs.setActiveTab}
        connectionState={activeTab?.connectionState}
        latencyMs={null}
      />

      {isNewTab && (
        <NewTabPage
          onConnectDevice={handleConnectDevice}
          onConnectBookmark={handleConnectBookmark}
          onConnectAdhoc={handleConnectAdhoc}
        />
      )}

      {isSessionTab && activeTab && (
        <RemoteViewSession
          key={activeTab.id}
          tab={activeTab}
          onConnectionStateChange={handleConnectionStateChange}
        />
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/\[locale\]/\(console\)/remote-view/page.tsx
git commit -m "feat(remote-view): add page orchestrator with tab routing"
```

---

### Task 8: Sidebar Navigation

**Files:**
- Modify: `web/console/app/components/Sidebar.tsx:67`

- [ ] **Step 1: Add Remote View to sidebar nav**

In `web/console/app/components/Sidebar.tsx`, the Operations category starts at line 64. Add the Remote View item after the Files entry (line 67).

The `Monitor` icon import already exists at line 22 (used for the desktop session indicator). Use it for the nav item.

Add after line 67 (`{ href: "/files", ... }`):

```typescript
{ href: "/remote-view", label: "Remote View", translationKey: "remoteView", icon: Monitor },
```

- [ ] **Step 2: Verify it renders**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/components/Sidebar.tsx
git commit -m "feat(remote-view): add Remote View to sidebar navigation"
```

---

### Task 9: Next.js API Route (Proxy)

**Files:**
- Create: `web/console/app/api/remote-bookmarks/[[...path]]/route.ts`
- Reference: `web/console/app/api/file-connections/[[...path]]/route.ts` (exact same proxy pattern)

- [ ] **Step 1: Create the API proxy route**

Read `web/console/app/api/file-connections/[[...path]]/route.ts` first. Copy the exact proxy pattern, changing only the hub backend prefix from `/api/v1/file-connections` to `/api/v1/remote-bookmarks`.

```typescript
// web/console/app/api/remote-bookmarks/[[...path]]/route.ts
import { NextRequest, NextResponse } from "next/server";

const HUB_URL = process.env.HUB_INTERNAL_URL || "http://localhost:3001";
const API_PREFIX = "/api/v1/remote-bookmarks";

async function proxyToHub(
  request: NextRequest,
  { params }: { params: Promise<{ path?: string[] }> },
) {
  const { path } = await params;
  const subPath = path ? path.map(encodeURIComponent).join("/") : "";
  const searchParams = request.nextUrl.searchParams.toString();
  const query = searchParams ? `?${searchParams}` : "";

  const url = `${HUB_URL}${API_PREFIX}${subPath ? `/${subPath}` : ""}${query}`;

  const headers: Record<string, string> = {};
  const auth = request.headers.get("authorization");
  if (auth) headers["Authorization"] = auth;
  const cookie = request.headers.get("cookie");
  if (cookie) headers["Cookie"] = cookie;
  const ct = request.headers.get("content-type");
  if (ct) headers["Content-Type"] = ct;

  const init: RequestInit = { method: request.method, headers };
  if (request.method === "POST" || request.method === "PUT") {
    init.body = request.body;
    (init as any).duplex = "half";
  }

  const response = await fetch(url, init);
  const data = await response.json().catch(() => null);

  if (data === null) {
    return new NextResponse(null, { status: response.status });
  }
  return NextResponse.json(data, { status: response.status });
}

export const GET = proxyToHub;
export const POST = proxyToHub;
export const PUT = proxyToHub;
export const DELETE = proxyToHub;
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/api/remote-bookmarks/
git commit -m "feat(remote-view): add API proxy route for remote bookmarks"
```

---

### Task 10: Backend — RemoteBookmark Type & Migration

**Files:**
- Modify: `internal/persistence/types.go` — add `RemoteBookmark` struct
- Modify: `internal/persistence/postgres_schema_migrations.go` — add migration 72

- [ ] **Step 1: Add RemoteBookmark type**

In `internal/persistence/types.go`, add after the `FileConnection` struct (around line 508):

```go
// RemoteBookmark is a saved remote desktop connection to an external host.
type RemoteBookmark struct {
	ID             string    `json:"id"`
	Label          string    `json:"label"`
	Protocol       string    `json:"protocol"` // "vnc", "rdp", "spice", "ard"
	Host           string    `json:"host"`
	Port           int       `json:"port"`
	CredentialID   *string   `json:"credential_id,omitempty"`
	HasCredentials bool      `json:"has_credentials"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Add migration 72**

In `internal/persistence/postgres_schema_migrations.go`, append to the migrations slice (after version 71):

```go
{
	Version: 72,
	Name:    "remote_bookmarks",
	Statements: []string{
		`CREATE TABLE IF NOT EXISTS remote_bookmarks (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			protocol TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			credential_id TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	},
},
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/michael/Development/LabTether/hub && go vet ./internal/persistence/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/persistence/types.go internal/persistence/postgres_schema_migrations.go
git commit -m "feat(remote-view): add RemoteBookmark type and migration 72"
```

---

### Task 11: Backend — Remote Bookmarks Store

**Files:**
- Create: `internal/persistence/postgres_remote_bookmarks.go`
- Modify: `internal/persistence/types.go` — add store interface (alongside existing `FileConnectionStore` interface ~line 528)
- Reference: `internal/persistence/postgres_file_connections.go` (mirror this pattern exactly)

- [ ] **Step 1: Add store interface**

In `internal/persistence/types.go`, add a `RemoteBookmarkStore` interface after the `FileConnectionStore` interface (around line 528). Follow the same pattern:

```go
// Remote bookmarks
ListRemoteBookmarks(ctx context.Context) ([]RemoteBookmark, error)
GetRemoteBookmark(ctx context.Context, id string) (*RemoteBookmark, error)
CreateRemoteBookmark(ctx context.Context, bm *RemoteBookmark) error
UpdateRemoteBookmark(ctx context.Context, bm RemoteBookmark) error
DeleteRemoteBookmark(ctx context.Context, id string) error
```

- [ ] **Step 2: Create the Postgres implementation**

Create `internal/persistence/postgres_remote_bookmarks.go` following the exact pattern from `postgres_file_connections.go`:

```go
package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/idgen"
)

type remoteBookmarkScanner interface {
	Scan(dest ...any) error
}

func scanRemoteBookmark(row remoteBookmarkScanner) (RemoteBookmark, error) {
	bm := RemoteBookmark{}
	if err := row.Scan(
		&bm.ID,
		&bm.Label,
		&bm.Protocol,
		&bm.Host,
		&bm.Port,
		&bm.CredentialID,
		&bm.CreatedAt,
		&bm.UpdatedAt,
	); err != nil {
		return RemoteBookmark{}, err
	}
	bm.HasCredentials = bm.CredentialID != nil
	bm.CreatedAt = bm.CreatedAt.UTC()
	bm.UpdatedAt = bm.UpdatedAt.UTC()
	return bm, nil
}

const remoteBookmarkColumns = `id, label, protocol, host, port, credential_id, created_at, updated_at`

func (s *PostgresStore) ListRemoteBookmarks(ctx context.Context) ([]RemoteBookmark, error) {
	rows, err := s.pool.Query(ctx,
		fmt.Sprintf(`SELECT %s FROM remote_bookmarks ORDER BY updated_at DESC`, remoteBookmarkColumns),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RemoteBookmark, 0, 16)
	for rows.Next() {
		bm, scanErr := scanRemoteBookmark(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, bm)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetRemoteBookmark(ctx context.Context, id string) (*RemoteBookmark, error) {
	bm, err := scanRemoteBookmark(s.pool.QueryRow(ctx,
		fmt.Sprintf(`SELECT %s FROM remote_bookmarks WHERE id = $1`, remoteBookmarkColumns),
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &bm, nil
}

func (s *PostgresStore) CreateRemoteBookmark(ctx context.Context, bm *RemoteBookmark) error {
	if bm.ID == "" {
		bm.ID = idgen.New("rbm")
	}
	now := time.Now().UTC()
	bm.CreatedAt = now
	bm.UpdatedAt = now

	_, err := s.pool.Exec(ctx,
		`INSERT INTO remote_bookmarks (id, label, protocol, host, port, credential_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		bm.ID, bm.Label, bm.Protocol, bm.Host, bm.Port, bm.CredentialID, bm.CreatedAt, bm.UpdatedAt,
	)
	return err
}

func (s *PostgresStore) UpdateRemoteBookmark(ctx context.Context, bm RemoteBookmark) error {
	bm.UpdatedAt = time.Now().UTC()
	tag, err := s.pool.Exec(ctx,
		`UPDATE remote_bookmarks SET label=$2, protocol=$3, host=$4, port=$5, credential_id=$6, updated_at=$7 WHERE id=$1`,
		bm.ID, bm.Label, bm.Protocol, bm.Host, bm.Port, bm.CredentialID, bm.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) DeleteRemoteBookmark(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM remote_bookmarks WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/michael/Development/LabTether/hub && go vet ./internal/persistence/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/persistence/postgres_remote_bookmarks.go internal/persistence/types.go
git commit -m "feat(remote-view): add remote bookmarks Postgres store"
```

---

### Task 12: Backend — HTTP Handlers & Routing

**Files:**
- Create: `internal/hubapi/resources/remote_bookmark_handlers.go`
- Modify: `cmd/labtether/resources_bridge.go` — add bridge method
- Modify: `cmd/labtether/http_handlers.go` — register route with auth
- Reference: `internal/hubapi/resources/file_connection_handlers.go` (mirror pattern)
- Reference: `cmd/labtether/resources_bridge.go` — see how `handleFileConnections` is bridged

- [ ] **Step 1: Create HTTP handlers**

Read `internal/hubapi/resources/file_connection_handlers.go` first. Create `remote_bookmark_handlers.go` following the exact same pattern: path-based dispatch, JSON encoding, error responses.

```go
package resources

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

const remoteBookmarkAPIPrefix = "/api/v1/remote-bookmarks"

func (d *Deps) HandleRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, remoteBookmarkAPIPrefix)
	path = strings.TrimPrefix(path, "/")

	if d.RemoteBookmarkStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "remote bookmark store unavailable")
		return
	}

	// Collection routes: /api/v1/remote-bookmarks
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			d.handleListRemoteBookmarks(w, r)
		case http.MethodPost:
			d.handleCreateRemoteBookmark(w, r)
		default:
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// Item routes: /api/v1/remote-bookmarks/{id}
	parts := strings.SplitN(path, "/", 2)
	id := strings.TrimSpace(parts[0])
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}

	// Sub-resource: /api/v1/remote-bookmarks/{id}/credentials
	if len(parts) == 2 && parts[1] == "credentials" && r.Method == http.MethodGet {
		d.handleGetRemoteBookmarkCredentials(w, r, id)
		return
	}

	switch r.Method {
	case http.MethodPut:
		d.handleUpdateRemoteBookmark(w, r, id)
	case http.MethodDelete:
		d.handleDeleteRemoteBookmark(w, r, id)
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) handleListRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := d.RemoteBookmarkStore.ListRemoteBookmarks(r.Context())
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, bookmarks)
}

type createRemoteBookmarkRequest struct {
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func (d *Deps) handleCreateRemoteBookmark(w http.ResponseWriter, r *http.Request) {
	var req createRemoteBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Label == "" || req.Protocol == "" || req.Host == "" || req.Port == 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "label, protocol, host, and port are required")
		return
	}

	bm := persistence.RemoteBookmark{
		Label:    req.Label,
		Protocol: req.Protocol,
		Host:     req.Host,
		Port:     req.Port,
	}

	// TODO: If username/password provided, store via credentials manager
	// and set bm.CredentialID

	if err := d.RemoteBookmarkStore.CreateRemoteBookmark(r.Context(), &bm); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	created, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), bm.ID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	servicehttp.WriteJSON(w, http.StatusCreated, created)
}

func (d *Deps) handleUpdateRemoteBookmark(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), id)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req createRemoteBookmarkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Label != "" {
		existing.Label = req.Label
	}
	if req.Protocol != "" {
		existing.Protocol = req.Protocol
	}
	if req.Host != "" {
		existing.Host = req.Host
	}
	if req.Port != 0 {
		existing.Port = req.Port
	}

	if err := d.RemoteBookmarkStore.UpdateRemoteBookmark(r.Context(), *existing); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, _ := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), id)
	servicehttp.WriteJSON(w, http.StatusOK, updated)
}

func (d *Deps) handleDeleteRemoteBookmark(w http.ResponseWriter, r *http.Request, id string) {
	if err := d.RemoteBookmarkStore.DeleteRemoteBookmark(r.Context(), id); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d *Deps) handleGetRemoteBookmarkCredentials(w http.ResponseWriter, r *http.Request, id string) {
	bm, err := d.RemoteBookmarkStore.GetRemoteBookmark(r.Context(), id)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			servicehttp.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		servicehttp.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// TODO: Decrypt and return credentials via credentials manager
	_ = bm
	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"username": nil,
		"password": nil,
	})
}
```

- [ ] **Step 2: Wire the store into Deps**

In `internal/hubapi/resources/deps.go`, find the `Deps` struct and add:

```go
RemoteBookmarkStore interface {
	ListRemoteBookmarks(ctx context.Context) ([]persistence.RemoteBookmark, error)
	GetRemoteBookmark(ctx context.Context, id string) (*persistence.RemoteBookmark, error)
	CreateRemoteBookmark(ctx context.Context, bm *persistence.RemoteBookmark) error
	UpdateRemoteBookmark(ctx context.Context, bm persistence.RemoteBookmark) error
	DeleteRemoteBookmark(ctx context.Context, id string) error
}
```

Then in `cmd/labtether/resources_bridge.go`, in the `ensureResourcesDeps()` function, set `RemoteBookmarkStore: s.db` (following the pattern used for `FileConnectionStore`).

- [ ] **Step 3: Add bridge method and register the route**

In `cmd/labtether/resources_bridge.go`, find how `handleFileConnections` is bridged (it calls `s.ensureResourcesDeps().HandleFileConnections(w, r)`). Add an identical bridge method for remote bookmarks:

```go
func (s *apiServer) handleRemoteBookmarks(w http.ResponseWriter, r *http.Request) {
	s.ensureResourcesDeps().HandleRemoteBookmarks(w, r)
}
```

In `cmd/labtether/http_handlers.go`, find where the file-connections route is registered (e.g., `"/api/v1/file-connections": s.withAuth(s.handleFileConnections)`) and add below it:

```go
"/api/v1/remote-bookmarks":  s.withAuth(s.handleRemoteBookmarks),
"/api/v1/remote-bookmarks/": s.withAuth(s.handleRemoteBookmarks),
```

- [ ] **Step 4: Verify Go compiles**

Run: `cd /Users/michael/Development/LabTether/hub && go vet ./...`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add internal/hubapi/resources/remote_bookmark_handlers.go internal/hubapi/resources/deps.go cmd/labtether/resources_bridge.go cmd/labtether/http_handlers.go
git commit -m "feat(remote-view): add remote bookmarks HTTP handlers and routing"
```

---

### Task 13: Screenshot Button in Toolbar

**Files:**
- Modify: `web/console/app/components/RemoteViewToolbar.tsx`

- [ ] **Step 1: Add onScreenshot prop**

Read `RemoteViewToolbar.tsx` first. Add `onScreenshot?: () => void` to the `RemoteViewToolbarProps` interface.

- [ ] **Step 2: Add Screenshot button**

Find the Capture/Recording section in the toolbar (near the recording toggle). Add a Screenshot button next to it:

```tsx
{onScreenshot && (
  <button
    onClick={onScreenshot}
    className={/* match existing button styling */}
    title="Take screenshot"
  >
    <Camera className="w-3.5 h-3.5" />
    <span className="hidden sm:inline">Screenshot</span>
  </button>
)}
```

Add the `Camera` import from `lucide-react` at the top of the file.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/console/app/components/RemoteViewToolbar.tsx
git commit -m "feat(remote-view): add screenshot button to toolbar"
```

---

### Task 14: Integration Verification

- [ ] **Step 1: TypeScript check**

Run: `cd web/console && npx tsc --noEmit --pretty`
Expected: Clean compile, no errors in remote-view/ files

- [ ] **Step 2: Go vet**

Run: `cd /Users/michael/Development/LabTether/hub && go vet ./...`
Expected: Clean

- [ ] **Step 3: Verify navigation works**

Start the dev frontend and navigate to `/remote-view` in the browser. Verify:
- Sidebar shows "Remote View" entry with Monitor icon
- Tab bar renders with one "New Tab"
- Landing page shows device grid (may be empty) and bookmarks section
- `+` button creates new tabs
- Closing all tabs creates a fallback "New Tab"

- [ ] **Step 4: Update docs**

Update `notes/PROGRESS_LOG.md` and `notes/TODO.md` with the Remote View tab work:
- Progress: "Remote View tab scaffolded — route, tab state, tab bar, landing page, session wrapper, bookmarks API"
- TODO: "Remote View tab: wire viewer components into RemoteViewSession, multi-protocol device picker popover, add bookmark form, credential flow for bookmarks"

- [ ] **Step 5: Final commit**

```bash
git add notes/PROGRESS_LOG.md notes/TODO.md
git commit -m "docs: update progress log and TODO for remote view tab"
```
