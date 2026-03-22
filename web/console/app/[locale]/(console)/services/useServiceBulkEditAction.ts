"use client";

import { useCallback, useState } from "react";
import type { ServiceBulkEditChanges } from "../../../components/ServiceBulkEditModal";
import type {
  WebService,
  WebServiceOverride,
  WebServiceOverrideInput,
} from "../../../hooks/useWebServices";
import {
  runWithConcurrencyLimit,
  serviceOverrideLookupKey,
} from "./servicesPageHelpers";

interface UseServiceBulkEditActionArgs {
  bulkEditTargets: WebService[];
  selectionMode: boolean;
  clearSelection: () => void;
  listServiceOverrides: (hostAssetID?: string) => Promise<WebServiceOverride[]>;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  refresh: () => Promise<void>;
}

export function useServiceBulkEditAction({
  bulkEditTargets,
  selectionMode,
  clearSelection,
  listServiceOverrides,
  saveServiceOverride,
  refresh,
}: UseServiceBulkEditActionArgs) {
  const [bulkSaving, setBulkSaving] = useState(false);

  const handleApplyBulkEdits = useCallback(
    async (changes: ServiceBulkEditChanges) => {
      if (bulkEditTargets.length === 0) {
        return;
      }

      setBulkSaving(true);
      try {
        const hostAssetIDs = Array.from(
          new Set(bulkEditTargets.map((service) => service.host_asset_id))
        );
        const existingOverrides = new Map<string, WebServiceOverride>();

        await Promise.all(
          hostAssetIDs.map(async (hostAssetID) => {
            const hostOverrides = await listServiceOverrides(hostAssetID);
            for (const override of hostOverrides) {
              existingOverrides.set(
                serviceOverrideLookupKey(override.host_asset_id, override.service_id),
                override
              );
            }
          })
        );

        const failures: string[] = [];
        let successCount = 0;

        const tasks = bulkEditTargets.map((service) => async () => {
          const existingOverride = existingOverrides.get(
            serviceOverrideLookupKey(service.host_asset_id, service.id)
          );
          const hidden =
            changes.hidden !== undefined ? changes.hidden : service.metadata?.hidden === "true";

          let iconOverride = existingOverride?.icon_key_override;
          if (changes.iconMode === "set") {
            iconOverride = changes.iconValue ?? "";
          } else if (changes.iconMode === "clear") {
            iconOverride = "";
          }

          try {
            await saveServiceOverride({
              host_asset_id: service.host_asset_id,
              service_id: service.id,
              name_override: existingOverride?.name_override,
              category_override:
                changes.category !== undefined
                  ? changes.category
                  : existingOverride?.category_override,
              url_override: existingOverride?.url_override,
              icon_key_override: iconOverride,
              hidden,
            });
            successCount += 1;
          } catch (err) {
            failures.push(
              `${service.name}: ${err instanceof Error ? err.message : "failed to save override"}`
            );
          }
        });

        await runWithConcurrencyLimit(tasks, 6);
        await refresh();

        if (failures.length > 0) {
          const lines = failures
            .slice(0, 5)
            .map((entry) => `- ${entry}`)
            .join("\n");
          const overflow =
            failures.length > 5 ? `\n- ...and ${failures.length - 5} more` : "";
          window.alert(
            "Bulk edit applied with partial failures.\n" +
              `Updated: ${successCount}/${bulkEditTargets.length}\n` +
              `Failed: ${failures.length}\n\n` +
              `${lines}${overflow}`
          );
          return;
        }

        window.alert(
          `Bulk edit applied to ${successCount} service${successCount === 1 ? "" : "s"}.`
        );
      } finally {
        setBulkSaving(false);
        if (selectionMode) {
          clearSelection();
        }
      }
    },
    [
      bulkEditTargets,
      clearSelection,
      listServiceOverrides,
      refresh,
      saveServiceOverride,
      selectionMode,
    ]
  );

  return {
    bulkSaving,
    handleApplyBulkEdits,
  };
}
