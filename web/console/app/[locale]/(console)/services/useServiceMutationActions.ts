"use client";

import { useCallback } from "react";
import type {
  ManualWebServiceInput,
  WebService,
  WebServiceOverride,
  WebServiceOverrideInput,
} from "../../../hooks/useWebServices";

interface UseServiceMutationActionsArgs {
  hostFilter: string;
  hosts: Map<string, string>;
  refresh: () => Promise<void>;
  createManualService: (input: ManualWebServiceInput) => Promise<void>;
  updateManualService: (id: string, patch: Partial<ManualWebServiceInput>) => Promise<void>;
  deleteManualService: (id: string) => Promise<void>;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  listServiceOverrides: (hostAssetID?: string) => Promise<WebServiceOverride[]>;
}

export function useServiceMutationActions({
  hostFilter,
  hosts,
  refresh,
  createManualService,
  updateManualService,
  deleteManualService,
  saveServiceOverride,
  listServiceOverrides,
}: UseServiceMutationActionsArgs) {
  const findExistingOverride = useCallback(async (service: WebService): Promise<WebServiceOverride | null> => {
    const overrides = await listServiceOverrides(service.host_asset_id);
    return (
      overrides.find(
        (override) =>
          override.host_asset_id === service.host_asset_id && override.service_id === service.id
      ) ?? null
    );
  }, [listServiceOverrides]);

  const handleAddManualService = useCallback(async () => {
    const defaultHost = hostFilter !== "all" ? hostFilter : Array.from(hosts.keys())[0] ?? "";
    const hostAssetID = (window.prompt("Host asset ID", defaultHost) ?? "").trim();
    if (!hostAssetID) return;

    const name = (window.prompt("Service name", "Manual Service") ?? "").trim();
    if (!name) return;

    const url = (window.prompt("Service URL (http/https)", "http://localhost:8080") ?? "").trim();
    if (!url) return;

    const category = (window.prompt("Category", "Other") ?? "").trim() || "Other";

    try {
      await createManualService({
        host_asset_id: hostAssetID,
        name,
        url,
        category,
      });
      await refresh();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "Failed to create manual service");
    }
  }, [createManualService, hostFilter, hosts, refresh]);

  const handleRename = useCallback(async (service: WebService) => {
    const name = (window.prompt("Display name", service.name) ?? "").trim();
    if (!name) return;
    const category = (window.prompt("Category", service.category || "Other") ?? "").trim() || "Other";

    try {
      if (service.source === "manual") {
        await updateManualService(service.id, {
          name,
          category,
        });
      } else {
        const existingOverride = await findExistingOverride(service);
        await saveServiceOverride({
          host_asset_id: service.host_asset_id,
          service_id: service.id,
          name_override: name,
          category_override: category,
          hidden: service.metadata?.hidden === "true",
          url_override: existingOverride?.url_override,
          icon_key_override: existingOverride?.icon_key_override,
        });
      }
      await refresh();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "Failed to update service");
    }
  }, [findExistingOverride, refresh, saveServiceOverride, updateManualService]);

  const handleToggleHidden = useCallback(async (service: WebService) => {
    try {
      const existingOverride = await findExistingOverride(service);
      await saveServiceOverride({
        host_asset_id: service.host_asset_id,
        service_id: service.id,
        name_override: existingOverride?.name_override,
        category_override: existingOverride?.category_override,
        url_override: existingOverride?.url_override,
        icon_key_override: existingOverride?.icon_key_override,
        hidden: service.metadata?.hidden !== "true",
      });
      await refresh();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "Failed to update visibility");
    }
  }, [findExistingOverride, refresh, saveServiceOverride]);

  const handleDeleteManual = useCallback(async (service: WebService) => {
    if (service.source !== "manual") {
      return;
    }
    const confirmed = window.confirm(`Delete manual service "${service.name}"?`);
    if (!confirmed) {
      return;
    }
    try {
      await deleteManualService(service.id);
      await refresh();
    } catch (err) {
      window.alert(err instanceof Error ? err.message : "Failed to delete manual service");
    }
  }, [deleteManualService, refresh]);

  return {
    handleAddManualService,
    handleRename,
    handleToggleHidden,
    handleDeleteManual,
  };
}
