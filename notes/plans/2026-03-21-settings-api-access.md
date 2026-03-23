# Settings API Access Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a three-tab structure to the Settings page (General, API Access, Advanced) with full API key CRUD, MCP connection info, and API reference link. Remove /api-docs from the sidebar.

**Architecture:** Settings page gets SegmentedTabs at the top, rendering different card groups per tab. New `useApiKeys` hook handles API key CRUD against `/api/v2/keys`. Three new card components: ApiKeysCard (full CRUD with key reveal modal), McpConnectionCard (informational), ApiReferenceCard (link). Sidebar nav updated to remove standalone API Docs entry.

**Tech Stack:** Next.js App Router, TypeScript, existing Card/Button/SegmentedTabs components, next-intl translations, lucide-react icons.

**Spec:** `notes/specs/2026-03-21-settings-api-access-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `web/console/app/hooks/useApiKeys.ts` | Data fetching hook for `/api/v2/keys` — list, create, revoke |
| `web/console/app/[locale]/(console)/settings/components/ApiKeysCard.tsx` | API key management card: create form, key table, reveal modal, revoke dialog |
| `web/console/app/[locale]/(console)/settings/components/McpConnectionCard.tsx` | MCP endpoint info, connection snippets, tool list |
| `web/console/app/[locale]/(console)/settings/components/ApiReferenceCard.tsx` | Link card to /api-docs with endpoint count |

### Modified Files
| File | Change |
|------|--------|
| `web/console/messages/en/settings.json` | Add translation keys for tabs, API keys, MCP, API reference |
| `web/console/app/[locale]/(console)/settings/page.tsx` | Add SegmentedTabs for 3 tabs, reorganize cards, remove Advanced toggle |
| `web/console/app/components/Sidebar.tsx` | Remove `/api-docs` nav entry |

---

### Task 1: Add i18n Translation Keys

**Files:**
- Modify: `web/console/messages/en/settings.json`

- [ ] **Step 1: Read the current settings.json**

Already read — it's at `web/console/messages/en/settings.json` (184 lines).

- [ ] **Step 2: Add new translation keys**

Add these keys to the existing `settings.json`, after the `"discoveryDefaults"` section:

```json
"tabs": {
  "general": "General",
  "apiAccess": "API Access",
  "advanced": "Advanced"
},
"apiKeys": {
  "heading": "// API Keys",
  "description": "Create and manage API keys for programmatic access to the LabTether REST API v2.",
  "adminRequired": "API key management requires admin access.",
  "createKey": "Create Key",
  "name": "Name",
  "namePlaceholder": "My integration",
  "role": "Role",
  "roleAdmin": "Admin",
  "roleOperator": "Operator",
  "roleViewer": "Viewer",
  "scopes": "Scopes",
  "scopesFullAccess": "Full access",
  "scopesSelectCategories": "Select categories",
  "allowedAssets": "Allowed Assets",
  "allowedAssetsAll": "All assets",
  "allowedAssetsSelect": "Restrict to specific assets",
  "expiresAt": "Expires",
  "expires30d": "30 days",
  "expires90d": "90 days",
  "expires1y": "1 year",
  "expiresNever": "Never",
  "emptyState": "No API keys yet. Create one to enable programmatic access.",
  "revealTitle": "API Key Created",
  "revealWarning": "This key will not be shown again. Copy it now.",
  "revealCopied": "Key copied to clipboard",
  "revealDismiss": "I've copied the key",
  "colPrefix": "Key",
  "colName": "Name",
  "colRole": "Role",
  "colScopes": "Scopes",
  "colCreated": "Created",
  "colLastUsed": "Last Used",
  "colActions": "",
  "never": "Never",
  "revoke": "Revoke",
  "revokeTitle": "Revoke API Key",
  "revokeBody": "Revoke key \"{name}\"? Any integrations using this key will immediately lose access. This cannot be undone.",
  "revokeConfirm": "Revoke Key",
  "cancel": "Cancel"
},
"mcp": {
  "heading": "// MCP Connection",
  "description": "Connect AI agents (Claude, OpenClaw, etc.) to LabTether via the Model Context Protocol.",
  "endpoint": "MCP Endpoint",
  "transport": "HTTP Streamable",
  "claudeCode": "Claude Code",
  "genericConfig": "Generic MCP Config",
  "copy": "Copy",
  "copied": "Copied",
  "authNote": "Uses session authentication. API key authentication coming soon.",
  "toolsHeading": "Available Tools",
  "toolsToggle": "Show {count} tools"
},
"apiReference": {
  "heading": "// API Reference",
  "description": "Interactive documentation for the LabTether REST API v2 with request/response schemas.",
  "endpointCount": "150+ endpoints",
  "viewDocs": "View API Documentation"
}
```

- [ ] **Step 3: Verify JSON is valid**

Run: `cd web/console && node -e "JSON.parse(require('fs').readFileSync('messages/en/settings.json','utf8')); console.log('OK')"`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add web/console/messages/en/settings.json
git commit -m "feat(console): add i18n keys for settings tabs and API access"
```

