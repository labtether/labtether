"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useFastStatus, useStatusSettings } from "../contexts/StatusContext";
import { useConnectedAgents } from "./useConnectedAgents";
import { buildBrowserWsUrl } from "../lib/ws";

export type SessionType = "terminal" | "desktop";
export type DesktopProtocol = "vnc" | "rdp" | "spice" | "webrtc";

export interface SpiceTicket {
  wsUrl: string;
  password: string;
  type?: string;
  ca?: string;
  proxy?: string;
}

export type SessionConnectionState =
  | "idle"
  | "connecting"
  | "authenticating" // desktop only
  | "connected"
  | "error";

export type SessionConnectionPhase =
  | "idle"
  | "creating-session"
  | "requesting-ticket"
  | "opening-stream"
  | "starting-shell"
  | "reconnecting"
  | "connected"
  | "error";

export interface SessionConnectionProgress {
  phase: SessionConnectionPhase;
  message: string;
  phaseElapsedMs: number;
  totalElapsedMs: number;
}

export interface SessionStreamStatus {
  type?: string;
  stage?: string;
  message?: string;
  attempt?: number;
  attempts?: number;
  elapsed_ms?: number;
  hop_index?: number;
  hop_count?: number;
  hop_host?: string;
}

export interface QuickConnectParams {
  host: string;
  port?: number;
  username: string;
  auth_method: "password" | "private_key";
  password?: string;
  private_key?: string;
  passphrase?: string;
  strict_host_key?: boolean;
}

export interface UseSessionOptions {
  type: SessionType;
  /** When set, skips the device picker and always targets this asset. */
  fixedTarget?: string;
  /** Enables auto-reconnect for terminal sessions on abnormal disconnects. */
  autoReconnect?: boolean;
  /** When set, creates an ephemeral quick-connect session instead of an asset-based one. */
  quickConnectParams?: QuickConnectParams;
}

