"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Group, MaintenanceWindow } from "../../../console/models";
import { ensureArray, ensureRecord, ensureString } from "../../../lib/responseGuards";

export type NewDowntime = {
  name: string;
  start_at: string;
  end_at: string;
  suppress_alerts: boolean;
  block_actions: boolean;
  block_updates: boolean;
};

const base = (groupId: string) => `/api/groups/${encodeURIComponent(groupId)}/maintenance-windows`;

async function send(url: string, init: RequestInit): Promise<Record<string, unknown> | null> {
  const response = await fetch(url, init);
  const payload = ensureRecord(await response.json().catch(() => null));
  if (!response.ok) throw new Error(ensureString(payload?.error) || `failed (${response.status})`);
  return payload;
}

/**
 * Downtime windows of every group, in one list. The hub keeps them per group, so this asks each group once and
 * merges the answers; a group that fails to answer leaves `partial` set and the rest still show.
 */
export function useDowntimeWindows(groups: Group[]) {
  const ids = useMemo(() => JSON.stringify(groups.map((g) => g.id)), [groups]);
  const [windows, setWindows] = useState<MaintenanceWindow[]>([]);
  const [loading, setLoading] = useState(false);
  const [partial, setPartial] = useState(false);

  const load = useCallback(async () => {
    const list = JSON.parse(ids) as string[];
    if (list.length === 0) {
      setWindows([]);
      setPartial(false);
      return;
    }
    setLoading(true);
    const answers = await Promise.allSettled(
      list.map(async (id) => {
        const payload = await send(`${base(id)}?limit=20`, { cache: "no-store" });
        return ensureArray<MaintenanceWindow>(payload?.windows).map((w) => ({ ...w, group_id: w.group_id || id }));
      }),
    );
    setWindows(answers.flatMap((a) => (a.status === "fulfilled" ? a.value : [])));
    setPartial(answers.some((a) => a.status === "rejected"));
    setLoading(false);
  }, [ids]);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (groupId: string, body: NewDowntime) => {
    await send(base(groupId), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    await load();
  }, [load]);

  const remove = useCallback(async (window: MaintenanceWindow) => {
    await send(`${base(window.group_id)}/${encodeURIComponent(window.id)}`, { method: "DELETE" });
    await load();
  }, [load]);

  return { windows, loading, partial, create, remove, reload: load };
}
