"use client";

import { useMemo, useRef } from "react";

/**
 * Keeps track of the session password a Remote View page sends by itself. The Hub makes that password for a VNC
 * server the device's agent started; a VNC server the person runs has its own. When the device refuses it, the page
 * connects again and asks the person, and does not send it again in this view (each try is a failed login there).
 */
export function useSessionPasswordAttempt() {
  const pendingRef = useRef(false);
  const refusedRef = useRef(false);
  return useMemo(
    () => ({
      /** Whether a password request may be answered with the session password. */
      allowed: () => !refusedRef.current,
      /** The session password was sent; the screen has not appeared yet. */
      sent: () => {
        pendingRef.current = true;
      },
      /** The screen appeared, or a new attempt starts: nothing is waiting on the device's answer. */
      settled: () => {
        pendingRef.current = false;
      },
      /**
       * The viewer failed. True when that was the device refusing the session password just sent: the caller
       * connects again, and the next password request goes to the person.
       */
      takeRefusal: () => {
        if (!pendingRef.current) return false;
        pendingRef.current = false;
        refusedRef.current = true;
        return true;
      },
    }),
    [],
  );
}

export type SessionPasswordAttempt = ReturnType<typeof useSessionPasswordAttempt>;