export interface TerminalConnectOptions {
  terminalShell?: string;
  protocol?: DesktopProtocol;
  display?: string;
  record?: boolean;
}

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
  const [connectionPhase, setConnectionPhase] =
    useState<SessionConnectionPhase>("idle");
  const [connectionMessage, setConnectionMessage] = useState("Idle");
  const [progressNowMs, setProgressNowMs] = useState(() => Date.now());
  const maxReconnectAttempts = 5;

  const sessionRef = useRef<{ id: string; target: string } | null>(null);
  const connectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);
  const manualDisconnectRef = useRef(false);
  const connectStartedAtRef = useRef<number | null>(null);
  const phaseStartedAtRef = useRef<number | null>(null);
  const lastDesktopOptionsRef = useRef<{
    protocol: DesktopProtocol;
    display: string;
    record: boolean;
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
      if (type === "desktop") {
        lastDesktopOptionsRef.current = { protocol, display, record };
      }

      const isLatestAttempt = () => connectAttemptRef.current === attemptID;

      // Track reconnection
      const reconnecting = !!(
        sessionRef.current && sessionRef.current.target === connectTarget
      );
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

      try {
        let sessionId = "";

        if (type === "terminal") {
          // Reuse terminal session for same target
          if (
            sessionRef.current &&
            sessionRef.current.target === connectTarget
          ) {
            sessionId = sessionRef.current.id;
          } else if (quickConnectParams) {
            // Quick Connect: ephemeral session with inline SSH credentials
            const sessionRes = await fetch("/api/terminal/quick-session", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(quickConnectParams),
            });
            const sessionPayload = (await sessionRes.json()) as {
              error?: string;
              session?: { id: string };
              sessionId?: string;
            };
            if (!sessionRes.ok) {
              throw new Error(
                sessionPayload.error ||
                  `Failed to create quick session (${sessionRes.status})`,
              );
            }
            sessionId =
              sessionPayload.session?.id || sessionPayload.sessionId || "";
            if (!sessionId) throw new Error("No session ID returned");
          } else {
            // Use persistent sessions so tmux sessions survive disconnect/reconnect.
            // Step 1: Ensure a persistent session exists for this actor+target.
            const ensureRes = await fetch("/api/terminal/persistent-sessions", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ target: connectTarget }),
            });
            let persistentId = "";
            if (ensureRes.ok) {
              const ensurePayload = (await ensureRes.json()) as {
                session?: { id?: string };
                id?: string;
              } | null;
              persistentId =
                ensurePayload?.session?.id || ensurePayload?.id || "";
            }

            if (persistentId) {
              // Step 2: Attach to the persistent session (reuses tmux).
              const attachRes = await fetch(
                `/api/terminal/persistent-sessions/${encodeURIComponent(persistentId)}/attach`,
                {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({}),
                },
              );
              const attachPayload = (await attachRes.json()) as {
                error?: string;
                session?: { id?: string; persistent_session_id?: string };
              } | null;
              if (!attachRes.ok) {
                throw new Error(
                  attachPayload?.error ||
                    `Failed to attach session (${attachRes.status})`,
                );
              }
              sessionId = attachPayload?.session?.id || "";
              if (!sessionId) throw new Error("No session ID returned from attach");
            } else {
              // Fallback: create a plain ephemeral session if persistent flow fails.
              const sessionRes = await fetch("/api/terminal/session", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({
                  target: connectTarget,
                  actorId: defaultActorID,
                  mode: "interactive",
                }),
              });
              const sessionPayload = (await sessionRes.json()) as {
                error?: string;
                session?: { id: string };
                sessionId?: string;
              };
              if (!sessionRes.ok) {
                throw new Error(
                  sessionPayload.error ||
                    `Failed to create session (${sessionRes.status})`,
                );
              }
              sessionId =
                sessionPayload.session?.id || sessionPayload.sessionId || "";
              if (!sessionId) throw new Error("No session ID returned");
            }
          }
        } else {
          // Desktop: always create new session
          const sessionRes = await fetch("/api/desktop/session", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              target: connectTarget,
              quality,
              protocol,
              display,
              record,
            }),
          });
          const sessionPayload = (await sessionRes.json()) as {
            error?: string;
            session?: { id: string };
            sessionId?: string;
          };
          if (!sessionRes.ok) {
            throw new Error(
              sessionPayload.error ||
                `Failed to create session (${sessionRes.status})`,
            );
          }
          sessionId =
            sessionPayload.session?.id || sessionPayload.sessionId || "";
          if (!sessionId) throw new Error("No session ID returned");
        }

        if (!isLatestAttempt()) {
          return;
        }
        sessionRef.current = { id: sessionId, target: connectTarget };
        setActiveSessionId(sessionId);

        if (type === "desktop" && protocol === "spice") {
          setProgress("requesting-ticket", "Requesting SPICE ticket...");
          const spiceRes = await fetch("/api/desktop/spice-ticket", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ sessionId }),
          });
          const spicePayload = (await spiceRes.json()) as {
            error?: string;
            wsUrl?: string;
            streamPath?: string;
            secure?: boolean;
            password?: string;
            type?: string;
            ca?: string;
            proxy?: string;
          };
          if (!spiceRes.ok || !spicePayload.password) {
            throw new Error(
              spicePayload.error ||
                `Failed to get SPICE ticket (${spiceRes.status})`,
            );
          }
          if (!isLatestAttempt()) {
            return;
          }
          const resolvedSpiceWsUrl =
            spicePayload.wsUrl ||
            (spicePayload.streamPath
              ? buildBrowserWsUrl(spicePayload.streamPath, {
                  secure: spicePayload.secure,
                })
              : undefined);
          if (!resolvedSpiceWsUrl) {
            throw new Error("SPICE ticket response missing stream endpoint");
          }
          setSpiceTicket({
            wsUrl: resolvedSpiceWsUrl,
            password: spicePayload.password,
            type: spicePayload.type,
            ca: spicePayload.ca,
            proxy: spicePayload.proxy,
          });
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
        const ticketEndpoint =
          type === "terminal"
            ? "/api/terminal/stream-ticket"
            : "/api/desktop/stream-ticket";
        const ticketBody: { sessionId: string; terminalShell?: string } = {
          sessionId,
        };
        if (terminalShell) {
          ticketBody.terminalShell = terminalShell;
        }
        const ticketRes = await fetch(ticketEndpoint, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(ticketBody),
        });
        const ticketPayload = (await ticketRes.json()) as {
          error?: string;
          wsUrl?: string;
          streamPath?: string;
          audioWsUrl?: string;
          audioStreamPath?: string;
          secure?: boolean;
          vncPassword?: string;
        };
        if (
          !ticketRes.ok ||
          (!ticketPayload.wsUrl && !ticketPayload.streamPath)
        ) {
          throw new Error(
            ticketPayload.error ||
              `Failed to get stream ticket (${ticketRes.status})`,
          );
        }

        if (!isLatestAttempt()) {
          return;
        }
        const builtWsUrl =
          ticketPayload.wsUrl ||
          buildBrowserWsUrl(ticketPayload.streamPath!, {
            secure: ticketPayload.secure,
          });
        const builtAudioWsUrl =
          ticketPayload.audioWsUrl ||
          (ticketPayload.audioStreamPath
            ? buildBrowserWsUrl(ticketPayload.audioStreamPath, {
                secure: ticketPayload.secure,
              })
            : null);
        setVncPassword(ticketPayload.vncPassword?.trim() || null);
        setWsUrl(builtWsUrl);
        setAudioWsUrl(builtAudioWsUrl);
        setConnectionState(
          type === "desktop" ? "authenticating" : "connecting",
        );
        setProgress("opening-stream", "Opening secure stream...");
      } catch (err) {
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
    ],
  );

  const disconnect = useCallback(() => {
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
  }, [clearReconnectTimer, setProgress]);

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
    ],
  );

  const handleError = useCallback(
    (message: string) => {
      setError(message);
      setConnectionState("error");
      setVncPassword(null);
      setAudioWsUrl(null);
      setProgress("error", message.trim() || "Connection failed");
    },
    [setProgress],
  );

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

  useEffect(() => {
    return () => {
      clearReconnectTimer();
    };
  }, [clearReconnectTimer]);

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
