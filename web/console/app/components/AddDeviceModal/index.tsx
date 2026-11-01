"use client";

import { useState, useCallback, useEffect } from "react";
import { X, ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
import { Modal } from "../ui/Modal";
import { SourcePicker, type SourceType } from "./SourcePicker";
import { AgentSetupStep } from "./AgentSetupStep";
import { ProxmoxSetupStep } from "./ProxmoxSetupStep";
import { PBSSetupStep } from "./PBSSetupStep";
import { DockerSetupStep } from "./DockerSetupStep";
import { PortainerSetupStep } from "./PortainerSetupStep";
import { TrueNASSetupStep } from "./TrueNASSetupStep";
import { HomeAssistantSetupStep } from "./HomeAssistantSetupStep";
import { ManualDeviceSetupStep } from "./ManualDeviceSetupStep";
import { ensureArray, ensureRecord } from "../../lib/responseGuards";
import type { AddDeviceAddedEvent, AddDeviceCompatPrefill } from "./types";

type AddDeviceModalProps = {
  open: boolean;
  onClose: () => void;
  onAdded?: (event: AddDeviceAddedEvent) => void;
};

// Step titles (English, as the setup steps' own text is).
const sourceLabels: Record<SourceType, string> = {
  agent: "Install Agent",
  proxmox: "Connect Proxmox",
  pbs: "Connect PBS",
  docker: "Connect Docker",
  portainer: "Connect Portainer",
  truenas: "Connect TrueNAS",
  homeassistant: "Connect Home Assistant",
  manual: "Add a manual device",
};

/**
 * Add a device: pick how it connects (the agent, an API, or by hand), then that system's setup step.
 * The header names the step; Back in each step returns to the list.
 */
export function AddDeviceModal({ open, onClose, onAdded }: AddDeviceModalProps) {
  const t = useTranslations("auth.addDevice");
  const [selectedSource, setSelectedSource] = useState<SourceType | null>(null);
  const [prefillsBySource, setPrefillsBySource] = useState<Partial<Record<SourceType, AddDeviceCompatPrefill[]>>>({});

  useEffect(() => {
    if (!open) return;

    const controller = new AbortController();
    const loadCompatPrefills = async () => {
      try {
        const res = await fetch("/api/services/web/compat?include_hidden=true", {
          cache: "no-store",
          signal: controller.signal,
        });
        if (!res.ok) {
          return;
        }
        const payload = ensureRecord(await res.json().catch(() => null));
        const compatItems = ensureArray<CompatibleAPI>(payload?.compatible);
        const grouped: Partial<Record<SourceType, AddDeviceCompatPrefill[]>> = {};
        for (const item of compatItems) {
          const source = sourceForCompatConnector(item.connector_id);
          if (!source) continue;
          const baseURL = originFromURL(item.service_url);
          if (!baseURL) continue;

          const suggestion: AddDeviceCompatPrefill = {
            source,
            connectorID: item.connector_id,
            baseURL,
            serviceURL: item.service_url,
            serviceName: item.service_name,
            confidence: item.confidence ?? 0,
            hostAssetID: item.host_asset_id,
            authHint: item.auth_hint,
          };
          const bucket = grouped[source] ?? [];
          bucket.push(suggestion);
          grouped[source] = bucket;
        }

        const next: Partial<Record<SourceType, AddDeviceCompatPrefill[]>> = {};
        for (const sourceKey of Object.keys(grouped) as SourceType[]) {
          const bucket = grouped[sourceKey] ?? [];
          if (bucket.length === 0) continue;

          const dedupedByBaseURL = new Map<string, AddDeviceCompatPrefill>();
          for (const suggestion of bucket) {
            const key = suggestion.baseURL.toLowerCase();
            const existing = dedupedByBaseURL.get(key);
            if (!existing || suggestion.confidence > existing.confidence) {
              dedupedByBaseURL.set(key, suggestion);
            }
          }

          const sorted = Array.from(dedupedByBaseURL.values()).sort((a, b) => {
            if (a.confidence !== b.confidence) {
              return b.confidence - a.confidence;
            }
            if (a.serviceName !== b.serviceName) {
              return a.serviceName.localeCompare(b.serviceName);
            }
            return a.baseURL.localeCompare(b.baseURL);
          });
          if (sorted.length > 0) {
            next[sourceKey] = sorted;
          }
        }
        setPrefillsBySource(next);
      } catch {
        // Ignore compat prefill failures and keep setup flow functional.
      }
    };

    void loadCompatPrefills();
    return () => controller.abort();
  }, [open]);

  const handleClose = useCallback(() => {
    setSelectedSource(null);
    onClose();
  }, [onClose]);

  const handleBack = useCallback(() => {
    setSelectedSource(null);
  }, []);

  return (
    <Modal
      open={open}
      onClose={handleClose}
      ariaTitle={selectedSource ? `${t("title")}: ${sourceLabels[selectedSource]}` : t("title")}
      description={t("description")}
      className="si-ad"
    >
      <div className="si-ad-h">
        <div className="si-ad-crumb">
          {selectedSource ? (
            <>
              <button type="button" className="si-ad-up" onClick={handleBack}>{t("title")}</button>
              <ChevronRight aria-hidden="true" strokeWidth={2} />
              <span>{sourceLabels[selectedSource]}</span>
            </>
          ) : (
            <span>{t("title")}</span>
          )}
        </div>
        <button type="button" onClick={handleClose} aria-label={t("close")} className="si-ad-x">
          <X aria-hidden="true" />
        </button>
      </div>

      {/* The body scrolls inside the dialog so it fits the visible viewport (100dvh tracks mobile toolbars). */}
      <div className="si-ad-b">
        {selectedSource === null && (
          <SourcePicker
            onSelect={setSelectedSource}
            prefillBySource={primaryPrefillBySource(prefillsBySource)}
          />
        )}
        {selectedSource === "agent" && (
          <AgentSetupStep onBack={handleBack} onClose={handleClose} onAdded={onAdded} />
        )}
        {selectedSource === "proxmox" && (
          <ProxmoxSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.proxmox}
            setupMode="advanced"
          />
        )}
        {selectedSource === "pbs" && (
          <PBSSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.pbs}
            setupMode="advanced"
          />
        )}
        {selectedSource === "portainer" && (
          <PortainerSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.portainer}
            setupMode="advanced"
          />
        )}
        {selectedSource === "docker" && (
          <DockerSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.docker}
            setupMode="advanced"
          />
        )}
        {selectedSource === "truenas" && (
          <TrueNASSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.truenas}
            setupMode="advanced"
          />
        )}
        {selectedSource === "homeassistant" && (
          <HomeAssistantSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
            compatPrefills={prefillsBySource.homeassistant}
            setupMode="advanced"
          />
        )}
        {selectedSource === "manual" && (
          <ManualDeviceSetupStep
            onBack={handleBack}
            onClose={handleClose}
            onAdded={onAdded}
          />
        )}
      </div>
    </Modal>
  );
}

