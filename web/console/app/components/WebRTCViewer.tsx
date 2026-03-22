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

export interface WebRTCConnectionStats {
  rttMs: number | null;
  packetsLost: number | null;
  bitrateKbps: number | null;
  fps: number | null;
  routeClass: "direct" | "reflexive" | "relay" | null;
}

export interface WebRTCDisplayLayout {
  name: string;
  width: number;
  height: number;
  primary: boolean;
  offset_x: number;
  offset_y: number;
}

interface WebRTCViewerProps {
  wsUrl: string | null;
  onConnect?: () => void;
  onDisconnect?: (detail: { clean: boolean; reason?: string }) => void;
  scalingMode?: "fit" | "native" | "fill";
  audioEnabled?: boolean;
  volume?: number;
  onStats?: (stats: WebRTCConnectionStats) => void;
  onStream?: (stream: MediaStream | null) => void;
  displayLayout?: WebRTCDisplayLayout[];
}

export interface WebRTCViewerHandle {
  disconnect: () => void;
  sendCtrlAltDel: () => void;
  sendKey: (keysym: number, down: boolean) => void;
  focus: () => void;
  setVolume: (volume: number) => void;
  requestClipboardText: () => Promise<string>;
  writeClipboardText: (text: string) => Promise<void>;
  uploadFile: (
    file: File,
    targetPath: string,
    onProgress?: (loaded: number, total: number) => void,
  ) => Promise<void>;
}

interface ClipboardChannelMessage {
  type?: string;
  format?: string;
  text?: string;
  error?: string;
}

interface FileTransferChannelMessage {
  type?: string;
  request_id?: string;
  path?: string;
  data?: string;
  done?: boolean;
  bytes_written?: number;
  error?: string;
}

type PendingClipboardRequest =
  | {
      resolve: (value: string) => void;
      reject: (reason?: unknown) => void;
      mode: "get";
    }
  | {
      resolve: () => void;
      reject: (reason?: unknown) => void;
      mode: "set";
    };

function clampVolume(value: number): number {
  if (Number.isNaN(value)) return 1;
  if (value < 0) return 0;
  if (value > 1) return 1;
  return value;
}

function assignStreamToVideoElement(
  element: HTMLVideoElement | null,
  stream: MediaStream | null,
  audioEnabled: boolean,
  volume: number,
) {
  if (!element) {
    return;
  }
  element.srcObject = stream;
  element.muted = !audioEnabled;
  element.volume = clampVolume(volume);
}

