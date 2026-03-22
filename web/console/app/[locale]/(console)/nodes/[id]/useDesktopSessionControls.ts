"use client";

import { useCallback, useEffect, type Dispatch, type RefObject, type SetStateAction } from "react";

import type { GuacamoleViewerHandle } from "../../../../components/GuacamoleViewer";
import type { SPICEViewerHandle } from "../../../../components/SPICEViewer";
import type { VNCViewerHandle } from "../../../../components/VNCViewer";
import type { WebRTCConnectionStats, WebRTCViewerHandle } from "../../../../components/WebRTCViewer";

import {
  chooseBrowserRecordingMimeType,
  downloadBrowserRecording,
} from "./desktopTabMetrics";
import {
  resolveDesktopDisplay,
  type DesktopProtocol,
} from "./desktopTabPreferences";

type DesktopSessionLike = {
  activeSessionId?: string;
  connectionState: string;
  connect: (
    target?: string,
    options?: {
      protocol: DesktopProtocol;
      display: string;
      record: boolean;
    },
  ) => Promise<unknown>;
  disconnect: () => void;
  handleDisconnected: (detail: { clean: boolean; reason?: string }) => void;
};

type DesktopSessionControlsOptions = {
  nodeId: string;
  nodeName: string;
  protocol: DesktopProtocol;
  selectedDisplay: string;
  serverRecording: boolean;
  browserRecordingSupported: boolean;
  serverRecordingSupported: boolean;
  webrtcStream: MediaStream | null;
  browserRecorderRef: RefObject<MediaRecorder | null>;
  browserRecordingChunksRef: RefObject<Blob[]>;
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
  session: DesktopSessionLike;
  resetCredentialForm: () => void;
  dismissCredentialPrompt: () => void;
  clearSession: () => void;
  setSelectedDisplay: Dispatch<SetStateAction<string>>;
  setProtocol: (
    protocol: DesktopProtocol,
    options?: { explicit?: boolean },
  ) => void;
  setServerRecording: Dispatch<SetStateAction<boolean>>;
  setBrowserRecording: Dispatch<SetStateAction<boolean>>;
  setWebRTCStats: Dispatch<SetStateAction<WebRTCConnectionStats | null>>;
  setWebRTCStream: Dispatch<SetStateAction<MediaStream | null>>;
};

function stopBrowserRecorder(
  browserRecorderRef: RefObject<MediaRecorder | null>,
  setBrowserRecording?: Dispatch<SetStateAction<boolean>>,
) {
  const recorder = browserRecorderRef.current;
  if (!recorder) {
    return;
  }
  if (recorder.state !== "inactive") {
    recorder.stop();
    return;
  }
  browserRecorderRef.current = null;
  setBrowserRecording?.(false);
}

function disconnectAllViewers(options: {
  vncRef: RefObject<VNCViewerHandle | null>;
  guacRef: RefObject<GuacamoleViewerHandle | null>;
  spiceRef: RefObject<SPICEViewerHandle | null>;
  webrtcRef: RefObject<WebRTCViewerHandle | null>;
}) {
  options.vncRef.current?.disconnect();
  options.guacRef.current?.disconnect();
  options.spiceRef.current?.disconnect();
  options.webrtcRef.current?.disconnect();
}

function resetDesktopRuntimeState(options: {
  clearSession: () => void;
  setServerRecording: Dispatch<SetStateAction<boolean>>;
  setBrowserRecording: Dispatch<SetStateAction<boolean>>;
  setWebRTCStats: Dispatch<SetStateAction<WebRTCConnectionStats | null>>;
  setWebRTCStream: Dispatch<SetStateAction<MediaStream | null>>;
}) {
  options.setServerRecording(false);
  options.setBrowserRecording(false);
  options.setWebRTCStats(null);
  options.setWebRTCStream(null);
  options.clearSession();
}

