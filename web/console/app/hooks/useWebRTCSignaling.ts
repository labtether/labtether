"use client";

import { useCallback, useEffect, useRef, useState } from "react";

interface SignalingMessage {
  type: string;
  data: unknown;
}

export function useWebRTCSignaling(wsUrl: string | null) {
  const wsRef = useRef<WebSocket | null>(null);
  const [ready, setReady] = useState(false);
  const handlersRef = useRef<Map<string, (data: unknown) => void>>(new Map());

  useEffect(() => {
    if (!wsUrl) {
      setReady(false);
      return;
    }

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setReady(true);
    };
    ws.onclose = () => {
      setReady(false);
    };
    ws.onerror = () => {
      setReady(false);
    };
    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(String(event.data)) as SignalingMessage;
        handlersRef.current.get(msg.type)?.(msg.data);
      } catch {
        // Ignore malformed signaling frames.
      }
    };

    return () => {
      ws.close();
      wsRef.current = null;
      setReady(false);
    };
  }, [wsUrl]);

  const send = useCallback((type: string, data: unknown) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      return;
    }
    ws.send(JSON.stringify({ type, data }));
  }, []);

  const on = useCallback((type: string, handler: (data: unknown) => void) => {
    handlersRef.current.set(type, handler);
    return () => {
      handlersRef.current.delete(type);
    };
  }, []);

  return { ready, send, on };
}
