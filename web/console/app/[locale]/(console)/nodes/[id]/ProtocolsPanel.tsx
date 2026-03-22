"use client";

import { useState } from "react";
import {
  Terminal,
  Monitor,
  Wifi,
  Network,
  Apple,
  Plus,
  Pencil,
  Trash2,
  FlaskConical,
  Key,
  ExternalLink,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Card } from "../../../../components/ui/Card";
import { Button } from "../../../../components/ui/Button";
import { useProtocolConfigs, type ProtocolConfig, type ProtocolType } from "./useProtocolConfigs";
import { ProtocolForm } from "./ProtocolForm";

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

function formatLastTested(value: string | null): string {
  if (!value) return "Never tested";
  try {
    const d = new Date(value);
    return `Tested ${d.toLocaleDateString()} ${d.toLocaleTimeString()}`;
  } catch {
    return "Never tested";
  }
}

function TestStatusDot({ status }: { status: ProtocolConfig["test_status"] }) {
  switch (status) {
    case "success":
      return <span className="inline-block h-2 w-2 rounded-full bg-[var(--ok)]" title="Test passed" />;
    case "failed":
      return <span className="inline-block h-2 w-2 rounded-full bg-[var(--bad)]" title="Test failed" />;
    default:
      return <span className="inline-block h-2 w-2 rounded-full bg-[var(--muted)]" title="Untested" />;
  }
}

type ProtocolsPanelProps = {
  nodeId: string;
};

