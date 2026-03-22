"use client";

import { useRef, useState } from "react";

import type { ScalingMode } from "../../../../components/RemoteViewToolbar";
import type { WebRTCConnectionStats } from "../../../../components/WebRTCViewer";

import type { DesktopProtocol } from "./desktopTabPreferences";

type UseDesktopTabStateOptions = {
  protocol: DesktopProtocol;
  targetHasAgent: boolean;
};

export function useDesktopTabState({
  protocol,
  targetHasAgent,
}: UseDesktopTabStateOptions) {
  const [selectedDisplay, setSelectedDisplay] = useState("");
  const [scalingMode, setScalingMode] = useState<ScalingMode>("fit");
  const [viewOnly, setViewOnly] = useState(false);
  const [audioMuted, setAudioMuted] = useState(false);
  const [volume, setVolume] = useState(1);
  const [serverRecording, setServerRecording] = useState(false);
  const [browserRecording, setBrowserRecording] = useState(false);
  const [webrtcStats, setWebRTCStats] = useState<WebRTCConnectionStats | null>(
    null,
  );
  const [webrtcStream, setWebRTCStream] = useState<MediaStream | null>(null);

  const viewerWrapperRef = useRef<HTMLDivElement>(null);
  const browserRecorderRef = useRef<MediaRecorder | null>(null);
  const browserRecordingChunksRef = useRef<Blob[]>([]);

  const uploadTargetDir = "~/Downloads";
  const browserRecordingSupported =
    protocol === "webrtc" &&
    typeof MediaRecorder !== "undefined" &&
    webrtcStream !== null;
  const serverRecordingSupported = protocol === "vnc" && targetHasAgent;
  const recordingSupported =
    browserRecordingSupported || serverRecordingSupported;
  const recording = protocol === "webrtc" ? browserRecording : serverRecording;

  return {
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
    browserRecording,
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
  };
}
