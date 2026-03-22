import type { Asset } from "../../../../console/models";

function normalizeDockerIdentity(value: string): string {
  return value.trim().toLowerCase().replaceAll(" ", "-").replaceAll(".", "-");
}

function stripDockerHostPrefix(value: string): string {
  return value.replace(/^docker-host-/, "").replace(/^docker-/, "");
}

export function deriveDockerHostId(asset: Asset | null): string {
  if (!asset || asset.source !== "docker" || asset.type !== "container-host") return "";

  const metadataAgentID = (asset.metadata?.agent_id ?? "").trim();
  if (metadataAgentID) {
    return normalizeDockerIdentity(metadataAgentID);
  }

  const assetID = (asset.id ?? "").trim();
  if (assetID.startsWith("docker-host-")) {
    return assetID.slice("docker-host-".length);
  }
  if (assetID.startsWith("docker-")) {
    return assetID.slice("docker-".length);
  }
  return assetID;
}

export function deriveDockerStackHostId(asset: Asset | null, assets: Asset[] | undefined): string {
  if (!asset || asset.source !== "docker" || asset.type !== "compose-stack") return "";

  const metadataAgentID = (asset.metadata?.agent_id ?? "").trim();
  if (metadataAgentID) {
    return normalizeDockerIdentity(metadataAgentID);
  }

  const assetID = (asset.id ?? "").trim();
  if (!assetID.startsWith("docker-stack-")) return "";

  const rest = assetID.slice("docker-stack-".length);
  const hostCandidates = (assets ?? [])
    .filter((entry) => entry.source === "docker" && entry.type === "container-host")
    .map((entry) => stripDockerHostPrefix(normalizeDockerIdentity(entry.metadata?.agent_id ?? entry.id)))
    .filter((value) => value !== "")
    .sort((a, b) => b.length - a.length);

  for (const candidate of hostCandidates) {
    if (rest === candidate || rest.startsWith(`${candidate}-`)) {
      return candidate;
    }
  }
  return "";
}

export function deriveCanonicalDockerHostNodeID(asset: Asset | null): string {
  if (!asset || asset.source !== "docker" || asset.type !== "container-host") return "";

  const agentID = (asset.metadata?.agent_id ?? "").trim();
  if (!agentID) return "";

  const normalized = normalizeDockerIdentity(agentID);
  if (!normalized) return "";
  return `docker-host-${normalized}`;
}

export function deriveFallbackDockerHostNodeID(
  nodeId: string,
  assets: Asset[] | undefined,
  hasResolvedAsset: boolean,
): string {
  const normalizedNodeID = (nodeId ?? "").trim().toLowerCase();
  if (!normalizedNodeID || !assets || hasResolvedAsset) return "";
  if (normalizedNodeID.startsWith("docker-ct-") || normalizedNodeID.startsWith("docker-stack-")) return "";

  const candidateIDs: string[] = [];
  if (normalizedNodeID.startsWith("docker-host-")) {
    candidateIDs.push(normalizedNodeID);
  } else if (normalizedNodeID.startsWith("docker-")) {
    candidateIDs.push(`docker-host-${normalizedNodeID.slice("docker-".length)}`);
  } else {
    candidateIDs.push(`docker-host-${normalizedNodeID}`);
  }

  for (const candidateID of candidateIDs) {
    const exists = assets.some((entry) =>
      entry.id.toLowerCase() === candidateID &&
      entry.source === "docker" &&
      entry.type === "container-host"
    );
    if (exists) return candidateID;
  }
  return "";
}

export function deriveMergedDockerHostId(mergedDockerHost: Asset | null): string {
  if (!mergedDockerHost) return "";

  const agentID = (mergedDockerHost.metadata?.agent_id ?? "").trim();
  if (agentID) return normalizeDockerIdentity(agentID);
  return mergedDockerHost.id;
}
