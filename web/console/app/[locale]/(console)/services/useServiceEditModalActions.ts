"use client";

import { useCallback } from "react";
import type {
  WebServiceOverrideInput,
} from "../../../hooks/useWebServices";

interface UseServiceEditModalActionsArgs {
  editingService: { id: string; host_asset_id: string } | null;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  refresh: () => Promise<void>;
}

export function useServiceEditModalActions({
  editingService,
  saveServiceOverride,
  refresh,
}: UseServiceEditModalActionsArgs) {
  const handleSaveEditModal = useCallback(async (override: WebServiceOverrideInput) => {
    await saveServiceOverride(override);
    await refresh();
  }, [refresh, saveServiceOverride]);

  const handleResetEditModal = useCallback(async () => {
    if (!editingService) {
      throw new Error("Failed to reset override (missing editing service)");
    }
    const params = new URLSearchParams();
    params.set("host", editingService.host_asset_id);
    params.set("service_id", editingService.id);
    const response = await fetch(`/api/services/web/overrides?${params}`, {
      method: "DELETE",
    });
    if (!response.ok && response.status !== 204) {
      throw new Error(`Failed to reset override (HTTP ${response.status})`);
    }
    await refresh();
  }, [editingService, refresh]);

  const handleAddAltURLEditModal = useCallback(async (webServiceID: string, url: string) => {
    const response = await fetch("/api/services/web/alt-urls", {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        web_service_id: webServiceID,
        url,
        source: "manual",
      }),
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => null) as { error?: string } | null;
      throw new Error(payload?.error ?? `Failed to add alt URL (HTTP ${response.status})`);
    }
    await refresh();
  }, [refresh]);

  const handleRemoveAltURLEditModal = useCallback(async (altURLID: string) => {
    const response = await fetch(`/api/services/web/alt-urls/${encodeURIComponent(altURLID)}`, {
      method: "DELETE",
      cache: "no-store",
    });
    if (!response.ok) {
      const payload = await response.json().catch(() => null) as { error?: string } | null;
      throw new Error(payload?.error ?? `Failed to remove alt URL (HTTP ${response.status})`);
    }
    await refresh();
  }, [refresh]);

  return {
    handleSaveEditModal,
    handleResetEditModal,
    handleAddAltURLEditModal,
    handleRemoveAltURLEditModal,
  };
}
