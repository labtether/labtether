"use client";

import { useCallback, useEffect, useRef, type RefObject } from "react";
import type { normalizeDisplayLayout } from "./webrtcMediaLayout";
import { remotePointerCoordinates } from "./webrtcPointerCoordinates";

type WebRTCInputOptions = {
  dcRef: RefObject<RTCDataChannel | null>;
  connected: boolean;
  scalingMode: "fit" | "fill" | "native";
  stitchedLayout: ReturnType<typeof normalizeDisplayLayout>;
  stitchedStageRef: RefObject<HTMLDivElement | null>;
  videoRef: RefObject<HTMLVideoElement | null>;
};

export function useWebRTCInput({ dcRef, connected, scalingMode, stitchedLayout, stitchedStageRef, videoRef }: WebRTCInputOptions) {
  const pressedKeysRef = useRef(new Map<string, { keyCode: number; code: string; key: string }>());
  const pressedButtonsRef = useRef(new Set<number>());
  const activePointerRef = useRef<number | null>(null);
  const sendInput = useCallback((payload: Record<string, unknown>) => {
    const dc = dcRef.current;
    if (!dc || dc.readyState !== "open") return;
    try { dc.send(JSON.stringify(payload)); } catch { /* A closing channel cannot accept input. */ }
  }, [dcRef]);

  const updatePointerButtons = useCallback((buttons: number) => {
    for (const [button, mask] of [[0, 1], [1, 4], [2, 2]]) {
      const down = (buttons & mask) !== 0;
      if (down === pressedButtonsRef.current.has(button)) continue;
      if (down) pressedButtonsRef.current.add(button);
      else pressedButtonsRef.current.delete(button);
      sendInput({ type: down ? "mousedown" : "mouseup", button });
    }
  }, [sendInput]);

  const releasePointer = useCallback(() => {
    for (const button of pressedButtonsRef.current) sendInput({ type: "mouseup", button });
    pressedButtonsRef.current.clear();
    activePointerRef.current = null;
  }, [sendInput]);

  const releasePressedInputs = useCallback(() => {
    for (const payload of pressedKeysRef.current.values()) sendInput({ type: "keyup", ...payload });
    pressedKeysRef.current.clear();
    releasePointer();
  }, [releasePointer, sendInput]);

  const handleKeyDown = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    event.preventDefault();
    const keyID = event.code.trim() || `keyCode:${event.keyCode}:${event.key}`;
    if (event.repeat && pressedKeysRef.current.has(keyID)) return;
    const payload = { keyCode: event.keyCode, code: event.code, key: event.key };
    pressedKeysRef.current.set(keyID, payload);
    sendInput({ type: "keydown", ...payload });
  }, [sendInput]);

  const handleKeyUp = useCallback((event: React.KeyboardEvent<HTMLDivElement>) => {
    event.preventDefault();
    pressedKeysRef.current.delete(event.code.trim() || `keyCode:${event.keyCode}:${event.key}`);
    sendInput({ type: "keyup", keyCode: event.keyCode, code: event.code, key: event.key });
  }, [sendInput]);

  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.visibilityState !== "visible") releasePressedInputs();
    };
    window.addEventListener("blur", releasePressedInputs);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      window.removeEventListener("blur", releasePressedInputs);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      releasePressedInputs();
    };
  }, [releasePressedInputs]);

  useEffect(() => {
    if (!connected) {
      pressedKeysRef.current.clear();
      pressedButtonsRef.current.clear();
      activePointerRef.current = null;
    }
  }, [connected]);

  const movePointer = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    const video = videoRef.current;
    const rect = stitchedLayout ? stitchedStageRef.current?.getBoundingClientRect() : video?.getBoundingClientRect();
    if (!rect) return;
    const coordinates = remotePointerCoordinates(event.clientX, event.clientY, rect,
      video?.videoWidth ?? 0,
      video?.videoHeight ?? 0,
      stitchedLayout ? "stretch" : scalingMode);
    if (coordinates) sendInput({ type: "mousemove", ...coordinates });
  }, [scalingMode, sendInput, stitchedLayout, stitchedStageRef, videoRef]);

  const handlePointerMove = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (!event.isPrimary || (activePointerRef.current !== null && activePointerRef.current !== event.pointerId)) return;
    event.preventDefault();
    movePointer(event);
    // Mouse button chords arrive as pointermove, not another pointerdown/up.
    updatePointerButtons(event.buttons);
  }, [movePointer, updatePointerButtons]);

  const handlePointerDown = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (!event.isPrimary || (activePointerRef.current !== null && activePointerRef.current !== event.pointerId)) return;
    event.preventDefault();
    event.currentTarget.focus();
    activePointerRef.current = event.pointerId;
    try { event.currentTarget.setPointerCapture(event.pointerId); } catch { /* Pointer may have ended. */ }
    // A touch tap has no preceding mousemove. Place the cursor before pressing.
    movePointer(event);
    updatePointerButtons(event.buttons);
  }, [movePointer, updatePointerButtons]);

  const handlePointerUp = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (activePointerRef.current !== event.pointerId) return;
    event.preventDefault();
    movePointer(event);
    updatePointerButtons(event.buttons);
    if (event.buttons === 0) {
      releasePointer();
      try { event.currentTarget.releasePointerCapture(event.pointerId); } catch { /* Already released. */ }
    }
  }, [movePointer, releasePointer, updatePointerButtons]);

  const handlePointerCancel = useCallback((event: React.PointerEvent<HTMLDivElement>) => {
    if (activePointerRef.current === event.pointerId) releasePointer();
  }, [releasePointer]);

  const handleWheel = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    event.preventDefault();
    sendInput({ type: "scroll", deltaY: Math.round(event.deltaY) });
  }, [sendInput]);

  return { releasePressedInputs, handleKeyDown, handleKeyUp, handlePointerMove,
    handlePointerDown, handlePointerUp, handlePointerCancel, handleWheel };
}
