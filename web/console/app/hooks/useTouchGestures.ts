"use client";

import { useEffect, useRef } from "react";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DOUBLE_TAP_MS = 300;
const LONG_PRESS_MS = 500;
const TAP_MOVE_THRESHOLD = 10;

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

export interface TouchGestureCallbacks {
  onTap: (x: number, y: number) => void;
  onDoubleTap: (x: number, y: number) => void;
  onRightClick: (x: number, y: number) => void;
  onScroll: (deltaY: number) => void;
  onPinchZoom?: (scale: number) => void;
  onDrag: (x: number, y: number) => void;
}

// ---------------------------------------------------------------------------
// Internal state shape
// ---------------------------------------------------------------------------

interface GestureState {
  // Single-finger tracking
  startX: number;
  startY: number;
  lastX: number;
  lastY: number;
  isDragging: boolean;
  longPressTimer: ReturnType<typeof setTimeout> | null;

  // Double-tap tracking
  lastTapTime: number;
  lastTapX: number;
  lastTapY: number;

  // Two-finger tracking (scroll / pinch)
  prevTwoFingerY: number | null;
  prevPinchDistance: number | null;
}

function distance(t1: Touch, t2: Touch): number {
  const dx = t1.clientX - t2.clientX;
  const dy = t1.clientY - t2.clientY;
  return Math.sqrt(dx * dx + dy * dy);
}

