"use client";

import { useTranslations } from "next-intl";
import { useCallback, useMemo, useState } from "react";
import { executeDockerHostAction } from "../../../../lib/docker";
import type { WebService } from "../../../hooks/useWebServices";
import { buildDockerImagePullPlan } from "./servicesLayoutHelpers";
import { readableError, type Notify } from "./servicesListModel";

interface UsePullImagesActionArgs {
  services: WebService[];
  hostFilter: string;
  assetNameMap: Map<string, string>;
  refresh: () => Promise<void>;
  notify: Notify;
}

/**
 * Pulls the newest image for each Docker service shown (one pull per image and host, four at a time). The page asks
 * first; the toast says how many pulled and names the first failure.
 */
export function usePullImagesAction({ services, hostFilter, assetNameMap, refresh, notify }: UsePullImagesActionArgs) {
  const t = useTranslations("services.toasts");
  const [pullingImages, setPullingImages] = useState(false);

  const pullPlan = useMemo(
    () => buildDockerImagePullPlan(services, hostFilter, assetNameMap),
    [assetNameMap, hostFilter, services],
  );

  const pullImages = useCallback(async () => {
    if (pullPlan.items.length === 0) return;
    setPullingImages(true);
    try {
      const queue = [...pullPlan.items];
      const failures: string[] = [];
      let pulled = 0;
      await Promise.all(Array.from({ length: Math.min(4, queue.length) }, async () => {
        for (let item = queue.shift(); item; item = queue.shift()) {
          try {
            const result = await executeDockerHostAction(item.hostAssetID, "image.pull", { image: item.image });
            if (result.status.toLowerCase() === "failed") {
              failures.push(`${item.hostLabel}: ${item.image}${result.message ? ` (${result.message})` : ""}`);
            } else {
              pulled += 1;
            }
          } catch (err) {
            failures.push(`${item.hostLabel}: ${item.image} (${readableError(err, t("pullFailedOne"))})`);
          }
        }
      }));
      if (failures.length > 0) {
        notify("error", t("pullPartial", { pulled, total: pullPlan.items.length, first: failures[0] }));
      } else {
        notify("success", t("pulled", { count: pulled }));
      }
      await refresh();
    } finally {
      setPullingImages(false);
    }
  }, [notify, pullPlan.items, refresh, t]);

  return { pullPlan, pullingImages, pullImages };
}
