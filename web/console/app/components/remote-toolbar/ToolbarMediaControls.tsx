"use client";
import {
  Camera,
  Circle,
  Maximize,
  Minimize,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import type { RemoteViewToolbarProps } from "./types";
import { IconButton, LabeledIconButton } from "./ToolbarButtons";

export function ToolbarRecording({
  onToggleRecording,
  recording = false,
}: Pick<RemoteViewToolbarProps, "onToggleRecording" | "recording">) {
  return onToggleRecording ? (
    <IconButton
      onClick={onToggleRecording}
      active={recording}
      title={recording ? "Stop recording" : "Start recording"}
    >
      <Circle className="w-3.5 h-3.5" />
    </IconButton>
  ) : null;
}

export function ToolbarScreenshot({
  onScreenshot,
}: Pick<RemoteViewToolbarProps, "onScreenshot">) {
  return onScreenshot ? (
    <LabeledIconButton
      onClick={onScreenshot}
      title="Take screenshot"
      label="Screenshot"
    >
      <Camera className="w-3.5 h-3.5" />
    </LabeledIconButton>
  ) : null;
}

export function ToolbarAudio({
  onAudioToggle,
  audioUnavailable = false,
  audioMuted = false,
  onVolumeChange,
  volume = 1,
}: Pick<
  RemoteViewToolbarProps,
  | "onAudioToggle"
  | "audioUnavailable"
  | "audioMuted"
  | "onVolumeChange"
  | "volume"
>) {
  return onAudioToggle || audioUnavailable ? (
    <>
      <IconButton
        onClick={onAudioToggle ?? (() => {})}
        active={!audioMuted && !audioUnavailable}
        disabled={audioUnavailable}
        title={
          audioUnavailable
            ? "Audio unavailable"
            : audioMuted
              ? "Unmute audio"
              : "Mute audio"
        }
      >
        {audioMuted || audioUnavailable ? (
          <VolumeX className="w-3.5 h-3.5" />
        ) : (
          <Volume2 className="w-3.5 h-3.5" />
        )}
      </IconButton>
      {onVolumeChange && !audioUnavailable && (
        <input
          aria-label="Volume"
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={Math.max(0, Math.min(1, volume))}
          onChange={(event) => onVolumeChange(Number(event.target.value))}
          className="w-20 accent-[var(--accent)]"
          title="Volume"
        />
      )}
    </>
  ) : null;
}

export function ToolbarFullscreen({
  onFullscreenToggle,
  isFullscreen,
}: Pick<RemoteViewToolbarProps, "onFullscreenToggle" | "isFullscreen">) {
  return (
    <IconButton
      onClick={onFullscreenToggle}
      title={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
    >
      {isFullscreen ? (
        <Minimize className="w-3.5 h-3.5" />
      ) : (
        <Maximize className="w-3.5 h-3.5" />
      )}
    </IconButton>
  );
}

export function ToolbarDisconnect({
  onDisconnect,
}: Pick<RemoteViewToolbarProps, "onDisconnect">) {
  return (
    <IconButton onClick={onDisconnect} danger title="Disconnect">
      <X className="w-3.5 h-3.5" />
    </IconButton>
  );
}
