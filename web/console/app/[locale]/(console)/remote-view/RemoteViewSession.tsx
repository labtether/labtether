"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import type { GuacamoleViewerHandle } from "../../../components/GuacamoleViewer";
import {
  RemoteViewerShell,
  type RemoteViewerShellProps,
} from "../../../components/RemoteViewerShell";
import type { ScalingMode } from "../../../components/RemoteViewToolbar";
import type { SPICEViewerHandle } from "../../../components/SPICEViewer";
import type { VNCViewerHandle, VNCCredentialRequest } from "../../../components/VNCViewer";
import type {
  WebRTCConnectionStats,
  WebRTCViewerHandle,
} from "../../../components/WebRTCViewer";
import { useConnectedAgents } from "../../../hooks/useConnectedAgents";
import { useDisplayList } from "../../../hooks/useDisplayList";
import { useFullscreen } from "../../../hooks/useFullscreen";
import { useLatency } from "../../../hooks/useLatency";
import { useSession } from "../../../hooks/useSession";
import { useDesktopViewerFocus } from "../nodes/[id]/useDesktopViewerFocus";
import { useDesktopViewerRuntime } from "../nodes/[id]/useDesktopViewerRuntime";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";
import { toDesktopProtocol } from "./types";

// ── Helpers ──

/** Build a target string suitable for useSession from tab data. */
function buildSessionTarget(tab: RemoteViewTab): string {
  if (tab.type === "device" && tab.target?.assetId) {
    return tab.target.assetId;
  }
  if (tab.target) {
    const host = tab.target.host.includes(":") ? `[${tab.target.host}]` : tab.target.host;
    return `${host}:${tab.target.port}`;
  }
  return "";
}

// ── Component ──

interface RemoteViewSessionProps {
  tab: RemoteViewTab;
  onConnectionStateChange: (state: RemoteViewConnectionState) => void;
  /** Pre-collected credentials to auto-submit when the remote requests auth. */
  initialCredentials?: { username?: string; password?: string };
}

