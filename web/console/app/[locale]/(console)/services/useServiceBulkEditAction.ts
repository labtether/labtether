"use client";

import { useTranslations } from "next-intl";
import { useCallback, useState } from "react";
import type { ServiceBulkEditChanges } from "../../../components/ServiceBulkEditModal";
import type {
  WebService,
  WebServiceOverride,
  WebServiceOverrideInput,
} from "../../../hooks/useWebServices";
import { runWithConcurrencyLimit, serviceOverrideLookupKey } from "./servicesLayoutHelpers";
import { readableError, type Notify } from "./servicesListModel";

interface UseServiceBulkEditActionArgs {
  listServiceOverrides: (hostAssetID?: string) => Promise<WebServiceOverride[]>;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  refresh: () => Promise<void>;
  notify: Notify;
}

/**
 * Applies one set of changes (category, hidden, icon) to many services, keeping each one's other overrides. Six saves
 * run at a time; the toast says how many saved and names the first that failed.
 */
export function useServiceBulkEditAction({ listServiceOverrides, saveServiceOverride, refresh, notify }: UseServiceBulkEditActionArgs) {
  const t = useTranslations("services.toasts");
  const [bulkSaving, setBulkSaving] = useState(false);

  const applyBulkEdits = useCallback(async (targets: WebService[], changes: ServiceBulkEditChanges) => {
    if (targets.length === 0) return;
    setBulkSaving(true);
    try {
      const existing = new Map<string, WebServiceOverride>();
      const hosts = Array.from(new Set(targets.map((service) => service.host_asset_id)));
      await Promise.all(hosts.map(async (host) => {
        for (const override of await listServiceOverrides(host)) {
          existing.set(serviceOverrideLookupKey(override.host_asset_id, override.service_id), override);
        }
      }));

      const failures: string[] = [];
      let saved = 0;
      await runWithConcurrencyLimit(targets.map((service) => async () => {
        const prior = existing.get(serviceOverrideLookupKey(service.host_asset_id, service.id));
        let icon = prior?.icon_key_override;
        if (changes.iconMode === "set") icon = changes.iconValue ?? "";
        else if (changes.iconMode === "clear") icon = "";
        try {
          await saveServiceOverride({
            host_asset_id: service.host_asset_id,
            service_id: service.id,
            name_override: prior?.name_override,
            category_override: changes.category ?? prior?.category_override,
            url_override: prior?.url_override,
            icon_key_override: icon,
            tags_override: prior?.tags_override,
            hidden: changes.hidden ?? service.metadata?.hidden === "true",
          });
          saved += 1;
        } catch (err) {
          failures.push(`${service.name}: ${readableError(err, t("notSaved"))}`);
        }
      }), 6);
      await refresh();

      if (failures.length > 0) {
        notify("error", t("bulkPartial", { saved, total: targets.length, first: failures[0] }));
      } else {
        notify("success", t("bulkSaved", { count: saved }));
      }
    } catch (err) {
      notify("error", readableError(err, t("bulkFailed")));
    } finally {
      setBulkSaving(false);
    }
  }, [listServiceOverrides, notify, refresh, saveServiceOverride, t]);

  return { bulkSaving, applyBulkEdits };
}
