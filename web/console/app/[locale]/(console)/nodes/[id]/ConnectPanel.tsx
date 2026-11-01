"use client";

import { useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { Plus } from "lucide-react";
import { useTranslations } from "next-intl";
import { useRouter } from "../../../../../i18n/navigation";
import { Button } from "../../../../components/ui/Button";
import { useProtocolConfigs, type ProtocolConfig, type ProtocolType } from "./useProtocolConfigs";
import { useWebServices, type WebService } from "../../../../hooks/useWebServices";
import { ProtocolForm } from "./ProtocolForm";
import { ConnectionTypePicker } from "./ConnectionTypePicker";
import { WebServiceForm } from "./WebServiceForm";
import { CONNECTION_COLS, PROTOCOL_LABELS, ProtocolRow, WebServiceRow } from "./settingsViewConnections";
import { safeHttpURL } from "../../../../../lib/safeExternalURL";

const CONNECT_TARGETS: Record<ProtocolType, string> = {
  ssh: "terminal",
  telnet: "terminal",
  vnc: "desktop",
  rdp: "desktop",
  ard: "desktop",
};

type ConnectPanelProps = {
  nodeId: string;
};

/** Settings › Connections: how the hub reaches this device besides the agent (SSH, Telnet, VNC, RDP, ARD, web). */
export function ConnectPanel({ nodeId }: ConnectPanelProps) {
  const t = useTranslations("devices.settingsTab.connections");
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
    router.push(`/nodes/${encodeURIComponent(nodeId)}?panel=${CONNECT_TARGETS[protocol]}`);
  };

  const handleConnectWebService = (url: string) => {
    const href = safeHttpURL(url);
    if (href) window.open(href, "_blank", "noopener,noreferrer");
  };

  const handleProtocolAdd = async (data: Partial<ProtocolConfig>) => {
    const result = await addProtocol(data);
    if (result.ok) clearAddFlow();
    return result;
  };

  const handleProtocolUpdate = async (data: Partial<ProtocolConfig>) => {
    if (!editingProtocol) return { ok: false as const, error: t("noneSelected") };
    const result = await updateProtocol(editingProtocol, data);
    if (result.ok) setEditingProtocol(null);
    return result;
  };

  const handleProtocolDelete = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await deleteProtocol(protocol);
    if (!result.ok) setActionError(result.error ?? t("removeFailed"));
  };

  const handleTestProtocol = async (protocol: ProtocolType) => {
    setActionError(null);
    const result = await testConnection(protocol);
    if (!result.success) {
      setActionError(t("testFailedFor", { name: PROTOCOL_LABELS[protocol], error: result.error ?? t("noReason") }));
    }
    refetchProtocols();
  };

  const handlePushHubKey = async () => {
    setActionError(null);
    const result = await pushHubKey();
    if (!result.ok) setActionError(result.error ?? t("pushFailed"));
    return result;
  };

  const handleWebServiceCreate = async (input: { name: string; url: string; category: string }) => {
    try {
      await createManualService({ host_asset_id: nodeId, ...input });
      refreshWS();
      clearAddFlow();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : t("webAddFailed"));
    }
  };

  const handleWebServiceUpdate = async (id: string, patch: { name?: string; url?: string; category?: string }) => {
    try {
      await updateManualService(id, patch);
      refreshWS();
      setEditingWebService(null);
    } catch (e) {
      setActionError(e instanceof Error ? e.message : t("webSaveFailed"));
    }
  };

  const handleWebServiceDelete = async (id: string) => {
    try {
      await deleteManualService(id);
      refreshWS();
    } catch (e) {
      setActionError(e instanceof Error ? e.message : t("webRemoveFailed"));
    }
  };

  const isEditing = editingProtocol !== null || editingWebService !== null;
  const isAdding = addingType !== null;
  const loading = protocolsLoading || wsLoading;
  const hasProtocols = protocols.length > 0;
  const hasWebServices = webServices.length > 0;
  const isEmpty = !hasProtocols && !hasWebServices;

  return (
    <section className="dv-sec" aria-labelledby="dst-conn-t">
      <div className="dv-sh">
        <h2 id="dst-conn-t">{t("title")}</h2>
        {!isAdding && !isEditing && (
          <Button size="sm" onClick={() => { setAddingType("picker"); setActionError(null); }}>
            <Plus size={14} aria-hidden="true" />
            {t("add")}
          </Button>
        )}
      </div>

      {actionError && <p className="dv-err dst-msg-line" role="alert">{actionError}</p>}
      {!actionError && protocolsError && !protocolsLoading ? <p className="dv-err dst-msg-line" role="alert">{protocolsError}</p> : null}

      {addingType === "picker" && (
        <ConnectionTypePicker
          onSelectProtocol={() => setAddingType("protocol")}
          onSelectWebService={() => setAddingType("webservice")}
          onCancel={clearAddFlow}
        />
      )}

      {addingType === "protocol" && (
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("newProtocol")}</h3>
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
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("newWeb")}</h3>
          <WebServiceForm onSave={handleWebServiceCreate} onCancel={clearAddFlow} />
        </div>
      )}

      {editingProtocol !== null && (
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("editTitle", { name: PROTOCOL_LABELS[editingProtocol] })}</h3>
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
        <div className="dv-sheet dst-form-sheet">
          <h3 className="dst-form-t">{t("editTitle", { name: editingWebService.name })}</h3>
          <WebServiceForm
            initial={editingWebService}
            onSave={(data) => handleWebServiceUpdate(editingWebService.id, data)}
            onCancel={() => { setEditingWebService(null); setActionError(null); }}
          />
        </div>
      )}

      {loading && isEmpty ? (
        <div className="dv-list"><p className="dst-quiet">{t("loading")}</p></div>
      ) : isEmpty ? (
        !isAdding && !isEditing ? (
          <div className="dv-list">
            <div className="dv-empty">
              <b>{t("empty")}</b>
              <span>{t("emptyHint")}</span>
            </div>
          </div>
        ) : null
      ) : (
        <div className="dv-list dst-conns" style={CONNECTION_COLS}>
          {hasProtocols ? <p className="dst-grp">{t("protocols")}</p> : null}
          {protocols.map(p => (
            <ProtocolRow
              key={p.protocol}
              config={p}
              onConnect={() => handleConnectProtocol(p.protocol)}
              onTest={() => void handleTestProtocol(p.protocol)}
              onPushKey={p.protocol === "ssh" ? () => void handlePushHubKey() : undefined}
              onEdit={() => { setEditingProtocol(p.protocol); setAddingType(null); setActionError(null); }}
              onRemove={() => void handleProtocolDelete(p.protocol)}
            />
          ))}
          {hasWebServices ? <p className="dst-grp">{t("webServices")}</p> : null}
          {webServices.map(ws => (
            <WebServiceRow
              key={ws.id}
              service={ws}
              onOpen={() => handleConnectWebService(ws.url)}
              onEdit={() => { setEditingWebService(ws); setAddingType(null); setActionError(null); }}
              onRemove={() => void handleWebServiceDelete(ws.id)}
            />
          ))}
        </div>
      )}
    </section>
  );
}
