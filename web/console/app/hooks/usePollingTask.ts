"use client";

import { useCallback, useEffect, useRef } from "react";

type PollTask = (signal: AbortSignal, manual: boolean) => Promise<void>;
type Request = { controller: AbortController; promise: Promise<void> };

// Passive page reads share one slot, even across target/visibility changes.
// Tasks pass the signal to fetch and check it before publishing their result.
export function usePollingTask(task: PollTask, intervalMs = 0, enabled = true) {
  const requestRef = useRef<Request | null>(null);
  const runRef = useRef<(() => Promise<void>) | null>(null);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    let needsRefresh = true;
    let pendingManual = false;
    let timer: ReturnType<typeof setInterval> | null = null;

    const run = async (manual = false): Promise<void> => {
      if (!active || document.visibilityState === "hidden") return;
      if (!manual && intervalMs <= 0 && !needsRefresh) return;
      const previous = requestRef.current;
      if (previous) {
        const replacing = previous.controller.signal.aborted;
        await previous.promise;
        // A replacement view waits for cancellation to settle before refreshing.
        if (replacing) return run(manual);
        return;
      }
      const controller = new AbortController();
      manual ||= pendingManual;
      needsRefresh = true;
      const request: Request = { controller, promise: Promise.resolve() };
      requestRef.current = request;
      request.promise = Promise.resolve()
        .then(() => { if (!controller.signal.aborted) return task(controller.signal, manual); })
        .catch(() => { /* Each task owns its user-facing error state. */ })
        .then(() => {
          if (!controller.signal.aborted) {
            needsRefresh = false;
            if (manual) pendingManual = false;
          }
        })
        .finally(() => {
          // Also stop any sibling fetch after one member of a fanout fails.
          controller.abort();
          if (requestRef.current === request) requestRef.current = null;
        });
      await request.promise;
    };

    const stop = () => {
      if (timer !== null) clearInterval(timer);
      timer = null;
      requestRef.current?.controller.abort();
    };
    const update = () => {
      stop();
      if (document.visibilityState === "hidden") return;
      void run();
      if (intervalMs > 0) {
        timer = setInterval(() => {
          if (!requestRef.current) void run();
        }, intervalMs);
      }
    };
    runRef.current = () => {
      // A user refresh (including one after an action) must read fresh data.
      needsRefresh = true;
      pendingManual = true;
      requestRef.current?.controller.abort();
      return run(true);
    };
    update();
    document.addEventListener("visibilitychange", update);
    return () => {
      active = false;
      runRef.current = null;
      document.removeEventListener("visibilitychange", update);
      stop();
    };
  }, [enabled, intervalMs, task]);

  return useCallback(() => runRef.current?.() ?? Promise.resolve(), []);
}
