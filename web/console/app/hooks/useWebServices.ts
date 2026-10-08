"use client";

import type { UseWebServicesOptions } from "./web-services/types";
import { useServiceList } from "./web-services/useServiceList";
import { useServiceSync } from "./web-services/useServiceSync";
import { useServiceDetails } from "./web-services/useServiceDetails";
import {
  createManualService,
  updateManualService,
  deleteManualService,
  saveServiceOverride,
  listServiceOverrides,
  deleteServiceOverride,
} from "./web-services/serviceApi";
import {
  listCustomServiceIcons,
  createCustomServiceIcon,
  deleteCustomServiceIcon,
  renameCustomServiceIcon,
} from "./web-services/iconApi";

// Keep the public hook and type imports stable for service pages and node views.
export type * from "./web-services/types";

export function useWebServices(options: UseWebServicesOptions = {}) {
  const { servicesRef, setError, ...list } = useServiceList(options);
  const { syncing, sync } = useServiceSync(options.host, setError);
  const loadServiceDetails = useServiceDetails(options.host, options.includeHidden ?? false, servicesRef);

  return {
    ...list,
    syncing,
    sync,
    createManualService,
    updateManualService,
    deleteManualService,
    saveServiceOverride,
    listServiceOverrides,
    deleteServiceOverride,
    listCustomServiceIcons,
    createCustomServiceIcon,
    deleteCustomServiceIcon,
    renameCustomServiceIcon,
    loadServiceDetails,
  };
}