export default function RemoteViewSession({
  tab,
  onConnectionStateChange,
  initialCredentials,
}: RemoteViewSessionProps) {
  const desktopProtocol = tab.protocol ? toDesktopProtocol(tab.protocol) : "vnc";
  const sessionTarget = buildSessionTarget(tab);
  const directTarget = useMemo(
    () =>
      tab.type !== "device" && tab.target
        ? {
            host: tab.target.host,
            port: tab.target.port,
            ...(desktopProtocol === "rdp" || desktopProtocol === "spice"
              ? {
                  username: initialCredentials?.username,
                  password: initialCredentials?.password,
                }
              : {}),
          }
        : undefined,
    [tab.type, tab.target, desktopProtocol, initialCredentials],
  );

  // ── Core session hook ──
  const session = useSession({
    type: "desktop",
    fixedTarget: sessionTarget || undefined,
  });

  // ── Connected agents (for agent-aware features) ──
  const { connectedAgentIds } = useConnectedAgents();
  const targetHasAgent =
    tab.type === "device" && !!tab.target?.assetId && connectedAgentIds.has(tab.target.assetId);

  // ── Desktop UI state ──
  const viewerWrapperRef = useRef<HTMLDivElement>(null);
  const vncRef = useRef<VNCViewerHandle>(null);
  const guacRef = useRef<GuacamoleViewerHandle>(null);
  const spiceRef = useRef<SPICEViewerHandle>(null);
  const webrtcRef = useRef<WebRTCViewerHandle>(null);

  const [scalingMode, setScalingMode] = useState<ScalingMode>("fit");
  const [viewOnly, setViewOnly] = useState(false);
  const [audioMuted, setAudioMuted] = useState(false);
  const [volume, setVolume] = useState(1);
  const [recording, setRecording] = useState(false);
  const [recordingBusy, setRecordingBusy] = useState(false);
  const [fileDrawerOpen, setFileDrawerOpen] = useState(false);
  const [selectedDisplay, setSelectedDisplay] = useState("");
  const [pointerLocked, setPointerLocked] = useState(false);
  const [webrtcStats, setWebRTCStats] = useState<WebRTCConnectionStats | null>(null);
  const [webrtcStream, setWebRTCStream] = useState<MediaStream | null>(null);

  const { isFullscreen, toggleFullscreen } = useFullscreen(viewerWrapperRef);
  const latencyMs = useLatency(session.connectionState === "connected");

  const uploadTargetDir = "~/Downloads";
  const recordingSupported = desktopProtocol === "vnc" && targetHasAgent;
  const pointerLockSupported = desktopProtocol === "vnc";
  const nodeId = tab.target?.assetId || tab.id;
  const displayList = useDisplayList(
    nodeId,
    targetHasAgent && session.connectionState === "connected",
  );
  const { focusActiveViewer, restoreViewerFocus } = useDesktopViewerFocus({
      protocol: desktopProtocol,
      viewerWrapperRef,
      vncRef,
      guacRef,
      spiceRef,
      webrtcRef,
    });
  const viewerRuntime = useDesktopViewerRuntime({
    nodeId,
    protocol: desktopProtocol,
    targetHasAgent,
    isFullscreen,
    audioMuted,
    volume,
    viewerWrapperRef,
    vncRef,
    guacRef,
    spiceRef,
    webrtcRef,
    session,
    webrtcStats,
    webrtcStream,
    transportLabel: (tab.protocol ?? desktopProtocol).toUpperCase(),
    focusActiveViewer,
    restoreViewerFocus,
  });

  // ── Credential state ──
  const [credentialRequest, setCredentialRequest] = useState<VNCCredentialRequest | null>(null);
  const [credPassword, setCredPassword] = useState("");
  const [credUsername, setCredUsername] = useState("");
  // Track whether initial credentials have been consumed so we only auto-submit once.
  const initialCredentialsUsedRef = useRef(false);

  // ── Desktop event callbacks ──
  const handleDesktopConnect = useCallback(() => {
    session.handleConnected();
  }, [session]);

  const handleDesktopDisconnect = useCallback(
    (detail: { clean: boolean; reason?: string }) => {
      setRecording(false);
      session.handleDisconnected(detail);
    },
    [session],
  );

  const handleDesktopError = useCallback(
    (message: string) => {
      session.handleError(message);
    },
    [session],
  );

  const handleCredentialsRequired = useCallback((request: VNCCredentialRequest) => {
    // Priority 1: initial credentials from the pre-connect dialog (one-shot)
    if (initialCredentials && !initialCredentialsUsedRef.current) {
      const hasCreds =
        (request.types.includes("username") && !!initialCredentials.username) ||
        (request.types.includes("password") && !!initialCredentials.password);
      if (hasCreds) {
        initialCredentialsUsedRef.current = true;
        const creds: { username?: string; password?: string } = {};
        if (request.types.includes("username")) creds.username = initialCredentials.username;
        if (request.types.includes("password")) creds.password = initialCredentials.password;
        vncRef.current?.sendCredentials(creds);
        return;
      }
    }
    // Priority 2: VNC password from the stream ticket (password-only flows)
    const autoPassword = session.vncPassword;
    if (autoPassword && request.types.includes("password") && !request.types.includes("username")) {
      vncRef.current?.sendCredentials({ password: autoPassword });
      return;
    }
    // Priority 3: show the runtime credential overlay
    setCredentialRequest(request);
  }, [initialCredentials, session.vncPassword]);

  const handleCredentialSubmit = useCallback(() => {
    if (!credentialRequest) return;
    const creds: { username?: string; password?: string } = {};
    if (credentialRequest.types.includes("username")) creds.username = credUsername;
    if (credentialRequest.types.includes("password")) creds.password = credPassword;
    vncRef.current?.sendCredentials(creds);
    setCredentialRequest(null);
    setCredPassword("");
    setCredUsername("");
  }, [credentialRequest, credPassword, credUsername]);

  // ── Actions ──
  const handleCtrlAltDel = useCallback(() => {
    if (desktopProtocol === "vnc") vncRef.current?.sendCtrlAltDel();
    if (desktopProtocol === "rdp") guacRef.current?.sendCtrlAltDel();
    if (desktopProtocol === "spice") spiceRef.current?.sendCtrlAltDel();
    restoreViewerFocus();
  }, [desktopProtocol, restoreViewerFocus]);

  const handleDisconnect = useCallback(() => {
    vncRef.current?.disconnect();
    guacRef.current?.disconnect();
    spiceRef.current?.disconnect();
    webrtcRef.current?.disconnect();
    setRecording(false);
    session.disconnect();
  }, [session]);

  const handleRetry = useCallback(() => {
    void session.connect(undefined, {
      protocol: desktopProtocol,
      directTarget,
      display: selectedDisplay,
      record: false,
    });
  }, [session, desktopProtocol, directTarget, selectedDisplay]);

  const handlePointerLockToggle = useCallback(() => {
    if (!pointerLockSupported) return;
    if (document.pointerLockElement) {
      vncRef.current?.exitPointerLock();
    } else {
      vncRef.current?.requestPointerLock();
    }
    restoreViewerFocus();
  }, [pointerLockSupported, restoreViewerFocus]);

  const toggleRecording = useCallback(async () => {
    if (!recordingSupported || !session.activeSessionId || recordingBusy) {
      return;
    }
    setRecordingBusy(true);
    try {
      const response = recording
        ? await fetch(
            `/api/recordings/${encodeURIComponent(session.activeSessionId)}`,
            { method: "POST" },
          )
        : await fetch("/api/recordings", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              session_id: session.activeSessionId,
            }),
          });
      if (response.ok) {
        setRecording((current) => !current);
      }
    } catch {
      // The existing recording state remains authoritative on failure.
    } finally {
      setRecordingBusy(false);
    }
  }, [
    recording,
    recordingBusy,
    recordingSupported,
    session.activeSessionId,
  ]);

  const handleDisplayChange = useCallback(
    (displayName: string) => {
      setSelectedDisplay(displayName);
      if (session.connectionState !== "connected") return;
      vncRef.current?.disconnect();
      guacRef.current?.disconnect();
      spiceRef.current?.disconnect();
      webrtcRef.current?.disconnect();
      setRecording(false);
      session.disconnect();
      void session.connect(undefined, {
        protocol: desktopProtocol,
        directTarget,
        display: displayName,
        record: false,
      });
    },
    [desktopProtocol, directTarget, session],
  );

  const handleKeyboardGrabToggle = useCallback(() => {
    if (viewerRuntime.keyboardGrab.state === "active") {
      viewerRuntime.keyboardGrab.deactivate();
      restoreViewerFocus();
      return;
    }
    focusActiveViewer();
    void viewerRuntime.keyboardGrab.activate(
      viewerWrapperRef.current ?? undefined,
    );
    restoreViewerFocus();
  }, [
    focusActiveViewer,
    restoreViewerFocus,
    viewerRuntime.keyboardGrab,
  ]);

  const handleQualitySuggestionApply = useCallback(() => {
    const suggestion = viewerRuntime.adaptiveQuality.suggestion;
    const next = suggestion
      ? viewerRuntime.adaptiveQuality.getNextQuality(suggestion)
      : null;
    if (!next) return;
    session.setQuality(next);
    viewerRuntime.adaptiveQuality.applyQuality(next);
  }, [session, viewerRuntime.adaptiveQuality]);

  const handleScreenshot = useCallback(() => {
    const canvas = viewerWrapperRef.current?.querySelector("canvas");
    if (!canvas) return;
    try {
      const safeLabel =
        tab.label.replace(/[^a-z0-9._-]+/gi, "-").slice(0, 80) ||
        "remote-view";
      const link = document.createElement("a");
      link.download = `${safeLabel}-${new Date().toISOString().slice(0, 19)}.png`;
      link.href = (canvas as HTMLCanvasElement).toDataURL("image/png");
      link.click();
    } catch {
      // A cross-origin or otherwise tainted canvas cannot be exported safely.
    }
  }, [tab.label]);

  useEffect(() => {
    const updatePointerLock = () => {
      const locked = document.pointerLockElement;
      setPointerLocked(
        desktopProtocol === "vnc" &&
          !!locked &&
          !!viewerWrapperRef.current?.contains(locked),
      );
    };
    updatePointerLock();
    document.addEventListener("pointerlockchange", updatePointerLock);
    return () => {
      document.removeEventListener("pointerlockchange", updatePointerLock);
    };
  }, [desktopProtocol]);

  // ── Auto-connect on mount when target is available ──
  const hasAutoConnected = useRef(false);
  useEffect(() => {
    if (
      sessionTarget &&
      session.connectionState === "idle" &&
      !hasAutoConnected.current
    ) {
      hasAutoConnected.current = true;
      void session.connect(sessionTarget, {
        protocol: desktopProtocol,
        directTarget,
        display: selectedDisplay,
        record: false,
      });
    }
  }, [sessionTarget, session, desktopProtocol, directTarget, selectedDisplay]);

  // ── Sync connection state to parent tab bar ──
  useEffect(() => {
    const state = session.connectionState;
    let mappedState: RemoteViewConnectionState;
    switch (state) {
      case "idle":
        mappedState = "idle";
        break;
      case "connecting":
        mappedState = "connecting";
        break;
      case "authenticating":
        mappedState = "authenticating";
        break;
      case "connected":
        mappedState = "connected";
        break;
      case "error":
        mappedState = "error";
        break;
      default:
        mappedState = "idle";
    }
    onConnectionStateChange(mappedState);
  }, [session.connectionState, onConnectionStateChange]);

  // ── Credential overlay ──
  const credentialOverlay = credentialRequest ? (
    <div className="absolute inset-0 flex items-center justify-center bg-black/60 z-20">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="remote-view-auth-title"
        className="bg-[var(--surface)] border border-[var(--line)] rounded-lg p-6 w-80 shadow-lg"
      >
        <h3
          id="remote-view-auth-title"
          className="text-sm font-semibold text-[var(--text)] mb-3"
        >
          Authentication Required
        </h3>
        {credentialRequest.types?.includes("username") && (
          <input
            type="text"
            aria-label="Username"
            placeholder="Username"
            value={credUsername}
            onChange={(e) => setCredUsername(e.target.value)}
            className="w-full mb-2 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
            autoFocus
          />
        )}
        {credentialRequest.types?.includes("password") && (
          <input
            type="password"
            aria-label="Password"
            placeholder="Password"
            value={credPassword}
            onChange={(e) => setCredPassword(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleCredentialSubmit();
            }}
            className="w-full mb-3 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
            autoFocus={!credentialRequest.types?.includes("username")}
          />
        )}
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={() => {
              setCredentialRequest(null);
              setCredPassword("");
              setCredUsername("");
            }}
            className="px-3 py-1.5 text-xs rounded border border-[var(--line)] text-[var(--muted)] hover:text-[var(--text)]"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleCredentialSubmit}
            className="px-3 py-1.5 text-xs rounded bg-[var(--accent)] text-white"
          >
            Connect
          </button>
        </div>
      </div>
    </div>
  ) : null;

  // ── Build RemoteViewerShell props ──
  const shellProps: RemoteViewerShellProps = {
    // Identity
    nodeId,
    viewerWrapperRef,

    // Connection state
    connectionState: session.connectionState,
    latencyMs,
    transportLabel: viewerRuntime.effectiveTransportLabel,
    networkQuality: viewerRuntime.networkQuality,

    // Protocol / stream
    protocol: desktopProtocol,
    spiceTicket: session.spiceTicket,
    wsUrl: session.wsUrl,
    error: session.error,
    target: session.target,
    connectedAgentIds: session.connectedAgentIds,

    // Quality / scaling
    quality: session.quality,
    onQualityChange: session.setQuality,
    scalingMode,
    onScalingModeChange: setScalingMode,

    // Pointer lock
    pointerLocked,
    pointerLockSupported,
    onPointerLockToggle: handlePointerLockToggle,

    // View-only
    viewOnly,
    onViewOnlyToggle: () => setViewOnly((v) => !v),

    // Recording
    recording,
    recordingSupported: recordingSupported && !recordingBusy,
    onToggleRecording: () => {
      void toggleRecording();
    },

    // Audio
    audioMuted,
    onAudioToggle: () => setAudioMuted((v) => !v),
    audioUnavailable:
      desktopProtocol === "vnc" &&
      (viewerRuntime.audioSideband.status === "unavailable" ||
        viewerRuntime.audioSideband.status === "error"),
    volume,
    onVolumeChange: setVolume,

    // Fullscreen
    isFullscreen,
    onFullscreenToggle: toggleFullscreen,

    // Actions
    onCtrlAltDel: handleCtrlAltDel,
    onDisconnect: handleDisconnect,
    onRetry: handleRetry,
    onScreenshot: handleScreenshot,

    // File drop
    targetHasAgent,
    uploadTargetDir,

    // Viewer refs
    vncRef,
    guacRef,
    spiceRef,
    webrtcRef,

    // Desktop event callbacks
    onDesktopConnect: handleDesktopConnect,
    onDesktopDisconnect: handleDesktopDisconnect,
    onDesktopError: handleDesktopError,
    onCredentialsRequired: handleCredentialsRequired,
    onWebRTCStats: setWebRTCStats,
    onWebRTCStream: setWebRTCStream,
    webrtcDisplayLayout: undefined,

    // Credential overlay
    credentialOverlay,

    // Performance overlay
    metrics: viewerRuntime.viewerMetrics,
    showPerformanceOverlay: viewerRuntime.showPerfOverlay,
    onPerformanceOverlayToggle: viewerRuntime.togglePerfOverlay,

    // Keyboard grab and shortcuts
    keyboardGrabState: viewerRuntime.keyboardGrab.state,
    onKeyboardGrabToggle: handleKeyboardGrabToggle,
    onSendShortcut:
      desktopProtocol === "spice"
        ? undefined
        : viewerRuntime.handleSendShortcut,

    // Reconnect overlay
    reconnectState: viewerRuntime.reconnectState,
    onReconnectNow: () => {
      void session.connect(sessionTarget, {
        protocol: desktopProtocol,
        directTarget,
        display: selectedDisplay,
        record: false,
      });
    },

    // Touch support
    isTouchDevice:
      desktopProtocol === "spice"
        ? false
        : viewerRuntime.virtualKeyboard.isTouchDevice,
    onToggleVirtualKeyboard:
      desktopProtocol === "spice"
        ? undefined
        : viewerRuntime.virtualKeyboard.toggle,

    // Clipboard sync
    clipboardSyncing: targetHasAgent
      ? viewerRuntime.clipboard.syncing
      : undefined,
    clipboardLastSync: targetHasAgent
      ? viewerRuntime.clipboard.lastSync
      : undefined,
    onClipboardPull: targetHasAgent
      ? viewerRuntime.clipboard.pullFromRemote
      : undefined,
    onClipboardPush: targetHasAgent
      ? viewerRuntime.clipboard.pushToRemote
      : undefined,

    // Adaptive quality
    adaptiveQualityEnabled: viewerRuntime.adaptiveQuality.autoEnabled,
    onAdaptiveQualityToggle: () =>
      viewerRuntime.adaptiveQuality.setAutoEnabled(
        !viewerRuntime.adaptiveQuality.autoEnabled,
      ),
    qualitySuggestion: viewerRuntime.adaptiveQuality.suggestion,
    onQualitySuggestionApply: handleQualitySuggestionApply,
    onQualitySuggestionDismiss:
      viewerRuntime.adaptiveQuality.dismissSuggestion,

    // File download / drawer
    onDownloadFile: targetHasAgent
      ? viewerRuntime.fileDownload.downloadFile
      : undefined,
    fileDownloading: targetHasAgent
      ? viewerRuntime.fileDownload.downloading
      : false,
    fileDrawerOpen,
    onFileDrawerToggle: () => setFileDrawerOpen((v) => !v),

    // Monitor picker
    displays:
      desktopProtocol === "vnc" && targetHasAgent
        ? displayList.displays
        : undefined,
    selectedDisplay:
      desktopProtocol === "vnc" && targetHasAgent
        ? selectedDisplay
        : undefined,
    onDisplayChange:
      desktopProtocol === "vnc" && targetHasAgent
        ? handleDisplayChange
        : undefined,
  };

  return <RemoteViewerShell {...shellProps} />;
}
