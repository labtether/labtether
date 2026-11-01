"use client";

import type { Group } from "../../../console/models";

/** From this depth on (0 = top level), the forms note that deep nesting slows the dashboard. */
export const NESTING_DEPTH_WARN = 5;

export function computeGroupDepth(groups: Group[], groupId: string): number {
  const byId = new Map(groups.map((g) => [g.id, g]));
  let depth = 0;
  let current = byId.get(groupId);
  while (current?.parent_group_id) {
    depth++;
    current = byId.get(current.parent_group_id);
    if (depth > 20) break; // guard against cycles
  }
  return depth;
}

export async function apiMoveGroup(
  groupID: string,
  parentGroupID: string | null,
): Promise<void> {
  const response = await fetch(
    `/api/groups/${encodeURIComponent(groupID)}/move`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ parent_group_id: parentGroupID ?? "" }),
    },
  );
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as
      | { error?: string }
      | null;
    throw new Error(
      payload?.error ?? `Failed to move group (${response.status})`,
    );
  }
}
