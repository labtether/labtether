"use client";

import { useRef } from "react";

import type { GuacamoleViewerHandle } from "../../../../components/GuacamoleViewer";
import type { SPICEViewerHandle } from "../../../../components/SPICEViewer";
import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type { WebRTCViewerHandle } from "../../../../components/WebRTCViewer";
import { useDesktopSession } from "../../../../contexts/DesktopSessionContext";
import { useDisplayList } from "../../../../hooks/useDisplayList";
import { useFullscreen } from "../../../../hooks/useFullscreen";
import { useLatency } from "../../../../hooks/useLatency";
import { useProtocolSwitch } from "../../../../hooks/useProtocolSwitch";
import { useSession } from "../../../../hooks/useSession";
import { useDesktopConnectionUx } from "./useDesktopConnectionUx";
import { DesktopRemoteViewSection } from "./DesktopRemoteViewSection";
import { useDesktopSessionControls } from "./useDesktopSessionControls";
import { useDesktopSessionPresence } from "./useDesktopSessionPresence";
import { useDesktopTabState } from "./useDesktopTabState";
import { useDesktopTabViewProps } from "./useDesktopTabViewProps";
import { useDesktopViewerFocus } from "./useDesktopViewerFocus";
import { useDesktopViewerPreferences } from "./useDesktopViewerPreferences";
import { useDesktopViewerRuntime } from "./useDesktopViewerRuntime";
import { useDesktopProtocolState } from "./useDesktopProtocolState";