---

### Task 2: Create useApiKeys Hook

**Files:**
- Create: `web/console/app/hooks/useApiKeys.ts`

- [ ] **Step 1: Create the hook file**

Follow the `useNotificationChannels` pattern at `web/console/app/hooks/useNotificationChannels.ts`. The hook calls `/api/v2/keys` (admin-only endpoints).

```typescript
"use client";

import { useCallback, useEffect, useState } from "react";
import { sanitizeErrorMessage } from "../lib/sanitizeErrorMessage";

export type ApiKeyInfo = {
  id: string;
  name: string;
  prefix: string;
  role: string;
  scopes: string[];
  allowed_assets?: string[];
  expires_at?: string | null;
  created_by: string;
  created_at: string;
  last_used_at?: string | null;
};

export type CreateKeyRequest = {
  name: string;
  role: string;
  scopes: string[];
  allowed_assets?: string[];
  expires_at?: string | null;
};

export type CreatedKeyResponse = ApiKeyInfo & {
  raw_key: string;
};

/* v2 API response envelopes — all responses wrapped in { request_id, data } or { error, message, status } */

type V2ListPayload = {
  data?: ApiKeyInfo[];
  meta?: { total: number; page: number; per_page: number };
  error?: string;
  message?: string;
};

type V2CreatePayload = {
  data?: CreatedKeyResponse;
  error?: string;
  message?: string;
};

type V2MutatePayload = {
  data?: { status?: string };
  error?: string;
  message?: string;
};

export function useApiKeys() {
  const [keys, setKeys] = useState<ApiKeyInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const response = await fetch("/api/v2/keys", {
        cache: "no-store",
        signal: AbortSignal.timeout(15_000),
      });
      const payload = (await response.json()) as V2ListPayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to load API keys (${response.status})`);
      }
      setKeys(payload.data ?? []);
    } catch (err) {
      setError(sanitizeErrorMessage(err instanceof Error ? err.message : "", "failed to load API keys"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const createKey = useCallback(
    async (req: CreateKeyRequest): Promise<CreatedKeyResponse> => {
      const response = await fetch("/api/v2/keys", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        signal: AbortSignal.timeout(15_000),
        body: JSON.stringify(req),
      });
      const payload = (await response.json()) as V2CreatePayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to create API key (${response.status})`);
      }
      await refresh();
      return payload.data!;
    },
    [refresh],
  );

  const revokeKey = useCallback(
    async (id: string) => {
      const response = await fetch(`/api/v2/keys/${encodeURIComponent(id)}`, {
        method: "DELETE",
        signal: AbortSignal.timeout(15_000),
      });
      const payload = (await response.json()) as V2MutatePayload;
      if (!response.ok) {
        throw new Error(payload.message || payload.error || `failed to revoke API key (${response.status})`);
      }
      await refresh();
    },
    [refresh],
  );

  return { keys, loading, error, refresh, createKey, revokeKey };
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors related to `useApiKeys.ts`

- [ ] **Step 3: Commit**

```bash
git add web/console/app/hooks/useApiKeys.ts
git commit -m "feat(console): add useApiKeys data fetching hook"
```

---

### Task 3: Create McpConnectionCard Component

**Files:**
- Create: `web/console/app/[locale]/(console)/settings/components/McpConnectionCard.tsx`

- [ ] **Step 1: Create the MCP connection info card**

This is a read-only informational card — no data fetching needed. It derives the MCP URL from `window.location.origin`.

```typescript
"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Copy, Check, ChevronDown, ChevronRight } from "lucide-react";
import { Card } from "../../../../components/ui/Card";

const MCP_TOOL_GROUPS = [
  { name: "Asset Management", tools: "assets_list, assets_get" },
  { name: "Command Execution", tools: "exec, exec_multi" },
  { name: "Service Management", tools: "services_list, services_restart" },
  { name: "File Operations", tools: "files_list, files_read" },
  { name: "Docker", tools: "docker_hosts, docker_containers, docker_container_restart, docker_container_logs, docker_container_stats" },
  { name: "System Info", tools: "system_processes, system_network, system_disks, system_packages" },
  { name: "Alerts", tools: "alerts_list, alerts_acknowledge" },
  { name: "Power Management", tools: "asset_reboot, asset_shutdown, asset_wake" },
  { name: "Other", tools: "groups, metrics, schedules, webhooks, saved_actions, credentials, topology, updates, connectors" },
] as const;

const TOTAL_TOOLS = 23;

function CopyButton({ text, label }: { text: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const t = useTranslations("settings");

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      type="button"
      onClick={() => { void handleCopy(); }}
      className="inline-flex items-center gap-1 px-2 py-1 text-[10px] font-mono text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] rounded transition-colors cursor-pointer bg-transparent border-none"
      aria-label={label}
    >
      {copied ? <Check size={12} className="text-[var(--good)]" /> : <Copy size={12} />}
      {copied ? t("mcp.copied") : t("mcp.copy")}
    </button>
  );
}

export function McpConnectionCard() {
  const t = useTranslations("settings");
  const [toolsOpen, setToolsOpen] = useState(false);
  const [origin, setOrigin] = useState("https://<hub-url>");

  useEffect(() => { setOrigin(window.location.origin); }, []);

  const mcpUrl = `${origin}/mcp`;

  const claudeSnippet = `claude mcp add labtether ${mcpUrl}`;
  const genericSnippet = JSON.stringify(
    { mcpServers: { labtether: { url: mcpUrl, transport: "streamable-http" } } },
    null,
    2,
  );

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
        {t("mcp.heading")}
      </p>
      <p className="text-xs text-[var(--muted)] mb-4">{t("mcp.description")}</p>

      {/* Endpoint */}
      <div className="flex items-center gap-2 mb-4">
        <span className="text-xs text-[var(--muted)]">{t("mcp.endpoint")}:</span>
        <code className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] px-2 py-1 rounded">
          {mcpUrl}
        </code>
        <CopyButton text={mcpUrl} label="Copy MCP URL" />
        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">
          {t("mcp.transport")}
        </span>
      </div>

      {/* Claude Code snippet */}
      <div className="mb-3">
        <div className="flex items-center justify-between mb-1">
          <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)]">
            {t("mcp.claudeCode")}
          </span>
          <CopyButton text={claudeSnippet} label="Copy Claude Code command" />
        </div>
        <pre className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] border border-[var(--panel-border)] rounded-lg p-3 overflow-x-auto">
          {claudeSnippet}
        </pre>
      </div>

      {/* Generic config snippet */}
      <div className="mb-4">
        <div className="flex items-center justify-between mb-1">
          <span className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)]">
            {t("mcp.genericConfig")}
          </span>
          <CopyButton text={genericSnippet} label="Copy MCP config" />
        </div>
        <pre className="text-xs font-mono text-[var(--text)] bg-[var(--surface)] border border-[var(--panel-border)] rounded-lg p-3 overflow-x-auto">
          {genericSnippet}
        </pre>
      </div>

      {/* Auth note */}
      <p className="text-[10px] text-[var(--muted)] mb-4 italic">{t("mcp.authNote")}</p>

      {/* Collapsible tools list */}
      <button
        type="button"
        onClick={() => setToolsOpen((prev) => !prev)}
        className="flex items-center gap-1.5 text-xs text-[var(--muted)] hover:text-[var(--text)] transition-colors cursor-pointer bg-transparent border-none p-0"
      >
        {toolsOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        {t("mcp.toolsToggle", { count: TOTAL_TOOLS })}
      </button>

      {toolsOpen && (
        <div className="mt-2 space-y-1.5 ml-5">
          {MCP_TOOL_GROUPS.map((group) => (
            <div key={group.name}>
              <span className="text-xs text-[var(--text)]">{group.name}</span>
              <span className="text-[10px] font-mono text-[var(--muted)] ml-2">{group.tools}</span>
            </div>
          ))}
        </div>
      )}
    </Card>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors related to `McpConnectionCard.tsx`

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/settings/components/McpConnectionCard.tsx
git commit -m "feat(console): add MCP connection info card for settings"
```

---

### Task 4: Create ApiReferenceCard Component

**Files:**
- Create: `web/console/app/[locale]/(console)/settings/components/ApiReferenceCard.tsx`

- [ ] **Step 1: Create the API reference link card**

Small card linking to the `/api-docs` OpenAPI explorer page.

```typescript
"use client";

import { useTranslations } from "next-intl";
import { ExternalLink } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Link } from "../../../../../i18n/navigation";

