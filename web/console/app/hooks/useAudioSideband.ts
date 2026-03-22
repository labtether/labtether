"use client";

import { useEffect, useRef, useState } from "react";

type AudioSidebandStatus =
  | "idle"
  | "connecting"
  | "playing"
  | "unavailable"
  | "error";

interface UseAudioSidebandOptions {
  wsUrl: string | null;
  enabled: boolean;
  muted: boolean;
  volume: number;
}

interface AudioSidebandState {
  status: AudioSidebandStatus;
  error: string | null;
}

const audioMimeType = "audio/ogg; codecs=opus";

export function useAudioSideband({
  wsUrl,
  enabled,
  muted,
  volume,
}: UseAudioSidebandOptions): AudioSidebandState {
  const [state, setState] = useState<AudioSidebandState>({
    status: "idle",
    error: null,
  });
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const mediaSourceRef = useRef<MediaSource | null>(null);
  const sourceBufferRef = useRef<SourceBuffer | null>(null);
  const queueRef = useRef<ArrayBuffer[]>([]);
  const objectUrlRef = useRef<string | null>(null);
  const mutedRef = useRef(muted);
  const volumeRef = useRef(volume);

  useEffect(() => {
    mutedRef.current = muted;
    volumeRef.current = volume;
    if (audioRef.current) {
      audioRef.current.muted = muted;
      audioRef.current.volume = Math.max(0, Math.min(1, volume));
    }
  }, [muted, volume]);

  useEffect(() => {
    if (!enabled || !wsUrl) {
      setState({ status: "idle", error: null });
      return;
    }
    if (
      typeof window === "undefined" ||
      typeof MediaSource === "undefined" ||
      !MediaSource.isTypeSupported(audioMimeType)
    ) {
      setState({
        status: "unavailable",
        error: "Browser does not support streamed Opus playback.",
      });
      return;
    }

    let cancelled = false;
    let ws: WebSocket | null = null;
    let hadSocketError = false;

    const flushQueue = () => {
      const sourceBuffer = sourceBufferRef.current;
      if (
        !sourceBuffer ||
        sourceBuffer.updating ||
        queueRef.current.length === 0
      ) {
        return;
      }
      sourceBuffer.appendBuffer(queueRef.current.shift()!);
    };

    const cleanup = () => {
      ws?.close();
      ws = null;
      queueRef.current = [];
      sourceBufferRef.current = null;
      if (audioRef.current) {
        audioRef.current.pause();
        audioRef.current.removeAttribute("src");
        audioRef.current.load();
        audioRef.current = null;
      }
      if (mediaSourceRef.current?.readyState === "open") {
        try {
          mediaSourceRef.current.endOfStream();
        } catch {
          // noop
        }
      }
      mediaSourceRef.current = null;
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current);
        objectUrlRef.current = null;
      }
    };

    const audio = new Audio();
    audio.autoplay = true;
    audio.preload = "auto";
    audio.muted = mutedRef.current;
    audio.volume = Math.max(0, Math.min(1, volumeRef.current));
    audioRef.current = audio;

    const mediaSource = new MediaSource();
    mediaSourceRef.current = mediaSource;
    objectUrlRef.current = URL.createObjectURL(mediaSource);
    audio.src = objectUrlRef.current;

    const handleSourceOpen = () => {
      if (cancelled || sourceBufferRef.current) {
        return;
      }
      const sourceBuffer = mediaSource.addSourceBuffer(audioMimeType);
      sourceBuffer.mode = "sequence";
      sourceBuffer.addEventListener("updateend", flushQueue);
      sourceBufferRef.current = sourceBuffer;
      flushQueue();
    };
    mediaSource.addEventListener("sourceopen", handleSourceOpen);

    setState({ status: "connecting", error: null });
    ws = new WebSocket(wsUrl);
    ws.binaryType = "arraybuffer";

    ws.onopen = () => {
      void audio.play().catch(() => {
        // Browser autoplay policies may require a later user gesture.
      });
    };

    ws.onmessage = async (event) => {
      if (cancelled) {
        return;
      }
      if (typeof event.data === "string") {
        try {
          const payload = JSON.parse(event.data) as {
            state?: string;
            error?: string;
          };
          const nextState = payload.state?.trim().toLowerCase();
          if (nextState === "unavailable") {
            setState({
              status: "unavailable",
              error: payload.error?.trim() || "Desktop audio unavailable.",
            });
            ws?.close();
            return;
          }
          if (nextState === "started") {
            setState({ status: "playing", error: null });
          }
          if (nextState === "stopped" && !cancelled) {
            setState((current) => ({
              status:
                current.status === "unavailable" ? current.status : "idle",
              error: current.error,
            }));
          }
        } catch {
          // Ignore malformed state payloads.
        }
        return;
      }

      const chunk =
        event.data instanceof ArrayBuffer
          ? event.data
          : await event.data.arrayBuffer();
      if (chunk.byteLength === 0) {
        return;
      }
      queueRef.current.push(chunk);
      flushQueue();
      setState((current) =>
        current.status === "playing" && current.error === null
          ? current
          : { status: "playing", error: null },
      );
      void audio.play().catch(() => {
        // ignore autoplay failures
      });
    };

    ws.onerror = () => {
      if (!cancelled) {
        hadSocketError = true;
        setState({ status: "error", error: "Desktop audio stream failed." });
      }
    };

    ws.onclose = () => {
      if (!cancelled) {
        setState((current) =>
          current.status === "unavailable" || hadSocketError
            ? current
            : { status: "idle", error: current.error },
        );
      }
    };

    return () => {
      cancelled = true;
      mediaSource.removeEventListener("sourceopen", handleSourceOpen);
      cleanup();
    };
  }, [enabled, wsUrl]);

  return state;
}

export type { AudioSidebandStatus };
