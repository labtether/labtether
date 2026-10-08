"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useFastStatus, useStatusSettings } from "../contexts/StatusContext";
import { useConnectedAgents } from "./useConnectedAgents";
import { useSessionProgress } from "./session/useSessionProgress";
import { createSession, requestSpiceTicket, requestStreamTicket } from "./session/sessionRequests";
import type { DesktopProtocol, SpiceTicket, SessionConnectionState, SessionStreamStatus, UseSessionOptions, TerminalConnectOptions } from "./session/sessionTypes";
export type * from "./session/sessionTypes";

function isNonRetryableDisconnectReason(reason: string): boolean {
  const normalized = reason.trim().toLowerCase();
  if (!normalized) {
    return false;
  }
  return (
    normalized.includes("auth") ||
    normalized.includes("credential") ||
    normalized.includes("password")
  );
}

export function useSession({
  type,
  fixedTarget,
  autoReconnect = false,
  quickConnectParams,
}: UseSessionOptions) {
  const status = useFastStatus();
  const { defaultActorID } = useStatusSettings();
  const { connectedAgentIds, refreshConnected } = useConnectedAgents();
  const [target, setTargetRaw] = useState(fixedTarget ?? "");
  const [connectionState, setConnectionState] =
    useState<SessionConnectionState>("idle");
  const [wsUrl, setWsUrl] = useState<string | null>(null);
  const [audioWsUrl, setAudioWsUrl] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [activeSessionId, setActiveSessionId] = useState("");
  const [quality, setQuality] = useState("medium");
  const [spiceTicket, setSpiceTicket] = useState<SpiceTicket | null>(null);
  const [vncPassword, setVncPassword] = useState<string | null>(null);
  const [isReconnecting, setIsReconnecting] = useState(false);
  const [reconnectExhausted, setReconnectExhausted] = useState(false);
  const [reconnectAttempt, setReconnectAttempt] = useState(0);
  const maxReconnectAttempts = 5;
  const { setProgress, updateProgressMessage, connectionProgress } = useSessionProgress();

  const sessionRef = useRef<{ id: string; target: string } | null>(null);
  const connectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const manualDisconnectRef = useRef(false);
  const lastDesktopOptionsRef = useRef<{
    protocol: DesktopProtocol;
    display: string;
    record: boolean;
    directTarget?: TerminalConnectOptions["directTarget"];
  }>({
    protocol: "vnc",
    display: "",
    record: false,
  });

  const isFixedTarget = fixedTarget != null;
  const assets = status?.assets ?? [];

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const terminateDesktopSession = useCallback(
    (sessionID: string | undefined) => {
      const normalizedSessionID = sessionID?.trim() ?? "";
      if (type !== "desktop" || !normalizedSessionID) return;
      void fetch(
        `/api/desktop/session/${encodeURIComponent(normalizedSessionID)}`,
        {
          method: "DELETE",
          cache: "no-store",
          keepalive: true,
        },
      ).catch(() => {
        // Session expiry remains the backend safety net if teardown is unavailable.
      });
    },
    [type],
  );

  const setTarget = useCallback(
    (value: string) => {
      if (!isFixedTarget) setTargetRaw(value);
    },
    [isFixedTarget],
  );

  const connect = useCallback(
    async (targetOverride?: string, options?: TerminalConnectOptions) => {
      const attemptID = ++connectAttemptRef.current;
      const connectTarget = (targetOverride ?? fixedTarget ?? target).trim();
      if (!connectTarget) {
        setError("Select a device to connect to.");
        return;
      }
      const terminalShell =
        type === "terminal" ? (options?.terminalShell?.trim() ?? "") : "";
      const remembered = lastDesktopOptionsRef.current;
      const protocol: DesktopProtocol =
        type === "desktop" ? (options?.protocol ?? remembered.protocol) : "vnc";
      const display =
        type === "desktop"
          ? (options?.display?.trim() ?? remembered.display)
          : "";
      const record =
        type === "desktop" ? (options?.record ?? remembered.record) : false;
      const directTarget =
        type === "desktop"
          ? (options?.directTarget ?? remembered.directTarget)
          : undefined;
      if (type === "desktop") {
        lastDesktopOptionsRef.current = { protocol, display, record, directTarget };
      }

      const isLatestAttempt = () => connectAttemptRef.current === attemptID;

      // Track reconnection
      const reconnecting = !!(
        sessionRef.current && sessionRef.current.target === connectTarget
      );
      if (type === "desktop" && sessionRef.current) {
        terminateDesktopSession(sessionRef.current.id);
        sessionRef.current = null;
        setActiveSessionId("");
      }
      setIsReconnecting(reconnecting);
      setReconnectExhausted(false);
      manualDisconnectRef.current = false;
      clearReconnectTimer();

      setConnectionState("connecting");
      setError(null);
      setWsUrl(null);
      setAudioWsUrl(null);
      setSpiceTicket(null);
      setVncPassword(null);
      setProgress(
        "creating-session",
        reconnecting ? "Reconnecting session..." : "Creating session...",
      );

      let sessionId = "";
      try {
        sessionId = await createSession({
          type, connectTarget, existingSession: sessionRef.current, quickConnectParams,
          defaultActorID, quality, protocol, display, record, directTarget,
        });

        if (!isLatestAttempt()) {
          terminateDesktopSession(sessionId);
          return;
        }
        sessionRef.current = { id: sessionId, target: connectTarget };
        setActiveSessionId(sessionId);

        if (type === "desktop" && protocol === "spice") {
          setProgress("requesting-ticket", "Requesting SPICE ticket...");
          const ticket = await requestSpiceTicket(sessionId);
          if (!isLatestAttempt()) {
            terminateDesktopSession(sessionId);
            return;
          }
          setSpiceTicket(ticket);
          setConnectionState("connected");
          setProgress("connected", "Connected");
          return;
        }

        // Get stream ticket
        setProgress(
          "requesting-ticket",
          type === "terminal"
            ? "Requesting terminal stream ticket..."
            : "Requesting desktop stream ticket...",
        );
        const ticket = await requestStreamTicket({ type, sessionId, terminalShell });
        if (!isLatestAttempt()) {
          terminateDesktopSession(sessionId);
          return;
        }
        setVncPassword(ticket.vncPassword);
        setWsUrl(ticket.wsUrl);
        setAudioWsUrl(ticket.audioWsUrl);
        setConnectionState(
          type === "desktop" ? "authenticating" : "connecting",
        );
        setProgress("opening-stream", "Opening secure stream...");
      } catch (err) {
        terminateDesktopSession(sessionId);
        if (sessionRef.current?.id === sessionId) {
          sessionRef.current = null;
          setActiveSessionId("");
        }
        if (!isLatestAttempt()) {
          return;
        }
        const message =
          err instanceof Error ? err.message : "Connection failed";
        setError(message);
        setConnectionState("error");
        setWsUrl(null);
        setAudioWsUrl(null);
        setProgress("error", message);
      } finally {
        if (isLatestAttempt()) {
          setIsReconnecting(false);
        }
      }
    },
    [
      target,
      fixedTarget,
      defaultActorID,
      quality,
      type,
      quickConnectParams,
      clearReconnectTimer,
      setProgress,
      terminateDesktopSession,
    ],
  );

  const disconnect = useCallback(() => {
    terminateDesktopSession(sessionRef.current?.id);
    manualDisconnectRef.current = true;
    clearReconnectTimer();
    reconnectAttemptRef.current = 0;
    setReconnectAttempt(0);
    setIsReconnecting(false);
    setReconnectExhausted(false);
    connectAttemptRef.current += 1;
    setWsUrl(null);
    setAudioWsUrl(null);
    setSpiceTicket(null);
    setVncPassword(null);
    setConnectionState("idle");
    setError(null);
    sessionRef.current = null;
    setActiveSessionId("");
    setProgress("idle", "Idle");
  }, [clearReconnectTimer, setProgress, terminateDesktopSession]);

  const handleConnected = useCallback(() => {
    clearReconnectTimer();
    setConnectionState(type === "terminal" ? "connecting" : "connected");
    setError(null);
    reconnectAttemptRef.current = 0;
    setReconnectAttempt(0);
    setIsReconnecting(false);
    setReconnectExhausted(false);
    if (type === "terminal") {
      setProgress("starting-shell", "Starting remote shell...");
      return;
    }
    setProgress("connected", "Connected");
  }, [clearReconnectTimer, setProgress, type]);

  const handleStreamReady = useCallback(
    (message?: string) => {
      clearReconnectTimer();
      setConnectionState("connected");
      setError(null);
      reconnectAttemptRef.current = 0;
      setReconnectAttempt(0);
      setIsReconnecting(false);
      setReconnectExhausted(false);
      setProgress("connected", message?.trim() || "Connected");
    },
    [clearReconnectTimer, setProgress],
  );

  const handleStreamStatus = useCallback(
    (status: SessionStreamStatus) => {
      const typeLabel = (status.type ?? "").trim().toLowerCase();
      const stage = (status.stage ?? "").trim().toLowerCase();
      let message = (status.message ?? "").trim();

      // Enrich message with hop progress when available
      if (
        status.hop_index != null &&
        status.hop_count != null &&
        status.hop_count > 0
      ) {
        const hopLabel = status.hop_host
          ? `hop ${status.hop_index + 1}/${status.hop_count} (${status.hop_host})`
          : `hop ${status.hop_index + 1}/${status.hop_count}`;
        if (!message) {
          message = `Connecting through ${hopLabel}...`;
        } else if (!message.toLowerCase().includes("hop")) {
          message = `${message} [${hopLabel}]`;
        }
      }

      if (typeLabel === "ready" || stage === "connected") {
        handleStreamReady(message);
        return;
      }
      if (typeLabel === "error") {
        const rendered = message || "Connection failed";
        setError(rendered);
        setConnectionState("error");
        setProgress("error", rendered);
        return;
      }

      if (stage.includes("shell")) {
        setProgress("starting-shell", message || "Starting remote shell...");
        return;
      }
      if (stage.includes("connect")) {
        setProgress("opening-stream", message || "Opening secure stream...");
        return;
      }
      if (message) {
        updateProgressMessage(message);
      }
    },
    [handleStreamReady, setProgress, updateProgressMessage],
  );

  const handleDisconnected = useCallback(
    (detail?: string | { clean: boolean; reason?: string }) => {
      if (manualDisconnectRef.current) {
        manualDisconnectRef.current = false;
        setConnectionState("idle");
        setIsReconnecting(false);
        setProgress("idle", "Idle");
        return;
      }
      terminateDesktopSession(sessionRef.current?.id);
      const isClean = typeof detail === "object" && detail?.clean;
      const disconnectReason =
        typeof detail === "object" ? detail.reason?.trim() ?? "" : "";
      const supportsAutoReconnect = !!(
        sessionRef.current &&
        ((type === "desktop" && fixedTarget) ||
          (type === "terminal" && autoReconnect))
      );
      const shouldAutoReconnect =
        !isClean &&
        supportsAutoReconnect &&
        !isNonRetryableDisconnectReason(disconnectReason);

      // Auto-reconnect on non-clean disconnect when enabled for this session type.
      const canAutoReconnect =
        shouldAutoReconnect &&
        reconnectAttemptRef.current < maxReconnectAttempts;
      if (canAutoReconnect) {
        const attempt = reconnectAttemptRef.current;
        reconnectAttemptRef.current = attempt + 1;
        setReconnectAttempt(attempt + 1);
        const delay = Math.pow(2, attempt) * 1000; // 1s, 2s, 4s, 8s, 16s
        setIsReconnecting(true);
        setReconnectExhausted(false);
        setConnectionState("connecting");
        setProgress(
          "reconnecting",
          `Reconnecting (${attempt + 1}/${maxReconnectAttempts})...`,
        );
        clearReconnectTimer();
        reconnectTimerRef.current = setTimeout(() => {
          if (type === "terminal") {
            const reconnectTarget = sessionRef.current?.target;
            void connect(reconnectTarget);
            return;
          }
          void connect();
        }, delay);
        return;
      }

      if (shouldAutoReconnect) {
        clearReconnectTimer();
        setIsReconnecting(false);
        setReconnectExhausted(true);
        setReconnectAttempt(maxReconnectAttempts);
        setConnectionState("error");
        setProgress(
          "error",
          `Unable to reconnect after ${maxReconnectAttempts} attempts`,
        );
      } else {
        setIsReconnecting(false);
        setReconnectExhausted(false);
      }

      // Preserve terminal session identity after abnormal disconnects so reconnect
      // can request a fresh stream ticket for the same backend session.
      const shouldClearSession = type !== "terminal" || isClean;
      if (shouldClearSession) {
        sessionRef.current = null;
        setActiveSessionId("");
        setSpiceTicket(null);
        setVncPassword(null);
        setAudioWsUrl(null);
      }

      reconnectAttemptRef.current = 0;
      if (!shouldAutoReconnect) {
        setConnectionState("idle");
        setReconnectAttempt(0);
        setProgress("idle", "Idle");
      }
      if (typeof detail === "string") {
        if (detail) setError(detail);
      } else if (detail?.reason) {
        const reason = detail.reason.trim();
        if (!reason) {
          return;
        }
        // For desktop sessions, surface close reasons even when the close was
        // clean so operator-facing startup/permission failures are visible.
        const shouldSurfaceReason = !detail.clean || type === "desktop";
        if (
          shouldSurfaceReason &&
          reason.toLowerCase() !== "user disconnected"
        ) {
          setError(reason);
        }
      }
    },
    [
      type,
      fixedTarget,
      autoReconnect,
      connect,
      clearReconnectTimer,
      setProgress,
      terminateDesktopSession,
    ],
  );

  const handleError = useCallback(
    (message: string) => {
      terminateDesktopSession(sessionRef.current?.id);
      sessionRef.current = null;
      setActiveSessionId("");
      setError(message);
      setConnectionState("error");
      setVncPassword(null);
      setAudioWsUrl(null);
      setProgress("error", message.trim() || "Connection failed");
    },
    [setProgress, terminateDesktopSession],
  );

  useEffect(() => {
    return () => {
      clearReconnectTimer();
      terminateDesktopSession(sessionRef.current?.id);
    };
  }, [clearReconnectTimer, terminateDesktopSession]);

  return {
    target,
    setTarget,
    isFixedTarget,
    assets,
    connectedAgentIds,
    refreshConnected,
    connectionState,
    wsUrl,
    audioWsUrl,
    error,
    quality,
    setQuality,
    spiceTicket,
    vncPassword,
    activeSessionId,
    isReconnecting,
    reconnectExhausted,
    reconnectAttempt,
    maxReconnectAttempts,
    connectionProgress,
    connect,
    disconnect,
    handleConnected,
    handleStreamReady,
    handleStreamStatus,
    handleDisconnected,
    handleError,
  };
}
