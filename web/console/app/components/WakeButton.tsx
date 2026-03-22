"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "./ui/Button";

type WakeState = "idle" | "sending" | "sent" | "error";

interface WakeButtonProps {
  assetId: string;
  onWoken?: () => void;
}

export default function WakeButton({ assetId, onWoken }: WakeButtonProps) {
  const [state, setState] = useState<WakeState>("idle");
  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const stopTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const errorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearTimers = () => {
    if (pollTimerRef.current) {
      clearInterval(pollTimerRef.current);
      pollTimerRef.current = null;
    }
    if (stopTimerRef.current) {
      clearTimeout(stopTimerRef.current);
      stopTimerRef.current = null;
    }
    if (errorTimerRef.current) {
      clearTimeout(errorTimerRef.current);
      errorTimerRef.current = null;
    }
  };

  useEffect(() => () => {
    clearTimers();
  }, []);

  const startPoll = useCallback(() => {
    clearTimers();
    pollTimerRef.current = setInterval(async () => {
      try {
        const response = await fetch("/api/status/live", { cache: "no-store" });
        if (!response.ok) return;
        const payload = (await response.json()) as { assets?: Array<{ id: string; status: string }> };
        const asset = payload.assets?.find((entry) => entry.id === assetId);
        if (asset?.status === "online") {
          clearTimers();
          setState("idle");
          onWoken?.();
        }
      } catch {
        // ignore transient poll failures
      }
    }, 3000);

    stopTimerRef.current = setTimeout(() => {
      clearTimers();
      setState("idle");
    }, 120_000);
  }, [assetId, onWoken]);

  const handleWake = useCallback(async () => {
    setState("sending");
    try {
      const response = await fetch(`/api/wol/${encodeURIComponent(assetId)}`, {
        method: "POST",
      });
      if (!response.ok) {
        throw new Error(`wake request failed (${response.status})`);
      }
      setState("sent");
      startPoll();
    } catch {
      setState("error");
      errorTimerRef.current = setTimeout(() => {
        setState("idle");
        errorTimerRef.current = null;
      }, 3000);
    }
  }, [assetId, startPoll]);

  return (
    <Button
      variant={state === "error" ? "danger" : "secondary"}
      size="sm"
      onClick={handleWake}
      disabled={state === "sending" || state === "sent"}
    >
      {state === "idle" && "Wake Node"}
      {state === "sending" && "Sending..."}
      {state === "sent" && "Packet Sent"}
      {state === "error" && "Wake Failed"}
    </Button>
  );
}
