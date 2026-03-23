# Unified Connectivity Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the split Services + Protocols section on the device detail page with a unified "Connect" card in the capability grid and a full Connect panel for managing all connections.

**Architecture:** Add a "connect" panel to the existing panel system (`devicePanelConnectorPanels.ts`). The panel groups protocols (from `useProtocolConfigs`) and web services (from `useWebServices({ host: nodeId })`) into one view. A new `ConnectPanel.tsx` component renders both sections. The "+ Add Connection" button is added to `DeviceIdentityBar` and navigates to `?panel=connect&adding=true`.

**Tech Stack:** Next.js App Router, TypeScript, existing `useProtocolConfigs` and `useWebServices` hooks, existing `ProtocolForm` component.

**Spec:** `notes/specs/2026-03-21-unified-connectivity-panel-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `web/console/app/[locale]/(console)/nodes/[id]/devicePanelConnectorPanels.ts` | Modify | Add "connect" panel definition |
| `web/console/app/[locale]/(console)/nodes/[id]/devicePanelTypes.ts` | Modify | Add `protocolCount`, `webServiceCount`, `connectionBadges` to `PanelContext` |
| `web/console/app/[locale]/(console)/nodes/[id]/nodeDetailPageModelBuilders.ts` | Modify | Update `buildNodePanelContext()` to accept and pass through new fields |
| `web/console/app/[locale]/(console)/nodes/[id]/useNodeDetailPageModel.ts` | Modify | Pass connection counts into `PanelContext` |
| `web/console/app/[locale]/(console)/nodes/[id]/ConnectPanel.tsx` | Create | Full panel view with grouped protocols + web services |
| `web/console/app/[locale]/(console)/nodes/[id]/WebServiceForm.tsx` | Create | Add/edit form for web services (name, URL, category) |
| `web/console/app/[locale]/(console)/nodes/[id]/ConnectionTypePicker.tsx` | Create | Type picker: "Protocol" or "Web Service" |
| `web/console/app/[locale]/(console)/nodes/[id]/nodePanelRenderers.tsx` | Modify | Register `connect` panel renderer |
| `web/console/app/components/DeviceIdentityBar.tsx` | Modify | Add `onAddConnection` prop and button |
| `web/console/app/[locale]/(console)/nodes/[id]/page.tsx` | Modify | Remove old Services/Protocols section, wire up `onAddConnection` |
| `web/console/app/[locale]/(console)/nodes/[id]/DeviceCapabilityGrid.tsx` | Modify | Render connection badges on the "connect" card |

---

### Task 1: Add "connect" panel definition

**Files:**
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/devicePanelTypes.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/devicePanelConnectorPanels.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/nodeDetailPageModelBuilders.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/useNodeDetailPageModel.ts`

- [ ] **Step 1: Add connection counts to PanelContext**

In `devicePanelTypes.ts`, add two fields to the `PanelContext` type:

```typescript
// Add after desktopEligible: boolean;
protocolCount: number;
webServiceCount: number;
```

- [ ] **Step 2: Wire connection counts into useNodeDetailPageModel**

In `useNodeDetailPageModel.ts`:
1. Import `useProtocolConfigs` from `./useProtocolConfigs`
2. Import `useWebServices` from `../../../../hooks/useWebServices`
3. Call both hooks inside the model:
```typescript
const { protocols } = useProtocolConfigs(nodeId);
const { services: webServicesForNode } = useWebServices({ host: nodeId, detailLevel: "compact" });
```
4. Pass `protocolCount` and `webServiceCount` to `buildNodePanelContext()`:
```typescript
protocolCount: protocols.length,
webServiceCount: webServicesForNode.filter(s => s.host_asset_id === nodeId).length,
```

- [ ] **Step 3: Update buildNodePanelContext in nodeDetailPageModelBuilders.ts**

In `nodeDetailPageModelBuilders.ts`, find the `buildNodePanelContext()` function. It has an explicit parameter list and constructs the return object. Add `protocolCount: number` and `webServiceCount: number` as parameters, and include them in the returned `PanelContext` object.

- [ ] **Step 4: Add "connect" panel to devicePanelConnectorPanels.ts**

Import `Cable` from `lucide-react` at the top. Add the panel definition — push it as the **first** item (so it appears early in the grid):

