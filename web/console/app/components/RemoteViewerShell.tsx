"use client";

import type { ReactNode, RefObject } from "react";

import FileDropOverlay from "./FileDropOverlay";
import { PerformanceOverlay } from "./PerformanceOverlay";
import ReconnectOverlay from "./ReconnectOverlay";
import { RemoteViewFileDrawer } from "./RemoteViewFileDrawer";
import { RemoteViewToolbar } from "./RemoteViewToolbar";
import type { ScalingMode } from "./RemoteViewToolbar";
import { ErrorBoundary } from "./ErrorBoundary";
import { SessionPanel } from "./SessionPanel";
import type { DesktopProtocol, SpiceTicket } from "./SessionPanel";
import type { GuacamoleViewerHandle } from "./GuacamoleViewer";
import type { SPICEViewerHandle } from "./SPICEViewer";
import type { VNCViewerHandle, VNCCredentialRequest } from "./VNCViewer";
import type {
  WebRTCConnectionStats,
  WebRTCDisplayLayout,
  WebRTCViewerHandle,
} from "./WebRTCViewer";
import type { SessionConnectionState } from "../hooks/useSession";
import type {
  KeyboardGrabState,
  ReconnectState,
  ViewerMetrics,
} from "../types/viewer";

// ── Props ──

export interface RemoteViewerShellProps {
  // ── Identity ──
  nodeId: string;
  viewerWrapperRef: RefObject<HTMLDivElement | null>;

  // ── Connection state ──
  connectionState: SessionConnectionState;
  latencyMs: number | null;
  transportLabel: string;
  networkQuality?: "good" | "fair" | "poor" | null;

  // ── Protocol / stream ──
  protocol: DesktopProtocol;
  spiceTicket: SpiceTicket | null;
  wsUrl: string | null;
  error: string | null;
  target: string;
  connectedAgentIds: Set<string>;

  // ── Quality / scaling ──
  quality: string;
  onQualityChange: (quality: string) => void;
  scalingMode: ScalingMode;
  onScalingModeChange: (mode: ScalingMode) => void;

  // ── Pointer lock ──
  pointerLocked: boolean;
  pointerLockSupported: boolean;
  onPointerLockToggle: () => void;

  // ── View-only ──
  viewOnly: boolean;
  onViewOnlyToggle: () => void;

  // ── Recording ──
  recording: boolean;
  recordingSupported: boolean;
  onToggleRecording: () => void;

  // ── Audio ──
  audioMuted: boolean;
  onAudioToggle: () => void;
  audioUnavailable?: boolean;
  volume: number;
  onVolumeChange: (volume: number) => void;

  // ── Fullscreen ──
  isFullscreen: boolean;
  onFullscreenToggle: () => void;

  // ── Actions ──
  onCtrlAltDel: () => void;
  onDisconnect: () => void;
  onRetry: () => void;
  onScreenshot?: () => void;

  // ── File drop ──
  targetHasAgent: boolean;
  uploadTargetDir: string;
  onFileDropUpload?: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;

  // ── Viewer refs ──
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;

  // ── Desktop event callbacks ──
  onDesktopConnect: () => void;
  onDesktopDisconnect: (detail: { clean: boolean; reason?: string }) => void;
  onDesktopError: (message: string) => void;
  onCredentialsRequired: (request: VNCCredentialRequest) => void;
  onWebRTCStats?: (stats: WebRTCConnectionStats) => void;
  onWebRTCStream?: (stream: MediaStream | null) => void;
  webrtcDisplayLayout?: WebRTCDisplayLayout[];

  // ── Credential overlay ──
  credentialOverlay?: ReactNode;

  // ── Phase 1: Performance overlay ──
  metrics?: ViewerMetrics;
  showPerformanceOverlay?: boolean;
  onPerformanceOverlayToggle?: () => void;

  // ── Phase 1: Keyboard grab ──
  keyboardGrabState?: KeyboardGrabState;
  onKeyboardGrabToggle?: () => void;
  onSendShortcut?: (keysyms: number[]) => void;

  // ── Phase 1: Reconnect overlay ──
  reconnectState?: ReconnectState;
  onReconnectNow?: () => void;

  // ── Phase 1: Touch support ──
  isTouchDevice?: boolean;
  onToggleVirtualKeyboard?: () => void;

  // ── Clipboard sync ──
  clipboardSyncing?: boolean;
  clipboardLastSync?: "idle" | "success" | "error";
  onClipboardPull?: () => void;
  onClipboardPush?: () => void;

  // ── Phase 2: Adaptive quality ──
  adaptiveQualityEnabled?: boolean;
  onAdaptiveQualityToggle?: () => void;
  qualitySuggestion?: "upgrade" | "downgrade" | null;
  onQualitySuggestionApply?: () => void;
  onQualitySuggestionDismiss?: () => void;

  // ── File download ──
  onDownloadFile?: (path: string) => void;
  fileDownloading?: boolean;
  fileDrawerOpen?: boolean;
  onFileDrawerToggle?: () => void;

  // ── Monitor picker ──
  displays?: Array<{
    name: string;
    width: number;
    height: number;
    primary: boolean;
  }>;
  selectedDisplay?: string;
  onDisplayChange?: (displayName: string) => void;
}

// ── Component ──

