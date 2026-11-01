"use client";

import { useTranslations } from "next-intl";
import type { ReconnectState } from "../types/viewer";

interface ReconnectOverlayProps {
  state: ReconnectState;
  onReconnectNow: () => void;
}

/** A lost connection, over the dimmed screen: what happened and how far the retries are, and one button. */
export default function ReconnectOverlay({ state, onReconnectNow }: ReconnectOverlayProps) {
  const t = useTranslations("desktop.screen");
  if (!state.active) return null;

  const exhausted = state.attempt >= state.maxAttempts;

  return (
    <div className="rv-over dim" role="status">
      <div className="rv-card">
        <p className="rv-st">
          <i className="rv-dot bad" aria-hidden="true" />
          {exhausted ? t("lostAfter", { max: state.maxAttempts }) : t("lost")}
        </p>
        {exhausted ? null : (
          <p className="rv-card-sub">{t("reconnecting", { attempt: state.attempt, max: state.maxAttempts })}</p>
        )}
        <div className="rv-card-foot">
          <button type="button" className={`rv-btn${exhausted ? " primary" : ""}`} onClick={onReconnectNow}>
            {t(exhausted ? "tryAgain" : "reconnectNow")}
          </button>
        </div>
      </div>
    </div>
  );
}
