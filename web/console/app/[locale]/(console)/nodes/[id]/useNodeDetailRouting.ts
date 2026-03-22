"use client";

import { useEffect, useMemo } from "react";
import { useSearchParams } from "next/navigation";
import { useRouter } from "../../../../../i18n/navigation";

type UseNodeDetailRoutingArgs = {
  nodeId: string;
  canonicalDockerHostNodeID: string;
  fallbackDockerHostNodeID: string;
};

type UseNodeDetailRoutingResult = {
  activePanel: string | null;
  activeSub: string | null;
  activeDetail: string | null;
  activeTabForHook: string;
};

export function useNodeDetailRouting({
  nodeId,
  canonicalDockerHostNodeID,
  fallbackDockerHostNodeID,
}: UseNodeDetailRoutingArgs): UseNodeDetailRoutingResult {
  const router = useRouter();
  const searchParams = useSearchParams();

  const activePanel = searchParams.get("panel");
  const activeSub = searchParams.get("sub");
  const activeDetail = activePanel === "system" ? searchParams.get("detail") : null;

  const activeTabForHook = useMemo(() => {
    if (!activePanel) return "overview";
    if (activePanel === "system" && activeDetail) return "telemetry";
    if (activePanel === "monitoring") return "telemetry";
    if (activePanel === "network") return "interfaces";
    return activePanel;
  }, [activeDetail, activePanel]);

  useEffect(() => {
    const targetNodeID = canonicalDockerHostNodeID || fallbackDockerHostNodeID;
    if (!targetNodeID) return;
    if (targetNodeID === nodeId) return;

    const query = searchParams.toString();
    router.replace(`/nodes/${targetNodeID}${query ? `?${query}` : ""}`);
  }, [canonicalDockerHostNodeID, fallbackDockerHostNodeID, nodeId, router, searchParams]);

  return {
    activePanel,
    activeSub,
    activeDetail,
    activeTabForHook,
  };
}
