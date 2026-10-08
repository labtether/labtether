"use client";

import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";

import { useWebRTCSignaling } from "../hooks/useWebRTCSignaling";
import { assignStreamToVideoElement, normalizeDisplayLayout } from "./webrtc/webrtcMediaLayout";
import { useWebRTCDataChannels } from "./webrtc/useWebRTCDataChannels";
import { useWebRTCStats } from "./webrtc/useWebRTCStats";
import { WebRTCVideoSurface } from "./webrtc/WebRTCVideoSurface";
import { useWebRTCInput } from "./webrtc/useWebRTCInput";
import type { WebRTCViewerHandle, WebRTCViewerProps } from "./webrtc/webrtcViewerTypes";
export type { WebRTCConnectionStats, WebRTCDisplayLayout, WebRTCViewerHandle } from "./webrtc/webrtcViewerTypes";

const WebRTCViewer = forwardRef<WebRTCViewerHandle, WebRTCViewerProps>(
  function WebRTCViewer(
    {
      wsUrl,
      onConnect,
      onDisconnect,
      scalingMode = "fit",
      audioEnabled = true,
      volume = 1,
      onStats,
      onStream,
      displayLayout,
    },
    ref,
  ) {
    const videoRef = useRef<HTMLVideoElement>(null);
    const stitchedStageRef = useRef<HTMLDivElement>(null);
    const stitchedVideoRefs = useRef<Array<HTMLVideoElement | null>>([]);
    const pcRef = useRef<RTCPeerConnection | null>(null);
    const dcRef = useRef<RTCDataChannel | null>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const onConnectRef = useRef(onConnect);
    const onDisconnectRef = useRef(onDisconnect);
    const onStatsRef = useRef(onStats);
    const onStreamRef = useRef(onStream);
    const audioEnabledRef = useRef(audioEnabled);
    const volumeRef = useRef(volume);
    const signalingSessionRef = useRef(0);
    const candidateTimerRefs = useRef<ReturnType<typeof setTimeout>[]>([]);
    const connectedRef = useRef(false);
    const streamRef = useRef<MediaStream | null>(null);
    const { send, on } = useWebRTCSignaling(wsUrl);
    const [connected, setConnected] = useState(false);
    const [streamTrackCounts, setStreamTrackCounts] = useState({
      video: 0,
      audio: 0,
    });
    const stitchedLayout = useMemo(
      () => normalizeDisplayLayout(displayLayout),
      [displayLayout],
    );

    useEffect(() => {
      onConnectRef.current = onConnect;
      onDisconnectRef.current = onDisconnect;
      onStatsRef.current = onStats;
      onStreamRef.current = onStream;
      audioEnabledRef.current = audioEnabled;
      volumeRef.current = volume;
    }, [audioEnabled, onConnect, onDisconnect, onStats, onStream, volume]);

    const { openDataChannels, closeDataChannels, hasDataChannels,
      requestClipboardText, writeClipboardText, uploadFile,
    } = useWebRTCDataChannels();

    const closePeer = useCallback((reason: string, clean = false) => {
      candidateTimerRefs.current.forEach((timer) => clearTimeout(timer));
      candidateTimerRefs.current = [];
      const hadConnection = Boolean(
        pcRef.current ||
        dcRef.current ||
        hasDataChannels() ||
        connectedRef.current,
      );
      const pc = pcRef.current;
      if (pc) {
        try {
          pc.close();
        } catch {
          // Ignore close errors.
        }
        pcRef.current = null;
      }
      dcRef.current = null;
      closeDataChannels(reason);
      assignStreamToVideoElement(videoRef.current, null, true, 1);
      stitchedVideoRefs.current.forEach((element) => {
        assignStreamToVideoElement(element, null, true, 1);
      });
      streamRef.current = null;
      onStreamRef.current?.(null);
      setStreamTrackCounts({ video: 0, audio: 0 });
      connectedRef.current = false;
      setConnected(false);
      if (hadConnection) {
        onDisconnectRef.current?.({ clean, reason });
      }
    }, [closeDataChannels, hasDataChannels]);

    useImperativeHandle(
      ref,
      () => ({
        disconnect: () => closePeer("user disconnected", true),
        sendCtrlAltDel: () => {
          const dc = dcRef.current;
          if (!dc || dc.readyState !== "open") {
            return;
          }
          const sendKey = (
            type: "keydown" | "keyup",
            keyCode: number,
            code?: string,
            key?: string,
          ) => {
            dc.send(JSON.stringify({ type, keyCode, code, key }));
          };
          sendKey("keydown", 0xffe3, "ControlLeft", "Control");
          sendKey("keydown", 0xffe9, "AltLeft", "Alt");
          sendKey("keydown", 0xffff, "Delete", "Delete");
          sendKey("keyup", 0xffff, "Delete", "Delete");
          sendKey("keyup", 0xffe9, "AltLeft", "Alt");
          sendKey("keyup", 0xffe3, "ControlLeft", "Control");
        },
        sendKey: (keysym: number, down: boolean) => {
          const dc = dcRef.current;
          if (!dc || dc.readyState !== "open") return;
          dc.send(
            JSON.stringify({
              type: down ? "keydown" : "keyup",
              keyCode: keysym,
            }),
          );
        },
        focus: () => {
          containerRef.current?.focus();
        },
        setVolume: (nextVolume: number) => {
          assignStreamToVideoElement(
            videoRef.current,
            streamRef.current,
            audioEnabledRef.current,
            nextVolume,
          );
          stitchedVideoRefs.current.forEach((element) => {
            assignStreamToVideoElement(
              element,
              streamRef.current,
              false,
              0,
            );
          });
        },
        requestClipboardText,
        writeClipboardText,
        uploadFile,
      }),
      [closePeer, requestClipboardText, uploadFile, writeClipboardText],
    );

    useEffect(() => {
      signalingSessionRef.current += 1;
      const sessionID = signalingSessionRef.current;
      const isCurrentSession = () => signalingSessionRef.current === sessionID;

      closePeer("signaling reset", true);
      if (!wsUrl) {
        return;
      }

      const offAnswer = on("answer", async (rawData) => {
        if (!isCurrentSession()) {
          return;
        }
        const data = rawData as { sdp?: string };
        const pc = pcRef.current;
        if (!pc || !data?.sdp) {
          return;
        }
        try {
          await pc.setRemoteDescription({ type: "answer", sdp: data.sdp });
        } catch {
          closePeer("WebRTC answer failed", false);
        }
      });

      const offICE = on("ice", async (rawData) => {
        if (!isCurrentSession()) {
          return;
        }
        const data = rawData as {
          candidate?: string;
          sdp_mid?: string;
          sdp_mline_index?: number;
        };
        const pc = pcRef.current;
        if (!pc || !data?.candidate) {
          return;
        }
        try {
          await pc.addIceCandidate({
            candidate: data.candidate,
            sdpMid: data.sdp_mid,
            sdpMLineIndex:
              typeof data.sdp_mline_index === "number"
                ? data.sdp_mline_index
                : null,
          });
        } catch {
          // Ignore invalid ICE candidates.
        }
      });

      const offStopped = on("stopped", (rawData) => {
        if (!isCurrentSession()) {
          return;
        }
        const data = rawData as { reason?: string };
        closePeer(data?.reason || "session ended", false);
      });

      const offReady = on("ready", async () => {
        if (!isCurrentSession() || pcRef.current) {
          return;
        }

        const applyStream = (stream: MediaStream | null) => {
          streamRef.current = stream;
          setStreamTrackCounts({
            video: stream?.getVideoTracks().length ?? 0,
            audio: stream?.getAudioTracks().length ?? 0,
          });
          assignStreamToVideoElement(
            videoRef.current,
            stream,
            audioEnabledRef.current,
            volumeRef.current,
          );
          stitchedVideoRefs.current.forEach((element) => {
            assignStreamToVideoElement(
              element,
              stream,
              false,
              0,
            );
          });
          onStreamRef.current?.(stream);
        };

        const attachIncomingTrack = (track: MediaStreamTrack) => {
          let stream = streamRef.current;
          if (!stream) {
            stream = new MediaStream();
          }

          for (const existingTrack of stream.getTracks()) {
            if (existingTrack.id === track.id) {
              applyStream(stream);
              return;
            }
            if (existingTrack.kind === track.kind) {
              stream.removeTrack(existingTrack);
            }
          }

          stream.addTrack(track);
          track.addEventListener(
            "ended",
            () => {
              if (streamRef.current !== stream) {
                return;
              }
              stream.removeTrack(track);
              applyStream(stream.getTracks().length > 0 ? stream : null);
            },
            { once: true },
          );
          applyStream(stream);
        };

        const pc = new RTCPeerConnection({
          iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
        });
        pcRef.current = pc;

        pc.ontrack = (event) => {
          if (!isCurrentSession()) {
            return;
          }
          if (event.track) {
            attachIncomingTrack(event.track);
            return;
          }
          const stream = event.streams[0];
          if (!stream) {
            return;
          }
          applyStream(stream);
        };

        pc.onicecandidate = (event) => {
          if (!isCurrentSession() || !event.candidate) {
            return;
          }
          const payload = {
            candidate: event.candidate.candidate,
            sdp_mid: event.candidate.sdpMid,
            sdp_mline_index: event.candidate.sdpMLineIndex,
          };
          const delay = iceCandidateSendDelay(event.candidate.candidate);
          if (delay <= 0) {
            send("ice", payload);
            return;
          }
          const timer = setTimeout(() => {
            candidateTimerRefs.current = candidateTimerRefs.current.filter(
              (entry) => entry !== timer,
            );
            if (!isCurrentSession()) {
              return;
            }
            send("ice", payload);
          }, delay);
          candidateTimerRefs.current.push(timer);
        };

        pc.onconnectionstatechange = () => {
          if (!isCurrentSession()) {
            return;
          }
          if (pc.connectionState === "connected") {
            connectedRef.current = true;
            setConnected(true);
            onConnectRef.current?.();
            return;
          }
          if (
            pc.connectionState === "failed" ||
            pc.connectionState === "closed" ||
            pc.connectionState === "disconnected"
          ) {
            closePeer(pc.connectionState, false);
          }
        };

        pc.addTransceiver("video", { direction: "recvonly" });
        pc.addTransceiver("audio", { direction: "recvonly" });
        dcRef.current = pc.createDataChannel("input", { ordered: true });
        openDataChannels(pc);

        try {
          const offer = await pc.createOffer();
          await pc.setLocalDescription(offer);
          if (!isCurrentSession()) {
            return;
          }
          send("offer", { type: "offer", sdp: offer.sdp });
        } catch {
          closePeer("WebRTC offer failed", false);
        }
      });

      return () => {
        offAnswer();
        offICE();
        offStopped();
        offReady();
        if (isCurrentSession()) {
          closePeer("session ended", true);
        }
      };
    }, [closePeer, on, send, wsUrl, openDataChannels]);

    useEffect(() => {
      assignStreamToVideoElement(
        videoRef.current,
        streamRef.current,
        audioEnabled,
        volume,
      );
      stitchedVideoRefs.current.forEach((element) => {
        assignStreamToVideoElement(element, streamRef.current, false, 0);
      });
    }, [audioEnabled, volume]);

    useWebRTCStats(connected, pcRef, onStatsRef);

    useEffect(() => {
      return () => {
        closePeer("session ended", true);
      };
    }, [closePeer]);

    const { releasePressedKeys, handleKeyDown, handleKeyUp, handleMouseMove,
      handleMouseDown, handleMouseUp, handleWheel,
    } = useWebRTCInput({ dcRef, connected, stitchedLayout, stitchedStageRef, videoRef });

    const nativeScaling = scalingMode === "native";

    return (
      <div
        ref={containerRef}
        className={`vncContainer${connected ? " vncConnected" : ""}${nativeScaling ? " vncNative" : ""}`}
        data-webrtc-video-tracks={streamTrackCounts.video}
        data-webrtc-audio-tracks={streamTrackCounts.audio}
        tabIndex={0}
        onClick={() => containerRef.current?.focus()}
        onBlur={releasePressedKeys}
        onKeyDown={handleKeyDown}
        onKeyUp={handleKeyUp}
        onMouseMove={handleMouseMove}
        onMouseDown={handleMouseDown}
        onMouseUp={handleMouseUp}
        onWheel={handleWheel}
        onContextMenu={(event) => event.preventDefault()}
      >
        <WebRTCVideoSurface
          connected={connected}
          scalingMode={scalingMode}
          stitchedLayout={stitchedLayout}
          videoRef={videoRef}
          stitchedStageRef={stitchedStageRef}
          stitchedVideoRefs={stitchedVideoRefs}
          streamRef={streamRef}
        />
      </div>
    );
  },
);

export default WebRTCViewer;

function iceCandidateSendDelay(candidate: string | null | undefined): number {
  switch (parseICECandidateType(candidate)) {
    case "relay":
      return 300;
    case "srflx":
    case "prflx":
      return 150;
    default:
      return 0;
  }
}

function parseICECandidateType(candidate: string | null | undefined): string {
  if (!candidate) {
    return "";
  }
  const parts = candidate.trim().split(/\s+/);
  for (let i = 0; i < parts.length - 1; i += 1) {
    if (parts[i] === "typ") {
      return parts[i + 1]?.toLowerCase() ?? "";
    }
  }
  return "";
}
