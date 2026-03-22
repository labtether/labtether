"use client";

import { useEffect, useEffectEvent, useRef, useState } from "react";

import type { DesktopProtocol, SessionConnectionState } from "./useSession";

interface UseProtocolSwitchOptions {
  protocol: DesktopProtocol;
  availableProtocols: DesktopProtocol[];
  connectionState: SessionConnectionState;
  networkQuality: "good" | "fair" | "poor" | null;
  webrtcRouteClass?: "direct" | "reflexive" | "relay" | null;
  allowAutomaticRecovery?: boolean;
  allowFallbackToVNC?: boolean;
  onSwitch: (nextProtocol: DesktopProtocol) => Promise<void> | void;
}

const POOR_WEBRTC_SWITCH_DELAY_MS = 12_000;
const RELAY_DEGRADED_SWITCH_DELAY_MS = 6_000;
const VNC_RECOVERY_TO_WEBRTC_DELAY_MS = 15_000;
const AUTO_SWITCH_NOTICE_MS = 8_000;

export function useProtocolSwitch({
  protocol,
  availableProtocols,
  connectionState,
  networkQuality,
  webrtcRouteClass = null,
  allowAutomaticRecovery = false,
  allowFallbackToVNC = true,
  onSwitch,
}: UseProtocolSwitchOptions) {
  const [notice, setNotice] = useState<string | null>(null);
  const switchingRef = useRef(false);
  const awaitingFallbackCompletionRef = useRef(false);
  const awaitingRecoveryCompletionRef = useRef(false);
  const autoFallbackActiveRef = useRef(false);
  const runSwitch = useEffectEvent(async (nextProtocol: DesktopProtocol) => {
    await Promise.resolve(onSwitch(nextProtocol));
  });

  useEffect(() => {
    if (!notice) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      setNotice(null);
    }, AUTO_SWITCH_NOTICE_MS);
    return () => {
      window.clearTimeout(timer);
    };
  }, [notice]);

  useEffect(() => {
    if (
      !awaitingFallbackCompletionRef.current &&
      !awaitingRecoveryCompletionRef.current
    ) {
      return;
    }

    if (
      awaitingFallbackCompletionRef.current &&
      protocol === "vnc" &&
      connectionState === "connected"
    ) {
      awaitingFallbackCompletionRef.current = false;
      switchingRef.current = false;
      autoFallbackActiveRef.current = true;
      setNotice(
        "WebRTC quality stayed poor, and LabTether switched back to VNC.",
      );
      return;
    }

    if (
      awaitingRecoveryCompletionRef.current &&
      protocol === "webrtc" &&
      connectionState === "connected"
    ) {
      awaitingRecoveryCompletionRef.current = false;
      switchingRef.current = false;
      autoFallbackActiveRef.current = false;
      setNotice("Connection recovered, and LabTether restored WebRTC.");
      return;
    }

    if (connectionState === "error") {
      const failedFallback = awaitingFallbackCompletionRef.current;
      const failedRecovery = awaitingRecoveryCompletionRef.current;
      awaitingFallbackCompletionRef.current = false;
      awaitingRecoveryCompletionRef.current = false;
      switchingRef.current = false;
      if (failedFallback) {
        setNotice(
          "WebRTC quality stayed poor, but the automatic fallback to VNC failed.",
        );
      } else if (failedRecovery) {
        setNotice(
          "Connection recovered, but the automatic return to WebRTC failed.",
        );
      }
    }
  }, [connectionState, protocol]);

  useEffect(() => {
    if (
      protocol !== "webrtc" ||
      connectionState !== "connected" ||
      !(
        networkQuality === "poor" ||
        (webrtcRouteClass === "relay" && networkQuality === "fair")
      ) ||
      !allowFallbackToVNC ||
      !availableProtocols.includes("vnc") ||
      switchingRef.current
    ) {
      return undefined;
    }

    const timer = window.setTimeout(() => {
      if (switchingRef.current) {
        return;
      }
      switchingRef.current = true;
      setNotice(
        webrtcRouteClass === "relay"
          ? "Relayed WebRTC quality stayed degraded, so LabTether is switching back to VNC."
          : "WebRTC quality stayed poor, so LabTether is switching back to VNC.",
      );
      void (async () => {
        try {
          await runSwitch("vnc");
          awaitingFallbackCompletionRef.current = true;
        } catch {
          awaitingFallbackCompletionRef.current = false;
          switchingRef.current = false;
          setNotice(
            "WebRTC quality stayed poor, but the automatic fallback to VNC failed.",
          );
        }
      })();
    }, webrtcRouteClass === "relay"
      ? RELAY_DEGRADED_SWITCH_DELAY_MS
      : POOR_WEBRTC_SWITCH_DELAY_MS);

    return () => {
      window.clearTimeout(timer);
    };
  }, [
    availableProtocols,
    allowFallbackToVNC,
    connectionState,
    networkQuality,
    protocol,
    webrtcRouteClass,
  ]);

  useEffect(() => {
    if (
      protocol !== "vnc" ||
      connectionState !== "connected" ||
      networkQuality !== "good" ||
      !allowAutomaticRecovery ||
      !autoFallbackActiveRef.current ||
      !availableProtocols.includes("webrtc") ||
      switchingRef.current
    ) {
      return undefined;
    }

    const timer = window.setTimeout(() => {
      if (switchingRef.current) {
        return;
      }
      switchingRef.current = true;
      setNotice("Connection recovered, so LabTether is switching back to WebRTC.");
      void (async () => {
        try {
          await runSwitch("webrtc");
          awaitingRecoveryCompletionRef.current = true;
        } catch {
          awaitingRecoveryCompletionRef.current = false;
          switchingRef.current = false;
          setNotice(
            "Connection recovered, but the automatic return to WebRTC failed.",
          );
        }
      })();
    }, VNC_RECOVERY_TO_WEBRTC_DELAY_MS);

    return () => {
      window.clearTimeout(timer);
    };
  }, [
    allowAutomaticRecovery,
    availableProtocols,
    connectionState,
    networkQuality,
    protocol,
  ]);

  const clearNotice = () => {
    awaitingFallbackCompletionRef.current = false;
    awaitingRecoveryCompletionRef.current = false;
    switchingRef.current = false;
    setNotice(null);
  };

  return { notice, clearNotice };
}
