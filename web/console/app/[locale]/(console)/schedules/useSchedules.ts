"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchAllSchedules } from "../../../lib/schedules";
import { autoMutate } from "../actions/_parts/autoApi";
import type { Schedule, ScheduleInput } from "./scheduleModel";

const url = (id: string) => `/api/v2/schedules/${encodeURIComponent(id)}`;

/** Every schedule from the hub, and the writes the page makes. Writes throw with the hub's sentence and reload after. */
export function useSchedules() {
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);

  const load = useCallback(async () => {
    setFailed(false);
    try {
      setSchedules((await fetchAllSchedules<Schedule>()).data);
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const create = useCallback(async (input: ScheduleInput) => {
    const { group_id: group, ...rest } = input;
    await autoMutate("/api/v2/schedules", "POST", "", group ? { ...rest, group_id: group } : rest);
    await load();
  }, [load]);

  const update = useCallback(async (id: string, input: ScheduleInput) => {
    await autoMutate(url(id), "PATCH", "", input);
    await load();
  }, [load]);

  const remove = useCallback(async (id: string) => {
    await autoMutate(url(id), "DELETE", "");
    setSchedules((list) => list.filter((s) => s.id !== id));
    await load();
  }, [load]);

  /** Turns a schedule on or off at once in the list; puts it back when the hub refuses. */
  const setEnabled = useCallback(async (schedule: Schedule, enabled: boolean) => {
    setSchedules((list) => list.map((s) => (s.id === schedule.id ? { ...s, enabled } : s)));
    try {
      await autoMutate(url(schedule.id), "PATCH", "", { enabled });
    } catch (cause) {
      setSchedules((list) => list.map((s) => (s.id === schedule.id ? { ...s, enabled: schedule.enabled } : s)));
      throw cause;
    }
    await load();
  }, [load]);

  return { schedules, loading, failed, load, create, update, remove, setEnabled };
}
