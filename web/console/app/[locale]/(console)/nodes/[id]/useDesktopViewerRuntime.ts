"use client";

import { useCallback, useEffect, type RefObject } from "react";

import type { GuacamoleViewerHandle } from "../../../../components/GuacamoleViewer";
import { usePerformanceOverlayToggle } from "../../../../components/PerformanceOverlay";
import type { SPICEViewerHandle } from "../../../../components/SPICEViewer";
import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type { WebRTCConnectionStats, WebRTCViewerHandle } from "../../../../components/WebRTCViewer";
import { useAdaptiveQuality } from "../../../../hooks/useAdaptiveQuality";
import { useAudioSideband } from "../../../../hooks/useAudioSideband";
import { useClipboardSync } from "../../../../hooks/useClipboardSync";
import { useCursorAutoHide } from "../../../../hooks/useCursorAutoHide";
import { useFileDownload } from "../../../../hooks/useFileDownload";
import { useKeyboardGrab } from "../../../../hooks/useKeyboardGrab";
import { useVirtualKeyboard } from "../../../../hooks/useVirtualKeyboard";
import { useViewerMetrics } from "../../../../hooks/useViewerMetrics";
import type { ReconnectState, ViewerMetrics } from "../../../../types/viewer";

import { buildWebRTCMetrics, deriveNetworkQuality } from "./desktopTabMetrics";
import type { DesktopProtocol } from "./desktopTabPreferences";

type DesktopRuntimeSessionLike = {
  audioWsUrl?: string | null;
  connectedAgentIds: Set<string>;
  connectionState: string;
  isReconnecting: boolean;
  maxReconnectAttempts: number;
  quality: string;
  reconnectAttempt: number;
  reconnectExhausted: boolean;
  setQuality: (quality: string) => void;
};

type DesktopViewerRuntimeOptions = {
  nodeId: string;
  protocol: DesktopProtocol;
  targetHasAgent: boolean;
  isFullscreen: boolean;
  audioMuted: boolean;
  volume: number;
  viewerWrapperRef: RefObject<HTMLDivElement | null>;
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
  session: DesktopRuntimeSessionLike;
  webrtcStats: WebRTCConnectionStats | null;
  webrtcStream: MediaStream | null;
  transportLabel: string;
  focusActiveViewer: () => void;
  restoreViewerFocus: () => void;
};

export function useDesktopViewerRuntime({
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
}: DesktopViewerRuntimeOptions) {
  const keyboardGrab = useKeyboardGrab();
  const { visible: showPerfOverlay, toggle: togglePerfOverlay } =
    usePerformanceOverlayToggle();

  useCursorAutoHide(viewerWrapperRef, isFullscreen);

  const virtualKeyboard = useVirtualKeyboard((keysym, down) => {
    if (protocol === "vnc") vncRef.current?.sendKey?.(keysym, down);
    else if (protocol === "webrtc") webrtcRef.current?.sendKey?.(keysym, down);
  });

  const clipboard = useClipboardSync({
    nodeId,
    enabled: targetHasAgent && session.connectionState === "connected",
    readRemoteText:
      protocol === "webrtc"
        ? async () => {
            const viewer = webrtcRef.current;
            if (!viewer) {
              throw new Error("WebRTC clipboard unavailable");
            }
            return viewer.requestClipboardText();
          }
        : undefined,
    writeRemoteText:
      protocol === "webrtc"
        ? async (text: string) => {
            const viewer = webrtcRef.current;
            if (!viewer) {
              throw new Error("WebRTC clipboard unavailable");
            }
            await viewer.writeClipboardText(text);
          }
        : undefined,
  });

  const audioSideband = useAudioSideband({
    wsUrl: protocol === "vnc" ? session.audioWsUrl ?? null : null,
    enabled:
      protocol === "vnc" &&
      targetHasAgent &&
      session.connectionState === "connected",
    muted: audioMuted,
    volume,
  });

  const fileDownload = useFileDownload(nodeId);

  const baseViewerMetrics = useViewerMetrics({
    protocol,
    connected: session.connectionState === "connected",
    vncContainerRef: viewerWrapperRef,
  });
  const viewerMetrics: ViewerMetrics =
    protocol === "webrtc"
      ? buildWebRTCMetrics(webrtcStats, webrtcStream)
      : baseViewerMetrics;
  const networkQuality = deriveNetworkQuality(viewerMetrics);
  const effectiveTransportLabel =
    protocol === "webrtc" && viewerMetrics.transport
      ? viewerMetrics.transport.toLowerCase()
      : transportLabel;

  const adaptiveQuality = useAdaptiveQuality(
    viewerMetrics,
    session.quality as "low" | "medium" | "high",
  );

  useEffect(() => {
    if (!adaptiveQuality.autoEnabled || !adaptiveQuality.suggestion) return;
    const next = adaptiveQuality.getNextQuality(adaptiveQuality.suggestion);
    if (next) {
      session.setQuality(next);
      adaptiveQuality.applyQuality(next);
    }
  }, [adaptiveQuality, session]);

  const reconnectState: ReconnectState | undefined =
    session.isReconnecting || session.reconnectExhausted
      ? {
          active: true,
          attempt: session.reconnectExhausted
            ? session.maxReconnectAttempts
            : session.reconnectAttempt,
          maxAttempts: session.maxReconnectAttempts,
          nextRetryMs: Math.pow(2, session.reconnectAttempt) * 1000,
        }
      : undefined;

  const handleSendShortcut = useCallback(
    (keysyms: number[]) => {
      focusActiveViewer();
      const sendKey = (keysym: number, down: boolean) => {
        if (protocol === "vnc") vncRef.current?.sendKey?.(keysym, down);
        else if (protocol === "webrtc")
          webrtcRef.current?.sendKey?.(keysym, down);
      };
      for (const ks of keysyms) sendKey(ks, true);
      for (const ks of [...keysyms].reverse()) sendKey(ks, false);
      restoreViewerFocus();
    },
    [focusActiveViewer, protocol, restoreViewerFocus, vncRef, webrtcRef],
  );

  const handleFileDropUpload = useCallback(
    async (
      file: File,
      targetPath: string,
      onProgress?: (loaded: number, total: number) => void,
    ) => {
      const viewer = webrtcRef.current;
      if (!viewer) {
        throw new Error("WebRTC file transfer unavailable");
      }
      await viewer.uploadFile(file, targetPath, onProgress);
    },
    [webrtcRef],
  );

  return {
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
  };
}
