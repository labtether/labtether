"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Asset } from "../../../../console/models";
import {
  normalizeAssetName,
  pickStrongIdentityMatch,
  type IdentitySnapshot,
} from "./clusterTopologyUtils";
import type { AssetDependency } from "./clusterTopologyTypes";
import {
  clearGuestRunsOnDependency,
  fetchGuestIdentitySnapshot,
  fetchGuestRunsOnDependency,
  upsertGuestRunsOnDependency,
} from "./clusterTopologyGuestLinkingApi";

type UseClusterTopologyGuestLinkingArgs = {
  guestAssets: Asset[];
  apiHosts: Asset[];
  apiHostsByID: Map<string, Asset>;
  apiHostsByName: Map<string, Asset[]>;
  hostIdentityByID: Map<string, IdentitySnapshot>;
};

// Collector-side identity linking is authoritative for cross-source runs_on
// relationships. Keep UI auto-linking as suggestion-only to avoid writing
// reverse runs_on edges that can fight backend chain inference.
const enableTopologyAutoRunsOnWrites = false;

export function useClusterTopologyGuestLinking({
  guestAssets,
  apiHosts,
  apiHostsByID,
  apiHostsByName,
  hostIdentityByID,
}: UseClusterTopologyGuestLinkingArgs) {
  const [guestRunsOn, setGuestRunsOn] = useState<Record<string, AssetDependency | null>>({});
  const [linkDrafts, setLinkDrafts] = useState<Record<string, string>>({});
  const [loadingGuestLinks, setLoadingGuestLinks] = useState(false);
  const [savingGuestID, setSavingGuestID] = useState<string | null>(null);
  const [linkErrors, setLinkErrors] = useState<Record<string, string>>({});
  const [guestIdentityCache, setGuestIdentityCache] = useState<Record<string, IdentitySnapshot>>({});
  const [autoAttemptedGuestIDs, setAutoAttemptedGuestIDs] = useState<Record<string, true>>({});
  const [autoLinkingGuestID, setAutoLinkingGuestID] = useState<string | null>(null);

  const fetchGuestRunsOn = useCallback((guestID: string): Promise<AssetDependency | null> => {
    return fetchGuestRunsOnDependency(guestID);
  }, []);

  const suggestedHostForGuest = useCallback((guest: Asset): Asset | undefined => {
    const key = normalizeAssetName(guest.name);
    if (!key) {
      return undefined;
    }
    const matches = apiHostsByName.get(key) ?? [];
    return matches[0];
  }, [apiHostsByName]);

  const fetchGuestIdentity = useCallback(async (guest: Asset): Promise<IdentitySnapshot> => {
    const cached = guestIdentityCache[guest.id];
    const mergedIdentity = await fetchGuestIdentitySnapshot(guest, cached);
    setGuestIdentityCache((prev) => prev[guest.id] ? prev : { ...prev, [guest.id]: mergedIdentity });
    return mergedIdentity;
  }, [guestIdentityCache]);

  const upsertGuestLink = useCallback(async (
    guestID: string,
    targetID: string,
    options: { mode: "manual" | "auto"; replaceExisting: boolean; matchReason?: string },
  ): Promise<{ ok: boolean; error?: string }> => {
    const current = guestRunsOn[guestID] ?? null;
    const result = await upsertGuestRunsOnDependency(guestID, targetID, {
      current,
      mode: options.mode,
      replaceExisting: options.replaceExisting,
      matchReason: options.matchReason,
    });
    if (result.ok) {
      setGuestRunsOn((prev) => ({ ...prev, [guestID]: result.dependency ?? null }));
    }
    return result;
  }, [guestRunsOn]);

  useEffect(() => {
    if (guestAssets.length === 0) {
      setGuestRunsOn({});
      setLinkDrafts({});
      setLinkErrors({});
      setLoadingGuestLinks(false);
      return;
    }

    let cancelled = false;
    setLoadingGuestLinks(true);

    void (async () => {
      const loaded = await Promise.all(
        guestAssets.map(async (guest) => ({
          guest,
          dependency: await fetchGuestRunsOn(guest.id),
        })),
      );

      if (cancelled) {
        return;
      }

      const nextRunsOn: Record<string, AssetDependency | null> = {};
      const fallbackDrafts: Record<string, string> = {};
      for (const entry of loaded) {
        nextRunsOn[entry.guest.id] = entry.dependency;
        const targetID = entry.dependency?.target_asset_id ?? suggestedHostForGuest(entry.guest)?.id ?? "";
        if (targetID) {
          fallbackDrafts[entry.guest.id] = targetID;
        }
      }

      setGuestRunsOn(nextRunsOn);
      setLinkDrafts((prev) => {
        const merged: Record<string, string> = {};
        for (const guest of guestAssets) {
          const prior = prev[guest.id];
          if (prior && apiHostsByID.has(prior)) {
            merged[guest.id] = prior;
            continue;
          }
          const fallback = fallbackDrafts[guest.id];
          if (fallback) {
            merged[guest.id] = fallback;
          }
        }
        return merged;
      });
      setLinkErrors((prev) => {
        const cleaned: Record<string, string> = {};
        for (const guest of guestAssets) {
          const message = prev[guest.id];
          if (message) {
            cleaned[guest.id] = message;
          }
        }
        return cleaned;
      });
      setGuestIdentityCache((prev) => {
        const kept: Record<string, IdentitySnapshot> = {};
        for (const guest of guestAssets) {
          const identity = prev[guest.id];
          if (identity) {
            kept[guest.id] = identity;
          }
        }
        return kept;
      });
      setAutoAttemptedGuestIDs((prev) => {
        const kept: Record<string, true> = {};
        for (const guest of guestAssets) {
          if (prev[guest.id]) {
            kept[guest.id] = true;
          }
        }
        return kept;
      });
      setLoadingGuestLinks(false);
    })().catch(() => {
      if (!cancelled) {
        setLoadingGuestLinks(false);
      }
    });

    return () => {
      cancelled = true;
    };
  }, [apiHostsByID, fetchGuestRunsOn, guestAssets, suggestedHostForGuest]);

  const pendingAutoGuest = useMemo(() => {
    if (!enableTopologyAutoRunsOnWrites) {
      return null;
    }
    if (loadingGuestLinks || apiHosts.length === 0) {
      return null;
    }
    return guestAssets.find((guest) => !guestRunsOn[guest.id] && !autoAttemptedGuestIDs[guest.id]) ?? null;
  }, [apiHosts.length, autoAttemptedGuestIDs, guestAssets, guestRunsOn, loadingGuestLinks]);

  useEffect(() => {
    if (!pendingAutoGuest) {
      return;
    }

    let cancelled = false;

    void (async () => {
      const guest = pendingAutoGuest;
      const guestIdentity = await fetchGuestIdentity(guest);
      if (cancelled) {
        return;
      }

      const identityMatch = pickStrongIdentityMatch(guestIdentity, hostIdentityByID, apiHostsByID);
      setAutoAttemptedGuestIDs((prev) => prev[guest.id] ? prev : { ...prev, [guest.id]: true });
      if (!identityMatch) {
        return;
      }

      setAutoLinkingGuestID(guest.id);
      const result = await upsertGuestLink(
        guest.id,
        identityMatch.hostID,
        { mode: "auto", replaceExisting: false, matchReason: identityMatch.reason },
      );

      if (cancelled) {
        return;
      }

      if (result.ok) {
        setLinkDrafts((prev) => ({ ...prev, [guest.id]: identityMatch.hostID }));
        setLinkErrors((prev) => ({ ...prev, [guest.id]: "" }));
      } else if (result.error) {
        setLinkErrors((prev) => ({ ...prev, [guest.id]: result.error ?? "failed to save API mapping" }));
      }
      setAutoLinkingGuestID((current) => current === guest.id ? null : current);
    })();

    return () => {
      cancelled = true;
      setAutoLinkingGuestID((current) => current === pendingAutoGuest.id ? null : current);
    };
  }, [apiHostsByID, fetchGuestIdentity, hostIdentityByID, pendingAutoGuest, upsertGuestLink]);

  const setGuestLinkDraft = useCallback((guestID: string, targetID: string) => {
    setLinkDrafts((prev) => ({ ...prev, [guestID]: targetID }));
  }, []);

  const saveGuestLink = useCallback(async (guest: Asset) => {
    const nextTargetID = (linkDrafts[guest.id] ?? "").trim();
    if (!nextTargetID) {
      return;
    }

    const current = guestRunsOn[guest.id] ?? null;
    if (current?.target_asset_id === nextTargetID) {
      return;
    }

    setSavingGuestID(guest.id);
    setLinkErrors((prev) => ({ ...prev, [guest.id]: "" }));

    const result = await upsertGuestLink(guest.id, nextTargetID, { mode: "manual", replaceExisting: true });
    if (!result.ok) {
      setLinkErrors((prev) => ({
        ...prev,
        [guest.id]: result.error ?? "failed to save API mapping",
      }));
    } else {
      setAutoAttemptedGuestIDs((prev) => ({ ...prev, [guest.id]: true }));
      setLinkErrors((prev) => ({ ...prev, [guest.id]: "" }));
    }
    setSavingGuestID(null);
  }, [guestRunsOn, linkDrafts, upsertGuestLink]);

  const clearGuestLink = useCallback(async (guest: Asset) => {
    const current = guestRunsOn[guest.id];
    if (!current?.id) {
      return;
    }

    setSavingGuestID(guest.id);
    setLinkErrors((prev) => ({ ...prev, [guest.id]: "" }));

    try {
      const result = await clearGuestRunsOnDependency(guest.id, current.id);
      if (!result.ok) {
        throw new Error(result.error ?? "failed to clear API mapping");
      }

      const refreshed = result.dependency ?? null;
      const suggestedID = refreshed?.target_asset_id ?? suggestedHostForGuest(guest)?.id ?? "";
      setGuestRunsOn((prev) => ({ ...prev, [guest.id]: refreshed }));
      setLinkDrafts((prev) => ({ ...prev, [guest.id]: suggestedID }));
      setLinkErrors((prev) => ({ ...prev, [guest.id]: "" }));
      setAutoAttemptedGuestIDs((prev) => ({ ...prev, [guest.id]: true }));
    } catch (error) {
      setLinkErrors((prev) => ({
        ...prev,
        [guest.id]: error instanceof Error ? error.message : "failed to clear API mapping",
      }));
    } finally {
      setSavingGuestID(null);
    }
  }, [guestRunsOn, suggestedHostForGuest]);

  return {
    guestRunsOn,
    linkDrafts,
    loadingGuestLinks,
    savingGuestID,
    linkErrors,
    autoLinkingGuestID,
    suggestedHostForGuest,
    setGuestLinkDraft,
    saveGuestLink,
    clearGuestLink,
  };
}
