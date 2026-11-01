"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";

export const AUTOHIDE_DELAY_MS = 2000;
const HOT_ZONE_PX = 64;

/**
 * The full-screen toolbar hides 2 s after the pointer stops moving over the screen, unless a menu of it is open or a
 * keyboard user is in it (focus left there by a mouse click, as when a menu returns focus, does not count). Moving the pointer to its edge of the screen, or using the handle, brings it back. Off outside full
 * screen (the docked toolbar always shows) and when the person turns hiding off.
 */
export function useToolbarAutoHide({
  enabled,
  busy,
  position,
  toolbarRef,
}: {
  enabled: boolean;
  busy: boolean;
  position: "top" | "bottom";
  toolbarRef: RefObject<HTMLElement | null>;
}) {
  const [visible, setVisible] = useState(true);
  const visibleRef = useRef(true);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const busyRef = useRef(busy);
  busyRef.current = busy;
  // Whether the person last used the keyboard (a key) or the pointer (a press or a move over the screen).
  const keyboardRef = useRef(false);

  const show = useCallback((next: boolean) => {
    visibleRef.current = next;
    setVisible(next);
  }, []);

  const schedule = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;
    if (!enabled) return;
    const hide = () => {
      const bar = toolbarRef.current;
      const held = busyRef.current || (keyboardRef.current && !!bar && bar.contains(document.activeElement));
      if (held) {
        timerRef.current = setTimeout(hide, AUTOHIDE_DELAY_MS);
        return;
      }
      timerRef.current = null;
      show(false);
    };
    timerRef.current = setTimeout(hide, AUTOHIDE_DELAY_MS);
  }, [enabled, show, toolbarRef]);

  const reveal = useCallback(() => {
    show(true);
    schedule();
  }, [schedule, show]);

  useEffect(() => {
    show(true);
    schedule();
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = null;
    };
  }, [schedule, show]);

  // The pointer moving over the screen keeps the toolbar up; while it is hidden, only its edge of the screen wakes it.
  useEffect(() => {
    if (!enabled) return undefined;
    const screen = toolbarRef.current?.parentElement;
    if (!screen) return undefined;
    const onKey = () => {
      keyboardRef.current = true;
    };
    const onPress = () => {
      keyboardRef.current = false;
    };
    const onMove = (event: MouseEvent) => {
      keyboardRef.current = false;
      const rect = screen.getBoundingClientRect();
      const y = event.clientY - rect.top;
      const nearEdge = position === "top" ? y <= HOT_ZONE_PX : y >= rect.height - HOT_ZONE_PX;
      if (!visibleRef.current && !nearEdge) return;
      if (!visibleRef.current) show(true);
      schedule();
    };
    screen.addEventListener("mousemove", onMove);
    document.addEventListener("keydown", onKey, true);
    document.addEventListener("pointerdown", onPress, true);
    return () => {
      screen.removeEventListener("mousemove", onMove);
      document.removeEventListener("keydown", onKey, true);
      document.removeEventListener("pointerdown", onPress, true);
    };
  }, [enabled, position, schedule, show, toolbarRef]);

  return { visible: visible || !enabled, reveal };
}