export function useDesktopSessionControls({
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
}: DesktopSessionControlsOptions) {
  const connectDesktop = useCallback(() => {
    resetCredentialForm();
    void session.connect(undefined, {
      protocol,
      display: resolveDesktopDisplay(protocol, selectedDisplay),
      record: protocol === "vnc" ? serverRecording : false,
    });
  }, [protocol, resetCredentialForm, selectedDisplay, serverRecording, session]);

  const handleDisplayChange = useCallback(
    (displayName: string) => {
      setSelectedDisplay(displayName);
      if (session.connectionState === "connected") {
        disconnectAllViewers({ vncRef, guacRef, spiceRef, webrtcRef });
        session.disconnect();
        resetCredentialForm();
        void session.connect(undefined, {
          protocol,
          display: resolveDesktopDisplay(protocol, displayName),
          record: protocol === "vnc" ? serverRecording : false,
        });
      }
    },
    [
      guacRef,
      protocol,
      resetCredentialForm,
      serverRecording,
      session,
      setSelectedDisplay,
      spiceRef,
      vncRef,
      webrtcRef,
    ],
  );

  const handleDisconnect = useCallback(() => {
    stopBrowserRecorder(browserRecorderRef);
    session.disconnect();
    disconnectAllViewers({ vncRef, guacRef, spiceRef, webrtcRef });
    dismissCredentialPrompt();
    resetDesktopRuntimeState({
      clearSession,
      setServerRecording,
      setBrowserRecording,
      setWebRTCStats,
      setWebRTCStream,
    });
  }, [
    browserRecorderRef,
    clearSession,
    dismissCredentialPrompt,
    guacRef,
    session,
    setBrowserRecording,
    setServerRecording,
    setWebRTCStats,
    setWebRTCStream,
    spiceRef,
    vncRef,
    webrtcRef,
  ]);

  const toggleRecording = useCallback(async () => {
    if (protocol === "webrtc") {
      if (!browserRecordingSupported || !webrtcStream) return;
      if (browserRecorderRef.current && browserRecorderRef.current.state !== "inactive") {
        browserRecorderRef.current.stop();
        return;
      }
      try {
        const mimeType = chooseBrowserRecordingMimeType();
        const recorder = mimeType
          ? new MediaRecorder(webrtcStream, { mimeType })
          : new MediaRecorder(webrtcStream);
        browserRecordingChunksRef.current = [];
        recorder.ondataavailable = (event) => {
          if (event.data.size > 0) {
            browserRecordingChunksRef.current.push(event.data);
          }
        };
        recorder.onerror = () => {
          browserRecorderRef.current = null;
          browserRecordingChunksRef.current = [];
          setBrowserRecording(false);
        };
        recorder.onstop = () => {
          const blob = new Blob(browserRecordingChunksRef.current, {
            type: recorder.mimeType || "video/webm",
          });
          browserRecorderRef.current = null;
          browserRecordingChunksRef.current = [];
          setBrowserRecording(false);
          if (blob.size > 0) {
            downloadBrowserRecording(nodeName, blob);
          }
        };
        browserRecorderRef.current = recorder;
        recorder.start(1000);
        setBrowserRecording(true);
      } catch {
        setBrowserRecording(false);
      }
      return;
    }

    if (!serverRecordingSupported || !session.activeSessionId) return;
    try {
      if (serverRecording) {
        const response = await fetch(
          `/api/recordings/${encodeURIComponent(session.activeSessionId)}`,
          {
            method: "POST",
          },
        );
        if (response.ok) {
          setServerRecording(false);
        }
        return;
      }
      const response = await fetch("/api/recordings", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          session_id: session.activeSessionId,
          asset_id: nodeId,
          protocol,
        }),
      });
      if (response.ok) {
        setServerRecording(true);
      }
    } catch {
      // noop
    }
  }, [
    browserRecorderRef,
    browserRecordingChunksRef,
    browserRecordingSupported,
    nodeId,
    nodeName,
    protocol,
    serverRecording,
    serverRecordingSupported,
    session.activeSessionId,
    setBrowserRecording,
    setServerRecording,
    webrtcStream,
  ]);

  const handleSessionPanelDisconnect = useCallback(
    (detail: { clean: boolean; reason?: string }) => {
      stopBrowserRecorder(browserRecorderRef);
      resetDesktopRuntimeState({
        clearSession,
        setServerRecording,
        setBrowserRecording,
        setWebRTCStats,
        setWebRTCStream,
      });
      session.handleDisconnected(detail);
    },
    [
      browserRecorderRef,
      clearSession,
      session,
      setBrowserRecording,
      setServerRecording,
      setWebRTCStats,
      setWebRTCStream,
    ],
  );

  const switchProtocol = useCallback(
    async (nextProtocol: DesktopProtocol) => {
      const reconnectDisplay = resolveDesktopDisplay(
        nextProtocol,
        selectedDisplay,
      );
      stopBrowserRecorder(browserRecorderRef, setBrowserRecording);
      session.disconnect();
      disconnectAllViewers({ vncRef, guacRef, spiceRef, webrtcRef });
      dismissCredentialPrompt();
      resetCredentialForm();
      setBrowserRecording(false);
      setServerRecording(false);
      setWebRTCStats(null);
      setWebRTCStream(null);
      setProtocol(nextProtocol, { explicit: false });
      await session.connect(undefined, {
        protocol: nextProtocol,
        display: reconnectDisplay,
        record: false,
      });
    },
    [
      browserRecorderRef,
      dismissCredentialPrompt,
      guacRef,
      resetCredentialForm,
      selectedDisplay,
      session,
      setBrowserRecording,
      setProtocol,
      setServerRecording,
      setWebRTCStats,
      setWebRTCStream,
      spiceRef,
      vncRef,
      webrtcRef,
    ],
  );

  useEffect(() => {
    if (!webrtcStream) {
      stopBrowserRecorder(browserRecorderRef, setBrowserRecording);
    }
  }, [browserRecorderRef, setBrowserRecording, webrtcStream]);

  useEffect(() => {
    if (protocol !== "webrtc") {
      stopBrowserRecorder(browserRecorderRef, setBrowserRecording);
    }
  }, [browserRecorderRef, protocol, setBrowserRecording]);

  useEffect(() => {
    if (protocol !== "vnc" && serverRecording) {
      setServerRecording(false);
    }
  }, [protocol, serverRecording, setServerRecording]);

  useEffect(() => {
    return () => {
      stopBrowserRecorder(browserRecorderRef);
    };
  }, [browserRecorderRef]);

  return {
    connectDesktop,
    handleDisplayChange,
    handleDisconnect,
    toggleRecording,
    handleSessionPanelDisconnect,
    switchProtocol,
  };
}