function normalizeDisplayLayout(
  displayLayout: WebRTCDisplayLayout[] | undefined,
): {
  displays: Array<
    WebRTCDisplayLayout & {
      leftPct: number;
      topPct: number;
      widthPct: number;
      heightPct: number;
    }
  >;
  totalWidth: number;
  totalHeight: number;
} | null {
  if (!displayLayout || displayLayout.length < 2) {
    return null;
  }

  const displays = displayLayout.filter(
    (entry) => entry.width > 0 && entry.height > 0,
  );
  if (displays.length < 2) {
    return null;
  }

  const minX = Math.min(...displays.map((entry) => entry.offset_x));
  const minY = Math.min(...displays.map((entry) => entry.offset_y));
  const maxX = Math.max(
    ...displays.map((entry) => entry.offset_x + entry.width),
  );
  const maxY = Math.max(
    ...displays.map((entry) => entry.offset_y + entry.height),
  );
  const totalWidth = maxX - minX;
  const totalHeight = maxY - minY;

  if (totalWidth <= 0 || totalHeight <= 0) {
    return null;
  }

  return {
    totalWidth,
    totalHeight,
    displays: displays.map((entry) => ({
      ...entry,
      leftPct: ((entry.offset_x - minX) / totalWidth) * 100,
      topPct: ((entry.offset_y - minY) / totalHeight) * 100,
      widthPct: (entry.width / totalWidth) * 100,
      heightPct: (entry.height / totalHeight) * 100,
    })),
  };
}

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
    const clipboardDCRef = useRef<RTCDataChannel | null>(null);
    const fileDCRef = useRef<RTCDataChannel | null>(null);
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
    const pendingClipboardRef = useRef<PendingClipboardRequest | null>(null);
    const fileTransferWaitersRef = useRef(
      new Map<
        string,
        {
          resolve: (value: FileTransferChannelMessage) => void;
          reject: (reason?: unknown) => void;
        }
      >(),
    );
    const pressedKeysRef = useRef(
      new Map<string, { keyCode: number; code: string; key: string }>(),
    );
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

    const closePeer = useCallback((reason: string, clean = false) => {
      candidateTimerRefs.current.forEach((timer) => clearTimeout(timer));
      candidateTimerRefs.current = [];
      const hadConnection = Boolean(
        pcRef.current ||
        dcRef.current ||
        clipboardDCRef.current ||
        fileDCRef.current ||
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
      clipboardDCRef.current = null;
      fileDCRef.current = null;
      if (pendingClipboardRef.current) {
        pendingClipboardRef.current.reject(new Error(reason));
        pendingClipboardRef.current = null;
      }
      fileTransferWaitersRef.current.forEach(({ reject }) =>
        reject(new Error(reason)),
      );
      fileTransferWaitersRef.current.clear();
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
    }, []);

    const withOpenChannel = useCallback(
      (channel: RTCDataChannel | null, label: string) => {
        if (!channel || channel.readyState !== "open") {
          throw new Error(`${label} channel unavailable`);
        }
        return channel;
      },
      [],
    );

    const requestClipboardText = useCallback(async () => {
      const channel = withOpenChannel(clipboardDCRef.current, "clipboard");
      if (pendingClipboardRef.current) {
        throw new Error("clipboard request already in progress");
      }
      return new Promise<string>((resolve, reject) => {
        pendingClipboardRef.current = { resolve, reject, mode: "get" };
        channel.send(JSON.stringify({ type: "get", format: "text" }));
      });
    }, [withOpenChannel]);

    const writeClipboardText = useCallback(
      async (text: string) => {
        const channel = withOpenChannel(clipboardDCRef.current, "clipboard");
        if (pendingClipboardRef.current) {
          throw new Error("clipboard request already in progress");
        }
        return new Promise<void>((resolve, reject) => {
          pendingClipboardRef.current = { resolve, reject, mode: "set" };
          channel.send(JSON.stringify({ type: "set", format: "text", text }));
        });
      },
      [withOpenChannel],
    );

    const waitForFileTransferResponse = useCallback((requestID: string) => {
      return new Promise<FileTransferChannelMessage>((resolve, reject) => {
        fileTransferWaitersRef.current.set(requestID, { resolve, reject });
      });
    }, []);

    const uploadFile = useCallback(
      async (
        file: File,
        targetPath: string,
        onProgress?: (loaded: number, total: number) => void,
      ) => {
        const channel = withOpenChannel(fileDCRef.current, "file transfer");
        const requestID = createRequestID();
        const readyPromise = waitForFileTransferResponse(requestID);
        channel.send(
          JSON.stringify({
            type: "start",
            request_id: requestID,
            name: file.name,
            path: targetPath,
          }),
        );
        const ready = await readyPromise;
        if (ready.type === "error") {
          throw new Error(ready.error || "file transfer failed");
        }

        const chunkSize = 64 * 1024;
        let offset = 0;
        while (offset < file.size) {
          while (channel.bufferedAmount > 256 * 1024) {
            await new Promise((resolve) => window.setTimeout(resolve, 25));
          }
          const chunk = await file
            .slice(offset, offset + chunkSize)
            .arrayBuffer();
          const loaded = Math.min(file.size, offset + chunk.byteLength);
          const ackPromise = waitForFileTransferResponse(requestID);
          channel.send(
            JSON.stringify({
              type: "chunk",
              request_id: requestID,
              path: targetPath,
              data: arrayBufferToBase64(chunk),
              done: loaded >= file.size,
            }),
          );
          const ack = await ackPromise;
          if (ack.type === "error") {
            throw new Error(ack.error || "file transfer failed");
          }
          offset = loaded;
          onProgress?.(loaded, file.size);
        }
      },
      [waitForFileTransferResponse, withOpenChannel],
    );

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
        clipboardDCRef.current = pc.createDataChannel("clipboard", {
          ordered: true,
        });
        fileDCRef.current = pc.createDataChannel("file-transfer", {
          ordered: true,
        });

        clipboardDCRef.current.onmessage = (event) => {
          try {
            const payload = JSON.parse(
              String(event.data),
            ) as ClipboardChannelMessage;
            const pending = pendingClipboardRef.current;
            if (!pending) {
              return;
            }
            if (payload.type === "error") {
              pending.reject(
                new Error(payload.error || "clipboard request failed"),
              );
            } else if (pending.mode === "get" && payload.type === "data") {
              pending.resolve(payload.text || "");
            } else if (pending.mode === "set" && payload.type === "ack") {
              pending.resolve();
            } else {
              return;
            }
            pendingClipboardRef.current = null;
          } catch {
            // Ignore malformed clipboard payloads.
          }
        };

        fileDCRef.current.onmessage = (event) => {
          try {
            const payload = JSON.parse(
              String(event.data),
            ) as FileTransferChannelMessage;
            const requestID = payload.request_id?.trim();
            if (!requestID) {
              return;
            }
            const waiter = fileTransferWaitersRef.current.get(requestID);
            if (!waiter) {
              return;
            }
            fileTransferWaitersRef.current.delete(requestID);
            if (payload.type === "error") {
              waiter.reject(new Error(payload.error || "file transfer failed"));
              return;
            }
            waiter.resolve(payload);
          } catch {
            // Ignore malformed file transfer payloads.
          }
        };

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
    }, [closePeer, on, send, wsUrl]);

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

    useEffect(() => {
      if (!connected || !pcRef.current) {
        return;
      }

      let cancelled = false;
      let lastBytes = 0;
      let lastAt = 0;

      const collect = async () => {
        const pc = pcRef.current;
        if (!pc || cancelled) {
          return;
        }
        try {
          const reports = await pc.getStats();
          let rttMs: number | null = null;
          let packetsLost: number | null = null;
          let bitrateKbps: number | null = null;
          let fps: number | null = null;
          let selectedCandidatePairID: string | null = null;
          let selectedLocalCandidateID: string | null = null;
          let selectedRemoteCandidateID: string | null = null;
          const localCandidateTypes = new Map<string, string>();
          const remoteCandidateTypes = new Map<string, string>();

          reports.forEach((report) => {
            const typed = report as RTCStats & {
              kind?: string;
              roundTripTime?: number;
              currentRoundTripTime?: number;
              packetsLost?: number;
              framesPerSecond?: number;
              bytesReceived?: number;
              selectedCandidatePairId?: string;
              selected?: boolean;
              localCandidateId?: string;
              remoteCandidateId?: string;
              candidateType?: string;
            };
            if (
              report.type === "transport" &&
              typeof typed.selectedCandidatePairId === "string"
            ) {
              selectedCandidatePairID = typed.selectedCandidatePairId;
            }
            if (
              report.type === "remote-inbound-rtp" &&
              typed.kind === "video"
            ) {
              if (typeof typed.roundTripTime === "number") {
                rttMs = Math.round(typed.roundTripTime * 1000);
              }
            }
            if (report.type === "inbound-rtp" && typed.kind === "video") {
              if (typeof typed.packetsLost === "number") {
                packetsLost = typed.packetsLost;
              }
              if (typeof typed.framesPerSecond === "number") {
                fps = Math.round(typed.framesPerSecond);
              }
              if (typeof typed.bytesReceived === "number") {
                const now = Date.now();
                if (lastAt > 0 && now > lastAt) {
                  const deltaBytes = typed.bytesReceived - lastBytes;
                  const deltaMs = now - lastAt;
                  if (deltaBytes >= 0 && deltaMs > 0) {
                    bitrateKbps = Math.round((deltaBytes * 8) / deltaMs);
                  }
                }
                lastBytes = typed.bytesReceived;
                lastAt = now;
              }
            }
            if (
              report.type === "candidate-pair" &&
              (typed.selected === true || report.id === selectedCandidatePairID)
            ) {
              selectedCandidatePairID = report.id;
              selectedLocalCandidateID =
                typeof typed.localCandidateId === "string"
                  ? typed.localCandidateId
                  : null;
              selectedRemoteCandidateID =
                typeof typed.remoteCandidateId === "string"
                  ? typed.remoteCandidateId
                  : null;
              if (
                rttMs === null &&
                typeof typed.currentRoundTripTime === "number"
              ) {
                rttMs = Math.round(typed.currentRoundTripTime * 1000);
              }
            }
            if (
              report.type === "local-candidate" &&
              typeof typed.candidateType === "string"
            ) {
              localCandidateTypes.set(report.id, typed.candidateType);
            }
            if (
              report.type === "remote-candidate" &&
              typeof typed.candidateType === "string"
            ) {
              remoteCandidateTypes.set(report.id, typed.candidateType);
            }
          });

          const localCandidateType =
            (selectedLocalCandidateID &&
              localCandidateTypes.get(selectedLocalCandidateID)) ||
            null;
          const remoteCandidateType =
            (selectedRemoteCandidateID &&
              remoteCandidateTypes.get(selectedRemoteCandidateID)) ||
            null;
          const routeClass = deriveCandidateRouteClass(
            localCandidateType,
            remoteCandidateType,
          );

          onStatsRef.current?.({
            rttMs,
            packetsLost,
            bitrateKbps,
            fps,
            routeClass,
          });
        } catch {
          // Ignore transient stats errors.
        }
      };

      void collect();
      const timer = window.setInterval(() => {
        void collect();
      }, 2000);

      return () => {
        cancelled = true;
        window.clearInterval(timer);
      };
    }, [connected]);

    useEffect(() => {
      return () => {
        closePeer("session ended", true);
      };
    }, [closePeer]);

    const sendInput = useCallback((payload: Record<string, unknown>) => {
      const dc = dcRef.current;
      if (!dc || dc.readyState !== "open") {
        return;
      }
      dc.send(JSON.stringify(payload));
    }, []);

    const releasePressedKeys = useCallback(() => {
      if (pressedKeysRef.current.size === 0) {
        return;
      }
      for (const payload of pressedKeysRef.current.values()) {
        sendInput({
          type: "keyup",
          keyCode: payload.keyCode,
          code: payload.code,
          key: payload.key,
        });
      }
      pressedKeysRef.current.clear();
    }, [sendInput]);

    const handleKeyDown = useCallback(
      (event: React.KeyboardEvent<HTMLDivElement>) => {
        event.preventDefault();
        const keyID =
          event.code.trim() || `keyCode:${event.keyCode}:${event.key}`;
        if (event.repeat && pressedKeysRef.current.has(keyID)) {
          return;
        }
        const payload = {
          type: "keydown",
          keyCode: event.keyCode,
          code: event.code,
          key: event.key,
        };
        pressedKeysRef.current.set(keyID, {
          keyCode: event.keyCode,
          code: event.code,
          key: event.key,
        });
        sendInput(payload);
      },
      [sendInput],
    );

    const handleKeyUp = useCallback(
      (event: React.KeyboardEvent<HTMLDivElement>) => {
        event.preventDefault();
        const keyID =
          event.code.trim() || `keyCode:${event.keyCode}:${event.key}`;
        pressedKeysRef.current.delete(keyID);
        sendInput({
          type: "keyup",
          keyCode: event.keyCode,
          code: event.code,
          key: event.key,
        });
      },
      [sendInput],
    );

    useEffect(() => {
      const handleWindowBlur = () => {
        releasePressedKeys();
      };
      const handleVisibilityChange = () => {
        if (document.visibilityState !== "visible") {
          releasePressedKeys();
        }
      };
      window.addEventListener("blur", handleWindowBlur);
      document.addEventListener("visibilitychange", handleVisibilityChange);
      return () => {
        window.removeEventListener("blur", handleWindowBlur);
        document.removeEventListener("visibilitychange", handleVisibilityChange);
      };
    }, [releasePressedKeys]);

    useEffect(() => {
      if (!connected) {
        pressedKeysRef.current.clear();
      }
    }, [connected]);

    const handleMouseMove = useCallback(
      (event: React.MouseEvent<HTMLDivElement>) => {
        const rect =
          stitchedLayout?.totalWidth && stitchedLayout?.totalHeight
            ? stitchedStageRef.current?.getBoundingClientRect()
            : videoRef.current?.getBoundingClientRect();
        if (!rect) {
          return;
        }
        const xRatio = Math.max(
          0,
          Math.min(1, (event.clientX - rect.left) / Math.max(rect.width, 1)),
        );
        const yRatio = Math.max(
          0,
          Math.min(1, (event.clientY - rect.top) / Math.max(rect.height, 1)),
        );
        sendInput({
          type: "mousemove",
          x: Math.round(
            xRatio * (stitchedLayout?.totalWidth ?? rect.width),
          ),
          y: Math.round(
            yRatio * (stitchedLayout?.totalHeight ?? rect.height),
          ),
        });
      },
      [sendInput, stitchedLayout],
    );

    const handleMouseDown = useCallback(
      (event: React.MouseEvent<HTMLDivElement>) => {
        sendInput({ type: "mousedown", button: event.button });
      },
      [sendInput],
    );

    const handleMouseUp = useCallback(
      (event: React.MouseEvent<HTMLDivElement>) => {
        sendInput({ type: "mouseup", button: event.button });
      },
      [sendInput],
    );

    const handleWheel = useCallback(
      (event: React.WheelEvent<HTMLDivElement>) => {
        event.preventDefault();
        sendInput({ type: "scroll", deltaY: Math.round(event.deltaY) });
      },
      [sendInput],
    );

    const nativeScaling = scalingMode === "native";
    const videoStyle: React.CSSProperties = nativeScaling
      ? {
          width: "auto",
          height: "auto",
          maxWidth: "none",
          maxHeight: "none",
          objectFit: "none",
          display: "block",
          cursor: connected ? "none" : "default",
        }
      : {
          width: "100%",
          height: "100%",
          objectFit: scalingMode === "fill" ? "cover" : "contain",
          display: "block",
          cursor: connected ? "none" : "default",
        };
    const stitchedStageStyle: React.CSSProperties | undefined =
      stitchedLayout
        ? {
            position: "relative",
            aspectRatio: nativeScaling
              ? undefined
              : `${stitchedLayout.totalWidth} / ${stitchedLayout.totalHeight}`,
            width: nativeScaling ? stitchedLayout.totalWidth : "100%",
            maxWidth: nativeScaling ? "none" : "100%",
            maxHeight: nativeScaling ? "none" : "100%",
            height: nativeScaling
              ? stitchedLayout.totalHeight
              : scalingMode === "fill"
                ? "100%"
                : "auto",
            overflow: "hidden",
            background: "var(--panel)",
          }
        : undefined;

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
        {stitchedLayout ? (
          <>
            <video
              ref={videoRef}
              style={{ display: "none" }}
              autoPlay
              playsInline
            />
            <div
              ref={stitchedStageRef}
              data-webrtc-layout="stitched"
              style={stitchedStageStyle}
            >
              {stitchedLayout.displays.map((display, index) => (
                <div
                  key={display.name}
                  data-webrtc-display={display.name}
                  style={{
                    position: "absolute",
                    left: `${display.leftPct}%`,
                    top: `${display.topPct}%`,
                    width: `${display.widthPct}%`,
                    height: `${display.heightPct}%`,
                    overflow: "hidden",
                    border: "1px solid rgba(255,255,255,0.08)",
                    borderRadius: "12px",
                    boxShadow: "0 10px 24px rgba(0,0,0,0.22)",
                  }}
                >
                  <video
                    ref={(node) => {
                      stitchedVideoRefs.current[index] = node;
                      assignStreamToVideoElement(node, streamRef.current, false, 0);
                    }}
                    style={{
                      position: "absolute",
                      left: `${-((display.leftPct / Math.max(display.widthPct, 0.01)) * 100)}%`,
                      top: `${-((display.topPct / Math.max(display.heightPct, 0.01)) * 100)}%`,
                      width: `${(100 / Math.max(display.widthPct, 0.01)) * 100}%`,
                      height: `${(100 / Math.max(display.heightPct, 0.01)) * 100}%`,
                      objectFit: "fill",
                      cursor: connected ? "none" : "default",
                    }}
                    autoPlay
                    playsInline
                  />
                  <div
                    style={{
                      position: "absolute",
                      left: 10,
                      top: 10,
                      padding: "4px 8px",
                      borderRadius: 999,
                      background: "rgba(7, 10, 18, 0.72)",
                      color: "rgba(255,255,255,0.92)",
                      fontSize: 12,
                      fontWeight: 600,
                      letterSpacing: 0.2,
                    }}
                  >
                    {display.name}
                  </div>
                </div>
              ))}
            </div>
          </>
        ) : (
          <video ref={videoRef} style={videoStyle} autoPlay playsInline />
        )}
      </div>
    );
  },
);

export default WebRTCViewer;

function createRequestID(): string {
  if (
    typeof crypto !== "undefined" &&
    typeof crypto.randomUUID === "function"
  ) {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

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

function deriveCandidateRouteClass(
  localCandidateType: string | null,
  remoteCandidateType: string | null,
): "direct" | "reflexive" | "relay" | null {
  const candidates = [localCandidateType, remoteCandidateType]
    .map((value) => value?.trim().toLowerCase() ?? "")
    .filter(Boolean);

  if (candidates.length === 0) {
    return null;
  }
  if (candidates.includes("relay")) {
    return "relay";
  }
  if (candidates.includes("srflx") || candidates.includes("prflx")) {
    return "reflexive";
  }
  if (candidates.includes("host")) {
    return "direct";
  }
  return null;
}
