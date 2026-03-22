export function normalizeHostID(hostId: string): string {
  return hostId
    .trim()
    .toLowerCase()
    .replaceAll(" ", "-")
    .replaceAll(".", "-")
    .replace(/^docker-host-/, "")
    .replace(/^docker-/, "");
}

export function hostAssetID(agentId: string): string {
  return `docker-host-${normalizeHostID(agentId)}`;
}

export function containerAssetID(hostId: string, containerID: string): string {
  const shortID = containerID.length > 12 ? containerID.slice(0, 12) : containerID;
  return `docker-ct-${normalizeHostID(hostId)}-${shortID}`;
}

export function containerBadgeStatus(state: string): "ok" | "pending" | "bad" {
  if (state === "running") return "ok";
  if (state === "paused") return "pending";
  return "bad";
}
