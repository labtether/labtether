"use client";

import { useSyncExternalStore } from "react";

function computeLabel(lastSeenAt: string): string {
  const diffMs = Date.now() - new Date(lastSeenAt).getTime();
  if (!Number.isFinite(diffMs) || diffMs < 0) return "n/a";

  const seconds = Math.floor(diffMs / 1000);
  if (seconds < 60) return `${seconds}s ago`;

  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

// ---------------------------------------------------------------------------
// Shared tick singleton — one timer for every FreshnessDisplay on the page.
// Ticks every 10 seconds while the page is visible; pauses when hidden.
// ---------------------------------------------------------------------------

type Listener = () => void;

const listeners = new Set<Listener>();
let timerId: ReturnType<typeof setInterval> | null = null;
let tick = 0; // monotonically increasing; used as the store snapshot

function getSnapshot(): number {
  return tick;
}

function getServerSnapshot(): number {
  return 0;
}

function notifyListeners(): void {
  tick += 1;
  listeners.forEach((l) => l());
}

function startTimer(): void {
  if (timerId !== null) return;
  timerId = setInterval(notifyListeners, 10_000);
}

function stopTimer(): void {
  if (timerId === null) return;
  clearInterval(timerId);
  timerId = null;
}

function handleVisibilityChange(): void {
  if (document.visibilityState === "hidden") {
    stopTimer();
  } else {
    // Page became visible — fire immediately so labels are fresh, then resume.
    notifyListeners();
    startTimer();
  }
}

function subscribe(listener: Listener): () => void {
  if (listeners.size === 0) {
    // First subscriber: attach the visibility listener and start the timer.
    document.addEventListener("visibilitychange", handleVisibilityChange);
    if (document.visibilityState !== "hidden") {
      startTimer();
    }
  }

  listeners.add(listener);

  return () => {
    listeners.delete(listener);

    if (listeners.size === 0) {
      // Last subscriber unsubscribed: clean everything up.
      stopTimer();
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    }
  };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function FreshnessDisplay({ lastSeenAt }: { lastSeenAt: string }) {
  // Re-render whenever the shared tick increments; the tick value itself is
  // unused — it just triggers React to call computeLabel again.
  useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

  return (
    <span className="text-xs tabular-nums text-[var(--muted)]">
      {computeLabel(lastSeenAt)}
    </span>
  );
}
