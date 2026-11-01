"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { Button } from "../../../../components/ui/Button";
import { useProtocolConfigs, type ProtocolConfig, type ProtocolType } from "./useProtocolConfigs";
import { ProtocolForm } from "./ProtocolForm";
import { CONNECTION_COLS, PROTOCOL_LABELS, ProtocolRow } from "./settingsViewConnections";

type ProtocolsPanelProps = {
  nodeId: string;
};

/** The device's remote protocols alone (SSH, Telnet, VNC, RDP, ARD): add, edit, test, push the hub key, remove. */
export function ProtocolsPanel({ nodeId }: ProtocolsPanelProps) {
  const t = useTranslations("devices.settingsTab.connections");
  const { protocols, loading, error, addProtocol, updateProtocol, deleteProtocol, testConnection, pushHubKey, refetch } =
    useProtocolConfigs(nodeId);

  const [showAddForm, setShowAddForm] = useState(false);
  const [editingProtocol, setEditingProtocol] = useState<ProtocolType | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [deletingProtocol, setDeletingProtocol] = useState<ProtocolType | null>(null);

  const editingConfig = editingProtocol !== null ? protocols.find((p) => p.protocol === editingProtocol) : undefined;

  const handleAdd = async (data: Partial<ProtocolConfig>) => {
    const result = await addProtocol(data);
    if (result.ok) setShowAddForm(false);
    return result;
  };

  const handleUpdate = async (data: Partial<ProtocolConfig>) => {
    if (!editingProtocol) return { ok: false as const, error: t("noneSelected") };
    const result = await updateProtocol(editingProtocol, data);
    if (result.ok) setEditingProtocol(null);
    return result;
  };

  const handleDelete = async (protocol: ProtocolType) => {
    setActionError(null);
    setDeletingProtocol(protocol);
    try {
      const result = await deleteProtocol(protocol);
      if (!result.ok) setActionError(result.error ?? t("removeFailed"));
    } finally {
      setDeletingProtocol(null);
    }
  };

  const handleTestInList = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await testConnection(protocol);
    if (!result.success) {
      setActionError(t("testFailedFor", { name: PROTOCOL_LABELS[protocol], error: result.error ?? t("noReason") }));
    }
    refetch();
  };

  const handlePushHubKey = async () => {
    setActionError(null);
    const result = await pushHubKey();
    if (!result.ok) setActionError(result.error ?? t("pushFailed"));
    return result;
  };

  const idle = !showAddForm && editingProtocol === null;

  return (
    <section className="dv-sec" aria-labelledby="dst-proto-t">
      <div className="dv-sh">
        <h2 id="dst-proto-t">{t("protocols")}</h2>
        {idle && (
          <Button size="sm" onClick={() => { setShowAddForm(true); setActionError(null); }}>
            <Plus size={14} aria-hidden="true" />
            {t("addProtocol")}
          </Button>
        )}
      </div>

      {actionError && <p className="dv-err dst-msg-line" role="alert">{actionError}</p>}

      {showAddForm && (
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("newProtocol")}</h3>
          <ProtocolForm
            assetId={nodeId}
            onSave={handleAdd}
            onTest={testConnection}
            onPushHubKey={handlePushHubKey}
            onCancel={() => { setShowAddForm(false); setActionError(null); }}
          />
        </div>
      )}

      {editingProtocol !== null && editingConfig !== undefined && (
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("editTitle", { name: PROTOCOL_LABELS[editingProtocol] })}</h3>
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

      {loading ? (
        <div className="dv-list"><p className="dst-quiet">{t("loading")}</p></div>
      ) : error ? (
        <div className="dv-list"><div className="dv-empty"><b>{t("loadFailed")}</b><span>{error}</span></div></div>
      ) : protocols.length === 0 ? (
        idle ? (
          <div className="dv-list">
            <div className="dv-empty"><b>{t("emptyProtocols")}</b><span>{t("emptyProtocolsHint")}</span></div>
          </div>
        ) : null
      ) : (
        <div className="dv-list" style={CONNECTION_COLS}>
          {protocols.map((p) => (
            <ProtocolRow
              key={p.protocol}
              config={p}
              onTest={() => void handleTestInList(p.protocol)}
              onPushKey={p.protocol === "ssh" ? () => void handlePushHubKey() : undefined}
              onEdit={() => { setEditingProtocol(p.protocol); setShowAddForm(false); setActionError(null); }}
              onRemove={() => void handleDelete(p.protocol)}
              removing={deletingProtocol === p.protocol}
            />
          ))}
        </div>
      )}
    </section>
  );
}
