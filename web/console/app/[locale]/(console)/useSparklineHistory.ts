"use client";

import { useEffect, useRef } from "react";

const MAX_POINTS = 30;

/**
 * Accumulates a rolling window of up to MAX_POINTS numeric snapshots for a
 * given KPI metric value. The history is stored in a ref so appending new
 * points does not trigger additional re-renders — the component re-renders
 * only when `value` itself changes (i.e. when the poll delivers new data).
 *
 * Returns the current history array reference. Consumers should treat it as
 * read-only and only render from it during the current render cycle.
 */
export function useSparklineHistory(
  value: number | undefined | null
): Array<{ value: number }> {
  const history = useRef<Array<{ value: number }>>([]);

  useEffect(() => {
    if (value === undefined || value === null) return;
    const arr = history.current;
    arr.push({ value });
    if (arr.length > MAX_POINTS) {
      arr.shift();
    }
  }, [value]);

  return history.current;
}
