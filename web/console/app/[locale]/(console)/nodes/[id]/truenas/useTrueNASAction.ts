"use client";

import { useCallback, useRef, useState } from "react";

/**
 * One action at a time for a TrueNAS list: which row is busy, and the last failure. A later action replaces an
 * earlier one's result, so a slow reply never overwrites a newer state.
 */
export function useTrueNASAction() {
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<{ key: string; message: string } | null>(null);
  const seq = useRef(0);

  const run = useCallback(async (key: string, work: () => Promise<unknown>, done?: () => void) => {
    const id = ++seq.current;
    setError(null);
    setBusy(key);
    try {
      await work();
      if (seq.current === id) done?.();
    } catch (err) {
      if (seq.current === id) setError({ key, message: err instanceof Error ? err.message : "" });
    } finally {
      if (seq.current === id) setBusy(null);
    }
  }, []);

  return { busy, error, run, clearError: () => setError(null) };
}
