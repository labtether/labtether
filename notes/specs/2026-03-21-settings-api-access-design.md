# Settings Reorganization + API Access Management

**Date:** 2026-03-21
**Status:** Design approved, pending implementation plan

## Problem

The backend has complete programmatic access infrastructure (150+ REST API v2 endpoints, 23 MCP tools, API key management, webhooks, scheduled tasks, saved actions) but the console has zero UI for managing API keys, viewing MCP connection info, or configuring external API access. The API Docs page exists as a standalone OpenAPI viewer at `/api-docs` but is disconnected from any management UI.

The settings page also has an awkward "Advanced" toggle hiding half the content.

## Solution

Add a three-tab structure to Settings with a new "API Access" tab for API key management, MCP connection info, and API documentation.

## Tab Structure

Settings page gets `SegmentedTabs` at the top (reusing existing component). URL stays `/settings`, active tab persisted to localStorage (`labtether-settings-tab`).

| Tab | Cards |
|-----|-------|
| **General** | Appearance, Notification Channels, Prometheus Export, Backup/Export, About |
| **API Access** | API Keys (CRUD), MCP Connection (info), API Reference (link to `/api-docs`) |
| **Advanced** | Service Discovery Defaults, Discovery Overview, Runtime Settings, Retention Settings |

### What stays the same

- General tab contains all existing top-level cards (no disruption)
- Advanced tab contains all cards previously behind the "Advanced" toggle (removes the toggle, promotes to a proper tab)
- Standalone pages for `/webhooks`, `/schedules`, and `/actions` remain unchanged with their own sidebar entries

### Sidebar Change

Remove `/api-docs` as a separate nav item. It's absorbed into Settings > API Access as a link card.

## Card Designs

### API Keys Card (API Access tab)

Backend: `GET/POST /api/v2/keys`, `GET/PATCH/DELETE /api/v2/keys/{id}` (admin-only via `withAdminAuth`)

**Admin gating:** Non-admin users see a message: "API key management requires admin access." (following the pattern in UserAccessCard).

**Create section** (top of card):
- Grid form fields:
  - `name` (text input, required, max 120 chars)
  - `role` (dropdown: admin/operator/viewer — immutable after creation, PATCH does not support role changes)
  - `scopes` (grouped multi-select checkboxes, at least 1 required — see Scope Categories below)
  - `allowed_assets` (optional asset multi-select picker, max 200 — empty means "all assets")
  - `expires_at` (dropdown: 30 days / 90 days / 1 year / Never)
- "Create Key" button

**Key reveal modal** (on successful create, 201 response):
- Shows raw key (`lt_xxxx_...`) in monospace, full key visible
- Copy-to-clipboard button
- Warning text: "This key will not be shown again. Copy it now."
- Modal cannot be dismissed by clicking outside — requires explicit "I've copied the key" button
- Once dismissed, raw key is gone forever (only prefix stored server-side)

**Key table** (below create form):
- Columns: Prefix (`lt_xxxx`), Name, Role, Scopes (inline badges, truncated with +N overflow), Created (relative time), Last Used (relative time or "Never"), Actions
- Actions: Revoke button (DELETE with confirmation dialog)
- Empty state: "No API keys yet. Create one to enable programmatic access."

**Scope categories** (from backend `knownScopeCategories` in `internal/apikeys/scope.go`):

Grouped for the UI:

| Group | Categories |
|-------|-----------|
| Assets & Inventory | assets, groups, topology, discovery |
| Operations | shell, files, services, processes, cron |
| Monitoring | alerts, metrics, logs, incidents, notifications |
| System | network, disks, packages, users, settings, updates |
| Integrations | docker, connectors, homeassistant, agents, collectors, web-services |
| Automation | webhooks, schedules, actions, events, bulk |
| Platform | hub, failover, terminal, search, dead-letters, credentials, audit |

Plus a "Full Access" toggle that sets `["*"]`.

### MCP Connection Card (API Access tab)

No backend mutations — purely informational.

**Content:**
- Endpoint URL with copy button: `<origin>/mcp` (derived from `window.location.origin`)
- Transport badge: "HTTP Streamable"
- Connection snippets (copyable code blocks):
  - Claude Code: `claude mcp add labtether <origin>/mcp`
  - Generic MCP client config JSON block