export function ApiReferenceCard() {
  const t = useTranslations("settings");

  return (
    <Card className="mb-6">
      <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
        {t("apiReference.heading")}
      </p>
      <p className="text-xs text-[var(--muted)] mb-3">{t("apiReference.description")}</p>
      <div className="flex items-center gap-3">
        <Link
          href="/api-docs"
          className="inline-flex items-center gap-2 rounded-lg px-2.5 py-1 text-xs font-medium bg-transparent border border-[var(--control-border)] text-[var(--control-fg)] hover:bg-[var(--control-bg-hover)] hover:border-[var(--text-secondary)] transition-[color,background-color,border-color,box-shadow,opacity] duration-[var(--dur-fast)]"
        >
          <ExternalLink size={13} />
          {t("apiReference.viewDocs")}
        </Link>
        <span className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
          {t("apiReference.endpointCount")}
        </span>
      </div>
    </Card>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors related to `ApiReferenceCard.tsx`

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/settings/components/ApiReferenceCard.tsx
git commit -m "feat(console): add API reference link card for settings"
```

---

### Task 5: Create ApiKeysCard Component

**Files:**
- Create: `web/console/app/[locale]/(console)/settings/components/ApiKeysCard.tsx`

This is the largest component. It has four sub-sections: admin gate, create form, key table, and two modals (reveal + revoke).

- [ ] **Step 1: Create the API keys card**

```typescript
"use client";

import { useState, useCallback } from "react";
import { useTranslations } from "next-intl";
import { Plus, Copy, Check, Trash2 } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { useApiKeys } from "../../../../hooks/useApiKeys";
import type { ApiKeyInfo, CreateKeyRequest, CreatedKeyResponse } from "../../../../hooks/useApiKeys";
import { useAuth } from "../../../../contexts/AuthContext";

/* ── Scope category groups for the create form ── */

const SCOPE_GROUPS: { label: string; categories: string[] }[] = [
  { label: "Assets & Inventory", categories: ["assets", "groups", "topology", "discovery"] },
  { label: "Operations", categories: ["shell", "files", "services", "processes", "cron"] },
  { label: "Monitoring", categories: ["alerts", "metrics", "logs", "incidents", "notifications"] },
  { label: "System", categories: ["network", "disks", "packages", "users", "settings", "updates"] },
  { label: "Integrations", categories: ["docker", "connectors", "homeassistant", "agents", "collectors", "web-services"] },
  { label: "Automation", categories: ["webhooks", "schedules", "actions", "events", "bulk"] },
  { label: "Platform", categories: ["hub", "failover", "terminal", "search", "dead-letters", "credentials", "audit"] },
];

/* ── Expiry options ── */

type ExpiryOption = "30d" | "90d" | "1y" | "never";

function expiryToDate(option: ExpiryOption): string | null {
  const now = new Date();
  switch (option) {
    case "30d": return new Date(now.getTime() + 30 * 86400000).toISOString();
    case "90d": return new Date(now.getTime() + 90 * 86400000).toISOString();
    case "1y": return new Date(now.getTime() + 365 * 86400000).toISOString();
    case "never": return null;
  }
}

/* ── Relative time helper ── */

function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  const seconds = Math.floor(diff / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

/* ── Key Reveal Modal ── */

function KeyRevealModal({
  rawKey,
  onDismiss,
}: {
  rawKey: string;
  onDismiss: () => void;
}) {
  const t = useTranslations("settings");
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(rawKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 3000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[32rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revealTitle")}</h3>
          <div className="bg-[var(--surface)] border border-[var(--panel-border)] rounded-lg p-3 font-mono text-xs text-[var(--text)] break-all select-all">
            {rawKey}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="sm" onClick={() => { void handleCopy(); }}>
              {copied ? <Check size={13} className="text-[var(--good)]" /> : <Copy size={13} />}
              {copied ? t("apiKeys.revealCopied") : t("mcp.copy")}
            </Button>
          </div>
          <p className="text-xs text-[var(--bad)]">{t("apiKeys.revealWarning")}</p>
          <div className="flex justify-end">
            <Button variant="primary" onClick={onDismiss}>{t("apiKeys.revealDismiss")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

/* ── Revoke Confirm Modal ── */

function RevokeConfirmModal({
  keyInfo,
  onClose,
  onConfirm,
}: {
  keyInfo: ApiKeyInfo;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  const t = useTranslations("settings");
  const [revoking, setRevoking] = useState(false);
  const [error, setError] = useState("");

  const handleConfirm = async () => {
    setRevoking(true);
    setError("");
    try {
      await onConfirm();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to revoke key.");
      setRevoking(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={() => { if (!revoking) onClose(); }}
    >
      <div onClick={(e) => e.stopPropagation()}>
        <Card className="w-[28rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">{t("apiKeys.revokeTitle")}</h3>
          <p className="text-xs text-[var(--muted)]">
            {t("apiKeys.revokeBody", { name: keyInfo.name })}
          </p>
          {error ? <p className="text-xs text-[var(--bad)]">{error}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={revoking}>{t("apiKeys.cancel")}</Button>
            <Button variant="danger" loading={revoking} onClick={() => { void handleConfirm(); }}>{t("apiKeys.revokeConfirm")}</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}

/* ── Main Card ── */

type Dialog =
  | { type: "reveal"; rawKey: string }
  | { type: "revoke"; key: ApiKeyInfo }
  | null;

export function ApiKeysCard() {
  const t = useTranslations("settings");
  const { user } = useAuth();
  const isAdmin = user?.role === "owner" || user?.role === "admin";
  const { keys, loading, error, createKey, revokeKey } = useApiKeys();

  const [dialog, setDialog] = useState<Dialog>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState("");

  /* Create form state */
  const [name, setName] = useState("");
  const [role, setRole] = useState("operator");
  const [fullAccess, setFullAccess] = useState(false);
  const [selectedScopes, setSelectedScopes] = useState<Set<string>>(new Set());
  const [expiry, setExpiry] = useState<ExpiryOption>("90d");

  const toggleScope = useCallback((category: string) => {
    setSelectedScopes((prev) => {
      const next = new Set(prev);
      if (next.has(category)) next.delete(category);
      else next.add(category);
      return next;
    });
  }, []);

  const handleCreate = async () => {
    setCreating(true);
    setCreateError("");
    try {
      const scopes = fullAccess ? ["*"] : [...selectedScopes];
      if (scopes.length === 0) {
        setCreateError("Select at least one scope.");
        setCreating(false);
        return;
      }
      const req: CreateKeyRequest = {
        name: name.trim(),
        role,
        scopes,
        expires_at: expiryToDate(expiry),
      };
      const result: CreatedKeyResponse = await createKey(req);
      setDialog({ type: "reveal", rawKey: result.raw_key });
      /* Reset form */
      setName("");
      setRole("operator");
      setFullAccess(false);
      setSelectedScopes(new Set());
      setExpiry("90d");
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : "Failed to create key.");
    } finally {
      setCreating(false);
    }
  };

  /* Admin gate */
  if (!isAdmin) {
    return (
      <Card className="mb-6">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
          {t("apiKeys.heading")}
        </p>
        <p className="text-xs text-[var(--muted)]">{t("apiKeys.adminRequired")}</p>
      </Card>
    );
  }

  return (
    <>
      <Card className="mb-6">
        <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
          {t("apiKeys.heading")}
        </p>
        <p className="text-xs text-[var(--muted)] mb-4">{t("apiKeys.description")}</p>

        {/* ── Create form ── */}
        <div className="grid gap-3 md:grid-cols-[1fr_120px_120px_auto] items-end mb-4">
          <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
            {t("apiKeys.name")}
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("apiKeys.namePlaceholder")}
              maxLength={120}
              className="rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-3 py-1.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/40"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
            {t("apiKeys.role")}
            <select
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-2 py-1.5 text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/40"
            >
              <option value="admin">{t("apiKeys.roleAdmin")}</option>
              <option value="operator">{t("apiKeys.roleOperator")}</option>
              <option value="viewer">{t("apiKeys.roleViewer")}</option>
            </select>
          </label>
          <label className="flex flex-col gap-1 text-xs text-[var(--muted)]">
            {t("apiKeys.expiresAt")}
            <select
              value={expiry}
              onChange={(e) => setExpiry(e.target.value as ExpiryOption)}
              className="rounded-lg border border-[var(--panel-border)] bg-[var(--panel-glass)] px-2 py-1.5 text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]/40"
            >
              <option value="30d">{t("apiKeys.expires30d")}</option>
              <option value="90d">{t("apiKeys.expires90d")}</option>
              <option value="1y">{t("apiKeys.expires1y")}</option>
              <option value="never">{t("apiKeys.expiresNever")}</option>
            </select>
          </label>
          <Button
            variant="primary"
            size="sm"
            loading={creating}
            disabled={!name.trim()}
            onClick={() => { void handleCreate(); }}
          >
            <Plus size={13} />
            {t("apiKeys.createKey")}
          </Button>
        </div>

        {/* ── Scope selector ── */}
        <div className="mb-4">
          <div className="flex items-center gap-3 mb-2">
            <span className="text-xs text-[var(--muted)]">{t("apiKeys.scopes")}:</span>
            <label className="flex items-center gap-1.5 text-xs text-[var(--text)] cursor-pointer">
              <input
                type="checkbox"
                checked={fullAccess}
                onChange={(e) => setFullAccess(e.target.checked)}
                className="accent-[var(--accent)]"
              />
              {t("apiKeys.scopesFullAccess")}
            </label>
          </div>
          {!fullAccess && (
            <div className="grid gap-2 md:grid-cols-2 lg:grid-cols-3">
              {SCOPE_GROUPS.map((group) => (
                <div key={group.label}>
                  <p className="text-[10px] font-mono uppercase tracking-wider text-[var(--muted)] mb-1">
                    {group.label}
                  </p>
                  <div className="flex flex-wrap gap-x-3 gap-y-1">
                    {group.categories.map((cat) => (
                      <label key={cat} className="flex items-center gap-1 text-xs text-[var(--text)] cursor-pointer">
                        <input
                          type="checkbox"
                          checked={selectedScopes.has(cat)}
                          onChange={() => toggleScope(cat)}
                          className="accent-[var(--accent)]"
                        />
                        {cat}
                      </label>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {createError && <p className="text-xs text-[var(--bad)] mb-3">{createError}</p>}

        {/* ── Key table ── */}
        {loading && <p className="text-xs text-[var(--muted)] py-2">&nbsp;</p>}

        {!loading && error && <p className="text-xs text-[var(--bad)]">{error}</p>}

        {!loading && !error && keys.length === 0 && (
          <p className="text-xs text-[var(--muted)] py-1">{t("apiKeys.emptyState")}</p>
        )}

        {!loading && keys.length > 0 && (
          <div className="border border-[var(--line)] rounded-xl overflow-hidden">
            {/* Header */}
            <div className="grid grid-cols-[80px_1fr_80px_1fr_100px_100px_40px] gap-3 px-3 py-2 bg-[var(--surface)] text-[10px] font-mono uppercase tracking-wider text-[var(--muted)]">
              <span>{t("apiKeys.colPrefix")}</span>
              <span>{t("apiKeys.colName")}</span>
              <span>{t("apiKeys.colRole")}</span>
              <span>{t("apiKeys.colScopes")}</span>
              <span>{t("apiKeys.colCreated")}</span>
              <span>{t("apiKeys.colLastUsed")}</span>
              <span />
            </div>
            {/* Rows */}
            {keys.map((key) => (
              <div
                key={key.id}
                className="grid grid-cols-[80px_1fr_80px_1fr_100px_100px_40px] gap-3 px-3 py-2 border-t border-[var(--line)] items-center text-xs"
              >
                <span className="font-mono text-[var(--muted)]">lt_{key.prefix}</span>
                <span className="text-[var(--text)] truncate">{key.name}</span>
                <span className="text-[var(--muted)]">{key.role}</span>
                <div className="flex flex-wrap gap-1">
                  {(key.scopes[0] === "*"
                    ? [<span key="all" className="text-[10px] px-1.5 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20">all</span>]
                    : key.scopes.slice(0, 3).map((s) => (
                        <span key={s} className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--surface)] text-[var(--muted)] border border-[var(--line)]">
                          {s}
                        </span>
                      ))
                  )}
                  {key.scopes[0] !== "*" && key.scopes.length > 3 && (
                    <span className="text-[10px] text-[var(--muted)]">+{key.scopes.length - 3}</span>
                  )}
                </div>
                <span className="text-[var(--muted)]">{relativeTime(key.created_at)}</span>
                <span className="text-[var(--muted)]">{key.last_used_at ? relativeTime(key.last_used_at) : t("apiKeys.never")}</span>
                <button
                  type="button"
                  onClick={() => setDialog({ type: "revoke", key })}
                  className="flex items-center justify-center h-7 w-7 rounded-md text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--hover)] transition-colors cursor-pointer bg-transparent border-none"
                  aria-label={t("apiKeys.revoke")}
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Modals */}
      {dialog?.type === "reveal" && (
        <KeyRevealModal rawKey={dialog.rawKey} onDismiss={() => setDialog(null)} />
      )}
      {dialog?.type === "revoke" && (
        <RevokeConfirmModal
          keyInfo={dialog.key}
          onClose={() => setDialog(null)}
          onConfirm={() => revokeKey(dialog.key.id)}
        />
      )}
    </>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors related to `ApiKeysCard.tsx`

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/settings/components/ApiKeysCard.tsx
git commit -m "feat(console): add API keys management card with CRUD and modals"
```

---

### Task 6: Refactor Settings Page with Tabs

**Files:**
- Modify: `web/console/app/[locale]/(console)/settings/page.tsx`

- [ ] **Step 1: Refactor the settings page**

Replace the entire settings page with the three-tab structure. The General tab contains all current top-level cards. The API Access tab contains the three new cards. The Advanced tab contains what was behind the toggle.

Key changes:
- Add `SegmentedTabs` at the top with General / API Access / Advanced
- Tab state stored in localStorage (`labtether-settings-tab`)
- Remove the "Advanced" toggle button
- Import the three new card components
- Preserve all existing hook calls and prop passing (only for Advanced tab)

The full replacement for `page.tsx`:

```typescript
"use client";

import { useCallback, useMemo, useState, useEffect } from "react";
import { useTranslations } from "next-intl";
import { PageHeader } from "../../../components/PageHeader";
import { useTheme, accentOptions } from "../../../contexts/ThemeContext";
import { useStatusAssetNameMap } from "../../../contexts/StatusContext";
import { useSettingsForm } from "../../../hooks/useSettingsForm";
import { useWebServices } from "../../../hooks/useWebServices";
import {
  densityOptions,
  runtimeSettingKeys,
  themeOptions,
} from "../../../console/models";
import { Card } from "../../../components/ui/Card";
import { SegmentedTabs } from "../../../components/ui/SegmentedTabs";
import { AdvancedSettingsCard } from "./components/AdvancedSettingsCard";
import { RetentionSettingsCard } from "./components/RetentionSettingsCard";
import { ServiceDiscoveryDefaultsCard } from "./components/ServiceDiscoveryDefaultsCard";
import { ServicesDiscoveryOverviewCard } from "../services/ServicesDiscoveryOverviewCard";
import { buildDiscoveryOverview } from "../services/servicesDiscoveryHelpers";
import { AboutCard } from "./components/AboutCard";
import { NotificationChannelsCard } from "./components/NotificationChannelsCard";
import { PrometheusExportCard } from "./components/PrometheusExportCard";
import { BackupExportCard } from "./components/BackupExportCard";
import { ApiKeysCard } from "./components/ApiKeysCard";
import { McpConnectionCard } from "./components/McpConnectionCard";
import { ApiReferenceCard } from "./components/ApiReferenceCard";

const SECTION_HEADING = "text-xs font-semibold uppercase tracking-wider text-[var(--muted)]";
const TAB_LS_KEY = "labtether-settings-tab";

type SettingsTab = "general" | "apiAccess" | "advanced";
const TAB_OPTIONS: SettingsTab[] = ["general", "apiAccess", "advanced"];

const POLL_INTERVAL_KEY = runtimeSettingKeys.pollIntervalSeconds;
const POLL_INTERVAL_VALUES = ["0", "5", "10", "30", "60"] as const;
type PollIntervalValue = (typeof POLL_INTERVAL_VALUES)[number];

export default function SettingsPage() {
  const t = useTranslations("settings");
  const [activeTab, setActiveTab] = useState<SettingsTab>("general");

  useEffect(() => {
    const stored = localStorage.getItem(TAB_LS_KEY);
    if (stored && TAB_OPTIONS.includes(stored as SettingsTab)) {
      setActiveTab(stored as SettingsTab);
    }
  }, []);

  const handleTabChange = useCallback((tab: SettingsTab) => {
    setActiveTab(tab);
    localStorage.setItem(TAB_LS_KEY, tab);
  }, []);

  const { theme, setTheme, density, setDensity, accent, setAccent } = useTheme();
  const assetNameMap = useStatusAssetNameMap();
  const { discoveryStats } = useWebServices({});
  const discoveryOverview = useMemo(
    () => buildDiscoveryOverview(discoveryStats, assetNameMap),
    [discoveryStats, assetNameMap],
  );
  const {
    runtimeSettings,
    runtimeDraftValues,
    setRuntimeDraftValues,
    runtimeSettingsLoading,
    runtimeSettingsSaving,
    runtimeSettingsError,
    runtimeSettingsMessage,
    runtimeSettingsByScope,
    saveRuntimeSettings,
    resetRuntimeSetting,
    retentionPresets,
    retentionDraftValues,
    setRetentionDraftValues,
    retentionLoading,
    retentionSaving,
    retentionMessage,
    applyRetentionPreset,
    saveRetentionSettings,
  } = useSettingsForm();

  const [pendingPollSave, setPendingPollSave] = useState(false);

  useEffect(() => {
    if (pendingPollSave) {
      setPendingPollSave(false);
      void saveRuntimeSettings([POLL_INTERVAL_KEY]);
    }
  }, [pendingPollSave, saveRuntimeSettings]);

  const handlePollIntervalChange = useCallback(
    (value: PollIntervalValue) => {
      setRuntimeDraftValues((prev) => ({ ...prev, [POLL_INTERVAL_KEY]: value }));
      setPendingPollSave(true);
    },
    [setRuntimeDraftValues],
  );

  const pollIntervalValue: PollIntervalValue = POLL_INTERVAL_VALUES.includes(
    runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue,
  )
    ? (runtimeDraftValues[POLL_INTERVAL_KEY] as PollIntervalValue)
    : "5";

  return (
    <>
      <PageHeader title={t("title")} subtitle={t("subtitle")} />

      {/* Tab navigation */}
      <div className="mb-6">
        <SegmentedTabs
          value={activeTab}
          options={TAB_OPTIONS.map((tab) => ({ id: tab, label: t(`tabs.${tab}`) }))}
          onChange={handleTabChange}
        />
      </div>

      {/* ── General tab ── */}
      {activeTab === "general" && (
        <>
          <Card className="mb-6">
            <p className="text-xs font-mono uppercase tracking-wider text-[var(--muted)] mb-3">
              {t("appearance.heading")}
            </p>
            <div className="flex flex-wrap items-center gap-6">
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t("appearance.theme")}</span>
                <SegmentedTabs
                  value={theme}
                  options={themeOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setTheme}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t("appearance.layout")}</span>
                <SegmentedTabs
                  value={density}
                  options={densityOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setDensity}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t("appearance.accent")}</span>
                <SegmentedTabs
                  value={accent}
                  options={accentOptions.map((option) => ({ id: option.id, label: option.label }))}
                  onChange={setAccent}
                />
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs text-[var(--muted)]">{t("appearance.autoRefreshLabel")}</span>
                <SegmentedTabs
                  value={pollIntervalValue}
                  options={[
                    { id: "0", label: t("appearance.autoRefreshOff") },
                    { id: "5", label: "5s" },
                    { id: "10", label: "10s" },
                    { id: "30", label: "30s" },
                    { id: "60", label: "60s" },
                  ]}
                  onChange={handlePollIntervalChange}
                />
              </div>
            </div>
          </Card>

          <NotificationChannelsCard />
          <PrometheusExportCard />
          <BackupExportCard />
          <AboutCard />
        </>
      )}

      {/* ── API Access tab ── */}
      {activeTab === "apiAccess" && (
        <>
          <ApiKeysCard />
          <McpConnectionCard />
          <ApiReferenceCard />
        </>
      )}

      {/* ── Advanced tab ── */}
      {activeTab === "advanced" && (
        <>
          <ServiceDiscoveryDefaultsCard
            runtimeSettings={runtimeSettings}
            runtimeDraftValues={runtimeDraftValues}
            setRuntimeDraftValues={setRuntimeDraftValues}
            runtimeSettingsLoading={runtimeSettingsLoading}
            runtimeSettingsSaving={runtimeSettingsSaving}
            saveRuntimeSettings={saveRuntimeSettings}
          />

          <ServicesDiscoveryOverviewCard discoveryOverview={discoveryOverview} />

          <AdvancedSettingsCard
            sectionHeadingClassName={SECTION_HEADING}
            runtimeSettingsLoading={runtimeSettingsLoading}
            runtimeSettingsError={runtimeSettingsError}
            runtimeSettingsByScope={runtimeSettingsByScope}
            runtimeDraftValues={runtimeDraftValues}
            setRuntimeDraftValues={setRuntimeDraftValues}
            runtimeSettingsSaving={runtimeSettingsSaving}
            runtimeSettingsMessage={runtimeSettingsMessage}
            saveRuntimeSettings={saveRuntimeSettings}
            resetRuntimeSetting={resetRuntimeSetting}
          />

          <RetentionSettingsCard
            sectionHeadingClassName={SECTION_HEADING}
            retentionPresets={retentionPresets}
            retentionDraftValues={retentionDraftValues}
            setRetentionDraftValues={setRetentionDraftValues}
            retentionLoading={retentionLoading}
            retentionSaving={retentionSaving}
            retentionMessage={retentionMessage}
            applyRetentionPreset={applyRetentionPreset}
            saveRetentionSettings={saveRetentionSettings}
          />
        </>
      )}
    </>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add web/console/app/[locale]/(console)/settings/page.tsx
git commit -m "feat(console): refactor settings page into three tabs (General, API Access, Advanced)"
```

---

### Task 7: Remove /api-docs from Sidebar

**Files:**
- Modify: `web/console/app/components/Sidebar.tsx`

- [ ] **Step 1: Read the sidebar nav structure**

The relevant section is around line 86 in `Sidebar.tsx`:
```
{ href: "/api-docs", label: "API Docs", translationKey: "apiDocs", icon: BookOpen },
```

- [ ] **Step 2: Remove the api-docs nav entry**

Remove the line `{ href: "/api-docs", label: "API Docs", translationKey: "apiDocs", icon: BookOpen },` from the `navGroups` array. Also remove the `BookOpen` import from lucide-react if it's no longer used elsewhere in the file.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npx tsc --noEmit --pretty 2>&1 | head -20`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add web/console/app/components/Sidebar.tsx
git commit -m "feat(console): remove standalone API Docs from sidebar (now in Settings > API Access)"
```

---

### Task 8: TypeScript Full Validation

**Files:**
- None (validation only)

- [ ] **Step 1: Run full TypeScript check**

Run: `cd web/console && npx tsc --noEmit --pretty`
Expected: Clean build, no errors

- [ ] **Step 2: Run lint if available**

Run: `cd web/console && npm run lint 2>&1 | tail -20`
Expected: No new lint errors

- [ ] **Step 3: Final commit if any fixups were needed**

```bash
git add -A
git commit -m "fix(console): address lint/type errors from settings API access work"
```

---

### Task 9: Update Documentation and Notes

**Files:**
- Modify: `notes/PROGRESS_LOG.md`
- Modify: `notes/TODO.md`

- [ ] **Step 1: Update PROGRESS_LOG.md**

Add an entry for this work:

```markdown
### 2026-03-21: Settings Reorganization + API Access Management
- Reorganized Settings page into three tabs: General, API Access, Advanced
- Added API Keys CRUD card with create form, scope selector, key reveal modal, revoke dialog
- Added MCP Connection info card with endpoint URL, connection snippets, tool list
- Added API Reference link card pointing to /api-docs OpenAPI explorer
- Removed /api-docs from sidebar nav (absorbed into Settings > API Access)
- Created useApiKeys hook for API key data fetching
- Added i18n translation keys for all new UI
```

- [ ] **Step 2: Update TODO.md**

Remove or mark complete any items related to "API key UI" or "settings reorganization". Add future work items if relevant:
- API key auth for MCP endpoint
- allowed_assets picker in API key create form (currently omitted from form, backend supports it)

- [ ] **Step 3: Commit**

```bash
git add notes/PROGRESS_LOG.md notes/TODO.md
git commit -m "docs: update progress log and TODO for settings API access work"
```
