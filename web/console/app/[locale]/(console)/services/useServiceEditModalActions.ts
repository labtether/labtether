"use client";

import { useTranslations } from "next-intl";
import { useCallback } from "react";
import type { WebServiceOverrideInput } from "../../../hooks/useWebServices";

interface UseServiceEditModalActionsArgs {
  editingService: { id: string; host_asset_id: string } | null;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  refresh: () => Promise<void>;
}

async function hubError(response: Response, fallback: string): Promise<Error> {
  const payload = await response.json().catch(() => null) as { error?: string } | null;
  return new Error(payload?.error?.trim() || fallback);
}

/** The edit dialog's saves: the override, a reset to what LabTether found, and other addresses for the service. */
export function useServiceEditModalActions({ editingService, saveServiceOverride, refresh }: UseServiceEditModalActionsArgs) {
  const t = useTranslations("services.edit");

  const handleSaveEditModal = useCallback(async (override: WebServiceOverrideInput) => {
    await saveServiceOverride(override);
    await refresh();
  }, [refresh, saveServiceOverride]);

  const handleResetEditModal = useCallback(async () => {
    if (!editingService) return;
    const params = new URLSearchParams({ host: editingService.host_asset_id, service_id: editingService.id });
    const response = await fetch(`/api/services/web/overrides?${params}`, { method: "DELETE" });
    if (!response.ok && response.status !== 204) throw await hubError(response, t("resetFailed"));
    await refresh();
  }, [editingService, refresh, t]);

  const handleAddAltURLEditModal = useCallback(async (webServiceID: string, url: string) => {
    const response = await fetch("/api/services/web/alt-urls", {
      method: "POST",
      cache: "no-store",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ web_service_id: webServiceID, url, source: "manual" }),
    });
    if (!response.ok) throw await hubError(response, t("altAddFailed"));
    await refresh();
  }, [refresh, t]);

  const handleRemoveAltURLEditModal = useCallback(async (altURLID: string) => {
    const response = await fetch(`/api/services/web/alt-urls/${encodeURIComponent(altURLID)}`, { method: "DELETE", cache: "no-store" });
    if (!response.ok) throw await hubError(response, t("altRemoveFailed"));
    await refresh();
  }, [refresh, t]);

  return { handleSaveEditModal, handleResetEditModal, handleAddAltURLEditModal, handleRemoveAltURLEditModal };
}
