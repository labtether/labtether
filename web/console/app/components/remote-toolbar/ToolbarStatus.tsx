"use client";

import { useTranslations } from "next-intl";
import type { RemoteViewToolbarProps } from "./types";

/**
 * The toolbar's one line: how the screen is reached and its latency, then what needs saying (a poor connection,
 * recording, a clipboard sync that just finished). pill: the full-screen pill also names the screen.
 */
export function ToolbarStatus({
  transportLabel,
  latencyMs,
  networkQuality,
  recording = false,
  targetLabel,
  clipboardFlash,
  pill,
}: Pick<RemoteViewToolbarProps, "transportLabel" | "latencyMs" | "networkQuality" | "recording" | "targetLabel"> & {
  clipboardFlash: "success" | "error" | null;
  pill: boolean;
}) {
  const t = useTranslations("desktop.tools");
  const parts = [
    pill && targetLabel ? targetLabel : "",
    transportLabel ? t("connectedVia", { transport: transportLabel }) : t("connected"),
    latencyMs != null ? t("latency", { ms: latencyMs }) : "",
  ].filter(Boolean);
  return (
    <span className="rv-status" aria-live="polite">
      <span className="rv-status-main">{parts.join(" · ")}</span>
      {networkQuality === "fair" || networkQuality === "poor" ? (
        <span className="rv-status-warn">{t(networkQuality === "poor" ? "poorConnection" : "fairConnection")}</span>
      ) : null}
      {recording ? (
        <span className="rv-rec">
          <i aria-hidden="true" />
          {t("recording")}
        </span>
      ) : null}
      {clipboardFlash ? (
        <span className={clipboardFlash === "error" ? "rv-status-warn" : "rv-status-note"}>
          {t(clipboardFlash === "error" ? "clipboardFailed" : "clipboardSynced")}
        </span>
      ) : null}
    </span>
  );
}
