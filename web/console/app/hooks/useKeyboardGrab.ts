"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardGrabState } from "../types/viewer";

// The Keyboard Lock API is not in standard TypeScript libs.
type KeyboardLockAPI = {
  lock: (keys?: string[]) => Promise<void>;
  unlock: () => void;
};

function getKeyboardAPI(): KeyboardLockAPI | undefined {
  return (navigator as { keyboard?: KeyboardLockAPI }).keyboard;
}

export function useKeyboardGrab() {
  const supported = typeof navigator !== "undefined" && "keyboard" in navigator;
  const [state, setState] = useState<KeyboardGrabState>(
    supported ? "off" : "unsupported",
  );

  const releaseCallbacksRef = useRef<Set<() => void>>(new Set());
  // Track which modifier keys are currently held.
  const modifiersRef = useRef({ ctrl: false, alt: false, shift: false });
  const isActiveRef = useRef(false);

  const deactivate = useCallback(() => {
    if (!isActiveRef.current) return;
    isActiveRef.current = false;

    const kb = getKeyboardAPI();
    kb?.unlock();

    if (document.pointerLockElement) {
      document.exitPointerLock();
    }

    setState("off");
  }, []);

  const activate = useCallback(
    async (_element?: Element) => {
      if (!supported) return;

      const kb = getKeyboardAPI();
      if (kb) {
        try {
          await kb.lock();
        } catch {
          // Lock may be rejected (e.g. document not focused). Proceed anyway —
          // the focused viewer and keydown listener still provide partial grab.
        }
      }

      isActiveRef.current = true;
      setState("active");
    },
    [supported],
  );

  const onRelease = useCallback((cb: () => void) => {
    releaseCallbacksRef.current.add(cb);
    return () => {
      releaseCallbacksRef.current.delete(cb);
    };
  }, []);

  // Monitor Ctrl+Alt+Shift release combo.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Control") modifiersRef.current.ctrl = true;
      if (e.key === "Alt") modifiersRef.current.alt = true;
      if (e.key === "Shift") modifiersRef.current.shift = true;

      if (
        isActiveRef.current &&
        modifiersRef.current.ctrl &&
        modifiersRef.current.alt &&
        modifiersRef.current.shift
      ) {
        deactivate();
        for (const cb of releaseCallbacksRef.current) {
          cb();
        }
      }
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.key === "Control") modifiersRef.current.ctrl = false;
      if (e.key === "Alt") modifiersRef.current.alt = false;
      if (e.key === "Shift") modifiersRef.current.shift = false;
    };

    // Reset modifiers if the window loses focus so we don't get stuck.
    const handleBlur = () => {
      modifiersRef.current = { ctrl: false, alt: false, shift: false };
    };

    window.addEventListener("keydown", handleKeyDown, { capture: true });
    window.addEventListener("keyup", handleKeyUp, { capture: true });
    window.addEventListener("blur", handleBlur);

    return () => {
      window.removeEventListener("keydown", handleKeyDown, { capture: true });
      window.removeEventListener("keyup", handleKeyUp, { capture: true });
      window.removeEventListener("blur", handleBlur);
    };
  }, [deactivate]);

  // Deactivate on unmount if grab is still active.
  useEffect(() => {
    return () => {
      if (isActiveRef.current) {
        deactivate();
      }
    };
  }, [deactivate]);

  return { state, supported, activate, deactivate, onRelease };
}
