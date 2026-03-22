"use client";

import { useState, useCallback, useEffect } from "react";
import { X, ChevronRight } from "lucide-react";
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

const sourceLabels: Record<SourceType, string> = {
  agent: "Install Agent",
  proxmox: "Connect Proxmox",
  pbs: "Connect PBS",
  docker: "Connect Docker",
  portainer: "Connect Portainer",
  truenas: "Connect TrueNAS",
  homeassistant: "Home Assistant Setup",
  manual: "Add Manual Device",
};

export function AddDeviceModal({ open, onClose, onAdded }: AddDeviceModalProps) {
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
    <Modal open={open} onClose={handleClose}>
      {/* Header */}
      <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--line)]">
        <div className="flex items-center gap-2 text-sm font-medium text-[var(--text)]">
          <span>Add Device</span>
          {selectedSource && (
            <>
              <ChevronRight size={14} className="text-[var(--muted)]" />
              <span>{sourceLabels[selectedSource]}</span>
            </>
          )}
        </div>
        <button onClick={handleClose} className="p-1 rounded hover:bg-[var(--hover)] transition-colors duration-150">
          <X size={16} className="text-[var(--muted)]" />
        </button>
      </div>

      {/* Body: max-height constrains scroll so the modal fits within the
          viewport on mobile/tablet/desktop. Uses 100dvh (dynamic viewport
          height) so the calculation tracks the visible area even when the
          browser toolbar is visible on mobile. The ~9rem overhead accounts
          for the dialog's top offset (1rem), bottom clearance (1rem),
          and the header row (~3.5rem). */}
      <div className="max-h-[calc(100dvh-9rem)] overflow-y-auto px-5 py-4">
        {selectedSource === null && (
          <>
            <p className="text-xs text-[var(--muted)] mb-4">How will this device connect to LabTether?</p>
            <SourcePicker
              onSelect={setSelectedSource}
              prefillBySource={primaryPrefillBySource(prefillsBySource)}
            />
          </>
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
