"use client";
import { BarChart3, Eye, EyeOff, Keyboard, MousePointer } from "lucide-react";
import type { RemoteViewToolbarProps } from "./types";
import { IconButton, LabeledIconButton } from "./ToolbarButtons";

export function ToolbarPointerLock({
  pointerLockSupported = true,
  onPointerLockToggle,
  pointerLocked,
}: Pick<
  RemoteViewToolbarProps,
  "pointerLockSupported" | "onPointerLockToggle" | "pointerLocked"
>) {
  return pointerLockSupported ? (
    <LabeledIconButton
      onClick={onPointerLockToggle}
      active={pointerLocked}
      title={
        pointerLocked
          ? "Mouse ready in remote session"
          : "Send mouse to remote session"
      }
      label="Mouse"
    >
      <MousePointer className="w-3.5 h-3.5" />
    </LabeledIconButton>
  ) : null;
}

export function ToolbarViewOnly({
  onViewOnlyToggle,
  viewOnly,
}: Pick<RemoteViewToolbarProps, "onViewOnlyToggle" | "viewOnly">) {
  return (
    <IconButton
      onClick={onViewOnlyToggle}
      active={viewOnly}
      title={viewOnly ? "Enable input" : "View only"}
    >
      {viewOnly ? (
        <EyeOff className="w-3.5 h-3.5" />
      ) : (
        <Eye className="w-3.5 h-3.5" />
      )}
    </IconButton>
  );
}

export function ToolbarKeyboardGrab({
  keyboardGrabState,
  onKeyboardGrabToggle,
}: Pick<RemoteViewToolbarProps, "keyboardGrabState" | "onKeyboardGrabToggle">) {
  return keyboardGrabState &&
    keyboardGrabState !== "unsupported" &&
    onKeyboardGrabToggle ? (
    <LabeledIconButton
      onClick={onKeyboardGrabToggle}
      active={keyboardGrabState === "active"}
      title={
        keyboardGrabState === "active"
          ? "Release keyboard from remote session (Ctrl+Alt+Shift)"
          : "Send keyboard to remote session"
      }
      label="Keyboard"
    >
      <Keyboard className="w-3.5 h-3.5" />
    </LabeledIconButton>
  ) : null;
}

export function ToolbarPerformance({
  onPerformanceOverlayToggle,
  showPerformanceOverlay = false,
}: Pick<
  RemoteViewToolbarProps,
  "onPerformanceOverlayToggle" | "showPerformanceOverlay"
>) {
  return onPerformanceOverlayToggle ? (
    <IconButton
      onClick={onPerformanceOverlayToggle}
      active={showPerformanceOverlay}
      title="Performance overlay (Ctrl+F1)"
    >
      <BarChart3 className="w-3.5 h-3.5" />
    </IconButton>
  ) : null;
}

export function ToolbarVirtualKeyboard({
  isTouchDevice = false,
  onToggleVirtualKeyboard,
}: Pick<RemoteViewToolbarProps, "isTouchDevice" | "onToggleVirtualKeyboard">) {
  return isTouchDevice && onToggleVirtualKeyboard ? (
    <button
      type="button"
      onClick={onToggleVirtualKeyboard}
      title="On-screen keyboard"
      aria-label="On-screen keyboard"
      className="flex items-center justify-center h-7 rounded-md px-1.5 font-mono font-semibold text-white/60 hover:text-white/90 hover:bg-white/8 transition-colors duration-[var(--dur-fast)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--control-focus-ring)]"
      style={{ fontSize: 10 }}
    >
      KB
    </button>
  ) : null;
}