export function ProtocolsPanel({ nodeId }: ProtocolsPanelProps) {
  const { protocols, loading, error, addProtocol, updateProtocol, deleteProtocol, testConnection, pushHubKey, refetch } =
    useProtocolConfigs(nodeId);

  const [showAddForm, setShowAddForm] = useState(false);
  const [editingProtocol, setEditingProtocol] = useState<ProtocolType | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [deletingProtocol, setDeletingProtocol] = useState<ProtocolType | null>(null);

  const editingConfig = editingProtocol !== null ? protocols.find((p) => p.protocol === editingProtocol) : undefined;

  const handleAdd = async (data: Partial<ProtocolConfig>) => {
    const result = await addProtocol(data);
    if (result.ok) {
      setShowAddForm(false);
    }
    return result;
  };

  const handleUpdate = async (data: Partial<ProtocolConfig>) => {
    if (!editingProtocol) return { ok: false as const, error: "No protocol selected." };
    const result = await updateProtocol(editingProtocol, data);
    if (result.ok) {
      setEditingProtocol(null);
    }
    return result;
  };

  const handleDelete = async (protocol: ProtocolType) => {
    setActionError(null);
    setDeletingProtocol(protocol);
    try {
      const result = await deleteProtocol(protocol);
      if (!result.ok) {
        setActionError(result.error ?? "Failed to remove protocol.");
      }
    } finally {
      setDeletingProtocol(null);
    }
  };

  const handleTestInList = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await testConnection(protocol);
    if (!result.success) {
      setActionError(`${PROTOCOL_LABELS[protocol]} test failed: ${result.error ?? "unknown error"}`);
    }
    refetch();
  };

  const handlePushHubKey = async () => {
    setActionError(null);
    const result = await pushHubKey();
    if (!result.ok) {
      setActionError(result.error ?? "Failed to push hub key.");
    }
    return result;
  };

  return (
    <Card className="mb-4">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-medium text-[var(--text)]">Protocols</h2>
        {!showAddForm && editingProtocol === null && (
          <Button
            size="sm"
            onClick={() => { setShowAddForm(true); setActionError(null); }}
          >
            <Plus size={14} />
            Add Protocol
          </Button>
        )}
      </div>

      {actionError && (
        <p className="text-xs text-[var(--bad)] mb-3">{actionError}</p>
      )}

      {/* Add form */}
      {showAddForm && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">New Protocol</p>
          <ProtocolForm
            assetId={nodeId}
            onSave={handleAdd}
            onTest={testConnection}
            onPushHubKey={handlePushHubKey}
            onCancel={() => { setShowAddForm(false); setActionError(null); }}
          />
        </div>
      )}

      {/* Edit form */}
      {editingProtocol !== null && editingConfig !== undefined && (
        <div className="mb-4 rounded-lg border border-[var(--line)] p-3">
          <p className="text-xs font-medium text-[var(--text)] mb-3">
            Edit {PROTOCOL_LABELS[editingProtocol]}
          </p>
          <ProtocolForm
            assetId={nodeId}
            initial={editingConfig}
            editMode
            onSave={handleUpdate}
            onTest={testConnection}
            onPushHubKey={editingProtocol === "ssh" ? handlePushHubKey : undefined}
            onCancel={() => { setEditingProtocol(null); setActionError(null); }}
          />
        </div>
      )}

      {/* Protocol list */}
      {loading && (
        <p className="text-sm text-[var(--muted)]">Loading protocols...</p>
      )}

      {error && !loading && (
        <p className="text-xs text-[var(--bad)]">{error}</p>
      )}

      {!loading && !error && protocols.length === 0 && !showAddForm && editingProtocol === null && (
        <div className="flex flex-col items-center justify-center py-8 gap-2">
          <p className="text-sm font-medium text-[var(--text)]">No protocols configured</p>
          <p className="text-xs text-[var(--muted)] text-center max-w-xs">
            Add SSH, Telnet, VNC, RDP, or ARD to connect to this device.
          </p>
        </div>
      )}

      {!loading && protocols.length > 0 && (
        <div className="divide-y divide-[var(--line)]">
          {protocols.map((p) => {
            const Icon = PROTOCOL_ICONS[p.protocol] ?? Network;
            const isBeingDeleted = deletingProtocol === p.protocol;
            return (
              <div key={p.protocol} className="flex items-center gap-3 py-3">
                {/* Icon + info */}
                <div className="flex items-center gap-2.5 flex-1 min-w-0">
                  <span className="flex items-center justify-center h-7 w-7 rounded-md bg-[var(--accent-subtle)] shrink-0">
                    <Icon size={14} className="text-[var(--accent-text)]" />
                  </span>
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5">
                      <span className="text-sm font-medium text-[var(--text)]">
                        {PROTOCOL_LABELS[p.protocol]}
                      </span>
                      <TestStatusDot status={p.test_status} />
                    </div>
                    <p className="text-[11px] text-[var(--muted)] truncate">
                      {p.host ? `${p.host}:${p.port}` : `Port ${p.port}`} &middot; {formatLastTested(p.last_tested_at)}
                    </p>
                  </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1.5 shrink-0">
                  <button
                    type="button"
                    title="Connect"
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
                    style={{ transitionDuration: "var(--dur-fast)" }}
                  >
                    <ExternalLink size={13} />
                  </button>
                  <button
                    type="button"
                    title="Test connection"
                    onClick={() => void handleTestInList(p.protocol)}
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
                    style={{ transitionDuration: "var(--dur-fast)" }}
                  >
                    <FlaskConical size={13} />
                  </button>
                  {p.protocol === "ssh" && (
                    <button
                      type="button"
                      title="Push hub SSH key"
                      onClick={() => void handlePushHubKey()}
                      className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
                      style={{ transitionDuration: "var(--dur-fast)" }}
                    >
                      <Key size={13} />
                    </button>
                  )}
                  <button
                    type="button"
                    title="Edit"
                    onClick={() => {
                      setEditingProtocol(p.protocol);
                      setShowAddForm(false);
                      setActionError(null);
                    }}
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--text)] hover:bg-[var(--hover)] transition-colors"
                    style={{ transitionDuration: "var(--dur-fast)" }}
                  >
                    <Pencil size={13} />
                  </button>
                  <button
                    type="button"
                    title="Remove"
                    disabled={isBeingDeleted}
                    onClick={() => void handleDelete(p.protocol)}
                    className="p-1.5 rounded-md text-[var(--muted)] hover:text-[var(--bad)] hover:bg-[var(--bad-glow)] transition-colors disabled:opacity-40 disabled:pointer-events-none"
                    style={{ transitionDuration: "var(--dur-fast)" }}
                  >
                    <Trash2 size={13} />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
