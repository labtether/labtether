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

  const [addingType, setAddingType] = useState<"picker" | "protocol" | "webservice" | null>(
    addingFromURL ? "picker" : null
  );
  const [editingProtocol, setEditingProtocol] = useState<ProtocolType | null>(null);
  const [editingWebService, setEditingWebService] = useState<WebService | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

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

      {addingType === "picker" && (
        <ConnectionTypePicker
          onSelectProtocol={() => setAddingType("protocol")}
          onSelectWebService={() => setAddingType("webservice")}
          onCancel={clearAddFlow}
        />
      )}

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

      {addingType === "webservice" && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">New Web Service</p>
          <WebServiceForm onSave={handleWebServiceCreate} onCancel={clearAddFlow} />
        </div>
      )}

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
