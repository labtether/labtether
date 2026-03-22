"use client";

import { useCallback, useEffect, useRef } from "react";

/**
 * Manages a hidden <input> element that triggers the mobile on-screen keyboard
 * when focused. Translates keydown/keyup events from the input into X11 keysym
 * calls via the optional `onKeyEvent` callback.
 *
 * Only activates on touch-capable devices. On desktop the hook is a no-op and
 * `isTouchDevice` is false.
 *
 * Usage:
 *   const { isTouchDevice, toggle } = useVirtualKeyboard(sendKey);
 *   // Pass `toggle` to RemoteViewToolbar's `onToggleVirtualKeyboard` prop.
 *   // Pass `isTouchDevice` to RemoteViewToolbar's `isTouchDevice` prop.
 */
export function useVirtualKeyboard(
  onKeyEvent?: (keysym: number, down: boolean) => void,
) {
  const isTouchDevice =
    typeof window !== "undefined" && "ontouchstart" in window;

  // Keep the callback in a ref so the effect never needs to re-run when the
  // caller passes a new function reference on each render (same pattern as
  // useTouchGestures in this codebase).
  const onKeyEventRef = useRef(onKeyEvent);
  useEffect(() => {
    onKeyEventRef.current = onKeyEvent;
  });

  const inputRef = useRef<HTMLInputElement | null>(null);

  // Create, mount, and attach event listeners to the hidden input element.
  // Cleaned up on unmount or when `isTouchDevice` changes (which is never in
  // practice — the value is stable for the lifetime of the page load).
  useEffect(() => {
    if (!isTouchDevice) return;

    const input = document.createElement("input");
    input.type = "text";
    input.autocomplete = "off";
    input.setAttribute("autocapitalize", "off");
    input.setAttribute("autocorrect", "off");
    // Position off-screen so it is invisible but still focusable.
    input.style.cssText =
      "position:fixed;left:-9999px;top:-9999px;opacity:0;width:1px;height:1px;";
    document.body.appendChild(input);
    inputRef.current = input;

    const handleKeyDown = (e: KeyboardEvent) => {
      e.preventDefault();
      onKeyEventRef.current?.(e.keyCode, true);
    };

    const handleKeyUp = (e: KeyboardEvent) => {
      e.preventDefault();
      onKeyEventRef.current?.(e.keyCode, false);
    };

    // Prevent value from accumulating — we only care about key events.
    const handleInput = () => {
      if (inputRef.current) inputRef.current.value = "";
    };

    input.addEventListener("keydown", handleKeyDown);
    input.addEventListener("keyup", handleKeyUp);
    input.addEventListener("input", handleInput);

    return () => {
      input.removeEventListener("keydown", handleKeyDown);
      input.removeEventListener("keyup", handleKeyUp);
      input.removeEventListener("input", handleInput);
      input.remove();
      inputRef.current = null;
    };
  }, [isTouchDevice]);

  /**
   * Toggle the on-screen keyboard.
   *
   * Focuses the hidden input to raise the keyboard, or blurs it to dismiss.
   */
  const toggle = useCallback(() => {
    const input = inputRef.current;
    if (!input) return;
    if (document.activeElement === input) {
      input.blur();
    } else {
      input.focus();
    }
  }, []);

  return { isTouchDevice, toggle };
}