export function DesktopTab({ nodeId }: { nodeId: string }) {
  const session = useSession({ type: "desktop", fixedTarget: nodeId });
  const vncRef = useRef<VNCViewerHandle>(null);
  const guacRef = useRef<GuacamoleViewerHandle>(null);
  const spiceRef = useRef<SPICEViewerHandle>(null);
  const webrtcRef = useRef<WebRTCViewerHandle>(null);
  const displayList = useDisplayList(nodeId, !!session.connectedAgentIds.size);
  const { registerSession, clearSession } = useDesktopSession();

  const targetAsset = session.assets.find((entry) => entry.id === nodeId);
  const nodeName = targetAsset?.name ?? nodeId;

  const {
    protocol,
    setProtocol,
    hasExplicitProtocolPreference,
    availableProtocols,
    targetHasAgent,
    transportLabel,
    protocolLabel,
    allowAutomaticFallbackToVNC,
  } = useDesktopProtocolState({
    nodeId,
    targetAsset,
    connectedAgentIds: session.connectedAgentIds,
    connectionState: session.connectionState,
  });

  const {
    selectedDisplay,
    setSelectedDisplay,
    scalingMode,
    setScalingMode,
    viewOnly,
    setViewOnly,
    audioMuted,
    setAudioMuted,
    volume,
    setVolume,
    serverRecording,
    setServerRecording,
    setBrowserRecording,
    webrtcStats,
    setWebRTCStats,
    webrtcStream,
    setWebRTCStream,
    viewerWrapperRef,
    browserRecorderRef,
    browserRecordingChunksRef,
    uploadTargetDir,
    browserRecordingSupported,
    serverRecordingSupported,
    recordingSupported,
    recording,
  } = useDesktopTabState({
    protocol,
    targetHasAgent,
  });

  const { isFullscreen, toggleFullscreen } = useFullscreen(viewerWrapperRef);
  const latencyMs = useLatency(session.connectionState === "connected");

  const {
    viewerFocused,
    focusActiveViewer,
    restoreViewerFocus,
  } = useDesktopViewerFocus({
    protocol,
    viewerWrapperRef,
    vncRef,
    guacRef,
    spiceRef,
    webrtcRef,
  });

  useDesktopSessionPresence({
    nodeId,
    nodeName,
    connectionState: session.connectionState,
    activeSessionId: session.activeSessionId,
    registerSession,
    clearSession,
  });

  const {
    handleCredentialsRequired,
    resetCredentialForm,
    dismissCredentialPrompt,
    credentialOverlay,
  } = useDesktopConnectionUx({
    protocol,
    vncRef,
    autoPassword: session.vncPassword,
  });

  const {
    connectDesktop,
    handleDisplayChange,
    handleDisconnect,
    toggleRecording,
    handleSessionPanelDisconnect,
    switchProtocol,
  } = useDesktopSessionControls({
    nodeId,
    nodeName,
    protocol,
    selectedDisplay,
    serverRecording,
    browserRecordingSupported,
    serverRecordingSupported,
    webrtcStream,
    browserRecorderRef,
    browserRecordingChunksRef,
    vncRef,
    guacRef,
    spiceRef,
    webrtcRef,
    session,
    resetCredentialForm,
    dismissCredentialPrompt,
    clearSession,
    setSelectedDisplay,
    setProtocol,
    setServerRecording,
    setBrowserRecording,
    setWebRTCStats,
    setWebRTCStream,
  });

  const {
    keyboardGrab,
    showPerfOverlay,
    togglePerfOverlay,
    virtualKeyboard,
    clipboard,
    audioSideband,
    fileDownload,
    viewerMetrics,
    networkQuality,
    effectiveTransportLabel,
    adaptiveQuality,
    reconnectState,
    handleSendShortcut,
    handleFileDropUpload,
  } = useDesktopViewerRuntime({
    nodeId,
    protocol,
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
    transportLabel,
    focusActiveViewer,
    restoreViewerFocus,
  });

  const {
    notice: protocolSwitchNotice,
    clearNotice: clearProtocolSwitchNotice,
  } = useProtocolSwitch({
    protocol,
    availableProtocols,
    connectionState: session.connectionState,
    networkQuality,
    webrtcRouteClass: webrtcStats?.routeClass ?? null,
    allowAutomaticRecovery: !hasExplicitProtocolPreference,
    allowFallbackToVNC: allowAutomaticFallbackToVNC,
    onSwitch: switchProtocol,
  });

  useDesktopViewerPreferences({
    nodeId,
    availableProtocols,
    protocol,
    hasExplicitProtocolPreference,
    connectionState: session.connectionState,
    quality: session.quality,
    scalingMode,
    viewOnly,
    audioMuted,
    volume,
    selectedDisplay,
    setScalingMode,
    setViewOnly,
    setAudioMuted,
    setVolume,
    setSelectedDisplay,
    setQuality: session.setQuality,
    setProtocol,
    connect: session.connect,
  });

  const { controlsCardProps, remoteViewerShellProps } = useDesktopTabViewProps({
    nodeId,
    connectionState: session.connectionState,
    quality: session.quality,
    setQuality: session.setQuality,
    protocol,
    setProtocol,
    clearProtocolSwitchNotice,
    availableProtocols,
    protocolLabel,
    selectedDisplay,
    setSelectedDisplay,
    targetStatus: targetAsset?.status,
    refreshConnected: session.refreshConnected,
    isReconnecting: session.isReconnecting,
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
    audioUnavailable:
      protocol === "vnc" &&
      (audioSideband.status === "unavailable" ||
        audioSideband.status === "error"),
    volume,
    setVolume,
    isFullscreen,
    toggleFullscreen,
    targetHasAgent,
    uploadTargetDir,
    handleFileDropUpload,
    spiceTicket: session.spiceTicket,
    wsUrl: session.wsUrl,
    error: session.error,
    target: session.target,
    connectedAgentIds: session.connectedAgentIds,
    handleSessionPanelDisconnect,
    handleConnected: session.handleConnected,
    handleError: session.handleError,
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
    connect: session.connect,
    virtualKeyboard,
    clipboard,
    fileDownload,
    handleDisplayChange,
    adaptiveQuality,
  });

  return (
    <DesktopRemoteViewSection
      controlsCardProps={controlsCardProps}
      protocolSwitchNotice={protocolSwitchNotice}
      remoteViewerShellProps={remoteViewerShellProps}
    />
  );
}
