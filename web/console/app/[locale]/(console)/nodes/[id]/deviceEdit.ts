"use client";

import { useCallback, useState } from "react";
import type { Asset } from "../../../../console/models";

/** What a person can change on any device: its name, its group and its tags. */
export type DeviceEditDraft = { name: string; groupId: string; tags: string };

export const draftOf = (asset: Pick<Asset, "name" | "group_id" | "tags">): DeviceEditDraft => ({
  name: asset.name,
  groupId: asset.group_id ?? "",
  tags: (asset.tags ?? []).join(", "),
});

/**
 * The form after the Hub's values move from `base` to `next`: each field the person left as it was takes the new
 * value; a field they changed keeps their text.
 */
export function mergeDraft(current: DeviceEditDraft, base: DeviceEditDraft, next: DeviceEditDraft): DeviceEditDraft {
  const sameTags = parseTags(current.tags).join(",") === parseTags(base.tags).join(",");
  return {
    name: current.name.trim() === base.name.trim() ? next.name : current.name,
    groupId: current.groupId === base.groupId ? next.groupId : current.groupId,
    tags: sameTags ? next.tags : current.tags,
  };
}

/** "nas, backup,  nas" → ["nas", "backup"]: comma-separated, trimmed, each tag once. */
export function parseTags(text: string): string[] {
  return [...new Set(text.split(",").map((tag) => tag.trim()).filter(Boolean))];
}

/**
 * The fields a save sends (PATCH /api/assets/{id}): only what changed, a group of "" as null (no group). null when
 * nothing changed; "nameRequired" when the name is empty.
 */
export function deviceEditPatch(
  asset: Pick<Asset, "name" | "group_id" | "tags">,
  draft: DeviceEditDraft,
): Record<string, string | string[] | null> | null | "nameRequired" {
  const name = draft.name.trim();
  if (!name) return "nameRequired";
  const patch: Record<string, string | string[] | null> = {};
  if (name !== asset.name) patch.name = name;
  if (draft.groupId !== (asset.group_id ?? "")) patch.group_id = draft.groupId || null;
  const tags = parseTags(draft.tags);
  const before = asset.tags ?? [];
  if (tags.length !== before.length || tags.some((tag, i) => tag !== before[i])) patch.tags = tags;
  return Object.keys(patch).length ? patch : null;
}

/** The Hub's error text from a failed response, without status codes; "" when it sent none. */
async function errorText(res: Response): Promise<string> {
  const text = await res.text().catch(() => "");
  if (!text) return "";
  try {
    const body = JSON.parse(text) as { error?: string };
    return body.error ?? "";
  } catch {
    return text.length < 200 ? text : "";
  }
}

export type DeviceEditResult = { ok: true; changed: boolean } | { ok: false; error: "nameRequired" | "failed"; reason?: string };

/** Saves a device's name, group and tags; the caller shows the result in words. */
export function useDeviceEdit(asset: Pick<Asset, "id" | "name" | "group_id" | "tags">) {
  const [saving, setSaving] = useState(false);
  const save = useCallback(async (draft: DeviceEditDraft): Promise<DeviceEditResult> => {
    const patch = deviceEditPatch(asset, draft);
    if (patch === "nameRequired") return { ok: false, error: "nameRequired" };
    if (!patch) return { ok: true, changed: false };
    setSaving(true);
    try {
      const res = await fetch(`/api/assets/${encodeURIComponent(asset.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(patch),
      });
      if (!res.ok) return { ok: false, error: "failed", reason: await errorText(res) };
      return { ok: true, changed: true };
    } catch {
      return { ok: false, error: "failed" };
    } finally {
      setSaving(false);
    }
  }, [asset]);
  return { save, saving };
}
