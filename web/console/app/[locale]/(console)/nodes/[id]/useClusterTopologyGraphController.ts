"use client";

import { useCallback, useEffect, useState, type MouseEvent } from "react";
import { useRouter } from "../../../../../i18n/navigation";
import {
  type Edge,
  type Node,
} from "@xyflow/react";
import type { Asset } from "../../../../console/models";
import {
  parseGuestIDFromGraphEdgeID,
  parseGuestIDFromGraphNodeID,
} from "./clusterTopologyUtils";
import type { ClusterTopologyNodeData } from "./clusterTopologyFlowModel";

type UseClusterTopologyGraphControllerArgs = {
  guestsByID: Map<string, Asset>;
};

export function useClusterTopologyGraphController({
  guestsByID,
}: UseClusterTopologyGraphControllerArgs) {
  const router = useRouter();
  const [graphSelectedGuestID, setGraphSelectedGuestID] = useState<string | null>(null);

  useEffect(() => {
    if (!graphSelectedGuestID) {
      return;
    }
    if (guestsByID.has(graphSelectedGuestID)) {
      return;
    }
    setGraphSelectedGuestID(null);
  }, [graphSelectedGuestID, guestsByID]);

  const openAssetDetails = useCallback((assetID: string) => {
    const trimmed = assetID.trim();
    if (!trimmed) {
      return;
    }
    void router.push(`/nodes/${encodeURIComponent(trimmed)}`);
  }, [router]);

  const onGraphNodeClick = useCallback((_: MouseEvent, node: Node<ClusterTopologyNodeData>) => {
    const guestID = parseGuestIDFromGraphNodeID(node.id);
    if (guestID) {
      setGraphSelectedGuestID(guestID);
      return;
    }

    const assetID = node.data.assetID?.trim();
    if (!assetID) {
      return;
    }
    openAssetDetails(assetID);
  }, [openAssetDetails]);

  const onGraphNodeDoubleClick = useCallback((_: MouseEvent, node: Node<ClusterTopologyNodeData>) => {
    const guestID = parseGuestIDFromGraphNodeID(node.id);
    if (guestID) {
      openAssetDetails(guestID);
    }
  }, [openAssetDetails]);

  const onGraphEdgeClick = useCallback((_: MouseEvent, edge: Edge) => {
    const guestID = parseGuestIDFromGraphEdgeID(edge.id);
    if (!guestID) {
      return;
    }
    setGraphSelectedGuestID(guestID);
  }, []);

  const onGraphPaneClick = useCallback(() => {
    setGraphSelectedGuestID(null);
  }, []);

  return {
    graphSelectedGuestID,
    openAssetDetails,
    onGraphNodeClick,
    onGraphNodeDoubleClick,
    onGraphEdgeClick,
    onGraphPaneClick,
  };
}
