export async function safeJSON<T>(response: Response): Promise<T | null> {
  try {
    return (await response.json()) as T;
  } catch {
    return null;
  }
}

export async function readErrorMessage(response: Response, fallback: string): Promise<string> {
  const payload = await safeJSON<{ error?: string }>(response);
  const message = typeof payload?.error === "string" ? payload.error.trim() : "";
  if (message.length > 0) {
    return message;
  }
  return fallback;
}

export function parseGuestIDFromGraphNodeID(nodeID: string): string {
  const prefix = "guest:";
  if (!nodeID.startsWith(prefix)) {
    return "";
  }
  return nodeID.slice(prefix.length).trim();
}

export function parseGuestIDFromGraphEdgeID(edgeID: string): string {
  const runsOnPrefix = "edge-runs-on:";
  if (edgeID.startsWith(runsOnPrefix)) {
    return edgeID.slice(runsOnPrefix.length).split(":")[0]?.trim() ?? "";
  }

  const missingPrefix = "edge-runs-on-missing:";
  if (edgeID.startsWith(missingPrefix)) {
    return edgeID.slice(missingPrefix.length).split(":")[0]?.trim() ?? "";
  }

  const placementPrefix = "edge-placement:";
  if (edgeID.startsWith(placementPrefix)) {
    const parts = edgeID.slice(placementPrefix.length).split(":");
    return parts[parts.length - 1]?.trim() ?? "";
  }

  return "";
}
