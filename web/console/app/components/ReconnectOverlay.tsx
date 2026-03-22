"use client";

import { ReconnectState } from "../types/viewer";

interface ReconnectOverlayProps {
  state: ReconnectState;
  onReconnectNow: () => void;
}

export default function ReconnectOverlay({ state, onReconnectNow }: ReconnectOverlayProps) {
  if (!state.active) return null;

  const exhausted = state.attempt >= state.maxAttempts;

  return (
    <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 z-45 bg-black/75 backdrop-blur-sm">
      {exhausted ? (
        <>
          <p className="text-sm font-medium" style={{ color: "var(--bad)" }}>
            Connection lost
          </p>
          <p className="text-xs text-white/60 text-center max-w-sm">
            Unable to reconnect after {state.maxAttempts} attempts
          </p>
          <button
            onClick={onReconnectNow}
            className="mt-1 px-4 py-1.5 rounded text-sm font-medium bg-[var(--accent)] text-[var(--accent-contrast)]"
          >
            Try Again
          </button>
        </>
      ) : (
        <>
          <div className="h-6 w-6 rounded-full border-2 border-white/20 border-t-[var(--accent)] animate-spin" />
          <p className="text-sm font-medium text-white">
            Reconnecting (attempt {state.attempt}/{state.maxAttempts})...
          </p>
          <button
            onClick={onReconnectNow}
            className="mt-1 px-4 py-1.5 rounded text-sm font-medium bg-white/10 hover:bg-white/20 text-white"
          >
            Reconnect Now
          </button>
        </>
      )}
    </div>
  );
}
