"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { SessionConnectionPhase, SessionConnectionProgress } from "./sessionTypes";

export function useSessionProgress() {
  const [connectionPhase, setConnectionPhase] =
    useState<SessionConnectionPhase>("idle");
  const [connectionMessage, setConnectionMessage] = useState("Idle");
  const [progressNowMs, setProgressNowMs] = useState(() => Date.now());
  const connectStartedAtRef = useRef<number | null>(null);
  const phaseStartedAtRef = useRef<number | null>(null);
  const setProgress = useCallback(
    (phase: SessionConnectionPhase, message: string) => {
      const now = Date.now();
      if (phase === "idle") {
        connectStartedAtRef.current = null;
        phaseStartedAtRef.current = null;
      } else {
        if (connectStartedAtRef.current == null) {
          connectStartedAtRef.current = now;
        }
        phaseStartedAtRef.current = now;
      }
      setConnectionPhase(phase);
      setConnectionMessage(message);
      setProgressNowMs(now);
    },
    [],
  );

  const updateProgressMessage = useCallback((message: string) => {
    const trimmed = message.trim();
    if (!trimmed) return;
    setConnectionMessage(trimmed);
    setProgressNowMs(Date.now());
  }, []);

  useEffect(() => {
    if (
      connectionPhase === "idle" ||
      connectionPhase === "connected" ||
      connectionPhase === "error"
    ) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      setProgressNowMs(Date.now());
    }, 200);
    return () => {
      window.clearInterval(timer);
    };
  }, [connectionPhase]);

  const connectionProgress: SessionConnectionProgress = {
    phase: connectionPhase,
    message: connectionMessage,
    phaseElapsedMs: Math.max(
      0,
      progressNowMs - (phaseStartedAtRef.current ?? progressNowMs),
    ),
    totalElapsedMs: Math.max(
      0,
      progressNowMs - (connectStartedAtRef.current ?? progressNowMs),
    ),
  };

  return { setProgress, updateProgressMessage, connectionProgress };
}