function midpoint(t1: Touch, t2: Touch): { x: number; y: number } {
  return {
    x: (t1.clientX + t2.clientX) / 2,
    y: (t1.clientY + t2.clientY) / 2,
  };
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Maps touch events on `elementRef` to logical pointer/scroll callbacks.
 *
 * Gesture mapping:
 *   Single tap             → onTap (left click)
 *   Double tap             → onDoubleTap (double click)
 *   Long press (500 ms)    → onRightClick
 *   Three-finger tap       → onRightClick (averaged position)
 *   Single-finger drag     → onDrag (after > TAP_MOVE_THRESHOLD px)
 *   Two-finger vertical    → onScroll (deltaY between frames)
 *   Two-finger pinch       → onPinchZoom (scale ratio relative to initial)
 */
export function useTouchGestures(
  elementRef: React.RefObject<HTMLElement | null>,
  callbacks: TouchGestureCallbacks,
  enabled = true,
): void {
  // Keep callbacks in a ref so listeners never need to be re-attached when
  // the caller passes new function references on each render.
  const callbacksRef = useRef<TouchGestureCallbacks>(callbacks);
  useEffect(() => {
    callbacksRef.current = callbacks;
  });

  useEffect(() => {
    const el = elementRef.current;
    if (!el || !enabled) return;

    const state: GestureState = {
      startX: 0,
      startY: 0,
      lastX: 0,
      lastY: 0,
      isDragging: false,
      longPressTimer: null,
      lastTapTime: 0,
      lastTapX: 0,
      lastTapY: 0,
      prevTwoFingerY: null,
      prevPinchDistance: null,
    };

    // ------------------------------------------------------------------
    // Helpers
    // ------------------------------------------------------------------

    function cancelLongPress() {
      if (state.longPressTimer !== null) {
        clearTimeout(state.longPressTimer);
        state.longPressTimer = null;
      }
    }

    function resetTwoFingerState() {
      state.prevTwoFingerY = null;
      state.prevPinchDistance = null;
    }

    // ------------------------------------------------------------------
    // touchstart
    // ------------------------------------------------------------------

    function handleTouchStart(e: TouchEvent) {
      e.preventDefault();
      const touches = e.touches;

      // Three-finger tap — fire right-click immediately on start.
      if (touches.length === 3) {
        cancelLongPress();
        const avgX =
          (touches[0].clientX + touches[1].clientX + touches[2].clientX) / 3;
        const avgY =
          (touches[0].clientY + touches[1].clientY + touches[2].clientY) / 3;
        callbacksRef.current.onRightClick(avgX, avgY);
        return;
      }

      if (touches.length === 2) {
        cancelLongPress();
        const mid = midpoint(touches[0], touches[1]);
        state.prevTwoFingerY = mid.y;
        state.prevPinchDistance = distance(touches[0], touches[1]);
        return;
      }

      if (touches.length === 1) {
        resetTwoFingerState();
        const t = touches[0];
        state.startX = t.clientX;
        state.startY = t.clientY;
        state.lastX = t.clientX;
        state.lastY = t.clientY;
        state.isDragging = false;

        // Arm long-press → right-click.
        cancelLongPress();
        state.longPressTimer = setTimeout(() => {
          state.longPressTimer = null;
          // Only fire if the finger hasn't moved significantly.
          const dx = state.lastX - state.startX;
          const dy = state.lastY - state.startY;
          if (Math.sqrt(dx * dx + dy * dy) < TAP_MOVE_THRESHOLD) {
            callbacksRef.current.onRightClick(state.lastX, state.lastY);
          }
        }, LONG_PRESS_MS);
      }
    }

    // ------------------------------------------------------------------
    // touchmove
    // ------------------------------------------------------------------

    function handleTouchMove(e: TouchEvent) {
      e.preventDefault();
      const touches = e.touches;

      if (touches.length === 2) {
        cancelLongPress();

        // Scroll: track Y midpoint delta.
        const mid = midpoint(touches[0], touches[1]);
        if (state.prevTwoFingerY !== null) {
          const deltaY = state.prevTwoFingerY - mid.y;
          if (deltaY !== 0) {
            callbacksRef.current.onScroll(deltaY);
          }
        }
        state.prevTwoFingerY = mid.y;

        // Pinch: track distance delta.
        const currentDist = distance(touches[0], touches[1]);
        if (
          state.prevPinchDistance !== null &&
          state.prevPinchDistance > 0 &&
          callbacksRef.current.onPinchZoom
        ) {
          const scale = currentDist / state.prevPinchDistance;
          callbacksRef.current.onPinchZoom(scale);
        }
        state.prevPinchDistance = currentDist;
        return;
      }

      if (touches.length === 1) {
        const t = touches[0];
        const dx = t.clientX - state.startX;
        const dy = t.clientY - state.startY;
        const moved = Math.sqrt(dx * dx + dy * dy);

        if (!state.isDragging && moved > TAP_MOVE_THRESHOLD) {
          // Crossed movement threshold — cancel long-press and start drag.
          cancelLongPress();
          state.isDragging = true;
        }

        if (state.isDragging) {
          callbacksRef.current.onDrag(t.clientX, t.clientY);
        }

        state.lastX = t.clientX;
        state.lastY = t.clientY;
      }
    }

    // ------------------------------------------------------------------
    // touchend
    // ------------------------------------------------------------------

    function handleTouchEnd(e: TouchEvent) {
      e.preventDefault();

      // If any fingers remain, keep two-finger state live.
      if (e.touches.length >= 2) return;
      if (e.touches.length === 1) {
        // One finger lifted from a two-finger gesture — reset two-finger state.
        resetTwoFingerState();
        return;
      }

      // All fingers lifted.
      cancelLongPress();
      resetTwoFingerState();

      if (!state.isDragging) {
        // Evaluate tap / double-tap.
        const now = Date.now();
        const dx = state.lastX - state.lastTapX;
        const dy = state.lastY - state.lastTapY;
        const distFromLastTap = Math.sqrt(dx * dx + dy * dy);

        if (
          now - state.lastTapTime < DOUBLE_TAP_MS &&
          distFromLastTap < TAP_MOVE_THRESHOLD
        ) {
          // Double tap.
          callbacksRef.current.onDoubleTap(state.lastX, state.lastY);
          // Reset so a third tap doesn't re-trigger a double-tap.
          state.lastTapTime = 0;
        } else {
          // Single tap.
          callbacksRef.current.onTap(state.lastX, state.lastY);
          state.lastTapTime = now;
          state.lastTapX = state.lastX;
          state.lastTapY = state.lastY;
        }
      }

      state.isDragging = false;
    }

    // ------------------------------------------------------------------
    // touchcancel
    // ------------------------------------------------------------------

    function handleTouchCancel() {
      cancelLongPress();
      resetTwoFingerState();
      state.isDragging = false;
    }

    // ------------------------------------------------------------------
    // Register listeners  { passive: false } to allow preventDefault()
    // ------------------------------------------------------------------

    const opts: AddEventListenerOptions = { passive: false };
    el.addEventListener("touchstart", handleTouchStart, opts);
    el.addEventListener("touchmove", handleTouchMove, opts);
    el.addEventListener("touchend", handleTouchEnd, opts);
    el.addEventListener("touchcancel", handleTouchCancel, opts);

    return () => {
      cancelLongPress();
      el.removeEventListener("touchstart", handleTouchStart, opts);
      el.removeEventListener("touchmove", handleTouchMove, opts);
      el.removeEventListener("touchend", handleTouchEnd, opts);
      el.removeEventListener("touchcancel", handleTouchCancel, opts);
    };
  }, [elementRef, enabled]);
}
