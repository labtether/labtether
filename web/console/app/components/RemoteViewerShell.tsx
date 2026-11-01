"use client";

import { useCallback, useRef, useState, type CSSProperties, type ReactNode, type RefObject } from "react";

import FileDropOverlay from "./FileDropOverlay";
import { PerformanceOverlay } from "./PerformanceOverlay";
import ReconnectOverlay from "./ReconnectOverlay";
import { RemoteViewFileDrawer } from "./RemoteViewFileDrawer";
import { RemoteViewToolbar } from "./RemoteViewToolbar";
import type { ScalingMode } from "./RemoteViewToolbar";
import { ErrorBoundary } from "./ErrorBoundary";
import type { DesktopProtocol, SpiceTicket } from "./SessionPanel";
import type { GuacamoleViewerHandle } from "./GuacamoleViewer";
import type { SPICEViewerHandle } from "./SPICEViewer";
import type { VNCViewerHandle, VNCCredentialRequest } from "./VNCViewer";
import type { WebRTCConnectionStats, WebRTCDisplayLayout, WebRTCViewerHandle } from "./WebRTCViewer";
import type { SessionConnectionPhase, SessionConnectionState } from "../hooks/useSession";
import type { KeyboardGrabState, ReconnectState, ViewerMetrics } from "../types/viewer";
import { RemoteScreen } from "./remote-toolbar/RemoteScreen";
import { ScreenOverlay } from "./remote-toolbar/ScreenOverlay";
import { ToolbarPortalContext } from "./remote-toolbar/ToolbarButtons";
import { useStageHeight } from "./remote-toolbar/useStageHeight";

/** The remote screen's width over height from "1920x1080", or 16:9 until it is known. */
export function screenAspect(resolution: string | null | undefined): number {
  const match = /^(\d+)x(\d+)$/.exec(resolution ?? "");
  const w = match ? Number(match[1]) : 0;
  const h = match ? Number(match[2]) : 0;
  return w > 0 && h > 0 ? w / h : 16 / 9;
}

export interface RemoteViewerShellProps {
  // Identity
  nodeId: string;
  viewerWrapperRef: RefObject<HTMLDivElement | null>;
  /** The screen's name for the words over it ("mac-mini · Not connected") and the full-screen pill. */
  targetLabel?: string;

  // Layout: what sits left of the tools in the row above the screen (Remote View's tabs), the tools shown there
  // while not connected (a device's protocol and quality), and one line of notice under the row.
  leading?: ReactNode;
  offlineTools?: ReactNode;
  notice?: ReactNode;
  /** The idle screen's button: Connect before the first connection, Reconnect after a disconnect. */
  startLabel?: "connect" | "reconnect";

  // Connection state
  connectionState: SessionConnectionState;
  connectionPhase?: SessionConnectionPhase;
  isReconnecting?: boolean;
  latencyMs: number | null;
  transportLabel: string;
  networkQuality?: "good" | "fair" | "poor" | null;

  // Protocol / stream
  protocol: DesktopProtocol;
  spiceTicket: SpiceTicket | null;
  wsUrl: string | null;
  error: string | null;
  target: string;
  connectedAgentIds: Set<string>;

  // Quality / scaling
  quality: string;
  onQualityChange: (quality: string) => void;
  scalingMode: ScalingMode;
  onScalingModeChange: (mode: ScalingMode) => void;

  // Input
  pointerLocked: boolean;
  pointerLockSupported: boolean;
  onPointerLockToggle: () => void;
  viewOnly: boolean;
  onViewOnlyToggle: () => void;

  // Recording and sound
  recording: boolean;
  recordingSupported: boolean;
  onToggleRecording: () => void;
  audioMuted: boolean;
  onAudioToggle: () => void;
  audioUnavailable?: boolean;
  volume: number;
  onVolumeChange: (volume: number) => void;

  // Full screen and actions
  isFullscreen: boolean;
  onFullscreenToggle: () => void;
  onCtrlAltDel: () => void;
  onDisconnect: () => void;
  onRetry: () => void;
  onScreenshot?: () => void;

  // File drop
  targetHasAgent: boolean;
  uploadTargetDir: string;
  onFileDropUpload?: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;

  // Viewer refs and events
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
  onDesktopConnect: () => void;
  onDesktopDisconnect: (detail: { clean: boolean; reason?: string }) => void;
  onDesktopError: (message: string) => void;
  onCredentialsRequired: (request: VNCCredentialRequest) => void;
  onWebRTCStats?: (stats: WebRTCConnectionStats) => void;
  onWebRTCStream?: (stream: MediaStream | null) => void;
  webrtcDisplayLayout?: WebRTCDisplayLayout[];

  // The login prompt over the screen
  credentialOverlay?: ReactNode;

  // Performance numbers
  metrics?: ViewerMetrics;
  showPerformanceOverlay?: boolean;
  onPerformanceOverlayToggle?: () => void;

  // Keyboard capture and key combinations
  keyboardGrabState?: KeyboardGrabState;
  onKeyboardGrabToggle?: () => void;
  onSendShortcut?: (keysyms: number[]) => void;

