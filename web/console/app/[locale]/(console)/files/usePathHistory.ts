"use client";

import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Back for a folder view: the folders opened before this one, per tab (scope). Only settled folders count (ready:
 * nothing is loading), so a tab's first listing and a switch between tabs add no step; going back adds none either.
 */
export function usePathHistory(scope: string, currentPath: string, ready: boolean) {
  const [stack, setStack] = useState<string[]>([]);
  const last = useRef<{ scope: string; path: string } | null>(null);
  const goingBack = useRef(false);

  useEffect(() => {
    if (!ready) return;
    const prev = last.current;
    last.current = { scope, path: currentPath };
    if (!prev || prev.scope !== scope) {
      setStack([]);
      goingBack.current = false;
      return;
    }
    if (prev.path === currentPath) return;
    if (goingBack.current) {
      goingBack.current = false;
      return;
    }
    setStack((s) => [...s.slice(-49), prev.path]);
  }, [scope, currentPath, ready]);

  /** The folder to go back to (removed from the history), or null. */
  const back = useCallback((): string | null => {
    const prev = stack[stack.length - 1];
    if (prev === undefined) return null;
    goingBack.current = true;
    setStack(stack.slice(0, -1));
    return prev;
  }, [stack]);

  return { canGoBack: stack.length > 0, back };
}