export function RemoteViewerShell({
  nodeId,
  viewerWrapperRef,
  connectionState,
  latencyMs,
  transportLabel,
  networkQuality,
  protocol,
  spiceTicket,
  wsUrl,
  error,
  target,
  connectedAgentIds,
  quality,
  onQualityChange,
  scalingMode,
  onScalingModeChange,
  pointerLocked,
  pointerLockSupported,
  onPointerLockToggle,
  viewOnly,
  onViewOnlyToggle,
  recording,
  recordingSupported,
  onToggleRecording,
  audioMuted,
  onAudioToggle,
  audioUnavailable = false,
  volume,
  onVolumeChange,
  isFullscreen,
  onFullscreenToggle,
  onCtrlAltDel,
  onDisconnect,
  onRetry,
  onScreenshot,
  targetHasAgent,
  uploadTargetDir,
  onFileDropUpload,
  vncRef,
  guacRef,
  spiceRef,
  webrtcRef,
  onDesktopConnect,
  onDesktopDisconnect,
  onDesktopError,
  onCredentialsRequired,
  onWebRTCStats,
  onWebRTCStream,
  webrtcDisplayLayout,
  credentialOverlay,
  metrics,
  showPerformanceOverlay = false,
  onPerformanceOverlayToggle,
  keyboardGrabState,
  onKeyboardGrabToggle,
  onSendShortcut,
  reconnectState,
  onReconnectNow,
  isTouchDevice,
  onToggleVirtualKeyboard,
  clipboardSyncing,
  clipboardLastSync,
  onClipboardPull,
  onClipboardPush,
  onDownloadFile,
  fileDownloading,
  fileDrawerOpen,
  onFileDrawerToggle,
  displays,
  selectedDisplay,
  onDisplayChange,
}: RemoteViewerShellProps) {
  const canDrop = connectionState === "connected" && targetHasAgent;
  const toolbar = (
    <RemoteViewToolbar
      layout={isFullscreen ? "overlay" : "dock"}
      connectionState={connectionState}
      latencyMs={latencyMs}
      transportLabel={transportLabel}
      networkQuality={networkQuality}
      protocol={protocol}
      quality={quality}
      onQualityChange={onQualityChange}
      scalingMode={scalingMode}
      onScalingModeChange={onScalingModeChange}
      pointerLocked={pointerLocked}
      pointerLockSupported={pointerLockSupported}
      onPointerLockToggle={onPointerLockToggle}
      viewOnly={viewOnly}
      onViewOnlyToggle={onViewOnlyToggle}
      recording={recording}
      onToggleRecording={
        connectionState === "connected" && recordingSupported
          ? onToggleRecording
          : undefined
      }
      audioMuted={audioMuted}
      onAudioToggle={onAudioToggle}
      audioUnavailable={audioUnavailable}
      volume={volume}
      onVolumeChange={audioUnavailable ? undefined : onVolumeChange}
      isFullscreen={isFullscreen}
      onFullscreenToggle={onFullscreenToggle}
      onCtrlAltDel={onCtrlAltDel}
      onScreenshot={onScreenshot}
      onDisconnect={onDisconnect}
      keyboardGrabState={keyboardGrabState}
      onKeyboardGrabToggle={onKeyboardGrabToggle}
      onSendShortcut={onSendShortcut}
      showPerformanceOverlay={showPerformanceOverlay}
      onPerformanceOverlayToggle={onPerformanceOverlayToggle}
      isTouchDevice={isTouchDevice}
      onToggleVirtualKeyboard={onToggleVirtualKeyboard}
      clipboardSyncing={clipboardSyncing}
      clipboardLastSync={clipboardLastSync}
      onClipboardPull={onClipboardPull}
      onClipboardPush={onClipboardPush}
      onDownloadFile={onDownloadFile}
      fileDownloading={fileDownloading}
      fileDrawerOpen={fileDrawerOpen}
      onFileDrawerToggle={onFileDrawerToggle}
      displays={displays}
      selectedDisplay={selectedDisplay}
      onDisplayChange={onDisplayChange}
    />
  );

  return (
    <div ref={viewerWrapperRef} className="vncViewerWrapper">
      <div className="vncViewerStage">
        {isFullscreen && toolbar}

        {metrics && (
          <PerformanceOverlay
            metrics={metrics}
            visible={showPerformanceOverlay}
          />
        )}

        {reconnectState && onReconnectNow && (
          <ReconnectOverlay
            state={reconnectState}
            onReconnectNow={onReconnectNow}
          />
        )}

        <FileDropOverlay
          assetId={nodeId}
          disabled={!canDrop}
          targetDir={uploadTargetDir}
          uploadFile={protocol === "webrtc" ? onFileDropUpload : undefined}
        >
          <ErrorBoundary>
            <SessionPanel
              type="desktop"
              protocol={protocol}
              spiceTicket={spiceTicket}
              connectionState={connectionState}
              wsUrl={wsUrl}
              error={error}
              target={target}
              connectedAgentIds={connectedAgentIds}
              onRetry={onRetry}
              vncRef={vncRef}
              guacRef={guacRef}
              spiceRef={spiceRef}
              webrtcRef={webrtcRef}
              quality={quality}
              scalingMode={scalingMode}
              viewOnly={viewOnly}
              audioEnabled={!audioMuted}
              volume={volume}
              onWebRTCStats={onWebRTCStats}
              onDesktopConnect={onDesktopConnect}
              onDesktopDisconnect={onDesktopDisconnect}
              onDesktopError={onDesktopError}
              onCredentialsRequired={onCredentialsRequired}
              webrtcDisplayLayout={webrtcDisplayLayout}
              onWebRTCStream={onWebRTCStream}
              credentialOverlay={credentialOverlay}
            />
          </ErrorBoundary>
        </FileDropOverlay>

        {targetHasAgent && (
          <RemoteViewFileDrawer
            nodeId={nodeId}
            open={fileDrawerOpen ?? false}
            onClose={onFileDrawerToggle ?? (() => {})}
          />
        )}
      </div>

      {!isFullscreen && toolbar}
    </div>
  );
}
