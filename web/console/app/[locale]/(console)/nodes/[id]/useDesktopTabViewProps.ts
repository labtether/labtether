"use client";

import {
  useCallback,
  useMemo,
  useState,
  type ComponentProps,
  type Dispatch,
  type ReactNode,
  type RefObject,
  type SetStateAction,
} from "react";

import type { RemoteViewerShellProps } from "../../../../components/RemoteViewerShell";
import type { ScalingMode } from "../../../../components/RemoteViewToolbar";
import type { DesktopProtocol } from "../../../../components/SessionPanel";
import type { GuacamoleViewerHandle } from "../../../../components/GuacamoleViewer";
import type { SPICEViewerHandle } from "../../../../components/SPICEViewer";
import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type {
  WebRTCConnectionStats,
  WebRTCViewerHandle,
} from "../../../../components/WebRTCViewer";
import type { SessionConnectionState } from "../../../../hooks/useSession";
import type { KeyboardGrabState } from "../../../../types/viewer";
import { DesktopConnectionControlsCard } from "./DesktopConnectionControlsCard";
import type { DisplayInfo } from "../../../../hooks/useDisplayList";

type QualityLevel = "low" | "medium" | "high";

type UseDesktopTabViewPropsArgs = {
  nodeId: string;
  connectionState: SessionConnectionState;
  quality: string;
  setQuality: (quality: string) => void;
  protocol: DesktopProtocol;
  setProtocol: (
    protocol: DesktopProtocol,
    options?: { explicit?: boolean },
  ) => void;
  clearProtocolSwitchNotice: () => void;
  availableProtocols: DesktopProtocol[];
  protocolLabel: (protocol: DesktopProtocol) => string;
  selectedDisplay: string;
  setSelectedDisplay: (value: string) => void;
  targetStatus: string | null | undefined;
  refreshConnected: () => void;
  isReconnecting: boolean;
  connectDesktop: () => void;
  handleDisconnect: () => void;
  viewerWrapperRef: RefObject<HTMLDivElement | null>;
  latencyMs: number | null;
  effectiveTransportLabel: string;
  networkQuality: "good" | "fair" | "poor" | null;
  scalingMode: ScalingMode;
  setScalingMode: (mode: ScalingMode) => void;
  viewerFocused: boolean;
  focusActiveViewer: () => void;
  restoreViewerFocus: () => void;
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
  viewOnly: boolean;
  setViewOnly: Dispatch<SetStateAction<boolean>>;
  recording: boolean;
  recordingSupported: boolean;
  toggleRecording: () => void;
  audioMuted: boolean;
  setAudioMuted: Dispatch<SetStateAction<boolean>>;
  audioUnavailable: boolean;
  volume: number;
  setVolume: Dispatch<SetStateAction<number>>;
  isFullscreen: boolean;
  toggleFullscreen: () => void;
  targetHasAgent: boolean;
  uploadTargetDir: string;
  handleFileDropUpload?: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;
  spiceTicket: RemoteViewerShellProps["spiceTicket"];
  wsUrl: string | null;
  error: string | null;
  target: string;
  connectedAgentIds: Set<string>;
  handleSessionPanelDisconnect: RemoteViewerShellProps["onDesktopDisconnect"];
  handleConnected: RemoteViewerShellProps["onDesktopConnect"];
  handleError: RemoteViewerShellProps["onDesktopError"];
  handleCredentialsRequired: RemoteViewerShellProps["onCredentialsRequired"];
  setWebRTCStats: (stats: WebRTCConnectionStats) => void;
  setWebRTCStream: (stream: MediaStream | null) => void;
  displayList: { displays: DisplayInfo[] };
  credentialOverlay: ReactNode;
  viewerMetrics: RemoteViewerShellProps["metrics"];
  showPerfOverlay: boolean;
  togglePerfOverlay: () => void;
  keyboardGrab: {
    state: KeyboardGrabState;
    activate: (root?: Element) => Promise<void>;
    deactivate: () => void;
    onRelease?: (cb: () => void) => () => void;
  };
  handleSendShortcut: (keysyms: number[]) => void;
  reconnectState: RemoteViewerShellProps["reconnectState"];
  connect: () => Promise<void>;
  virtualKeyboard: {
    isTouchDevice: boolean;
    toggle: () => void;
  };
  clipboard: {
    syncing: boolean;
    lastSync: "idle" | "success" | "error";
    pullFromRemote: () => void;
    pushToRemote: () => void;
  };
  fileDownload: {
    downloadFile: (path: string) => void;
    downloading: boolean;
  };
  handleDisplayChange: (displayName: string) => void;
  adaptiveQuality: {
    autoEnabled: boolean;
    setAutoEnabled: (enabled: boolean) => void;
    suggestion: "upgrade" | "downgrade" | null;
    getNextQuality: (suggestion: "upgrade" | "downgrade") => QualityLevel | null;
    applyQuality: (quality: QualityLevel) => void;
    dismissSuggestion: () => void;
  };
};

