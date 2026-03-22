"use client";

import type { Composite } from "../console/models";

// ---------------------------------------------------------------------------
// Mutation helpers (not hooks)
// ---------------------------------------------------------------------------
// Note: there is no batch composites listing endpoint. The mutation helpers
// are the primary interface for this resource. If a listing hook is needed
// in the future, add a GET /api/v1/composites?asset_ids=... backend route
// and implement the hook following the useEdges pattern.

export async function createComposite(
  primaryID: string,
  facetIDs: string[],
): Promise<Composite> {
  const res = await fetch("/api/v1/composites", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ primary_asset_id: primaryID, facet_asset_ids: facetIDs }),
  });
  if (!res.ok) {
    throw new Error(`createComposite failed: ${res.status}`);
  }
  return res.json() as Promise<Composite>;
}

export async function changePrimary(
  compositeID: string,
  newPrimaryID: string,
): Promise<void> {
  const res = await fetch(`/api/v1/composites/${encodeURIComponent(compositeID)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ new_primary_asset_id: newPrimaryID }),
  });
  if (!res.ok) {
    throw new Error(`changePrimary failed: ${res.status}`);
  }
}

export async function detachMember(
  compositeID: string,
  memberID: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/composites/${encodeURIComponent(compositeID)}/members/${encodeURIComponent(memberID)}`,
    { method: "DELETE" },
  );
  if (!res.ok) {
    throw new Error(`detachMember failed: ${res.status}`);
  }
}
