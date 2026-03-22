"use client";

import { useCallback, useMemo, useState } from "react";
import { executeDockerHostAction } from "../../../../lib/docker";
import type { WebService } from "../../../hooks/useWebServices";
import { buildDockerImagePullPlan } from "./servicesPageHelpers";

interface UsePullImagesActionArgs {
  services: WebService[];
  hostFilter: string;
  assetNameMap: Map<string, string>;
  refresh: () => Promise<void>;
}

export function usePullImagesAction({
  services,
  hostFilter,
  assetNameMap,
  refresh,
}: UsePullImagesActionArgs) {
  const [pullingImages, setPullingImages] = useState(false);

  const pullPlan = useMemo(
    () => buildDockerImagePullPlan(services, hostFilter, assetNameMap),
    [assetNameMap, hostFilter, services]
  );

  const handlePullImages = useCallback(async () => {
    if (pullPlan.items.length === 0) {
      const scopeLabel =
        hostFilter === "all"
          ? "all hosts"
          : (assetNameMap.get(hostFilter) ?? hostFilter);
      const missingHint =
        pullPlan.missingImageCount > 0
          ? `\n${pullPlan.missingImageCount} docker service(s) in this scope are missing image metadata. Run Refresh first.`
          : "";
      window.alert(`No docker service images were found for ${scopeLabel}.${missingHint}`);
      return;
    }

    const scopeLabel =
      hostFilter === "all"
        ? "all hosts"
        : (assetNameMap.get(hostFilter) ?? hostFilter);
    const confirmed = window.confirm(
      `Pull latest images for ${pullPlan.items.length} unique service image(s) on ${scopeLabel}?`
    );
    if (!confirmed) {
      return;
    }

    setPullingImages(true);
    try {
      const queue = [...pullPlan.items];
      const failures: Array<{ image: string; host: string; message: string }> = [];
      let succeeded = 0;

      const workerCount = Math.min(4, queue.length);
      await Promise.all(
        Array.from({ length: workerCount }, async () => {
          while (queue.length > 0) {
            const item = queue.shift();
            if (!item) {
              return;
            }
            try {
              const result = await executeDockerHostAction(item.hostAssetID, "image.pull", {
                image: item.image,
              });
              if (result.status.toLowerCase() === "failed") {
                failures.push({
                  image: item.image,
                  host: item.hostLabel,
                  message: result.message || "image pull failed",
                });
                continue;
              }
              succeeded++;
            } catch (error) {
              failures.push({
                image: item.image,
                host: item.hostLabel,
                message: error instanceof Error ? error.message : "image pull failed",
              });
            }
          }
        })
      );

      const attempted = pullPlan.items.length;
      const failed = failures.length;
      const skippedMissing = pullPlan.missingImageCount;
      let summary =
        `Pulled service images.\n` +
        `Attempted: ${attempted}\n` +
        `Succeeded: ${succeeded}\n` +
        `Failed: ${failed}`;
      if (skippedMissing > 0) {
        summary += `\nSkipped (missing image metadata): ${skippedMissing}`;
      }
      if (failed > 0) {
        const topFailures = failures
          .slice(0, 5)
          .map((failure) => `- ${failure.host}: ${failure.image} (${failure.message})`)
          .join("\n");
        summary += `\n\nFailures:\n${topFailures}`;
        if (failures.length > 5) {
          summary += `\n- ...and ${failures.length - 5} more`;
        }
      }
      window.alert(summary);

      await refresh();
    } finally {
      setPullingImages(false);
    }
  }, [assetNameMap, hostFilter, pullPlan, refresh]);

  return {
    pullPlan,
    pullingImages,
    handlePullImages,
  };
}
