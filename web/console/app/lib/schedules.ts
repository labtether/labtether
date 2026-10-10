import { apiFetch } from "./api";

type SchedulePage<T extends { id: string }> = {
  data?: T[];
  meta?: {
    total?: number;
    page?: number;
    per_page?: number;
  };
};

export type CompleteScheduleList<T extends { id: string }> = {
  data: T[];
  meta: { total: number; page: 1; per_page: number };
};

const PAGE_SIZE = 100;
const MAX_PAGES = 100;

/** Fetch every schedule definition, or fail instead of returning a partial list. */
export async function fetchAllSchedules<T extends { id: string }>(): Promise<CompleteScheduleList<T>> {
  const schedules: T[] = [];
  const seenIDs = new Set<string>();
  let expectedTotal: number | null = null;

  for (let page = 1; page <= MAX_PAGES; page++) {
    const { response, data } = await apiFetch<SchedulePage<T>>(
      `/api/v2/schedules?page=${page}&per_page=${PAGE_SIZE}`,
    );
    if (!response.ok) {
      throw new Error(`Failed to load schedules (page ${page}, HTTP ${response.status}).`);
    }

    const total = data?.meta?.total;
    const items = data?.data;
    if (
      !Array.isArray(items)
      || typeof total !== "number"
      || !Number.isSafeInteger(total)
      || total < 0
      || data?.meta?.page !== page
      || data?.meta?.per_page !== PAGE_SIZE
      || (expectedTotal !== null && total !== expectedTotal)
      || items.length !== Math.min(PAGE_SIZE, total - schedules.length)
    ) {
      throw new Error(`Incomplete schedules response (page ${page}).`);
    }

    for (const item of items) {
      if (typeof item?.id !== "string" || !item.id.trim() || seenIDs.has(item.id)) {
        throw new Error(`Incomplete schedules response (page ${page}).`);
      }
      seenIDs.add(item.id);
    }

    expectedTotal = total;
    schedules.push(...items);
    if (schedules.length === total) {
      return { data: schedules, meta: { total, page: 1, per_page: total } };
    }
  }

  throw new Error("Schedule list exceeds the supported page limit.");
}
