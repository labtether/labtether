"use client";

import { Check } from "lucide-react";
import { useTranslations } from "next-intl";
import type { SessionConnectionPhase, SessionConnectionState } from "../../hooks/useSession";

const STEPS = ["session", "ticket", "stream"] as const;

/** Which connect step is running: 0 starting the session, 1 getting the stream ticket, 2 opening the stream. */
export function connectStep(state: SessionConnectionState, phase?: SessionConnectionPhase): number {
  if (state === "authenticating" || phase === "opening-stream") return 2;
  if (phase === "requesting-ticket") return 1;
  return 0;
}

/**
 * What shows over the screen while it is not live: the connect steps, why it failed, or that it is not connected,
 * each with at most one button. Connected and in view only, a small pill offers Take control. Nothing otherwise.
 * start: the idle screen's button reads Connect (nothing connected yet) or Reconnect (after a disconnect).
 */
export function ScreenOverlay({
  connectionState,
  phase,
  reconnecting = false,
  error,
  targetLabel,
  start,
  onStart,
  onCancel,
  viewOnly,
  onViewOnlyToggle,
}: {
  connectionState: SessionConnectionState;
  phase?: SessionConnectionPhase;
  reconnecting?: boolean;
  error: string | null;
  targetLabel?: string;
  start: "connect" | "reconnect";
  onStart: () => void;
  onCancel: () => void;
  viewOnly: boolean;
  onViewOnlyToggle: () => void;
}) {
  const t = useTranslations("desktop.screen");

  if (connectionState === "connecting" || connectionState === "authenticating") {
    const step = connectStep(connectionState, phase);
    return (
      <div className="rv-over" role="status">
        <div className="rv-steps-wrap">
          <ol className="rv-steps" aria-label={targetLabel ? t("connectingTo", { name: targetLabel }) : t("connecting")}>
            {STEPS.map((key, i) => (
              <li key={key} className={i < step ? "done" : i === step ? "now" : ""}>
                {i < step ? <Check aria-hidden="true" /> : <i className={`rv-dot ${i === step ? "busy" : "off"}`} aria-hidden="true" />}
                {t(key === "session" && (reconnecting || phase === "reconnecting") ? "stepReconnect" : `step.${key}`)}
              </li>
            ))}
          </ol>
          <button type="button" className="rv-link" onClick={onCancel}>
            {t("cancel")}
          </button>
        </div>
      </div>
    );
  }

  if (connectionState === "error" || connectionState === "idle") {
    const failed = connectionState === "error";
    const word = failed ? error || t("failed") : t(start === "connect" ? "notConnected" : "disconnected");
    return (
      <div className="rv-over" role={failed ? "alert" : undefined}>
        <div className="rv-card">
          <p className="rv-st">
            <i className={`rv-dot ${failed ? "bad" : "off"}`} aria-hidden="true" />
            <span>{targetLabel && !failed ? `${targetLabel} · ${word}` : word}</span>
          </p>
          <div className="rv-card-foot">
            <button type="button" className="rv-btn primary" onClick={onStart}>
              {t(failed ? "tryAgain" : start)}
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (viewOnly) {
    return (
      <div className="rv-pill">
        <b>{t("viewOnly")}</b>
        <button type="button" className="rv-btn sm" onClick={onViewOnlyToggle}>
          {t("takeControl")}
        </button>
      </div>
    );
  }
  return null;
}
