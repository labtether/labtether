"use client";

import type { Asset } from "../../../../console/models";
import {
  collectIdentityFromAsset,
  collectIdentityFromRecord,
  mergeIdentitySnapshots,
  readErrorMessage,
  safeJSON,
  type IdentitySnapshot,
} from "./clusterTopologyUtils";
import type {
  AssetDependency,
  LinkMode,
  ProxmoxDetailsPayload,
} from "./clusterTopologyTypes";

export type GuestLinkMutationResult = {
  ok: boolean;
  dependency?: AssetDependency | null;
  error?: string;
};

export async function fetchGuestRunsOnDependency(guestID: string): Promise<AssetDependency | null> {
  try {
    const response = await fetch(`/api/assets/${encodeURIComponent(guestID)}/dependencies?limit=200`, {
      cache: "no-store",
    });
    if (!response.ok) {
      return null;
    }
    const payload = await safeJSON<{ dependencies?: AssetDependency[] }>(response);
    const dependencies = Array.isArray(payload?.dependencies) ? payload.dependencies : [];
    const runsOn = dependencies.filter(
      (dependency) => dependency.source_asset_id === guestID && dependency.relationship_type === "runs_on",
    );
    if (runsOn.length === 0) {
      return null;
    }
    const manual = runsOn.find((dependency) => dependency.metadata?.binding === "manual");
    if (manual) {
      return manual;
    }
    const auto = runsOn.find((dependency) => dependency.metadata?.binding === "auto");
    if (auto) {
      return auto;
    }
    return runsOn[0] ?? null;
  } catch {
    return null;
  }
}

export async function fetchGuestIdentitySnapshot(
  guest: Asset,
  cachedIdentity?: IdentitySnapshot,
): Promise<IdentitySnapshot> {
  if (cachedIdentity) {
    return cachedIdentity;
  }

  const baseIdentity = collectIdentityFromAsset(guest);
  let mergedIdentity = baseIdentity;

  try {
    const response = await fetch(`/api/proxmox/assets/${encodeURIComponent(guest.id)}/details`, {
      cache: "no-store",
    });
    if (response.ok) {
      const details = await safeJSON<ProxmoxDetailsPayload>(response);
      if (details?.config && typeof details.config === "object") {
        mergedIdentity = mergeIdentitySnapshots(baseIdentity, collectIdentityFromRecord(details.config));
      }
    }
  } catch {
    // best-effort identity enrichment
  }

  return mergedIdentity;
}

export async function upsertGuestRunsOnDependency(
  guestID: string,
  targetID: string,
  options: {
    current: AssetDependency | null;
    mode: LinkMode;
    replaceExisting: boolean;
    matchReason?: string;
  },
): Promise<GuestLinkMutationResult> {
  try {
    if (options.replaceExisting && options.current?.id && options.current.target_asset_id !== targetID) {
      const removeResponse = await fetch(
        `/api/assets/${encodeURIComponent(guestID)}/dependencies/${encodeURIComponent(options.current.id)}`,
        { method: "DELETE", cache: "no-store" },
      );
      if (!removeResponse.ok && removeResponse.status !== 404) {
        return {
          ok: false,
          error: await readErrorMessage(removeResponse, "failed to replace existing mapping"),
        };
      }
    }

    const metadata: Record<string, string> = {
      binding: options.mode,
      source: "cluster_topology",
    };
    if (options.matchReason) {
      metadata.match_reason = options.matchReason;
    }

    const createResponse = await fetch(`/api/assets/${encodeURIComponent(guestID)}/dependencies`, {
      method: "POST",
      cache: "no-store",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        target_asset_id: targetID,
        relationship_type: "runs_on",
        direction: "downstream",
        criticality: "medium",
        metadata,
      }),
    });
    if (!createResponse.ok && createResponse.status !== 409) {
      return {
        ok: false,
        error: await readErrorMessage(createResponse, "failed to save API mapping"),
      };
    }

    return {
      ok: true,
      dependency: await fetchGuestRunsOnDependency(guestID),
    };
  } catch (error) {
    return {
      ok: false,
      error: error instanceof Error ? error.message : "failed to save API mapping",
    };
  }
}

export async function clearGuestRunsOnDependency(
  guestID: string,
  currentDependencyID: string,
): Promise<GuestLinkMutationResult> {
  try {
    const removeResponse = await fetch(
      `/api/assets/${encodeURIComponent(guestID)}/dependencies/${encodeURIComponent(currentDependencyID)}`,
      { method: "DELETE", cache: "no-store" },
    );
    if (!removeResponse.ok && removeResponse.status !== 404) {
      return {
        ok: false,
        error: await readErrorMessage(removeResponse, "failed to clear API mapping"),
      };
    }

    return {
      ok: true,
      dependency: await fetchGuestRunsOnDependency(guestID),
    };
  } catch (error) {
    return {
      ok: false,
      error: error instanceof Error ? error.message : "failed to clear API mapping",
    };
  }
}
