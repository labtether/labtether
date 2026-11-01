"use client";

import { useTranslations } from "next-intl";
import { useCallback } from "react";
import type {
  ManualWebServiceInput,
  WebService,
  WebServiceOverride,
  WebServiceOverrideInput,
} from "../../../hooks/useWebServices";
import { isHidden, readableError, type Notify } from "./servicesListModel";

interface UseServiceMutationActionsArgs {
  refresh: () => Promise<void>;
  notify: Notify;
  createManualService: (input: ManualWebServiceInput) => Promise<void>;
  deleteManualService: (id: string) => Promise<void>;
  saveServiceOverride: (input: WebServiceOverrideInput) => Promise<void>;
  listServiceOverrides: (hostAssetID?: string) => Promise<WebServiceOverride[]>;
}

/** Add a service by hand, hide or show one, and remove one that was added by hand. Results go to toasts. */
export function useServiceMutationActions({
  refresh,
  notify,
  createManualService,
  deleteManualService,
  saveServiceOverride,
  listServiceOverrides,
}: UseServiceMutationActionsArgs) {
  const t = useTranslations("services.toasts");

  const addManual = useCallback(async (input: ManualWebServiceInput) => {
    await createManualService(input);
    notify("success", t("added", { name: input.name }));
    await refresh();
  }, [createManualService, notify, refresh, t]);

  const toggleHidden = useCallback(async (service: WebService) => {
    const hide = !isHidden(service);
    try {
      const overrides = await listServiceOverrides(service.host_asset_id);
      const existing = overrides.find((o) => o.host_asset_id === service.host_asset_id && o.service_id === service.id);
      await saveServiceOverride({
        host_asset_id: service.host_asset_id,
        service_id: service.id,
        name_override: existing?.name_override,
        category_override: existing?.category_override,
        url_override: existing?.url_override,
        icon_key_override: existing?.icon_key_override,
        tags_override: existing?.tags_override,
        hidden: hide,
      });
      notify("success", hide ? t("hidden", { name: service.name }) : t("shown", { name: service.name }));
      await refresh();
    } catch (err) {
      notify("error", readableError(err, t("saveFailed", { name: service.name })));
    }
  }, [listServiceOverrides, notify, refresh, saveServiceOverride, t]);

  const removeManual = useCallback(async (service: WebService) => {
    try {
      await deleteManualService(service.id);
      notify("success", t("removed", { name: service.name }));
      await refresh();
    } catch (err) {
      notify("error", readableError(err, t("removeFailed", { name: service.name })));
    }
  }, [deleteManualService, notify, refresh, t]);

  return { addManual, toggleHidden, removeManual };
}
