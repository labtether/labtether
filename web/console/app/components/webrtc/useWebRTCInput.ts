"use client";

import { useCallback, useEffect, useRef, type RefObject } from "react";
import type { normalizeDisplayLayout } from "./webrtcMediaLayout";

type WebRTCInputOptions = {
  dcRef: RefObject<RTCDataChannel | null>;
  connected: boolean;
  stitchedLayout: ReturnType<typeof normalizeDisplayLayout>;
  stitchedStageRef: RefObject<HTMLDivElement | null>;
  videoRef: RefObject<HTMLVideoElement | null>;
};

export function useWebRTCInput({ dcRef, connected, stitchedLayout, stitchedStageRef, videoRef }: WebRTCInputOptions) {
  const pressedKeysRef = useRef(
    new Map<string, { keyCode: number; code: string; key: string }>(),
  );
  const sendInput = useCallback((payload: Record<string, unknown>) => {
    const dc = dcRef.current;
    if (!dc || dc.readyState !== "open") {
      return;
    }
    dc.send(JSON.stringify(payload));
  }, [dcRef]);

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
    [sendInput, stitchedLayout, stitchedStageRef, videoRef],
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

  return { releasePressedKeys, handleKeyDown, handleKeyUp, handleMouseMove,
    handleMouseDown, handleMouseUp, handleWheel };
}