```typescript
// Add at the start of buildConnectorPanels, before any conditional panels
panels.push({
  id: "connect",
  label: "Connect",
  icon: Cable,
  summary: (ctx) => {
    const total = ctx.protocolCount + ctx.webServiceCount;
    if (total === 0) return ["No connections"];
    const parts: string[] = [];
    if (ctx.protocolCount > 0) parts.push(`${ctx.protocolCount} protocol${ctx.protocolCount !== 1 ? "s" : ""}`);
    if (ctx.webServiceCount > 0) parts.push(`${ctx.webServiceCount} web service${ctx.webServiceCount !== 1 ? "s" : ""}`);
    return parts;
  },
});
```

- [ ] **Step 5: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 6: Commit**

```
feat(ui): add "connect" panel definition to capability grid
```

---

### Task 2: Create ConnectPanel, ConnectionTypePicker, and WebServiceForm

**Files:**
- Create: `web/console/app/[locale]/(console)/nodes/[id]/ConnectPanel.tsx`
- Create: `web/console/app/[locale]/(console)/nodes/[id]/ConnectionTypePicker.tsx`
- Create: `web/console/app/[locale]/(console)/nodes/[id]/WebServiceForm.tsx`

- [ ] **Step 1: Create ConnectPanel.tsx**

This component renders the full panel view with grouped Protocols and Web Services sections. It reuses the protocol data from `useProtocolConfigs` and web service data from `useWebServices`.

