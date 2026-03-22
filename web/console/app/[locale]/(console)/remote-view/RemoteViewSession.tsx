"use client";

import { useCallback, useEffect, useRef, useState } from "react";

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
import { useFullscreen } from "../../../hooks/useFullscreen";
import { useLatency } from "../../../hooks/useLatency";
import { useSession } from "../../../hooks/useSession";
import type { RemoteViewTab, RemoteViewConnectionState } from "./types";
import { toDesktopProtocol } from "./types";

// ── Helpers ──

/** Build a target string suitable for useSession from tab data. */
function buildSessionTarget(tab: RemoteViewTab): string {
  if (tab.type === "device" && tab.target?.assetId) {
    return tab.target.assetId;
  }
  if (tab.target) {
    return `${tab.target.host}:${tab.target.port}`;
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
  const [fileDrawerOpen, setFileDrawerOpen] = useState(false);
  const [_webrtcStats, setWebRTCStats] = useState<WebRTCConnectionStats | null>(null);
  const [_webrtcStream, setWebRTCStream] = useState<MediaStream | null>(null);

  const { isFullscreen, toggleFullscreen } = useFullscreen(viewerWrapperRef);
  const latencyMs = useLatency(session.connectionState === "connected");

  const uploadTargetDir = "~/Downloads";
  const recordingSupported = desktopProtocol === "vnc" && targetHasAgent;
  const pointerLockSupported = desktopProtocol === "vnc";
  const nodeId = tab.target?.assetId || tab.id;

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
    const autoPassword = session.vncPassword?.trim();
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
  }, [desktopProtocol]);

  const handleDisconnect = useCallback(() => {
    session.disconnect();
  }, [session]);

  const handleRetry = useCallback(() => {
    void session.connect(undefined, { protocol: desktopProtocol });
  }, [session, desktopProtocol]);

  const handlePointerLockToggle = useCallback(() => {
    if (!pointerLockSupported) return;
    if (document.pointerLockElement) {
      vncRef.current?.exitPointerLock();
    }
  }, [pointerLockSupported]);

  const toggleRecording = useCallback(() => {
    setRecording((prev) => !prev);
  }, []);

  const handleScreenshot = useCallback(() => {
    const canvas = viewerWrapperRef.current?.querySelector("canvas");
    if (!canvas) return;
    const link = document.createElement("a");
    link.download = `${tab.label}-${new Date().toISOString().slice(0, 19)}.png`;
    link.href = (canvas as HTMLCanvasElement).toDataURL("image/png");
    link.click();
  }, [tab.label]);

  // ── Auto-connect on mount when target is available ──
  const hasAutoConnected = useRef(false);
  useEffect(() => {
    if (
      sessionTarget &&
      session.connectionState === "idle" &&
      !hasAutoConnected.current
    ) {
      hasAutoConnected.current = true;
      void session.connect(sessionTarget, { protocol: desktopProtocol });
    }
  }, [sessionTarget, session, desktopProtocol]);

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
      <div className="bg-[var(--surface)] border border-[var(--line)] rounded-lg p-6 w-80 shadow-lg">
        <h3 className="text-sm font-semibold text-[var(--text)] mb-3">
          Authentication Required
        </h3>
        {credentialRequest.types?.includes("username") && (
          <input
            type="text"
            placeholder="Username"
            value={credUsername}
            onChange={(e) => setCredUsername(e.target.value)}
            className="w-full mb-2 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
            autoFocus
          />
        )}
        <input
          type="password"
          placeholder="Password"
          value={credPassword}
          onChange={(e) => setCredPassword(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleCredentialSubmit();
          }}
          className="w-full mb-3 px-3 py-1.5 rounded border border-[var(--line)] bg-[var(--input)] text-[var(--text)] text-sm"
          autoFocus={!credentialRequest.types?.includes("username")}
        />
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
    transportLabel: desktopProtocol.toUpperCase(),
    networkQuality: undefined,

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
    pointerLocked: false,
    pointerLockSupported,
    onPointerLockToggle: handlePointerLockToggle,

    // View-only
    viewOnly,
    onViewOnlyToggle: () => setViewOnly((v) => !v),

    // Recording
    recording,
    recordingSupported,
    onToggleRecording: toggleRecording,

    // Audio
    audioMuted,
    onAudioToggle: () => setAudioMuted((v) => !v),
    audioUnavailable: desktopProtocol === "vnc",
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
    onFileDropUpload: undefined,

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

    // Performance overlay (Phase 1 — not wired yet for remote-view tabs)
    metrics: undefined,
    showPerformanceOverlay: false,
    onPerformanceOverlayToggle: undefined,

    // Keyboard grab (not wired yet for remote-view tabs)
    keyboardGrabState: undefined,
    onKeyboardGrabToggle: undefined,
    onSendShortcut: undefined,

    // Reconnect overlay
    reconnectState: undefined,
    onReconnectNow: () => {
      void session.connect(sessionTarget, { protocol: desktopProtocol });
    },

    // Touch support
    isTouchDevice: undefined,
    onToggleVirtualKeyboard: undefined,

    // Clipboard sync
    clipboardSyncing: undefined,
    clipboardLastSync: undefined,
    onClipboardPull: undefined,
    onClipboardPush: undefined,

    // Adaptive quality (not wired yet for remote-view tabs)
    adaptiveQualityEnabled: undefined,
    onAdaptiveQualityToggle: undefined,
    qualitySuggestion: undefined,
    onQualitySuggestionApply: undefined,
    onQualitySuggestionDismiss: undefined,

    // File download / drawer
    onDownloadFile: undefined,
    fileDownloading: false,
    fileDrawerOpen,
    onFileDrawerToggle: () => setFileDrawerOpen((v) => !v),

    // Monitor picker
    displays: undefined,
    selectedDisplay: undefined,
    onDisplayChange: undefined,
  };

  return <RemoteViewerShell {...shellProps} />;
}
