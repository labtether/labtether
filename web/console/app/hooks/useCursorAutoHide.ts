"use client";

import { useEffect, useRef, useState } from "react";

const IDLE_MS = 3000;

export function useCursorAutoHide(
  elementRef: React.RefObject<HTMLElement | null>,
  enabled: boolean,
): { hidden: boolean } {
  const [hidden, setHidden] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    const el = elementRef.current;
    if (!enabled || !el) {
      // Disabled or no element — ensure cursor is restored.
      if (el) {
        el.style.cursor = "";
      }
      setHidden(false);
      return;
    }

    function clearTimer() {
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    }

    function showCursor() {
      clearTimer();
      el!.style.cursor = "";
      setHidden(false);
      timerRef.current = setTimeout(() => {
        el!.style.cursor = "none";
        setHidden(true);
      }, IDLE_MS);
    }

    // Start the idle timer immediately on enable.
    showCursor();

    el.addEventListener("mousemove", showCursor);
    el.addEventListener("mousedown", showCursor);
    el.addEventListener("keydown", showCursor);

    return () => {
      clearTimer();
      el.removeEventListener("mousemove", showCursor);
      el.removeEventListener("mousedown", showCursor);
      el.removeEventListener("keydown", showCursor);
      // Restore cursor on cleanup.
      el.style.cursor = "";
      setHidden(false);
    };
  }, [elementRef, enabled]);

  return { hidden };
}