```typescript
"use client";

import { useState, useEffect } from "react";
import { useSearchParams } from "next/navigation";
import {
  Terminal,
  Monitor,
  Wifi,
  Network,
  Apple,
  Globe,
  Plus,
  Pencil,
  Trash2,
  FlaskConical,
  Key,
  ExternalLink,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { useRouter } from "../../../../../i18n/navigation";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { useProtocolConfigs, type ProtocolConfig, type ProtocolType } from "./useProtocolConfigs";
import { useWebServices, type WebService } from "../../../../hooks/useWebServices";
import { ProtocolForm } from "./ProtocolForm";
import { ConnectionTypePicker } from "./ConnectionTypePicker";
import { WebServiceForm } from "./WebServiceForm";

const PROTOCOL_ICONS: Record<ProtocolType, LucideIcon> = {
  ssh: Terminal,
  telnet: Wifi,
  vnc: Monitor,
  rdp: Network,
  ard: Apple,
};

const PROTOCOL_LABELS: Record<ProtocolType, string> = {
  ssh: "SSH",
  telnet: "Telnet",
  vnc: "VNC",
  rdp: "RDP",
  ard: "ARD",
};

const CONNECT_TARGETS: Record<ProtocolType, string> = {
  ssh: "terminal",
  telnet: "terminal",
  vnc: "desktop",
  rdp: "desktop",
  ard: "desktop",
};

function StatusDot({ status }: { status: string | null | undefined }) {
  if (status === "success" || status === "up") {
    return <span className="inline-block h-2 w-2 rounded-full bg-[var(--ok)]" title="OK" />;
  }
  if (status === "failed" || status === "down") {
    return <span className="inline-block h-2 w-2 rounded-full bg-[var(--bad)]" title="Failed" />;
  }
  return <span className="inline-block h-2 w-2 rounded-full bg-[var(--muted)]" title="Unknown" />;
}

function formatLastTested(value: string | null): string {
  if (!value) return "Never tested";
  try {
    const d = new Date(value);
    return `Tested ${d.toLocaleDateString()} ${d.toLocaleTimeString()}`;
  } catch {
    return "Never tested";
  }
}

type ConnectPanelProps = {
  nodeId: string;
};

export function ConnectPanel({ nodeId }: ConnectPanelProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const addingFromURL = searchParams.get("adding") === "true";

  const {
    protocols, loading: protocolsLoading, error: protocolsError,
    addProtocol, updateProtocol, deleteProtocol, testConnection, pushHubKey, refetch: refetchProtocols,
  } = useProtocolConfigs(nodeId);

  const {
    services: allWebServices, loading: wsLoading,
    createManualService, updateManualService, deleteManualService, refresh: refreshWS,
  } = useWebServices({ host: nodeId, detailLevel: "compact" });

  const webServices = allWebServices.filter(s => s.host_asset_id === nodeId);

  // Add flow state
  const [addingType, setAddingType] = useState<"picker" | "protocol" | "webservice" | null>(
    addingFromURL ? "picker" : null
  );
  const [editingProtocol, setEditingProtocol] = useState<ProtocolType | null>(null);
  const [editingWebService, setEditingWebService] = useState<WebService | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  // Clean ?adding from URL once the picker opens
  useEffect(() => {
    if (addingFromURL) {
      router.replace(`/nodes/${encodeURIComponent(nodeId)}?panel=connect`, { scroll: false });
    }
  }, [addingFromURL, nodeId, router]);

  const clearAddFlow = () => {
    setAddingType(null);
    setActionError(null);
  };

  const handleConnectProtocol = (protocol: ProtocolType) => {
    const target = CONNECT_TARGETS[protocol];
    router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=${target}`);
  };

  const handleConnectWebService = (url: string) => {
    window.open(url, "_blank", "noopener,noreferrer");
  };

  const handleProtocolAdd = async (data: Partial<ProtocolConfig>) => {
    const result = await addProtocol(data);
    if (result.ok) clearAddFlow();
    return result;
  };

  const handleProtocolUpdate = async (data: Partial<ProtocolConfig>) => {
    if (!editingProtocol) return { ok: false as const, error: "No protocol selected." };
    const result = await updateProtocol(editingProtocol, data);
    if (result.ok) setEditingProtocol(null);
    return result;
  };

  const handleProtocolDelete = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await deleteProtocol(protocol);
    if (!result.ok) setActionError(result.error ?? "Failed to remove protocol.");
  };

  const handleTestProtocol = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await testConnection(protocol);
    if (!result.success) setActionError(`${PROTOCOL_LABELS[protocol]} test failed: ${result.error ?? "unknown"}`);
    refetchProtocols();
  };

  const handlePushHubKey = async () => {
    setActionError(null);
    const result = await pushHubKey();
    if (!result.ok) setActionError(result.error ?? "Failed to push hub key.");
    return result;
  };

  const handleWebServiceCreate = async (input: { name: string; url: string; category: string }) => {
    try {
      await createManualService({ host_asset_id: nodeId, ...input });
      refreshWS();
      clearAddFlow();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Failed to create web service.");
    }
  };

  const handleWebServiceUpdate = async (id: string, patch: { name?: string; url?: string; category?: string }) => {
    try {
      await updateManualService(id, patch);
      refreshWS();
      setEditingWebService(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Failed to update web service.");
    }
  };

  const handleWebServiceDelete = async (id: string) => {
    try {
      await deleteManualService(id);
      refreshWS();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : "Failed to delete web service.");
    }
  };

  const isEditing = editingProtocol !== null || editingWebService !== null;
  const isAdding = addingType !== null;
  const loading = protocolsLoading || wsLoading;
  const hasProtocols = protocols.length > 0;
  const hasWebServices = webServices.length > 0;
  const isEmpty = !hasProtocols && !hasWebServices;

  return (
    <Card>
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-medium text-[var(--text)]">Connections</h2>
        {!isAdding && !isEditing && (
          <Button size="sm" onClick={() => { setAddingType("picker"); setActionError(null); }}>
            <Plus size={14} />
            Add Connection
          </Button>
        )}
      </div>

      {actionError && <p className="text-xs text-[var(--bad)] mb-3">{actionError}</p>}

      {/* Add flow: type picker */}
      {addingType === "picker" && (
        <ConnectionTypePicker
          onSelectProtocol={() => setAddingType("protocol")}
          onSelectWebService={() => setAddingType("webservice")}
          onCancel={clearAddFlow}
        />
      )}

      {/* Add flow: protocol form */}
      {addingType === "protocol" && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">New Protocol</p>
          <ProtocolForm
            assetId={nodeId}
            onSave={handleProtocolAdd}
            onTest={testConnection}
            onPushHubKey={handlePushHubKey}
            onCancel={clearAddFlow}
          />
        </div>
      )}

      {/* Add flow: web service form */}
      {addingType === "webservice" && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">New Web Service</p>
          <WebServiceForm onSave={handleWebServiceCreate} onCancel={clearAddFlow} />
        </div>
      )}

      {/* Edit protocol form */}
      {editingProtocol !== null && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">Edit {PROTOCOL_LABELS[editingProtocol]}</p>
          <ProtocolForm
            assetId={nodeId}
            initial={protocols.find(p => p.protocol === editingProtocol)}
            editMode
            onSave={handleProtocolUpdate}
            onTest={testConnection}
            onPushHubKey={editingProtocol === "ssh" ? handlePushHubKey : undefined}
            onCancel={() => { setEditingProtocol(null); setActionError(null); }}
          />
        </div>
      )}

      {/* Edit web service form */}
      {editingWebService !== null && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">Edit {editingWebService.name}</p>
          <WebServiceForm
            initial={editingWebService}
            onSave={(data) => handleWebServiceUpdate(editingWebService.id, data)}
            onCancel={() => { setEditingWebService(null); setActionError(null); }}
          />
        </div>
      )}

      {loading && <p className="text-sm text-[var(--muted)]">Loading connections...</p>}

      {!loading && isEmpty && !isAdding && !isEditing && (
        <div className="flex flex-col items-center justify-center py-8 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No connections configured</p>
          <p className="text-xs text-[var(--muted)]">Add protocols or web services to connect to this device.</p>
        </div>
      )}

      {/* Protocols section */}
      {!loading && hasProtocols && (
        <>
          <span className="text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wider">Protocols</span>
          <div className="divide-y divide-[var(--line)] mb-4">
            {protocols.map(p => {
              const Icon = PROTOCOL_ICONS[p.protocol] ?? Network;
              return (
                <div key={p.protocol} className="flex items-center gap-3 py-3">
                  <span className="flex items-center justify-center h-8 w-8 rounded-md bg-purple-500/10 shrink-0">
                    <Icon size={14} className="text-purple-400" />
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-medium text-[var(--text)]">{PROTOCOL_LABELS[p.protocol]}</span>
                      <StatusDot status={p.test_status} />
                    </div>
                    <p className="text-[11px] text-[var(--muted)] truncate">
                      {p.host ? `${p.host}:${p.port}` : `Port ${p.port}`} &middot; {formatLastTested(p.last_tested_at)}
                    </p>
                  </div>
                  <div className="flex items-center gap-1 shrink-0">
                    <button type="button" title="Connect" onClick={() => handleConnectProtocol(p.protocol)}
                      className="p-1.5 rounded-md text-sky-400 hover:bg-sky-500/10 transition-colors">
                      <ExternalLink size={13} />
                    </button>
                    <button type="button" title="Test" onClick={() => void handleTestProtocol(p.protocol)}
                      className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors">
                      <FlaskConical size={13} />
                    </button>
                    {p.protocol === "ssh" && (
                      <button type="button" title="Push hub key" onClick={() => void handlePushHubKey()}
                        className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors">
                        <Key size={13} />
                      </button>
                    )}
                    <button type="button" title="Edit" onClick={() => { setEditingProtocol(p.protocol); setActionError(null); }}
                      className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors">
                      <Pencil size={13} />
                    </button>
                    <button type="button" title="Remove" onClick={() => void handleProtocolDelete(p.protocol)}
                      className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors">
                      <Trash2 size={13} />
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </>
      )}

      {/* Web Services section */}
      {!loading && hasWebServices && (
        <>
          <span className="text-[10px] font-semibold text-[var(--muted)] uppercase tracking-wider">Web Services</span>
          <div className="divide-y divide-[var(--line)]">
            {webServices.map(ws => (
              <div key={ws.id} className="flex items-center gap-3 py-3">
                <span className="flex items-center justify-center h-8 w-8 rounded-md bg-sky-500/10 shrink-0">
                  <Globe size={14} className="text-sky-400" />
                </span>
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-1.5">
                    <span className="text-sm font-medium text-[var(--text)]">{ws.name}</span>
                    <StatusDot status={ws.status} />
                  </div>
                  <p className="text-[11px] text-[var(--muted)] truncate">{ws.url}</p>
                </div>
                <div className="flex items-center gap-1 shrink-0">
                  <button type="button" title="Open" onClick={() => handleConnectWebService(ws.url)}
                    className="p-1.5 rounded-md text-sky-400 hover:bg-sky-500/10 transition-colors">
                    <ExternalLink size={13} />
                  </button>
                  <button type="button" title="Edit" onClick={() => { setEditingWebService(ws); setActionError(null); }}
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors">
                    <Pencil size={13} />
                  </button>
                  <button type="button" title="Remove" onClick={() => void handleWebServiceDelete(ws.id)}
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors">
                    <Trash2 size={13} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
    </Card>
  );
}
```

- [ ] **Step 2: Create ConnectionTypePicker.tsx**

Simple two-option picker: Protocol or Web Service.

```typescript
"use client";

import { Terminal, Globe } from "lucide-react";
import { Button } from "../../../../components/ui/Button";

type ConnectionTypePickerProps = {
  onSelectProtocol: () => void;
  onSelectWebService: () => void;
  onCancel: () => void;
};

export function ConnectionTypePicker({ onSelectProtocol, onSelectWebService, onCancel }: ConnectionTypePickerProps) {
  return (
    <div className="mb-4 rounded-lg border border-[var(--line)] p-4">
      <p className="text-xs font-medium text-[var(--text)] mb-3">What type of connection?</p>
      <div className="grid grid-cols-2 gap-3 mb-3">
        <button
          type="button"
          onClick={onSelectProtocol}
          className="flex flex-col items-center gap-2 rounded-lg border border-[var(--line)] p-4
            hover:border-purple-400/40 hover:bg-purple-500/5 transition-colors text-center"
        >
          <span className="flex items-center justify-center h-9 w-9 rounded-md bg-purple-500/10">
            <Terminal size={16} className="text-purple-400" />
          </span>
          <span className="text-sm font-medium text-[var(--text)]">Protocol</span>
          <span className="text-[11px] text-[var(--muted)]">SSH, VNC, RDP, Telnet, ARD</span>
        </button>
        <button
          type="button"
          onClick={onSelectWebService}
          className="flex flex-col items-center gap-2 rounded-lg border border-[var(--line)] p-4
            hover:border-sky-400/40 hover:bg-sky-500/5 transition-colors text-center"
        >
          <span className="flex items-center justify-center h-9 w-9 rounded-md bg-sky-500/10">
            <Globe size={16} className="text-sky-400" />
          </span>
          <span className="text-sm font-medium text-[var(--text)]">Web Service</span>
          <span className="text-[11px] text-[var(--muted)]">HTTP/HTTPS URL</span>
        </button>
      </div>
      <div className="flex justify-end">
        <Button size="sm" variant="ghost" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create WebServiceForm.tsx**

Minimal form for adding/editing a web service: name, URL, category.

```typescript
"use client";

import { useState } from "react";
import { Button } from "../../../../components/ui/Button";
import type { WebService } from "../../../../hooks/useWebServices";

const CATEGORIES = [
  "dashboard", "monitoring", "storage", "media", "network",
  "automation", "development", "security", "database", "other",
];

type WebServiceFormProps = {
  initial?: WebService;
  onSave: (data: { name: string; url: string; category: string }) => void | Promise<void>;
  onCancel: () => void;
};

export function WebServiceForm({ initial, onSave, onCancel }: WebServiceFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [url, setUrl] = useState(initial?.url ?? "");
  const [category, setCategory] = useState(initial?.category ?? "other");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async () => {
    setError(null);
    if (!name.trim()) { setError("Name is required."); return; }
    if (!url.trim()) { setError("URL is required."); return; }
    try {
      new URL(url);
    } catch {
      setError("URL must be a valid URL (e.g., https://10.0.0.5:9443).");
      return;
    }
    setSaving(true);
    try {
      await onSave({ name: name.trim(), url: url.trim(), category });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to save.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-3">
      {error && <p className="text-xs text-[var(--bad)]">{error}</p>}
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">Name</label>
        <input
          type="text"
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="e.g., Portainer"
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)]"
        />
      </div>
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">URL</label>
        <input
          type="text"
          value={url}
          onChange={e => setUrl(e.target.value)}
          placeholder="e.g., https://10.0.0.5:9443"
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)] placeholder:text-[var(--muted)]"
        />
      </div>
      <div>
        <label className="text-xs text-[var(--muted)] block mb-1">Category</label>
        <select
          value={category}
          onChange={e => setCategory(e.target.value)}
          className="w-full rounded-md border border-[var(--line)] bg-[var(--panel)] px-3 py-1.5 text-sm text-[var(--text)]"
        >
          {CATEGORIES.map(c => (
            <option key={c} value={c}>{c.charAt(0).toUpperCase() + c.slice(1)}</option>
          ))}
        </select>
      </div>
      <div className="flex justify-end gap-2 pt-1">
        <Button size="sm" variant="ghost" onClick={onCancel} disabled={saving}>Cancel</Button>
        <Button size="sm" onClick={() => void handleSubmit()} disabled={saving}>
          {saving ? "Saving..." : initial ? "Update" : "Add"}
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 5: Commit**

```
feat(ui): create ConnectPanel, ConnectionTypePicker, and WebServiceForm
```

---

### Task 3: Register the Connect panel renderer

**Files:**
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/nodePanelRenderers.tsx`

- [ ] **Step 1: Add "connect" to the RegisteredPanelID union type**

In `nodePanelRenderers.tsx`, find the `RegisteredPanelID` type (a union of string literals). Add `| "connect"` to the union.

- [ ] **Step 2: Import ConnectPanel and register it**

Add import at the top of `nodePanelRenderers.tsx`:
```typescript
import { ConnectPanel } from "./ConnectPanel";
```

Add entry to the `PANEL_RENDERERS` object:
```typescript
connect: (context) => <ConnectPanel nodeId={context.nodeId} />,
```

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```
feat(ui): register connect panel in panel renderer system
```

---

### Task 4: Add "+ Add Connection" button to DeviceIdentityBar

**Files:**
- Modify: `web/console/app/components/DeviceIdentityBar.tsx`

- [ ] **Step 1: Add onAddConnection prop**

Add to the `DeviceIdentityBarProps` type:
```typescript
onAddConnection?: () => void;
```

- [ ] **Step 2: Render the button**

In the header row (Row 1), next to the edit and delete icon buttons, add the "+ Add Connection" button. Look for the area with `onEdit` and `onDelete` buttons and add before them:

```typescript
{onAddConnection && (
  <button
    type="button"
    onClick={onAddConnection}
    className="inline-flex items-center gap-1.5 text-xs px-2.5 py-1.5 rounded-md border border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)] hover:border-[var(--accent)]/40 transition-colors"
    style={{ transitionDuration: "var(--dur-fast)" }}
  >
    <Plus size={12} />
    Add Connection
  </button>
)}
```

Import `Plus` from `lucide-react` if not already imported.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 4: Commit**

```
feat(ui): add "Add Connection" button to DeviceIdentityBar
```

---

### Task 5: Wire up page.tsx — remove old section, connect new button

**Files:**
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/page.tsx`

- [ ] **Step 1: Remove the old Services/Protocols section**

Find and delete the `<div className="mt-3">` block (lines ~328–340) that contains:
- The "SERVICES" label
- The "Add Web Service" `<Link>`
- The `<ProtocolsPanel nodeId={nodeId} />`

Remove the entire `<div className="mt-3">...</div>` wrapper and its contents.

- [ ] **Step 2: Remove unused imports**

Remove the `ProtocolsPanel` import from the top of the file:
```typescript
// Remove this line:
import { ProtocolsPanel } from "./ProtocolsPanel";
```

Also remove the `Plus` import if it was only used for the "Add Web Service" button (check if it's still used elsewhere in the file — the `ArrowLeft` and `Plus` imports are at line 4).

- [ ] **Step 3: Wire up the onAddConnection handler**

Find the `<DeviceIdentityBar>` component usage. Add the `onAddConnection` prop:

```typescript
onAddConnection={() => router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=connect&adding=true`)}
```

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 5: Manual verification**

Start the dev server (`make dev-frontend-bg` or `cd web/console && npm run dev`) and navigate to a device detail page:
1. Verify the "Connect" card appears in the capability grid
2. Verify clicking it opens the Connect panel
3. Verify the "+ Add Connection" button appears in the header
4. Verify clicking "+ Add Connection" opens the Connect panel with the type picker
5. Verify the old "SERVICES" section and "Add Web Service" link are gone
6. Verify adding a protocol works (uses existing ProtocolForm)
7. Verify adding a web service works (new WebServiceForm)
8. Verify connect buttons route correctly (SSH → terminal, VNC → desktop, web service → new tab)

- [ ] **Step 6: Commit**

```
feat(ui): replace Services/Protocols section with unified Connect panel

Remove the split "SERVICES" label + "Add Web Service" link + standalone
ProtocolsPanel from the device detail dashboard. Replace with a "Connect"
card in the capability grid that opens a full panel view grouping both
protocols and web services. Add "+ Add Connection" button to the device
header for quick access.
```

---

### Task 6: Enhance the Connect card with connection badges

**Files:**
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/devicePanelTypes.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/nodeDetailPageModelBuilders.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/useNodeDetailPageModel.ts`
- Modify: `web/console/app/[locale]/(console)/nodes/[id]/DeviceCapabilityGrid.tsx`

- [ ] **Step 1: Add connection badge data to PanelContext**

In `devicePanelTypes.ts`, add a type for connection summaries and a field to PanelContext:

```typescript
export type ConnectionBadge = {
  label: string;
  status: "ok" | "bad" | "unknown";
};
```

Add to `PanelContext`:
```typescript
connectionBadges: ConnectionBadge[];
```

- [ ] **Step 2: Build connectionBadges in useNodeDetailPageModel**

In `useNodeDetailPageModel.ts`, build the badges array from protocol and web service data. Add after the `protocolCount` / `webServiceCount` wiring:

```typescript
const connectionBadges: ConnectionBadge[] = [
  ...protocols.map(p => ({
    label: `${PROTOCOL_LABELS_SHORT[p.protocol]} :${p.port}`,
    status: p.test_status === "success" ? "ok" as const : p.test_status === "failed" ? "bad" as const : "unknown" as const,
  })),
  ...webServicesForNode.filter(s => s.host_asset_id === nodeId).map(ws => ({
    label: ws.name,
    status: ws.status === "up" ? "ok" as const : ws.status === "down" ? "bad" as const : "unknown" as const,
  })),
];
```

You'll need a short labels map:
```typescript
const PROTOCOL_LABELS_SHORT: Record<string, string> = {
  ssh: "SSH", telnet: "Telnet", vnc: "VNC", rdp: "RDP", ard: "ARD",
};
```

Pass `connectionBadges` into `panelContext`.

- [ ] **Step 3: Update buildNodePanelContext for connectionBadges**

In `nodeDetailPageModelBuilders.ts`, update `buildNodePanelContext()` to accept `connectionBadges: ConnectionBadge[]` as a parameter and include it in the returned `PanelContext` object. Import `ConnectionBadge` from `./devicePanelTypes`.

- [ ] **Step 4: Render badges on the Connect card in DeviceCapabilityGrid**

In `DeviceCapabilityGrid.tsx`, add special rendering for the "connect" panel. After the summary line, if `panel.id === "connect"` and there are badges, render them:

```typescript
{/* Connection badges */}
{panel.id === "connect" && context.connectionBadges.length > 0 && (
  <div className="flex flex-wrap gap-1.5 mt-2 pl-[38px]">
    {context.connectionBadges.map((badge, i) => (
      <span key={i} className="inline-flex items-center gap-1 text-[10px] text-[var(--muted)] bg-[var(--hover)] px-2 py-0.5 rounded">
        <span className={`inline-block h-1.5 w-1.5 rounded-full ${
          badge.status === "ok" ? "bg-[var(--ok)]" : badge.status === "bad" ? "bg-[var(--bad)]" : "bg-[var(--muted)]"
        }`} />
        {badge.label}
      </span>
    ))}
  </div>
)}
```

Import `ConnectionBadge` type from `./devicePanelTypes` if needed.

- [ ] **Step 5: Verify TypeScript compiles**

Run: `cd web/console && npm run -s tsc -- --noEmit`
Expected: No errors

- [ ] **Step 6: Manual verification**

1. Navigate to a device with configured protocols — verify badges appear on the Connect card
2. Navigate to a device with no connections — verify "No connections" summary text, no badges
3. Verify badge status dots show correct colors

- [ ] **Step 7: Commit**

```
feat(ui): show connection badges on Connect card in capability grid
```