  // A lost connection
  reconnectState?: ReconnectState;
  onReconnectNow?: () => void;

  // Touch
  isTouchDevice?: boolean;
  onToggleVirtualKeyboard?: () => void;

  // Clipboard
  clipboardSyncing?: boolean;
  clipboardLastSync?: "idle" | "success" | "error";
  onClipboardPull?: () => void;
  onClipboardPush?: () => void;

  // Quality that follows the network
  adaptiveQualityEnabled?: boolean;
  onAdaptiveQualityToggle?: () => void;
  qualitySuggestion?: "upgrade" | "downgrade" | null;
  onQualitySuggestionApply?: () => void;
  onQualitySuggestionDismiss?: () => void;

  // Files
  onDownloadFile?: (path: string) => void;
  fileDownloading?: boolean;
  fileDrawerOpen?: boolean;
  onFileDrawerToggle?: () => void;

  // Displays
  displays?: Array<{ name: string; width: number; height: number; primary: boolean }>;
  selectedDisplay?: string;
  onDisplayChange?: (displayName: string) => void;
}

/**
 * One remote screen: the row above it (tabs or the device's options, then the tools), and the screen as large as the
 * window allows, with what needs saying over it. In full screen the screen covers everything and the tools float.
 */
export function RemoteViewerShell(props: RemoteViewerShellProps) {
  const { viewerWrapperRef, connectionState, isFullscreen, reconnectState } = props;
  const stageRef = useRef<HTMLDivElement>(null);
  const [wrapper, setWrapper] = useState<HTMLDivElement | null>(null);
  const attachWrapper = useCallback(
    (node: HTMLDivElement | null) => {
      viewerWrapperRef.current = node;
      setWrapper(node);
    },
    [viewerWrapperRef],
  );
  const height = useStageHeight(stageRef, !isFullscreen);
  // In Fit the screen keeps the remote's shape (so the frame hugs it); 100% scrolls and Fill resizes the remote, so
  // those take the whole area.
  const fit = props.scalingMode === "fit";
  const stageStyle = {
    ...(height ? { "--rv-h": `${height}px` } : {}),
    ...(fit ? { "--rv-ar": screenAspect(props.metrics?.resolution) } : {}),
  } as CSSProperties;
  const connected = connectionState === "connected";
  const drawer = props.targetHasAgent;

  const toolbar = (
    <RemoteViewToolbar
      {...props}
      layout={isFullscreen ? "overlay" : "dock"}
      onToggleRecording={connected && props.recordingSupported ? props.onToggleRecording : undefined}
      onVolumeChange={props.audioUnavailable ? undefined : props.onVolumeChange}
      onFileDrawerToggle={drawer ? props.onFileDrawerToggle : undefined}
    />
  );
  const reconnectShown = !!reconnectState?.active && !!props.onReconnectNow;

  return (
    <div
      ref={attachWrapper}
      className={`rv-shell${isFullscreen ? " full" : ""}`}
    >
      {isFullscreen ? null : (
        <div className="rv-head">
          {props.leading}
          {connected ? toolbar : props.offlineTools ? <div className="rv-offline">{props.offlineTools}</div> : null}
        </div>
      )}
      {props.notice && !isFullscreen ? <p className="rv-notice">{props.notice}</p> : null}
      <ToolbarPortalContext.Provider value={isFullscreen ? wrapper : null}>
        <div
          ref={stageRef}
          className={`rv-stage${fit ? " fit" : ""}`}
          style={stageStyle}
        >
          {isFullscreen ? toolbar : null}
          <FileDropOverlay
            assetId={props.nodeId}
            disabled={!(connected && props.targetHasAgent)}
            targetDir={props.uploadTargetDir}
            uploadFile={props.protocol === "webrtc" ? props.onFileDropUpload : undefined}
          >
            <ErrorBoundary>
              <RemoteScreen {...props} audioEnabled={!props.audioMuted} />
            </ErrorBoundary>
          </FileDropOverlay>
          {props.metrics ? <PerformanceOverlay metrics={props.metrics} visible={!!props.showPerformanceOverlay} /> : null}
          {props.credentialOverlay ? props.credentialOverlay : reconnectShown ? null : (
            <ScreenOverlay
              connectionState={connectionState}
              phase={props.connectionPhase}
              reconnecting={props.isReconnecting}
              error={props.error}
              targetLabel={props.targetLabel}
              start={props.startLabel ?? "reconnect"}
              onStart={props.onRetry}
              onCancel={props.onDisconnect}
              viewOnly={connected && props.viewOnly}
              onViewOnlyToggle={props.onViewOnlyToggle}
            />
          )}
          {reconnectShown ? <ReconnectOverlay state={reconnectState!} onReconnectNow={props.onReconnectNow!} /> : null}
          {drawer ? (
            <RemoteViewFileDrawer
              nodeId={props.nodeId}
              open={props.fileDrawerOpen ?? false}
              onClose={props.onFileDrawerToggle ?? (() => {})}
            />
          ) : null}
        </div>
      </ToolbarPortalContext.Provider>
    </div>
  );
}
