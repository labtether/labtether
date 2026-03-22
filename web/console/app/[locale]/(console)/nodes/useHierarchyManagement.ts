"use client";

import { useCallback, useState } from "react";

type ErrorPayload = { error?: string } | null;

async function parseAPIError(response: Response, fallback: string): Promise<Error> {
  const payload = (await response.json().catch(() => null)) as ErrorPayload;
  return new Error(payload?.error || fallback);
}

export function useHierarchyManagement(refreshData: () => void) {
  const [isManaging, setIsManaging] = useState(false);

  const createGroup = useCallback(
    async (name: string, parentGroupID?: string) => {
      const body: Record<string, string> = { name, slug: name.toLowerCase().replace(/[^a-z0-9]+/g, "-") };
      if (parentGroupID) body.parent_group_id = parentGroupID;
      const response = await fetch("/api/groups", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!response.ok) {
        throw await parseAPIError(response, `failed to create group (${response.status})`);
      }
      refreshData();
    },
    [refreshData],
  );

  const deleteGroup = useCallback(
    async (id: string) => {
      const response = await fetch(`/api/groups/${encodeURIComponent(id)}`, {
        method: "DELETE",
      });
      if (!response.ok) {
        throw await parseAPIError(response, `failed to delete group (${response.status})`);
      }
      refreshData();
    },
    [refreshData],
  );

  const renameGroup = useCallback(
    async (id: string, name: string) => {
      const response = await fetch(`/api/groups/${encodeURIComponent(id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });
      if (!response.ok) {
        throw await parseAPIError(response, `failed to rename group (${response.status})`);
      }
      refreshData();
    },
    [refreshData],
  );

  const moveAsset = useCallback(
    async (assetID: string, groupID: string | null) => {
      const response = await fetch(`/api/assets/${encodeURIComponent(assetID)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ group_id: groupID ?? "" }),
      });
      if (!response.ok) {
        throw await parseAPIError(response, `failed to move asset (${response.status})`);
      }
      refreshData();
    },
    [refreshData],
  );

  const moveGroup = useCallback(
    async (groupID: string, parentGroupID: string | null) => {
      const response = await fetch(`/api/groups/${encodeURIComponent(groupID)}/move`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ parent_group_id: parentGroupID ?? "" }),
      });
      if (!response.ok) {
        throw await parseAPIError(response, `failed to move group (${response.status})`);
      }
      refreshData();
    },
    [refreshData],
  );

  return {
    isManaging,
    setIsManaging,
    createGroup,
    deleteGroup,
    renameGroup,
    moveAsset,
    moveGroup,
  };
}