export function useDesktopTabViewProps({
  nodeId,
  connectionState,
  quality,
  setQuality,
  protocol,
  setProtocol,
  clearProtocolSwitchNotice,
  availableProtocols,
  protocolLabel,
  selectedDisplay,
  setSelectedDisplay,
  targetStatus,
  refreshConnected,
  isReconnecting,
  connectDesktop,
  handleDisconnect,
  viewerWrapperRef,
  latencyMs,
  effectiveTransportLabel,
  networkQuality,
  scalingMode,
  setScalingMode,
  viewerFocused,
  focusActiveViewer,
  restoreViewerFocus,
  vncRef,
  guacRef,
  spiceRef,
  webrtcRef,
  viewOnly,
  setViewOnly,
  recording,
  recordingSupported,
  toggleRecording,
  audioMuted,
  setAudioMuted,
  audioUnavailable,
  volume,
  setVolume,
  isFullscreen,
  toggleFullscreen,
  targetHasAgent,
  uploadTargetDir,
  handleFileDropUpload,
  spiceTicket,
  wsUrl,
  error,
  target,
  connectedAgentIds,
  handleSessionPanelDisconnect,
  handleConnected,
  handleError,
  handleCredentialsRequired,
  setWebRTCStats,
  setWebRTCStream,
  displayList,
  credentialOverlay,
  viewerMetrics,
  showPerfOverlay,
  togglePerfOverlay,
  keyboardGrab,
  handleSendShortcut,
  reconnectState,
  connect,
  virtualKeyboard,
  clipboard,
  fileDownload,
  handleDisplayChange,
  adaptiveQuality,
}: UseDesktopTabViewPropsArgs) {
  const [fileDrawerOpen, setFileDrawerOpen] = useState(false);
  const pointerLockSupported = protocol === "vnc";

  const handleProtocolChange = useCallback(
    (nextProtocol: DesktopProtocol) => {
      clearProtocolSwitchNotice();
      setProtocol(nextProtocol);
    },
    [clearProtocolSwitchNotice, setProtocol],
  );

  const handlePointerLockToggle = useCallback(() => {
    if (!pointerLockSupported) {
      return;
    }
    if (document.pointerLockElement) {
      vncRef.current?.exitPointerLock();
    }
    focusActiveViewer();
    restoreViewerFocus();
  }, [focusActiveViewer, pointerLockSupported, restoreViewerFocus, vncRef]);

  const handleCtrlAltDel = useCallback(() => {
    if (protocol === "vnc") vncRef.current?.sendCtrlAltDel();
    if (protocol === "rdp") guacRef.current?.sendCtrlAltDel();
    if (protocol === "spice") spiceRef.current?.sendCtrlAltDel();
    if (protocol === "webrtc") webrtcRef.current?.sendCtrlAltDel();
    restoreViewerFocus();
  }, [guacRef, protocol, restoreViewerFocus, spiceRef, vncRef, webrtcRef]);

  const handleKeyboardGrabToggle = useCallback(() => {
    if (keyboardGrab.state === "active") {
      keyboardGrab.deactivate();
      restoreViewerFocus();
      return;
    }
    focusActiveViewer();
    keyboardGrab.activate(viewerWrapperRef.current ?? undefined);
    restoreViewerFocus();
  }, [focusActiveViewer, keyboardGrab, restoreViewerFocus, viewerWrapperRef]);

  const handleQualitySuggestionApply = useCallback(() => {
    const next = adaptiveQuality.suggestion
      ? adaptiveQuality.getNextQuality(adaptiveQuality.suggestion)
      : null;
    if (next) {
      setQuality(next);
      adaptiveQuality.applyQuality(next);
    }
  }, [adaptiveQuality, setQuality]);

  const controlsCardProps = useMemo<ComponentProps<typeof DesktopConnectionControlsCard>>(
    () => ({
      nodeId,
      connectionState,
      quality,
      onQualityChange: setQuality,
      protocol,
      onProtocolChange: handleProtocolChange,
      availableProtocols,
      protocolLabel,
      selectedDisplay,
      onSelectedDisplayChange: setSelectedDisplay,
      targetStatus,
      onWake: refreshConnected,
      isReconnecting,
      onConnect: connectDesktop,
      onDisconnect: handleDisconnect,
    }),
    [
      availableProtocols,
      connectDesktop,
      connectionState,
      handleDisconnect,
      handleProtocolChange,
      isReconnecting,
      nodeId,
      protocol,
      protocolLabel,
      quality,
      refreshConnected,
      selectedDisplay,
      setQuality,
      setSelectedDisplay,
      targetStatus,
    ],
  );

  const remoteViewerShellProps = useMemo<RemoteViewerShellProps>(
    () => ({
      nodeId,
      viewerWrapperRef,
      connectionState,
      latencyMs,
      transportLabel: effectiveTransportLabel,
      networkQuality,
      protocol,
      quality,
      onQualityChange: setQuality,
      scalingMode,
      onScalingModeChange: setScalingMode,
      pointerLocked: protocol === "vnc" && viewerFocused,
      pointerLockSupported,
      onPointerLockToggle: handlePointerLockToggle,
      viewOnly,
      onViewOnlyToggle: () => setViewOnly((current) => !current),
      recording,
      recordingSupported,
      onToggleRecording: toggleRecording,
      audioMuted,
      onAudioToggle: () => setAudioMuted((current) => !current),
      audioUnavailable,
      volume,
      onVolumeChange: setVolume,
      isFullscreen,
      onFullscreenToggle: toggleFullscreen,
      onCtrlAltDel: handleCtrlAltDel,
      onDisconnect: handleDisconnect,
      targetHasAgent,
      uploadTargetDir,
      onFileDropUpload: protocol === "webrtc" ? handleFileDropUpload : undefined,
      spiceTicket,
      wsUrl,
      error,
      target,
      connectedAgentIds,
      onRetry: connectDesktop,
      vncRef,
      guacRef,
      spiceRef,
      webrtcRef,
      onDesktopConnect: handleConnected,
      onDesktopDisconnect: handleSessionPanelDisconnect,
      onDesktopError: handleError,
      onCredentialsRequired: handleCredentialsRequired,
      onWebRTCStats: setWebRTCStats,
      onWebRTCStream: setWebRTCStream,
      webrtcDisplayLayout: protocol === "webrtc" ? displayList.displays : undefined,
      credentialOverlay,
      metrics: viewerMetrics,
      showPerformanceOverlay: showPerfOverlay,
      onPerformanceOverlayToggle: togglePerfOverlay,
      keyboardGrabState: keyboardGrab.state,
      onKeyboardGrabToggle: handleKeyboardGrabToggle,
      onSendShortcut: handleSendShortcut,
      reconnectState,
      onReconnectNow: () => {
        void connect();
      },
      isTouchDevice: virtualKeyboard.isTouchDevice,
      onToggleVirtualKeyboard: virtualKeyboard.toggle,
      clipboardSyncing: clipboard.syncing,
      clipboardLastSync: clipboard.lastSync,
      onClipboardPull: clipboard.pullFromRemote,
      onClipboardPush: clipboard.pushToRemote,
      onDownloadFile: fileDownload.downloadFile,
      fileDownloading: fileDownload.downloading,
      fileDrawerOpen,
      onFileDrawerToggle: () => setFileDrawerOpen((v) => !v),
      displays: protocol === "vnc" ? displayList.displays : undefined,
      selectedDisplay: protocol === "vnc" ? selectedDisplay : undefined,
      onDisplayChange: protocol === "vnc" ? handleDisplayChange : undefined,
      adaptiveQualityEnabled: adaptiveQuality.autoEnabled,
      onAdaptiveQualityToggle: () => adaptiveQuality.setAutoEnabled(!adaptiveQuality.autoEnabled),
      qualitySuggestion: adaptiveQuality.suggestion,
      onQualitySuggestionApply: handleQualitySuggestionApply,
      onQualitySuggestionDismiss: adaptiveQuality.dismissSuggestion,
    }),
    [
      adaptiveQuality,
      audioMuted,
      audioUnavailable,
      clipboard.lastSync,
      clipboard.pullFromRemote,
      clipboard.pushToRemote,
      clipboard.syncing,
      connect,
      connectDesktop,
      connectedAgentIds,
      connectionState,
      credentialOverlay,
      displayList.displays,
      effectiveTransportLabel,
      error,
      fileDownload.downloadFile,
      fileDownload.downloading,
      fileDrawerOpen,
      guacRef,
      handleConnected,
      handleCredentialsRequired,
      handleCtrlAltDel,
      handleDisplayChange,
      handleDisconnect,
      handleError,
      handleFileDropUpload,
      handleKeyboardGrabToggle,
      handlePointerLockToggle,
      handleQualitySuggestionApply,
      handleSendShortcut,
      handleSessionPanelDisconnect,
      isFullscreen,
      keyboardGrab.state,
      latencyMs,
      networkQuality,
      nodeId,
      pointerLockSupported,
      protocol,
      quality,
      recording,
      recordingSupported,
      reconnectState,
      scalingMode,
      selectedDisplay,
      setAudioMuted,
      setQuality,
      setScalingMode,
      setViewOnly,
      setVolume,
      showPerfOverlay,
      spiceRef,
      spiceTicket,
      target,
      targetHasAgent,
      toggleFullscreen,
      togglePerfOverlay,
      toggleRecording,
      uploadTargetDir,
      viewOnly,
      viewerFocused,
      viewerMetrics,
      viewerWrapperRef,
      virtualKeyboard.isTouchDevice,
      virtualKeyboard.toggle,
      volume,
      vncRef,
      webrtcRef,
      wsUrl,
      setWebRTCStats,
      setWebRTCStream,
    ],
  );

  return {
    controlsCardProps,
    remoteViewerShellProps,
  };
}
