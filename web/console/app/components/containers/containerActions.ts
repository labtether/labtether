import { executeContainerAction } from "../../../lib/docker";
import { copyText as copyToClipboard } from "../../lib/clipboard";

export type CtAction = "start" | "stop" | "restart";

/** Where a container's actions go: the Docker agent (by the container's asset id) or its Portainer endpoint. */
export type CtTarget =
  | { source: "docker"; assetId: string }
  | { source: "portainer"; endpointAssetId: string; containerId: string };

/** Start, stop or restart one container. Throws with the hub's message when it fails. */
export async function runContainerAction(target: CtTarget, action: CtAction): Promise<void> {
  if (target.source === "docker") {
    const result = await executeContainerAction(target.assetId, `container.${action}`);
    if (result.status === "failed") throw new Error(result.message);
    return;
  }
  const path = `/api/portainer/assets/${encodeURIComponent(target.endpointAssetId)}/containers/${encodeURIComponent(target.containerId)}/${action}`;
  const response = await fetch(path, { method: "POST" });
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error || action);
  }
}

/** Copy text to the clipboard; false when the browser refuses. */
export async function copyText(text: string): Promise<boolean> {
  try {
    await copyToClipboard(text);
    return true;
  } catch {
    return false;
  }
}
