"use client";

import { useCallback } from "react";
import type { JumpChain } from "../../../console/models";

export type GroupStatus = "active" | "inactive" | "maintenance";

export type CreateGroupInput = {
  name: string;
  slug: string;
  parent_group_id?: string;
  icon?: string;
  sort_order?: number;
};

export type UpdateGroupInput = {
  name?: string;
  slug?: string;
  parent_group_id?: string;
  icon?: string;
  sort_order?: number;
  jump_chain?: JumpChain | null;
};

type ErrorPayload = { error?: string } | null;

async function parseGroupAPIError(response: Response, fallback: string): Promise<Error> {
  const payload = (await response.json().catch(() => null)) as ErrorPayload;
  return new Error(payload?.error || fallback);
}

export function parseGroupMutationError(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

export function useGroupMutationActions() {
  const createGroup = useCallback(async (input: CreateGroupInput): Promise<void> => {
    const response = await fetch("/api/groups", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    if (!response.ok) {
      throw await parseGroupAPIError(response, `failed to create group (${response.status})`);
    }
  }, []);

  const updateGroup = useCallback(async (groupID: string, patch: UpdateGroupInput): Promise<void> => {
    const response = await fetch(`/api/groups/${encodeURIComponent(groupID)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    });
    if (!response.ok) {
      throw await parseGroupAPIError(response, `failed to update group (${response.status})`);
    }
  }, []);

  const deleteGroup = useCallback(async (groupID: string): Promise<void> => {
    const response = await fetch(`/api/groups/${encodeURIComponent(groupID)}`, {
      method: "DELETE",
    });
    if (!response.ok) {
      throw await parseGroupAPIError(response, `failed to delete group (${response.status})`);
    }
  }, []);

  return {
    createGroup,
    updateGroup,
    deleteGroup,
  };
}