- Auth note: "Uses session authentication. API key authentication coming soon."
- Collapsible "Available Tools" section listing tool categories with descriptions:
  - Asset Management — assets_list, assets_get
  - Command Execution — exec, exec_multi
  - Service Management — services_list, services_restart
  - File Operations — files_list, files_read
  - Docker — docker_hosts, docker_containers, docker_container_restart, docker_container_logs, docker_container_stats
  - System Info — system_processes, system_network, system_disks, system_packages
  - Alerts — alerts_list, alerts_acknowledge
  - Power Management — asset_reboot, asset_shutdown, asset_wake
  - Other — groups, metrics, schedules, webhooks, saved actions, credentials, topology, updates, connectors

Note: Tool list is hardcoded. This will need maintenance when MCP tools are added/removed. A future improvement could fetch this from the MCP server's tool listing.

### API Reference Card (API Access tab)

Small link card:
- Endpoint count badge (e.g., "150+ endpoints")
- Brief description: "Interactive API documentation with request/response schemas"
- "View API Documentation" button navigating to `/api-docs`

## New Files

### Hook

| Hook | File | Endpoints |
|------|------|-----------|
| `useApiKeys` | `hooks/useApiKeys.ts` | `/api/v2/keys` |

Follows the existing `useNotificationChannels` pattern: `useState` for list/loading/error, `useCallback` for CRUD operations, `useEffect` for initial fetch, refresh after mutations, `{ cache: "no-store" }`, `AbortSignal.timeout(15_000)`.

Returns: `{ keys, loading, error, createKey, revokeKey, refresh }`.

`createKey` returns the full response including `raw_key` (shown once in the reveal modal).

### Components

| Component | File |
|-----------|------|
| `ApiKeysCard` | `settings/components/ApiKeysCard.tsx` |
| `McpConnectionCard` | `settings/components/McpConnectionCard.tsx` |
| `ApiReferenceCard` | `settings/components/ApiReferenceCard.tsx` |

### Modified Files

| File | Change |
|------|--------|
| `settings/page.tsx` | Add SegmentedTabs for 3 tabs, remove Advanced toggle, conditionally render card groups per tab |
| `components/Sidebar.tsx` | Remove `/api-docs` nav entry |

### i18n

New translation keys under `settings` namespace for:
- Tab labels: `tabs.general`, `tabs.apiAccess`, `tabs.advanced`
- API Keys card: `apiKeys.title`, `apiKeys.description`, `apiKeys.create`, `apiKeys.revoke`, `apiKeys.revealWarning`, `apiKeys.emptyState`, `apiKeys.adminRequired`, etc.
- MCP card: `mcp.title`, `mcp.endpoint`, `mcp.transport`, `mcp.authNote`, `mcp.tools`, etc.
- API Reference card: `apiReference.title`, `apiReference.description`, `apiReference.viewDocs`

## UI Patterns (from existing codebase)

- Cards: `<Card className="mb-6">` wrapper with `text-xs font-mono uppercase tracking-wider text-[var(--muted)]` headings
- CRUD tables: Grid-based layout (header row + `border-t border-[var(--line)]` item rows in `border border-[var(--line)] rounded-xl overflow-hidden` container)
- Dialogs: State union pattern (`type Dialog = { type: "reveal"; rawKey: string } | { type: "revoke"; key: KeyInfo } | null`)
- Form inputs: `flex flex-col gap-1.5` labels, input with `border-[var(--panel-border)]` and `focus:ring-[var(--accent)]/40`
- Copy buttons: Icon button with transient "Copied!" feedback (pattern from PrometheusExportCard)
- Data fetching: `fetch()` with `{ cache: "no-store" }`, `AbortSignal.timeout(15_000)`, refetch after mutations

## Existing Components NOT Affected

These settings components exist in `settings/components/` but are either conditionally rendered or planned for future use. This spec does not touch them:
- UserAccessCard, AccountSecurityCard (user management — rendered on `/users`)
- ConnectAgentsCard, OIDCSettingsCard (auth config)
- TailscaleServeCard (Tailscale integration)
- TLSStatusCard, TLSManagementCard (TLS config)
- ManagedDatabaseCard (DB config)
- DangerZoneCard (destructive operations)

## Relationship to Existing Pages

- `/api-docs` page remains intact (the API Reference card links to it). Sidebar entry is removed.
- `/webhooks` page remains intact with its own sidebar entry under Operations. Not duplicated in Settings.
- `/schedules` page remains intact with its own sidebar entry under Operations. Not duplicated in Settings.
- `/actions` page (connector-based action execution, distinct from Saved Actions v2 API) remains intact.

## Non-Goals

- Webhook/schedule/action CRUD in Settings (already have standalone pages)
- Webhook delivery engine (event dispatching) — backend not built yet
- Cron execution engine — backend not built yet
- API key auth for MCP — backend not wired yet
- Editing API key role after creation — backend PATCH doesn't support it