type CompatibleAPI = {
  host_asset_id: string;
  service_id: string;
  service_name: string;
  service_url: string;
  connector_id: string;
  confidence?: number;
  auth_hint?: string;
};

function sourceForCompatConnector(connectorID: string): SourceType | null {
  const normalized = connectorID.trim().toLowerCase();
  switch (normalized) {
    case "homeassistant":
    case "home-assistant":
      return "homeassistant";
    case "proxmox":
      return "proxmox";
    case "pbs":
      return "pbs";
    case "truenas":
      return "truenas";
    case "portainer":
      return "portainer";
    case "docker":
      return "docker";
    default:
      return null;
  }
}

function originFromURL(rawURL: string): string {
  const trimmed = rawURL.trim();
  if (!trimmed) return "";
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return "";
    }
    return parsed.origin;
  } catch {
    return "";
  }
}

function primaryPrefillBySource(
  prefillsBySource: Partial<Record<SourceType, AddDeviceCompatPrefill[]>>,
): Partial<Record<SourceType, AddDeviceCompatPrefill>> {
  const out: Partial<Record<SourceType, AddDeviceCompatPrefill>> = {};
  for (const sourceKey of Object.keys(prefillsBySource) as SourceType[]) {
    const first = prefillsBySource[sourceKey]?.[0];
    if (first) {
      out[sourceKey] = first;
    }
  }
  return out;
}
